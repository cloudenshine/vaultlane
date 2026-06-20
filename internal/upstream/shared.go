package upstream

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// sharedPost 商户对接 POST（带签名）
// 与 PHP `App\Service\Bind\Shared::post` 完全一致：
//  1. 把 app_id + app_key 加入到待签字段
//  2. 算 sign = md5(urldecode(http_build_query(sorted) . "&key=" . appKey))
//  3. sign 也加到 body 里一起提交
func (c *Client) sharedPost(ctx context.Context, path string, params url.Values) (*APIResponse, error) {
	if params == nil {
		params = url.Values{}
	}
	body := url.Values{}
	for k, vs := range params {
		body[k] = vs
	}
	body.Set("app_id", c.cfg.AppID)
	body.Set("app_key", c.cfg.AppKey)
	body.Set("sign", Sign(body, c.cfg.AppKey))

	endpoint := c.cfg.BaseURL + path
	data, _, err := c.doRequestWithRetry(ctx, "POST", endpoint, body, nil)
	if err != nil {
		return nil, err
	}

	var resp APIResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode shared api: %w (raw: %s)", err, string(data))
	}
	return &resp, nil
}

// Connect 测试连通性
// POST /shared/authentication/connect
func (c *Client) Connect(ctx context.Context) (*APIResponse, error) {
	return c.sharedPost(ctx, "/shared/authentication/connect", url.Values{})
}

// Stock 查库存（按 code + race）
func (c *Client) Stock(ctx context.Context, code, race string) (string, error) {
	params := url.Values{}
	params.Set("code", code)
	if race != "" {
		params.Set("race", race)
	}
	resp, err := c.sharedPost(ctx, "/shared/commodity/stock", params)
	if err != nil {
		return "", err
	}
	if resp.Code != 200 {
		return "", fmt.Errorf("upstream: %s", resp.Msg)
	}
	// 库存可能是 string 或 number
	switch v := resp.Data.(type) {
	case string:
		return v, nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case nil:
		return "0", nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

// Valuation 估价
func (c *Client) Valuation(ctx context.Context, code, race string, num int, sku []string) (float64, error) {
	params := url.Values{}
	params.Set("code", code)
	params.Set("num", strconv.Itoa(num))
	if race != "" {
		params.Set("race", race)
	}
	for _, s := range sku {
		params.Add("sku[]", s)
	}
	resp, err := c.sharedPost(ctx, "/shared/commodity/valuation", params)
	if err != nil {
		return 0, err
	}
	if resp.Code != 200 {
		return 0, fmt.Errorf("upstream: %s", resp.Msg)
	}
	switch v := resp.Data.(type) {
	case float64:
		return v, nil
	case map[string]any:
		if price, ok := v["price"].(float64); ok {
			return price, nil
		}
		return 0, nil
	default:
		return 0, nil
	}
}

// Trade 下单
// params 需含: shared_code, contact, num, race, password, request_no 等
// 返回 trade_no + contents
func (c *Client) Trade(ctx context.Context, params url.Values) (*TradeResult, error) {
	// Mock 模式：跳过真实请求，返回模拟卡密
	if c.mockEnabled {
		time.Sleep(time.Duration(c.mockDelayMs) * time.Millisecond)
		num := 1
		if v := params.Get("num"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				num = n
			}
		}
		code := params.Get("shared_code")
		lines := make([]string, 0, num)
		for i := 1; i <= num; i++ {
			lines = append(lines, fmt.Sprintf("%s%s-%04d-%08x", c.mockPrefix, code, i, time.Now().UnixNano()&0xFFFFFFFF))
		}
		return &TradeResult{
			Code:     200,
			Msg:      "success (mock)",
			TradeNo:  "MOCK" + params.Get("request_no"),
			Contents: strings.Join(lines, "\n"),
		}, nil
	}
	resp, err := c.sharedPost(ctx, "/shared/commodity/trade", params)
	if err != nil {
		return nil, err
	}
	out := &TradeResult{
		Code: resp.Code,
		Msg:  resp.Msg,
	}
	if resp.Code != 200 {
		return out, fmt.Errorf("upstream: %s", resp.Msg)
	}
	// 解析 data
	if m, ok := resp.Data.(map[string]any); ok {
		if v, ok := m["trade_no"].(string); ok {
			out.TradeNo = v
		}
		if v, ok := m["contents"].(string); ok {
			out.Contents = v
		}
	}
	return out, nil
}

// Draft 草稿/卡密查询
// race 用于精确定位，code 是商品 code
func (c *Client) Draft(ctx context.Context, code, race string, cardID int) (any, error) {
	params := url.Values{}
	params.Set("code", code)
	if race != "" {
		params.Set("race", race)
	}
	if cardID > 0 {
		params.Set("card_id", strconv.Itoa(cardID))
	}
	resp, err := c.sharedPost(ctx, "/shared/commodity/draft", params)
	if err != nil {
		return nil, err
	}
	if resp.Code != 200 {
		return nil, fmt.Errorf("upstream: %s", resp.Msg)
	}
	return resp.Data, nil
}

// Items 拉取全量可代理商品
func (c *Client) Items(ctx context.Context) (any, error) {
	resp, err := c.sharedPost(ctx, "/shared/commodity/items", url.Values{})
	if err != nil {
		return nil, err
	}
	if resp.Code != 200 {
		return nil, fmt.Errorf("upstream: %s", resp.Msg)
	}
	return resp.Data, nil
}
