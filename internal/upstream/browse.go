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

// Categories 拉取全部分类（公开接口，无签名）
// 缓存 5 分钟
func (c *Client) Categories(ctx context.Context) ([]Category, error) {
	const cacheKey = "browse:categories"
	if v, ok := c.cache.Get(cacheKey); ok {
		return v.([]Category), nil
	}

	endpoint := c.cfg.BaseURL + "/user/api/index/data"
	data, _, err := c.doRequestWithRetry(ctx, "GET", endpoint, nil, nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Code int        `json:"code"`
		Msg  string     `json:"msg"`
		Data []Category `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode categories: %w", err)
	}
	if resp.Code != 200 {
		return nil, fmt.Errorf("upstream error: %s", resp.Msg)
	}

	c.cache.Set(cacheKey, resp.Data, 5*time.Minute)
	return resp.Data, nil
}

// Commodities 商品列表（公开接口）
// 支持分页/分类/关键词搜索
func (c *Client) Commodities(ctx context.Context, page, limit int, categoryID int, keywords string) ([]Commodity, int, error) {
	if page < 1 {
		page = 1
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	cacheKey := fmt.Sprintf("browse:commodities:p%d:l%d:c%d:k%s", page, limit, categoryID, keywords)
	if v, ok := c.cache.Get(cacheKey); ok {
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
	if strings.TrimSpace(keywords) != "" {
		q.Set("keywords", strings.TrimSpace(keywords))
	}

	endpoint := c.cfg.BaseURL + "/user/api/index/commodity?" + q.Encode()
	data, _, err := c.doRequestWithRetry(ctx, "GET", endpoint, nil, nil)
	if err != nil {
		return nil, 0, err
	}

	var resp CommodityListResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, 0, fmt.Errorf("decode commodities: %w", err)
	}
	if resp.Code != 200 {
		return nil, 0, fmt.Errorf("upstream error: %s", resp.Msg)
	}

	c.cache.Set(cacheKey, struct {
		Data  []Commodity
		Total int
	}{Data: resp.Data, Total: resp.Total}, 1*time.Minute)
	return resp.Data, resp.Total, nil
}

// CommodityDetail 商品详情
func (c *Client) CommodityDetail(ctx context.Context, id int) (*CommodityDetail, error) {
	if id <= 0 {
		return nil, fmt.Errorf("invalid commodity id")
	}

	cacheKey := fmt.Sprintf("browse:commodity:%d", id)
	if v, ok := c.cache.Get(cacheKey); ok {
		return v.(*CommodityDetail), nil
	}

	endpoint := c.cfg.BaseURL + "/user/api/index/commodityDetail?commodityId=" + strconv.Itoa(id)
	data, _, err := c.doRequestWithRetry(ctx, "GET", endpoint, nil, nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Code int              `json:"code"`
		Msg  string           `json:"msg"`
		Data *CommodityDetail `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode commodity: %w", err)
	}
	if resp.Code != 200 {
		return nil, fmt.Errorf("upstream error: %s", resp.Msg)
	}
	if resp.Data == nil {
		return nil, fmt.Errorf("upstream returned empty data")
	}

	c.cache.Set(cacheKey, resp.Data, 30*time.Second)
	return resp.Data, nil
}

// ClearCache 清空缓存（管理面板使用）
func (c *Client) ClearCache() {
	c.cache.Clear()
}
