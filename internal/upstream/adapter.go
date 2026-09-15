package upstream

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"faka-gateway/internal/config"
)

// Adapter 统一上游适配器接口
// 每个上游（upstreama、mock、未来的 custom）都实现此接口
type Adapter interface {
	// Name 适配器标识（决定 source 字段）
	Name() string
	// Type 适配器类型
	Type() string
	// Enabled 是否启用
	Enabled() bool
	// FetchCategories 拉分类
	FetchCategories(ctx context.Context) ([]Category, error)
	// FetchCommodities 拉商品列表（page 从 1 开始）
	FetchCommodities(ctx context.Context, page, limit, categoryID int, keyword string) ([]Commodity, int, error)
	// FetchCommodityDetail 拉商品详情
	FetchCommodityDetail(ctx context.Context, id int) (*CommodityDetail, error)
	// SubmitOrder 下单（占位：实际下单由 delivery 调度）
	SubmitOrder(ctx context.Context, sharedCode string, num int, contact string) (*TradeResult, error)
	// QueryOrder 查单
	QueryOrder(ctx context.Context, tradeNo string) (*TradeResult, error)
	// Metrics 调用指标
	Metrics() Metric
	// SetMock 切换 mock 模式（仅 mock/upstreama 实现有效）
	SetMock(enabled bool, prefix string, delayMs int)
	// MockStatus
	MockStatus() map[string]any
}

// BuildAdapter 根据 config.UpstreamEntry 构造适配器
func BuildAdapter(e config.UpstreamEntry) Adapter {
	ua := e.UserAgent
	if ua == "" {
		ua = "Vaultlane/5.0"
	}
	timeout := e.Timeout
	if timeout == 0 {
		timeout = 15
	}
	entryCfg := config.UpstreamConfig{
		BaseURL:   e.BaseURL,
		AppID:     e.AppID,
		AppKey:    e.AppKey,
		Timeout:   timeout,
		RetryMax:  e.RetryMax,
		UserAgent: ua,
	}
	switch e.Type {
	case "mock":
		return NewMockAdapter(e.Name, e.Prefix, e.Count, entryCfg)
	case "upstreama", "":
		return NewUpstreamAAdapter(e.Name, entryCfg)
	default:
		// 未知类型也按 upstreama 处理（签名逻辑相同）
		return NewUpstreamAAdapter(e.Name, entryCfg)
	}
}

// ============== 兼容旧方法名（用于最小化改动）==============

// Trade 下单（兼容旧名，等价 SubmitOrder）
func Trade(a Adapter, ctx context.Context, params map[string][]string) (*TradeResult, error) {
	p := mapToValues(params)
	code := p.Get("shared_code")
	if code == "" {
		code = p.Get("code")
	}
	num, _ := strconv.Atoi(p.Get("num"))
	if num <= 0 {
		num = 1
	}
	contact := p.Get("contact")
	return a.SubmitOrder(ctx, code, num, contact)
}

// Valuation 估价（mock 模式直接拿 sale_price；upstreama 调 shared 接口）
func Valuation(a Adapter, ctx context.Context, code, race string, num int, sku []string) (float64, error) {
	if a.Type() == "upstreama" {
		// 走 shared/commodity/valuation
		if z, ok := a.(*UpstreamAAdapter); ok {
			p := url.Values{}
			p.Set("code", code)
			p.Set("num", strconv.Itoa(num))
			if race != "" {
				p.Set("race", race)
			}
			for _, s := range sku {
				p.Add("sku[]", s)
			}
			body := url.Values{}
			for k, vs := range p {
				body[k] = vs
			}
			body.Set("app_id", z.cfg.AppID)
			body.Set("app_key", z.cfg.AppKey)
			body.Set("sign", Sign(body, z.cfg.AppKey))
			endpoint := strings.TrimRight(z.cfg.BaseURL, "/") + "/shared/commodity/valuation"
			data, _, err := z.http.DoWithRetry(ctx, "POST", endpoint, body, z.cfg.RetryMax, z.metric)
			if err != nil {
				return 0, err
			}
			var resp struct {
				Code int            `json:"code"`
				Msg  string         `json:"msg"`
				Data map[string]any `json:"data"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return 0, err
			}
			for _, v := range resp.Data {
				if f, ok := v.(float64); ok {
					return f, nil
				}
			}
			return 0, nil
		}
	}
	// mock：返回 cost × num
	// 通过 SubmitOrder mock 看 cost（简化：直接返回 0 让上层用本地 sale_price）
	return 0, nil
}

// Items 拉上游共享商品池（仅 upstreama 有意义；其他返回空）
func Items(a Adapter, ctx context.Context) (any, error) {
	if a.Type() == "upstreama" {
		if z, ok := a.(*UpstreamAAdapter); ok {
			body := url.Values{}
			body.Set("app_id", z.cfg.AppID)
			body.Set("app_key", z.cfg.AppKey)
			body.Set("sign", Sign(body, z.cfg.AppKey))
			endpoint := strings.TrimRight(z.cfg.BaseURL, "/") + "/shared/commodity/items"
			data, _, err := z.http.DoWithRetry(ctx, "POST", endpoint, body, z.cfg.RetryMax, z.metric)
			if err != nil {
				return nil, err
			}
			var resp struct {
				Code int    `json:"code"`
				Msg  string `json:"msg"`
				Data any    `json:"data"`
			}
			json.Unmarshal(data, &resp)
			return resp.Data, nil
		}
	}
	return nil, nil
}

// helper
func mapToValues(m map[string][]string) url.Values {
	p := url.Values{}
	for k, vs := range m {
		for _, v := range vs {
			p.Add(k, v)
		}
	}
	return p
}
