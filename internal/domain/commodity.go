package domain

import "faka-gateway/internal/upstream"

// DisplayCommodity 展示给前端的商品结构（已应用加价 + 库存标签）
type DisplayCommodity struct {
	ID            int        `json:"id"`
	Name          string     `json:"name"`
	Cover         string     `json:"cover"`
	Tags          []string   `json:"tags"`
	Price         float64    `json:"price"`        // 下游售价
	CostPrice     float64    `json:"cost_price"`   // 上游成本
	UserPrice     float64    `json:"user_price"`   // 上游用户价
	PricePrefix   string     `json:"price_prefix"` // 展示用前缀
	Stock         any        `json:"stock"`        // 原始库存
	StockLabel    StockLabel `json:"stock_label"`
	StockState    int        `json:"stock_state"`
	DeliveryWay   int        `json:"delivery_way"`
	DeliveryLabel string     `json:"delivery_label"`
	CategoryID    int        `json:"category_id"`
	CategoryName  string     `json:"category_name"`
	CategoryIcon  string     `json:"category_icon"`
	OrderSold     int        `json:"order_sold"`
	Recommend     int        `json:"recommend"`
	SharedCode    string     `json:"shared_code,omitempty"` // 上游 code（用于下单）
}

// resolveImageURL 把上游相对路径（/assets/...）补成完整 URL
// 已经是 http(s):// 的不重复拼接；空字符串返回空。
func resolveImageURL(base, p string) string {
	if p == "" {
		return ""
	}
	if len(p) >= 7 && (p[:7] == "http://" || p[:8] == "https://") {
		return p
	}
	if base == "" {
		return p
	}
	if p[0] != '/' {
		return base + "/" + p
	}
	return base + p
}

// ResolveImageURL exported alias
func ResolveImageURL(base, p string) string {
	return resolveImageURL(base, p)
}

// ToDisplay 把上游商品转换为展示结构
//
//	imageBase: 上游 BaseURL（用于把 /assets/... 转成完整 URL）
func ToDisplay(imageBase string, p *PremiumEngine, c upstream.Commodity) DisplayCommodity {
	cost, sale := p.Apply(c.CategoryID, c.ID, c.UserPrice, c.Price)
	tags := c.TagsList
	if len(tags) == 0 {
		tags = TagsToList(c.Tags)
	}
	catName := ""
	catIcon := ""
	if c.Category != nil {
		catName = c.Category.Name
		catIcon = c.Category.Icon
	}
	return DisplayCommodity{
		ID:            c.ID,
		Name:          c.Name,
		Cover:         resolveImageURL(imageBase, c.Cover),
		Tags:          tags,
		Price:         sale,
		CostPrice:     cost,
		UserPrice:     c.UserPrice,
		Stock:         c.Stock,
		StockLabel:    ResolveStockLabel(c.Stock, c.StockState),
		StockState:    c.StockState,
		DeliveryWay:   c.DeliveryWay,
		DeliveryLabel: DeliveryLabel(c.DeliveryWay),
		CategoryID:    c.CategoryID,
		CategoryName:  catName,
		CategoryIcon:  resolveImageURL(imageBase, catIcon),
		OrderSold:     c.OrderSold,
		Recommend:     c.Recommend,
	}
}
