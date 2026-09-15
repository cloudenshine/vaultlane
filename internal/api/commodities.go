package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"faka-gateway/internal/domain"
	"faka-gateway/internal/store"
)

// toDisplaySelf 兼容旧调用：转给 toDisplayPool
func toDisplaySelf(imageBase string, p *domain.PremiumEngine, c store.Commodity) domain.DisplayCommodity {
	return toDisplayPool(imageBase, p, c)
}

// handleCategories 分类（v4：自营 + 商品池聚合的 source）
func (d *Deps) handleCategories(c *gin.Context) {
	// 1. 自营分类
	self, _ := d.Store.ListCategories()

	// 2. 商品池聚合的 source 标签（使用全量聚合避免截断）
	srcByName, _ := d.Store.CountCommoditiesBySource()
	if srcByName == nil {
		srcByName = map[string]int{}
	}
	delete(srcByName, "self")

	type mergedCat struct {
		ID       int    `json:"id"`
		Name     string `json:"name"`
		Icon     string `json:"icon"`
		Sort     int    `json:"sort"`
		Source   string `json:"source"`
		Children []any  `json:"children,omitempty"`
	}
	out := make([]mergedCat, 0, len(self)+len(srcByName))
	for _, cat := range self {
		out = append(out, mergedCat{
			ID: int(cat.ID), Name: cat.Name, Icon: cat.Icon, Sort: cat.Sort, Source: cat.Source,
		})
	}
	for src, cnt := range srcByName {
		out = append(out, mergedCat{
			ID:     int(-hashStringToInt(src)),
			Name:   fmt.Sprintf("【%s】(%d件)", src, cnt),
			Source: src,
		})
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success", "data": out})
}

// handleCommodities 商品列表（v4：统一走商品池）
func (d *Deps) handleCommodities(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "24"))
	catID, _ := strconv.Atoi(c.DefaultQuery("category_id", "0"))
	keywords := c.Query("keywords")
	source := c.Query("source")

	// 商品池只查 status=1 的（用户端）
	pool, _, err := d.Store.ListCommodities(int64(catID), source, page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}

	// 关键字在内存里再过滤一次（避免 SQL LIKE 索引失效导致卡池）
	filtered := pool
	if k := strings.TrimSpace(keywords); k != "" {
		filtered = pool[:0]
		for _, it := range pool {
			if strings.Contains(strings.ToLower(it.Name), strings.ToLower(k)) {
				filtered = append(filtered, it)
			}
		}
	}

	display := make([]domain.DisplayCommodity, 0, len(filtered))
	for i := range filtered {
		display = append(display, toDisplayPool(d.Cfg.Upstream.BaseURL, d.Premium, filtered[i]))
	}

	c.JSON(http.StatusOK, gin.H{
		"code":  200,
		"msg":   "success",
		"data":  display,
		"total": len(display),
		"page":  page,
		"limit": limit,
	})
}

// handleCommodityDetail 商品详情（v4：统一走商品池）
func (d *Deps) handleCommodityDetail(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "无效商品ID"})
		return
	}

	com, err := d.Store.GetCommodityByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	if com == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "商品不存在"})
		return
	}
	if com.Status == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "商品已下架"})
		return
	}

	display := toDisplayPool(d.Cfg.Upstream.BaseURL, d.Premium, *com)

	// description / config 优先用本地（管理员可编辑），空则尝试上游详情
	desc := com.Description
	configStr := com.Config
	minimum := com.Minimum
	maximum := com.Maximum
	if maximum <= 0 {
		maximum = com.Stock
	}
	deliveryWay := com.DeliveryWay

	// 如果本地没有 description，且来自上游，尝试调上游详情补
	if desc == "" && strings.HasPrefix(com.Source, "upstream:") {
		name := strings.TrimPrefix(com.Source, "upstream:")
		if adp, ok := d.Manager.Adapter(name); ok && adp.Enabled() {
			ctx, cancel := ctxWithTimeout(c, 8*time.Second)
			defer cancel()
			// outer_id 解析回 int
			oid, _ := strconv.Atoi(com.OuterID)
			if detail, err := adp.FetchCommodityDetail(ctx, oid); err == nil && detail != nil {
				desc = detail.Description
				if configStr == "" {
					if cfg, err := jsonMarshal(detail.Config); err == nil {
						configStr = string(cfg)
					}
				}
				if minimum <= 0 {
					minimum = detail.Minimum
				}
				if maximum <= 0 {
					maximum = detail.Maximum
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "success",
		"data": gin.H{
			"commodity":       display,
			"description":     desc,
			"config":          configStr,
			"minimum":         minimum,
			"maximum":         maximum,
			"password_status": 0,
			"contact_type":    0,
			"share_url":       "",
			"delivery_way":    deliveryWay,
			"pool_id":         com.ID,
			"source":          com.Source,
		},
	})
}

// toDisplayPool 商品池 → DisplayCommodity
func toDisplayPool(imageBase string, p *domain.PremiumEngine, c store.Commodity) domain.DisplayCommodity {
	price := c.SalePrice
	if price <= 0 {
		price = c.Price
	}
	cost := c.CostPrice
	if cost <= 0 {
		cost = price
	}
	desc := c.Description
	if desc == "" {
		desc = strings.TrimSpace(c.Name)
	}
	return domain.DisplayCommodity{
		ID:            int(c.ID),
		Name:          c.Name,
		Cover:         domain.ResolveImageURL(imageBase, c.Cover),
		Tags:          splitTags(c.Tags),
		Price:         price,
		CostPrice:     cost,
		UserPrice:     price,
		Stock:         c.Stock,
		StockLabel:    domain.ResolveStockLabel(c.Stock, 1),
		StockState:    1,
		DeliveryWay:   c.DeliveryWay,
		DeliveryLabel: domain.DeliveryLabel(c.DeliveryWay),
		CategoryID:    int(c.CategoryID),
		CategoryName:  "",
		CategoryIcon:  "",
		OrderSold:     c.Sold,
		Recommend:     0,
		SharedCode:    c.SharedCode,
	}
}

func splitTags(s string) []string {
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func jsonMarshal(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

func hashStringToInt(s string) int64 {
	h := int64(0)
	for _, c := range s {
		h = h*131 + int64(c)
	}
	if h < 0 {
		h = -h
	}
	return h
}

func getString(v any, key string) string {
	if m, ok := v.(map[string]any); ok {
		if s, ok := m[key].(string); ok {
			return s
		}
	}
	return ""
}
