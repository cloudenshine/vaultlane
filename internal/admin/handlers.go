package admin

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"faka-gateway/internal/config"
	"faka-gateway/internal/store"
	"faka-gateway/internal/upstream/downstreamb"
)

// Handlers 后台处理器
type Handlers struct {
	Cfg        *config.Config
	Store      *store.Store
	Downstream *downstreamb.Client
	Logger     *slog.Logger

	// upstream 指标（可选注入）
	UpstreamCalls func() map[string]any
	UpstreamCache func() map[string]any
	Upstream      UpstreamSetter // mock 切换用

	// v4：商品池
	Pool *PoolHandlers
}

// audit 写入审计日志（VULN-012）
// actor 从 c.Get("admin_user") 取，ip 从 c.ClientIP() 取。
func (h *Handlers) audit(c *gin.Context, action, target string, detail any) {
	actor := c.GetString("admin_user")
	if actor == "" {
		actor = "system"
	}
	h.Store.WriteAudit(actor, action, target, detail, c.ClientIP())
}

// UpstreamSetter 抽象（避免 admin 包反向依赖 upstream 包的实现）
type UpstreamSetter interface {
	SetMock(enabled bool, prefix string, delayMs int)
	MockStatus() map[string]any
}

// Register 注册后台路由
func Register(r *gin.Engine, h *Handlers) {
	g := r.Group("/admin")
	{
		// 公开：登录页 + 登录提交 + 静态资源
		g.GET("", func(c *gin.Context) {
			// 已登录直接跳 dashboard
			token := ReadSessionCookie(c)
			if _, err := DecodeSession(token, h.Cfg.Admin.SessionSecret); err == nil {
				c.Redirect(http.StatusFound, "/admin/dashboard")
				return
			}
			c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(LoginPageHTML))
		})
		g.GET("/", func(c *gin.Context) {
			c.Redirect(http.StatusFound, "/admin")
		})

		g.POST("/api/login", h.HandleLogin)
		g.POST("/api/logout", h.HandleLogout)

		// 静态资源
		staticFS, _ := fs.Sub(embeddedStatic, "static")
		g.StaticFS("/static", http.FS(staticFS))

		// SPA
		g.GET("/dashboard", func(c *gin.Context) {
			mod := h.Cfg.Modules
			modJSON := fmt.Sprintf(`{"finance_stats":%t,"config_center":%t,"integrations":%t,"coupon":%t,"referral":%t,"email":%t,"upstream_sync":%t,"downstream_callback":%t}`,
				mod.FinanceStats, mod.ConfigCenter, mod.Integrations, mod.Coupon, mod.Referral, mod.Email, mod.UpstreamSync, mod.DownstreamCallback)
			c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(renderDashboardSPA(modJSON)))
		})

		// 受保护 API
		api := g.Group("/api")
		api.Use(h.requireSession())
		api.Use(CSRFMiddleware(h.Cfg.Admin.SessionSecret))
		{
			api.GET("/dashboard", h.HandleDashboard)
			api.GET("/orders", h.HandleOrders)
			api.GET("/commodities", h.HandleListCommodities)
			api.POST("/commodities", h.HandleCreateCommodity)
			api.PUT("/commodities/:id", h.RequireTOTP(), h.HandleUpdateCommodity)
			api.DELETE("/commodities/:id", h.RequireTOTP(), h.HandleDeleteCommodity)
			api.GET("/secrets", h.HandleListSecrets)
			api.POST("/secrets/import", h.HandleImportSecrets)
			api.GET("/audit", h.HandleListAudit) // VULN-012
			api.GET("/categories", h.HandleListCategories)
			api.POST("/categories", h.HandleCreateCategory)

			// 优惠券管理
			api.GET("/coupons", h.HandleListCoupons)
			api.POST("/coupons", h.HandleCreateCoupon)

			// TOTP 双因子认证管理
			api.POST("/totp/setup", h.HandleTOTPSetup)
			api.POST("/totp/confirm", h.HandleTOTPConfirm)
			api.POST("/totp/disable", h.RequireTOTP(), h.HandleTOTPDisable)
			api.GET("/totp/status", func(c *gin.Context) {
				secret := h.Store.GetSettingValue("totp_secret")
				c.JSON(http.StatusOK, gin.H{"code": 200, "data": gin.H{"enabled": secret != ""}})
			})

			// 高危操作：批量改价 / 删除 / 余额调整需额外 TOTP
			// (TOTP 中间件按需挂在具体路由上)

			// 配置中心（可关闭）
			if h.Cfg.Modules.ConfigCenter {
				api.GET("/settings", h.HandleSettings)
				api.PUT("/settings", h.HandleSettingsPut)
				api.POST("/settings/mock", h.HandleToggleMock)
			}

			// 上下游对接（可关闭）
			if h.Cfg.Modules.Integrations {
				api.GET("/downstream/test", h.HandleDownstreamTest)
				api.GET("/downstream/products", h.HandleDownstreamListProducts)
				api.POST("/downstream/products/sync", h.HandleDownstreamSyncProduct)
				api.POST("/downstream/products/publish", h.HandleDownstreamPublishProduct)
				api.GET("/downstream/auths", h.HandleDownstreamListAuths)
			}

			// 财务统计（可关闭）
			if h.Cfg.Modules.FinanceStats {
				api.GET("/stats/overview", h.HandleStatsOverview)
				api.GET("/stats/revenue", h.HandleStatsRevenue)
				api.GET("/stats/top_commodities", h.HandleStatsTopCommodities)
				api.GET("/stats/payments", h.HandleStatsPayments)
			}

			// 商品池（v4 必备）
			if h.Pool != nil {
				h.RegisterPoolRoutes(api, h.Pool)
			}
		}
	}
}

func (h *Handlers) requireSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := ReadSessionCookie(c)
		sess, err := DecodeSession(token, h.Cfg.Admin.SessionSecret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "未登录或会话已过期"})
			return
		}
		c.Set("admin_user", sess.User)
		c.Next()
	}
}

// HandleLogin 登录
func (h *Handlers) HandleLogin(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	if h.Cfg.Admin.Username == "" || h.Cfg.Admin.PasswordHash == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "msg": "后台未初始化，请联系运维"})
		return
	}
	if req.Username != h.Cfg.Admin.Username {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "用户名或密码错误"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(h.Cfg.Admin.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "用户名或密码错误"})
		return
	}

	sess := &Session{
		User:      req.Username,
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(time.Duration(h.Cfg.Admin.CookieMaxAgeSec) * time.Second),
	}
	token, err := EncodeSession(sess, h.Cfg.Admin.SessionSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "会话编码失败"})
		return
	}
	SetSessionCookie(c, token, h.Cfg.Admin.CookieMaxAgeSec, h.Cfg.Admin.CookieSecure)
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "登录成功", "data": gin.H{"user": req.Username}})
}

// HandleLogout 退出
func (h *Handlers) HandleLogout(c *gin.Context) {
	ClearSessionCookie(c, h.Cfg.Admin.CookieSecure)
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已退出"})
}

// HandleDashboard 仪表盘
func (h *Handlers) HandleDashboard(c *gin.Context) {
	commodityCount, _ := h.Store.CountCommodities()
	orderCount, _ := h.Store.CountOrders()
	data := gin.H{
		"commodity_count": commodityCount,
		"order_count":     orderCount,
	}
	if h.UpstreamCalls != nil {
		data["upstream"] = h.UpstreamCalls()
	}
	if h.UpstreamCache != nil {
		data["cache"] = h.UpstreamCache()
	}
	if h.Downstream != nil {
		data["downstream"] = h.Downstream.Metrics()
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success", "data": data})
}

// HandleOrders 订单列表
func (h *Handlers) HandleOrders(c *gin.Context) {
	orders, err := h.Store.ListOrders(100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	for i := range orders {
		orders[i].Password = ""
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success", "data": orders})
}

// HandleListCommodities 商品列表（v4 用商品池接口）
func (h *Handlers) HandleListCommodities(c *gin.Context) {
	source := c.Query("source")
	keyword := c.Query("keyword")
	page, _ := parseIntQuery(c, "page", 1)
	limit, _ := parseIntQuery(c, "limit", 100)
	out, total, err := h.Store.ListAllCommodities(source, keyword, page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	bySource, _ := h.Store.CountCommoditiesBySource()
	c.JSON(http.StatusOK, gin.H{
		"code":  200,
		"msg":   "success",
		"data":  out,
		"total": total,
		"page":  page,
		"limit": limit,
		"stats": gin.H{"by_source": bySource},
	})
}

// HandleCreateCommodity 创建商品
func (h *Handlers) HandleCreateCommodity(c *gin.Context) {
	var com store.Commodity
	if err := c.ShouldBindJSON(&com); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	if com.Name == "" || com.Price <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "商品名和价格必填"})
		return
	}
	if com.Source == "" {
		com.Source = "self"
	}
	if com.DeliveryWay == 0 {
		com.DeliveryWay = 1
	}
	if com.Status == 0 {
		com.Status = 1
	}
	if err := h.Store.CreateCommodity(&com); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	h.audit(c, "commodity.create", strconv.FormatInt(com.ID, 10), gin.H{"name": com.Name, "price": com.Price, "source": com.Source})
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已创建", "data": com})
}

// HandleUpdateCommodity 更新商品
func (h *Handlers) HandleUpdateCommodity(c *gin.Context) {
	id, err := parseInt64(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "invalid id"})
		return
	}
	existing, err := h.Store.GetCommodityByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "商品不存在"})
		return
	}
	var com store.Commodity
	if err := c.ShouldBindJSON(&com); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	com.ID = id
	if err := h.Store.UpdateCommodity(&com); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	h.audit(c, "commodity.update", strconv.FormatInt(id, 10), gin.H{
		"before": gin.H{"name": existing.Name, "price": existing.Price, "status": existing.Status},
		"after":  gin.H{"name": com.Name, "price": com.Price, "status": com.Status},
	})
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已更新", "data": com})
}

// HandleDeleteCommodity 删除商品
func (h *Handlers) HandleDeleteCommodity(c *gin.Context) {
	id, err := parseInt64(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "invalid id"})
		return
	}
	if err := h.Store.DeleteCommodity(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	h.audit(c, "commodity.delete", strconv.FormatInt(id, 10), nil)
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已删除"})
}

// HandleListSecrets 卡密列表
func (h *Handlers) HandleListSecrets(c *gin.Context) {
	commID, _ := parseInt64Query(c, "commodity_id")
	status, _ := parseIntQuery(c, "status", -1)
	limit, _ := parseIntQuery(c, "limit", 50)
	out, err := h.Store.ListSecrets(commID, status, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success", "data": out})
}

// HandleImportSecrets 批量导入
func (h *Handlers) HandleImportSecrets(c *gin.Context) {
	var req struct {
		CommodityID int64    `json:"commodity_id"`
		Contents    []string `json:"contents"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	if req.CommodityID <= 0 || len(req.Contents) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "commodity_id 和 contents 必填"})
		return
	}
	n, err := h.Store.ImportSecrets(req.CommodityID, req.Contents)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	h.audit(c, "secrets.import", strconv.FormatInt(req.CommodityID, 10), gin.H{"count": n})
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "导入成功", "data": gin.H{"imported": n}})
}

// HandleListCategories 分类
func (h *Handlers) HandleListCategories(c *gin.Context) {
	out, err := h.Store.ListCategories()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success", "data": out})
}

// HandleListAudit 审计日志查询（VULN-012）
// GET /admin/api/audit?limit=50&offset=0
func (h *Handlers) HandleListAudit(c *gin.Context) {
	limit, _ := parseIntQuery(c, "limit", 50)
	offset, _ := parseIntQuery(c, "offset", 0)
	if limit > 500 {
		limit = 500
	}
	out, err := h.Store.ListAudit(limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	total, _ := h.Store.CountAudit()
	c.JSON(http.StatusOK, gin.H{
		"code": 200, "msg": "success",
		"data": out, "total": total, "limit": limit, "offset": offset,
	})
}

// HandleCreateCategory 创建分类
func (h *Handlers) HandleCreateCategory(c *gin.Context) {
	var cat store.Category
	if err := c.ShouldBindJSON(&cat); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	if cat.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "name 必填"})
		return
	}
	if cat.Source == "" {
		cat.Source = "self"
	}
	if err := h.Store.CreateCategory(&cat); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已创建", "data": cat})
}

// HandleDownstreamTest 下游网关连通测试
func (h *Handlers) HandleDownstreamTest(c *gin.Context) {
	if h.Downstream == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "msg": "下游网关未启用"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	resp, err := h.Downstream.GetUserAuthorizeList(ctx, &downstreamb.GetUserAuthorizeListReq{})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"ok":      false,
			"metrics": h.Downstream.Metrics(),
			"err":     err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"metrics": h.Downstream.Metrics(),
		"auths":   resp.List,
	})
}

// HandleDownstreamListProducts 下游网关商品列表
func (h *Handlers) HandleDownstreamListProducts(c *gin.Context) {
	if h.Downstream == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "msg": "下游网关未启用"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	pageNo, _ := parseIntQuery(c, "page", 1)
	pageSize, _ := parseIntQuery(c, "size", 50)
	resp, err := h.Downstream.GetOpenProductList(ctx, &downstreamb.GetOpenProductListReq{
		PageNo:   int32(pageNo),
		PageSize: int32(pageSize),
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": 502, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success", "data": resp})
}

// HandleDownstreamSyncProduct 同步本地商品到下游网关
func (h *Handlers) HandleDownstreamSyncProduct(c *gin.Context) {
	if h.Downstream == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "msg": "下游网关未启用"})
		return
	}
	var req struct {
		CommodityID int64 `json:"commodity_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	com, err := h.Store.GetCommodityByID(req.CommodityID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	if com == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "本地商品不存在"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	// 注：商品创建需要 publish_shop 字段（闲鱼授权账号），这里简化占位
	resp, err := h.Downstream.CreateOpenProduct(ctx, &downstreamb.OpenProductData{
		ItemBizType:  2, // 虚拟商品
		SpBizType:    1,
		ChannelCatID: "1001",
		Title:        com.Name,
		Price:        int64(com.Price * 100),
		Stock:        int32(com.Stock),
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": 502, "msg": err.Error()})
		return
	}
	// 保存 outer_id
	com.OuterID = strconvInt64(resp.ProductID)
	com.Source = "upstream:downstreamb"
	_ = h.Store.UpdateCommodity(com)
	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "已同步到下游网关",
		"data": resp,
	})
}

// HandleDownstreamPublishProduct 上架
func (h *Handlers) HandleDownstreamPublishProduct(c *gin.Context) {
	if h.Downstream == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "msg": "下游网关未启用"})
		return
	}
	var req struct {
		ProductID int64    `json:"product_id"`
		UserName  []string `json:"user_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	if req.ProductID <= 0 || len(req.UserName) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "product_id 与 user_name 必填"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	if err := h.Downstream.PublishOpenProduct(ctx, &downstreamb.PublishOpenProductReq{
		ProductID: req.ProductID,
		UserName:  req.UserName,
	}); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": 502, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已上架（异步）"})
}

// HandleDownstreamListAuths 闲鱼店铺授权
func (h *Handlers) HandleDownstreamListAuths(c *gin.Context) {
	if h.Downstream == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "msg": "下游网关未启用"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	resp, err := h.Downstream.GetUserAuthorizeList(ctx, &downstreamb.GetUserAuthorizeListReq{})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": 502, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success", "data": resp})
}

// HandleSettings 系统设置
func (h *Handlers) HandleSettings(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "success",
		"data": gin.H{
			"upstream": gin.H{
				"base_url": h.Cfg.Upstream.BaseURL,
				"app_id":   h.Cfg.Upstream.AppID,
			},
			"downstreamb": gin.H{
				"enabled":   h.Cfg.DownstreamB.Enabled,
				"app_key":   h.Cfg.DownstreamB.AppKey,
				"seller_id": h.Cfg.DownstreamB.SellerID,
				"base_url":  h.Cfg.DownstreamB.BaseURL,
			},
			"mock": gin.H{
				"enabled":  h.Cfg.Mock.Enabled,
				"prefix":   h.Cfg.Mock.Prefix,
				"delay_ms": h.Cfg.Mock.DelayMs,
			},
			"rate_limit": h.Cfg.RateLimit.PerIPPerMin,
			"modules": gin.H{
				"coupon":              h.Cfg.Modules.Coupon,
				"referral":            h.Cfg.Modules.Referral,
				"email":               h.Cfg.Modules.Email,
				"upstream_sync":       h.Cfg.Modules.UpstreamSync,
				"downstream_callback": h.Cfg.Modules.DownstreamCallback,
				"finance_stats":       h.Cfg.Modules.FinanceStats,
				"config_center":       h.Cfg.Modules.ConfigCenter,
				"integrations":        h.Cfg.Modules.Integrations,
			},
		},
	})
}

// HandleToggleMock 切换 Mock 模式（VULN-015 修复：需要二次密码确认）
// Mock 模式一旦开启，上游所有交易走模拟，可能被用来"无限拿卡不消耗真上游余额"
func (h *Handlers) HandleToggleMock(c *gin.Context) {
	var req struct {
		Enabled bool   `json:"enabled"`
		Confirm string `json:"confirm"` // 二次确认：传入 admin 密码
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	// 校验密码（防止 admin session 被窃取后滥用）
	if err := bcrypt.CompareHashAndPassword([]byte(h.Cfg.Admin.PasswordHash), []byte(req.Confirm)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "密码错误，无法切换 mock"})
		return
	}
	h.Cfg.Mock.Enabled = req.Enabled
	if h.Upstream != nil {
		h.Upstream.SetMock(req.Enabled, h.Cfg.Mock.Prefix, h.Cfg.Mock.DelayMs)
	}
	h.Logger.Warn("mock mode toggled", "enabled", req.Enabled, "by", c.GetString("admin_user"))
	h.audit(c, "mock.toggle", "global", gin.H{"enabled": req.Enabled, "prefix": h.Cfg.Mock.Prefix, "delay_ms": h.Cfg.Mock.DelayMs})
	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "Mock 已" + map[bool]string{true: "开启", false: "关闭"}[req.Enabled],
		"data": gin.H{"enabled": req.Enabled},
	})
}

// HandleSettingsPut 批量更新（与 GET 形状对应）
// 接受嵌套结构：{mock:{enabled,prefix,delay_ms}, rate_limit:N}
func (h *Handlers) HandleSettingsPut(c *gin.Context) {
	var req map[string]any
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	if mockAny, ok := req["mock"]; ok {
		if mock, ok := mockAny.(map[string]any); ok {
			if enabled, ok := mock["enabled"].(bool); ok {
				h.Cfg.Mock.Enabled = enabled
				if h.Upstream != nil {
					h.Upstream.SetMock(enabled, h.Cfg.Mock.Prefix, h.Cfg.Mock.DelayMs)
				}
			}
			if prefix, ok := mock["prefix"].(string); ok && prefix != "" {
				h.Cfg.Mock.Prefix = prefix
				if h.Upstream != nil {
					h.Upstream.SetMock(h.Cfg.Mock.Enabled, prefix, h.Cfg.Mock.DelayMs)
				}
			}
			if delay, ok := mock["delay_ms"].(float64); ok {
				h.Cfg.Mock.DelayMs = int(delay)
				if h.Upstream != nil {
					h.Upstream.SetMock(h.Cfg.Mock.Enabled, h.Cfg.Mock.Prefix, int(delay))
				}
			}
		}
	}
	if rl, ok := req["rate_limit"].(float64); ok && rl > 0 {
		h.Cfg.RateLimit.PerIPPerMin = int(rl)
	}
	h.audit(c, "settings.update", "global", req)
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已保存"})
}

// 解析助手（避免重复 import strconv）
func parseInt64(s string) (int64, error) {
	if s == "" {
		return 0, errors.New("empty")
	}
	var n int64
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, errors.New("invalid")
		}
		n = n*10 + int64(ch-'0')
	}
	return n, nil
}

func parseInt64Query(c *gin.Context, key string) (int64, error) {
	return parseInt64(c.Query(key))
}

func parseIntQuery(c *gin.Context, key string, def int) (int, error) {
	v := c.Query(key)
	if v == "" {
		return def, nil
	}
	var n int
	for _, ch := range v {
		if ch < '0' || ch > '9' {
			return def, nil
		}
		n = n*10 + int(ch-'0')
	}
	return n, nil
}

func strconvInt64(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// HandleListCoupons 列出所有优惠券
func (h *Handlers) HandleListCoupons(c *gin.Context) {
	coupons, err := h.Store.ListCoupons()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success", "data": coupons})
}

// HandleCreateCoupon 创建优惠券
func (h *Handlers) HandleCreateCoupon(c *gin.Context) {
	var coupon store.Coupon
	if err := c.ShouldBindJSON(&coupon); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}
	if coupon.Code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "券码不能为空"})
		return
	}
	if coupon.Discount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "折扣/减免金额必须大于0"})
		return
	}
	if err := h.Store.CreateCoupon(&coupon); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	h.audit(c, "coupon.create", coupon.Code, gin.H{"name": coupon.Name, "type": coupon.Type, "discount": coupon.Discount})
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "创建成功", "data": coupon})
}
