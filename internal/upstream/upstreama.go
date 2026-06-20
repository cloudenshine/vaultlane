package upstream

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"faka-gateway/internal/config"
)

// UpstreamAAdapter 上游发卡（upstreama）公开 API 适配器
// 公开路径：/user/api/index/*  无需签名
// 同时保留 mock 模式
type UpstreamAAdapter struct {
	name string
	cfg  config.UpstreamConfig
	http *HTTPDoer

	// 缓存（公开接口轻量 TTL）
	cache *TTLCache

	// Mock 模式
	mockEnabled bool
	mockPrefix  string
	mockDelayMs int

	// 指标
	metric *metricCounter

	// 共享下单接口（独立路径 /shared/* 带签名）
	sharedSign string
}

// NewUpstreamAAdapter 构造
func NewUpstreamAAdapter(name string, cfg config.UpstreamConfig) *UpstreamAAdapter {
	if name == "" {
		name = "upstreama"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 15
	}
	if cfg.RetryMax < 0 {
		cfg.RetryMax = 2
	}
	ua := cfg.UserAgent
	if ua == "" {
		ua = "FakaGateway/1.0"
	}
	return &UpstreamAAdapter{
		name:        name,
		cfg:         cfg,
		http:        NewHTTPDoer(time.Duration(cfg.Timeout)*time.Second, ua),
		cache:       NewTTLCache(256),
		metric:      newMetricCounter(cfg.BaseURL),
		sharedSign:  cfg.AppKey,
		mockPrefix:  "MOCK-",
		mockDelayMs: 800,
	}
}

func (z *UpstreamAAdapter) Name() string  { return z.name }
func (z *UpstreamAAdapter) Type() string  { return "upstreama" }
func (z *UpstreamAAdapter) Enabled() bool { return z.cfg.BaseURL != "" }

// getPublic 调公开 GET 接口
func (z *UpstreamAAdapter) getPublic(ctx context.Context, path string, q url.Values, ttl time.Duration, dst any) error {
	if z.mockEnabled {
		return z.mockPublic(path, q, dst)
	}
	endpoint := strings.TrimRight(z.cfg.BaseURL, "/") + path
	if q != nil && len(q) > 0 {
		endpoint += "?" + q.Encode()
	}
	data, _, err := z.http.DoWithRetry(ctx, "GET", endpoint, nil, z.cfg.RetryMax, z.metric)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("decode: %w (raw: %s)", err, string(data)[:min(200, len(data))])
	}
	return nil
}

func (z *UpstreamAAdapter) FetchCategories(ctx context.Context) ([]Category, error) {
	if v, ok := z.cache.Get("cats"); ok {
		return v.([]Category), nil
	}
	var resp struct {
		Code int        `json:"code"`
		Msg  string     `json:"msg"`
		Data []Category `json:"data"`
	}
	if err := z.getPublic(ctx, "/user/api/index/data", nil, 5*time.Minute, &resp); err != nil {
		return nil, err
	}
	if resp.Code != 200 {
		return nil, fmt.Errorf("upstream: %s", resp.Msg)
	}
	z.cache.Set("cats", resp.Data, 5*time.Minute)
	return resp.Data, nil
}

func (z *UpstreamAAdapter) FetchCommodities(ctx context.Context, page, limit, categoryID int, keyword string) ([]Commodity, int, error) {
	if page < 1 {
		page = 1
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	cacheKey := fmt.Sprintf("comms:p%d:l%d:c%d:k%s", page, limit, categoryID, keyword)
	if v, ok := z.cache.Get(cacheKey); ok {
		entry := v.(struct {
			Data  []Commodity
			Total int
		})
		return entry.Data, entry.Total, nil
	}

	q := url.Values{}
	q.Set("page", strconv.Itoa(page))
	q.Set("limit", strconv.Itoa(limit))
	if categoryID > 0 {
		q.Set("categoryId", strconv.Itoa(categoryID))
	}
	if k := strings.TrimSpace(keyword); k != "" {
		q.Set("keywords", k)
	}

	var resp CommodityListResponse
	if err := z.getPublic(ctx, "/user/api/index/commodity", q, time.Minute, &resp); err != nil {
		return nil, 0, err
	}
	if resp.Code != 200 {
		return nil, 0, fmt.Errorf("upstream: %s", resp.Msg)
	}
	z.cache.Set(cacheKey, struct {
		Data  []Commodity
		Total int
	}{Data: resp.Data, Total: resp.Total}, time.Minute)
	return resp.Data, resp.Total, nil
}

func (z *UpstreamAAdapter) FetchCommodityDetail(ctx context.Context, id int) (*CommodityDetail, error) {
	if id <= 0 {
		return nil, fmt.Errorf("invalid id")
	}
	cacheKey := fmt.Sprintf("comm:%d", id)
	if v, ok := z.cache.Get(cacheKey); ok {
		return v.(*CommodityDetail), nil
	}
	q := url.Values{}
	q.Set("commodityId", strconv.Itoa(id))
	var resp struct {
		Code int              `json:"code"`
		Msg  string           `json:"msg"`
		Data *CommodityDetail `json:"data"`
	}
	if err := z.getPublic(ctx, "/user/api/index/commodityDetail", q, 30*time.Second, &resp); err != nil {
		return nil, err
	}
	if resp.Code != 200 {
		return nil, fmt.Errorf("upstream: %s", resp.Msg)
	}
	if resp.Data == nil {
		return nil, fmt.Errorf("empty data")
	}
	z.cache.Set(cacheKey, resp.Data, 30*time.Second)
	return resp.Data, nil
}

// SubmitOrder 通过 shared 接口下单（带签名）
func (z *UpstreamAAdapter) SubmitOrder(ctx context.Context, sharedCode string, num int, contact string) (*TradeResult, error) {
	if z.mockEnabled {
		return z.mockSubmit(sharedCode, num)
	}
	p := url.Values{}
	p.Set("code", sharedCode)
	p.Set("contact", contact)
	p.Set("num", strconv.Itoa(num))

	// 复用 sharedSign
	body := url.Values{}
	for k, vs := range p {
		body[k] = vs
	}
	body.Set("app_id", z.cfg.AppID)
	body.Set("app_key", z.cfg.AppKey)
	body.Set("sign", Sign(body, z.cfg.AppKey))

	endpoint := strings.TrimRight(z.cfg.BaseURL, "/") + "/shared/commodity/trade"
	data, _, err := z.http.DoWithRetry(ctx, "POST", endpoint, body, z.cfg.RetryMax, z.metric)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Code int            `json:"code"`
		Msg  string         `json:"msg"`
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(data, &resp)
	out := &TradeResult{Code: resp.Code, Msg: resp.Msg}
	for k, v := range resp.Data {
		switch k {
		case "trade_no":
			if s, ok := v.(string); ok {
				out.TradeNo = s
			}
		case "contents":
			if s, ok := v.(string); ok {
				out.Contents = s
			}
		}
	}
	return out, nil
}

func (z *UpstreamAAdapter) QueryOrder(ctx context.Context, tradeNo string) (*TradeResult, error) {
	if z.mockEnabled {
		return &TradeResult{Code: 200, Msg: "ok", TradeNo: tradeNo, Contents: z.mockPrefix + "QUERY-001"}, nil
	}
	p := url.Values{}
	p.Set("trade_no", tradeNo)
	body := url.Values{}
	for k, vs := range p {
		body[k] = vs
	}
	body.Set("app_id", z.cfg.AppID)
	body.Set("app_key", z.cfg.AppKey)
	body.Set("sign", Sign(body, z.cfg.AppKey))
	endpoint := strings.TrimRight(z.cfg.BaseURL, "/") + "/shared/commodity/draft"
	data, _, err := z.http.DoWithRetry(ctx, "POST", endpoint, body, z.cfg.RetryMax, z.metric)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Code int            `json:"code"`
		Msg  string         `json:"msg"`
		Data map[string]any `json:"data"`
	}
	json.Unmarshal(data, &resp)
	return &TradeResult{Code: resp.Code, Msg: resp.Msg}, nil
}

func (z *UpstreamAAdapter) Metrics() Metric { return z.metric.snapshot() }

func (z *UpstreamAAdapter) SetMock(enabled bool, prefix string, delayMs int) {
	z.mockEnabled = enabled
	if prefix != "" {
		z.mockPrefix = prefix
	} else {
		z.mockPrefix = "MOCK-"
	}
	if delayMs > 0 {
		z.mockDelayMs = delayMs
	} else if delayMs == 0 {
		z.mockDelayMs = 800
	}
}

func (z *UpstreamAAdapter) MockStatus() map[string]any {
	return map[string]any{
		"enabled":  z.mockEnabled,
		"prefix":   z.mockPrefix,
		"delay_ms": z.mockDelayMs,
	}
}

// mockPublic 模拟公开接口（用于演示/无真实上游时）
func (z *UpstreamAAdapter) mockPublic(path string, q url.Values, dst any) error {
	if z.mockDelayMs > 0 {
		time.Sleep(time.Duration(z.mockDelayMs) * time.Millisecond)
	}
	switch {
	case strings.HasSuffix(path, "/data"):
		// categories
		_ = q
		// 通过反射不行，调用方传的是 *struct，我们手动填充
		if p, ok := dst.(*struct {
			Code int        `json:"code"`
			Msg  string     `json:"msg"`
			Data []Category `json:"data"`
		}); ok {
			p.Code = 200
			p.Msg = "success"
			p.Data = []Category{
				{ID: 56, Name: "今日推荐", Icon: "", Sort: 0, CommodityCount: 4, Status: 1},
				{ID: 100, Name: "示例A", Icon: "", Sort: 1, CommodityCount: 6, Status: 1},
				{ID: 101, Name: "示例B", Icon: "", Sort: 2, CommodityCount: 3, Status: 1},
				{ID: 102, Name: "示例D", Icon: "", Sort: 3, CommodityCount: 2, Status: 1},
				{ID: 103, Name: "示例E", Icon: "", Sort: 4, CommodityCount: 1, Status: 1},
				{ID: 104, Name: "示例F", Icon: "", Sort: 5, CommodityCount: 2, Status: 1},
				{ID: 105, Name: "推特", Icon: "", Sort: 6, CommodityCount: 1, Status: 1},
				{ID: 106, Name: "其他AI", Icon: "", Sort: 7, CommodityCount: 2, Status: 1},
			}
			return nil
		}
	case strings.HasSuffix(path, "/commodity"):
		// list
		if p, ok := dst.(*CommodityListResponse); ok {
			p.Code = 200
			p.Msg = "success"
			p.Total = 6
			keyword := q.Get("keywords")
			for i := 1; i <= 6; i++ {
				p.Data = append(p.Data, Commodity{
					ID:                 9000 + i,
					Name:               "【演示】" + keyword + "商品" + strconv.Itoa(i),
					Cover:              "",
					Tags:               "演示,MOCK",
					TagsList:           []string{"演示", "MOCK"},
					Price:              9.9 + float64(i),
					UserPrice:          8.8 + float64(i),
					Stock:              "100",
					StockState:         3,
					DeliveryWay:        1,
					CategoryID:         100,
					OrderSold:          10 * i,
					DisplayCategoryIDs: []int{100, 56},
				})
			}
			return nil
		}
	case strings.HasSuffix(path, "/commodityDetail"):
		if p, ok := dst.(*struct {
			Code int              `json:"code"`
			Msg  string           `json:"msg"`
			Data *CommodityDetail `json:"data"`
		}); ok {
			id, _ := strconv.Atoi(q.Get("commodityId"))
			p.Code = 200
			p.Msg = "success"
			p.Data = &CommodityDetail{
				Commodity: Commodity{
					ID:          id,
					Name:        "【演示】" + strconv.Itoa(id) + "号商品",
					Cover:       "",
					Tags:        "演示,MOCK",
					Price:       19.9,
					UserPrice:   18.8,
					Stock:       "100",
					StockState:  3,
					DeliveryWay: 1,
					CategoryID:  100,
				},
				Description:    "<h2>演示商品详情</h2><p>这是 mock 模式的演示商品。购买后会自动发货。</p><p>支持：日卡 / 周卡 / 月卡</p>",
				Minimum:        1,
				Maximum:        10,
				ContactType:    0,
				PasswordStatus: 0,
				Config:         map[string]any{"category": map[string]float64{"日卡": 1.0, "周卡": 5.0, "月卡": 18.0}},
			}
			return nil
		}
	}
	return fmt.Errorf("mock: unknown path %s", path)
}

func (z *UpstreamAAdapter) mockSubmit(sharedCode string, num int) (*TradeResult, error) {
	if z.mockDelayMs > 0 {
		time.Sleep(time.Duration(z.mockDelayMs) * time.Millisecond)
	}
	lines := make([]string, 0, num)
	for i := 1; i <= num; i++ {
		lines = append(lines, fmt.Sprintf("%s%s-%04d-%08x", z.mockPrefix, sharedCode, i, time.Now().UnixNano()&0xFFFFFFFF))
	}
	return &TradeResult{
		Code:     200,
		Msg:      "success (mock)",
		TradeNo:  "MOCK" + strconv.FormatInt(time.Now().UnixNano(), 10),
		Contents: strings.Join(lines, "\n"),
	}, nil
}

// 防止与 httpdoer.go 的 min2 冲突，upstreama 用本文件专属 min
var _ = sync.Mutex{} // 引用 sync 防止 import 警告
