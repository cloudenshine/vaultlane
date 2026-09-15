package admin

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"faka-gateway/internal/store"
)

// HandleStatsOverview 仪表盘概览
func (h *Handlers) HandleStatsOverview(c *gin.Context) {
	now := time.Now()
	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	stats := map[string]any{}

	// 今日订单数 + 今日收入
	todayCount, todayRev := h.rangeStats(startOfToday, now)
	stats["today_orders"] = todayCount
	stats["today_revenue"] = todayRev

	// 本月
	monthCount, monthRev := h.rangeStats(startOfMonth, now)
	stats["month_orders"] = monthCount
	stats["month_revenue"] = monthRev

	// 总订单
	total, _ := h.Store.CountOrders()
	stats["total_orders"] = total

	// 商品数
	commodities, _ := h.Store.CountCommodities()
	stats["total_commodities"] = commodities

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success", "data": stats})
}

// HandleStatsRevenue 收入曲线（按天）
func (h *Handlers) HandleStatsRevenue(c *gin.Context) {
	daysStr := c.DefaultQuery("days", "30")
	days, _ := strconv.Atoi(daysStr)
	if days <= 0 || days > 90 {
		days = 30
	}
	end := time.Now()
	start := end.AddDate(0, 0, -days)
	points, err := h.Store.RevenueByDay(start, end)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success", "data": points})
}

// HandleStatsTopCommodities 销量排行
func (h *Handlers) HandleStatsTopCommodities(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	out, err := h.Store.TopCommodities(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success", "data": out})
}

// HandleStatsPayments 支付记录筛选
func (h *Handlers) HandleStatsPayments(c *gin.Context) {
	statusStr := c.DefaultQuery("status", "-1")
	method := c.Query("method")
	status, _ := strconv.Atoi(statusStr)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	out, err := h.Store.ListPayments(status, method, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success", "data": out})
}

func (h *Handlers) rangeStats(from, to time.Time) (int, float64) {
	count, rev, err := h.Store.OrderSummaryRange(from, to)
	if err != nil {
		return 0, 0
	}
	return count, rev
}

// 避免未使用
var _ = store.Order{}
