package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"faka-gateway/internal/upstream"
)

// handleOrderCreate / handleOrderGet 已删除（VULN-007）：
// 这两个旧路由未注册到 router.go（payment_api.go 的 handleOrderCreateV3/handleOrderGetV3 已替代）。

// handleOrderCreate / handleOrderGet 已删除（VULN-007）
// 这两个旧路由未注册到 router.go（payment_api.go 的 V3 版本已替代）。
// 删除防止未来重构时误注册导致 VULN-001 回归。

// ---- helpers ----

func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b) + strconv.FormatInt(time.Now().UnixNano()%1000, 10)
}

// isValidContact 校验联系方式（邮箱 / 大陆手机号）
func isValidContact(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	// 邮箱
	at := strings.IndexByte(s, '@')
	if at > 0 && at < len(s)-1 && strings.Contains(s[at+1:], ".") {
		return true
	}
	// 大陆手机号
	if len(s) == 11 && s[0] == '1' && s[1] >= '3' && s[1] <= '9' {
		for i := 0; i < 11; i++ {
			if s[i] < '0' || s[i] > '9' {
				return false
			}
		}
		return true
	}
	return false
}

// splitSecrets 把上游 contents（多行卡密）拆成数组
func splitSecrets(contents string) []string {
	if contents == "" {
		return nil
	}
	lines := strings.Split(strings.ReplaceAll(contents, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// friendlyUpstreamError 把上游错误翻成中文友好提示
func friendlyUpstreamError(raw string) string {
	raw = strings.TrimSpace(raw)
	switch {
	case strings.Contains(raw, "余额不足"):
		return "上游账户余额不足，请联系站长充值后重试"
	case strings.Contains(raw, "库存不足"):
		return "商品库存不足，请稍后再试或选择其他规格"
	case strings.Contains(raw, "商品不存在") || strings.Contains(raw, "商品已下架"):
		return "商品已下架或不存在"
	case strings.Contains(raw, "参数错误") || strings.Contains(raw, "shared_code"):
		return "订单参数不正确，请刷新页面后重试"
	case strings.Contains(raw, "频繁") || strings.Contains(raw, "限流"):
		return "请求过于频繁，请稍后再试"
	case raw == "":
		return "上游下单失败，请稍后重试"
	default:
		return "上游下单失败: " + raw
	}
}

// 防止 import 抖动：upstream 包引用
var _ = upstream.Sign

// sharedItem 上游 /shared/commodity/items 返回的单个商品（精简字段）
type sharedItem struct {
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Cover       string  `json:"cover"`
	Price       float64 `json:"price"`
	UserPrice   float64 `json:"user_price"`
	Stock       any     `json:"stock"`
	StockState  int     `json:"stock_state"`
	CategoryID  int     `json:"category_id"`
	OrderSold   int     `json:"order_sold"`
	DeliveryWay int     `json:"delivery_way"`
	Description string  `json:"description"`
	Config      string  `json:"config"`
}

// handleOrderableItems 可下单商品列表
// 调上游 /shared/commodity/items，把全分类里的共享商品拍平成一维数组
func (d *Deps) handleOrderableItems(c *gin.Context) {
	ctx, cancel := ctxWithTimeout(c, 20*time.Second)
	defer cancel()
	data, err := upstream.Items(d.Up, ctx)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": 502, "msg": "上游错误: " + err.Error()})
		return
	}
	type itemGroup struct {
		CategoryID int          `json:"category_id"`
		Category   string       `json:"category"`
		Children   []sharedItem `json:"children"`
	}
	// 上游 /shared/commodity/items 的 data 实际是直接的 []interface{}（商品数组）
	// 也可能返回 {data: [...]}，做双兼容
	raw, _ := data.(map[string]any)
	out := []sharedItem{}
	groups := []itemGroup{}

	var arr []any
	if raw != nil {
		if a, ok := raw["data"].([]any); ok {
			arr = a
		}
	}
	if arr == nil {
		if a, ok := data.([]any); ok {
			arr = a
		}
	}
	if arr != nil {
		// items 真实结构：[{category_id, children:[{code,price,...},...]}, ...]
		// 也可能直接是商品数组 [{code,price,...}, ...]，双兼容
		for _, top := range arr {
			tm, ok := top.(map[string]any)
			if !ok {
				continue
			}
			catID := int(asFloat(tm["category_id"]))
			catName := asString(tm["category"])
			if children, ok := tm["children"].([]any); ok {
				for _, c := range children {
					cm, ok := c.(map[string]any)
					if !ok {
						continue
					}
					si := mapSharedItem(cm)
					if catID > 0 && si.CategoryID == 0 {
						si.CategoryID = catID
					}
					if catName != "" { /* keep item's own category if any */
					}
					out = append(out, si)
					// 找到/创建分类组
					found := false
					for i := range groups {
						if groups[i].CategoryID == si.CategoryID {
							groups[i].Children = append(groups[i].Children, si)
							found = true
							break
						}
					}
					if !found {
						groups = append(groups, itemGroup{CategoryID: si.CategoryID, Children: []sharedItem{si}})
					}
				}
			} else if code, _ := tm["code"].(string); code != "" {
				// 直接就是商品（无 children 包裹）
				si := mapSharedItem(tm)
				out = append(out, si)
			}
		}
	}
	// 把图片补全
	for i := range out {
		out[i].Cover = resolveImageURLShared(d.Cfg.Upstream.BaseURL, out[i].Cover)
	}
	c.JSON(http.StatusOK, gin.H{
		"code":   200,
		"msg":    "success",
		"data":   out,
		"groups": groups,
		"total":  len(out),
	})
}

func mapSharedItem(m map[string]any) sharedItem {
	return sharedItem{
		Code:        asString(m["code"]),
		Name:        asString(m["name"]),
		Cover:       asString(m["cover"]),
		Price:       asFloat(m["price"]),
		UserPrice:   asFloat(m["user_price"]),
		Stock:       m["stock"],
		StockState:  int(asFloat(m["stock_state"])),
		CategoryID:  int(asFloat(m["category_id"])),
		OrderSold:   int(asFloat(m["order_sold"])),
		DeliveryWay: int(asFloat(m["delivery_way"])),
		Description: asString(m["description"]),
		Config:      asString(m["config"]),
	}
}

func asFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	default:
		return 0
	}
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func resolveImageURLShared(base, p string) string {
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
