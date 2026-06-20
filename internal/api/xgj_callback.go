package api

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"faka-gateway/internal/store"
	"faka-gateway/internal/upstream/downstreamb"
)

// DownstreamCallbackHandler 下游网关订单回调
type DownstreamCallbackHandler struct {
	Store      *store.Store
	Downstream *downstreamb.Client
	Logger     *slog.Logger
	Token      string
}

// NewDownstreamCallbackHandler 构造
func NewDownstreamCallbackHandler(st *store.Store, downstream *downstreamb.Client, token string, logger *slog.Logger) *DownstreamCallbackHandler {
	return &DownstreamCallbackHandler{Store: st, Downstream: downstream, Logger: logger, Token: token}
}

// RegisterRoutes 注册
func (h *DownstreamCallbackHandler) RegisterRoutes(g *gin.RouterGroup) {
	g.POST("/callbacks/downstreamb/order", h.handleOrder)
}

// CallbackBody 下游网关订单回调
type downstreamOrderCallbackBody struct {
	AppKey     string `json:"app_key"`
	OrderNo    string `json:"order_no"` // 下游网关订单号
	OuterID    string `json:"outer_id"` // 我方订单号
	ItemID     string `json:"item_id"`
	Quantity   int    `json:"quantity"`
	CardSecret string `json:"card_secret"` // 卡密（如果上游已发）
	Status     int    `json:"status"`      // 1=已付款
	Sign       string `json:"sign"`
}

func (h *DownstreamCallbackHandler) handleOrder(c *gin.Context) {
	body, err := c.GetRawData()
	if err != nil {
		c.String(http.StatusBadRequest, "bad request")
		return
	}
	// 验签：所有非空字段排序拼接 + 共享密钥
	if !h.verifySign(string(body)) && h.Token != "" {
		c.String(http.StatusBadRequest, "sign error")
		return
	}
	var req downstreamOrderCallbackBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.String(http.StatusBadRequest, "bad json")
		return
	}
	if req.Status != 1 {
		c.String(http.StatusOK, "ignored")
		return
	}
	// 找订单
	var order *store.Order
	if req.OuterID != "" {
		order, _ = h.Store.GetOrderByTradeNo(req.OuterID)
	}
	if order == nil && req.OrderNo != "" {
		// 兜底：用 orderNo 当 trade_no 查
		order, _ = h.Store.GetOrderByTradeNo(req.OrderNo)
	}
	if order == nil {
		h.Logger.Warn("downstream callback: order not found", "order", req.OrderNo, "outer", req.OuterID)
		c.String(http.StatusOK, "ok")
		return
	}
	if order.Status >= 2 {
		c.String(http.StatusOK, "ok")
		return
	}
	if req.CardSecret != "" {
		order.Contents = req.CardSecret
	}
	order.Status = 2
	order.UpstreamCode = 200
	order.UpstreamMsg = "downstream callback"
	if err := h.Store.UpdateOrder(order); err != nil {
		c.String(http.StatusInternalServerError, "db error")
		return
	}
	c.String(http.StatusOK, "success")
}

func (h *DownstreamCallbackHandler) verifySign(rawBody string) bool {
	// 极简验签：从 body 解析出 sign，再把其它字段按字典序拼接 + token
	if h.Token == "" {
		return true
	}
	idx := strings.LastIndex(rawBody, "\"sign\":\"")
	if idx < 0 {
		return false
	}
	end := strings.Index(rawBody[idx+8:], "\"")
	if end < 0 {
		return false
	}
	sign := rawBody[idx+8 : idx+8+end]
	// 移除 sign 字段
	payload := rawBody[:idx] + rawBody[idx+8+end+1:]
	// 提取字段
	fields := extractJSONFields(payload)
	sort.Strings(fields)
	str := strings.Join(fields, "&") + h.Token
	sum := sha256.Sum256([]byte(str))
	return hex.EncodeToString(sum[:]) == sign
}

func extractJSONFields(payload string) []string {
	var out []string
	// 简化：扫描 "key":"value" 或 "key":number
	i := 0
	for i < len(payload) {
		if payload[i] == '"' {
			j := i + 1
			for j < len(payload) && payload[j] != '"' {
				if payload[j] == '\\' {
					j += 2
					continue
				}
				j++
			}
			if j >= len(payload) {
				break
			}
			key := payload[i+1 : j]
			// 跳过 :
			k := j + 1
			for k < len(payload) && (payload[k] == ':' || payload[k] == ' ') {
				k++
			}
			// 读 value
			var val string
			if k < len(payload) && payload[k] == '"' {
				m := k + 1
				for m < len(payload) && payload[m] != '"' {
					if payload[m] == '\\' {
						m += 2
						continue
					}
					m++
				}
				if m < len(payload) {
					val = payload[k+1 : m]
				}
			} else {
				// number / bool / null：读到 , 或 }
				m := k
				for m < len(payload) && payload[m] != ',' && payload[m] != '}' {
					m++
				}
				val = strings.TrimSpace(payload[k:m])
			}
			if val != "" {
				out = append(out, key+"="+val)
			}
			i = k
			continue
		}
		i++
	}
	return out
}
