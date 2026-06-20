package domain

import "strings"

// StockState 库存状态码（与上游 stock_state 对齐）
const (
	StockStateSoldOut  = 0
	StockStateLow      = 2
	StockStateNormal   = 3
	StockStateAbundant = 4
)

// StockLabel 库存标签
type StockLabel struct {
	Code  int
	Text  string
	Color string // CSS 颜色
}

// ResolveStockLabel 根据上游字段推算标签
// upstream stock 可能是：数字(int)/字符串("已售罄"/"紧张"/"一般"/"充足"/"非常多")/ nil
func ResolveStockLabel(stock any, state int) StockLabel {
	// 优先用 state 字段
	if state == StockStateSoldOut {
		return StockLabel{Code: state, Text: "已售罄", Color: "#B23A48"}
	}
	if state == StockStateLow {
		return StockLabel{Code: state, Text: "紧张", Color: "#B23A48"}
	}

	// 再看 stock 文本
	if s, ok := stock.(string); ok {
		switch s {
		case "已售罄":
			return StockLabel{Code: StockStateSoldOut, Text: "已售罄", Color: "#B23A48"}
		case "紧张":
			return StockLabel{Code: StockStateLow, Text: "紧张", Color: "#B23A48"}
		case "一般":
			return StockLabel{Code: StockStateNormal, Text: "一般", Color: "#C9A961"}
		case "充足":
			return StockLabel{Code: StockStateNormal, Text: "充足", Color: "#2E5D4F"}
		case "非常多":
			return StockLabel{Code: StockStateAbundant, Text: "充足", Color: "#2E5D4F"}
		case "0":
			return StockLabel{Code: StockStateSoldOut, Text: "已售罄", Color: "#B23A48"}
		}
	}

	// 数字
	if f, ok := stock.(float64); ok {
		if f <= 0 {
			return StockLabel{Code: StockStateSoldOut, Text: "已售罄", Color: "#B23A48"}
		}
		if f < 10 {
			return StockLabel{Code: StockStateLow, Text: "紧张", Color: "#B23A48"}
		}
		if f < 100 {
			return StockLabel{Code: StockStateNormal, Text: "一般", Color: "#C9A961"}
		}
		return StockLabel{Code: StockStateAbundant, Text: "充足", Color: "#2E5D4F"}
	}

	// 默认
	return StockLabel{Code: StockStateNormal, Text: "库存未知", Color: "#8B8B8B"}
}

// DeliveryLabel 发货方式
func DeliveryLabel(deliveryWay int) string {
	if deliveryWay == 0 {
		return "自动发货"
	}
	return "人工发货"
}

// TagsToList 逗号分隔 -> 数组
func TagsToList(s string) []string {
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
