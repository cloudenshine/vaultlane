package upstream

import (
	"time"
)

// 通用响应
type APIResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data"`
}

// 商品分类
type Category struct {
	ID             int        `json:"id"`
	Name           string     `json:"name"`
	Sort           int        `json:"sort"`
	Icon           string     `json:"icon"`
	CommodityCount int        `json:"commodity_count"`
	Status         int        `json:"status"`
	Hide           int        `json:"hide"`
	Children       []Category `json:"children,omitempty"`
}

// 商品列表
type Commodity struct {
	ID                 int       `json:"id"`
	Name               string    `json:"name"`
	Cover              string    `json:"cover"`
	Tags               string    `json:"tags"`
	TagsList           []string  `json:"tags_list,omitempty"`
	Price              float64   `json:"price"`
	UserPrice          float64   `json:"user_price"`
	Stock              any       `json:"stock"`       // 字符串或数字
	StockState         int       `json:"stock_state"` // 0=售罄 2=紧张 3=充足 4=非常充足
	DeliveryWay        int       `json:"delivery_way"`
	CategoryID         int       `json:"category_id"`
	Category           *Category `json:"category,omitempty"`
	OrderSold          int       `json:"order_sold"`
	Recommend          int       `json:"recommend"`
	InventoryHidden    int       `json:"inventory_hidden"`
	DisplayCategoryIDs []int     `json:"display_category_ids,omitempty"`
}

// 商品详情
type CommodityDetail struct {
	Commodity
	Description       string         `json:"description"`
	OnlyUser          int            `json:"only_user"`
	Status            int            `json:"status"`
	ContactType       int            `json:"contact_type"`
	PasswordStatus    int            `json:"password_status"`
	Coupon            int            `json:"coupon"`
	Config            map[string]any `json:"config"`
	Widget            []any          `json:"widget"`
	Minimum           int            `json:"minimum"`
	Maximum           int            `json:"maximum"`
	Code              string         `json:"code"`
	SharedCode        *string        `json:"shared_code,omitempty"`
	SharedID          *int           `json:"shared_id,omitempty"`
	CardID            *int           `json:"card_id,omitempty"`
	ShareURL          string         `json:"share_url"`
	MemberLevelPrices []any          `json:"member_level_prices"`
	GroupBuyEnabled   int            `json:"group_buy_enabled"`
}

// 商品列表响应
type CommodityListResponse struct {
	Code  int         `json:"code"`
	Msg   string      `json:"msg"`
	Data  []Commodity `json:"data"`
	Total int         `json:"total"`
}

// 分类列表响应
type CategoryListResponse struct {
	Code int        `json:"code"`
	Msg  string     `json:"msg"`
	Data []Category `json:"data"`
}

// 商户连通性
type ConnectResult struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data"`
}

// 下单响应
type TradeResult struct {
	Code     int    `json:"code"`
	Msg      string `json:"msg"`
	TradeNo  string `json:"trade_no"`
	Contents string `json:"contents"`
}

// 调用指标
type Metric struct {
	TotalCalls   int64
	SuccessCalls int64
	FailedCalls  int64
	CacheHits    int64
	CacheMisses  int64
	AvgLatencyMs int64
	LastError    string
	LastErrorAt  time.Time
	UpstreamURL  string
	StartedAt    time.Time
}
