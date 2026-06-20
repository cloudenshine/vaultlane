package api

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"faka-gateway/internal/config"
	"faka-gateway/internal/delivery"
	"faka-gateway/internal/domain"
	"faka-gateway/internal/payment"
	"faka-gateway/internal/store"
	"faka-gateway/internal/upstream"
)

// Deps 路由依赖
type Deps struct {
	Cfg      *config.Config
	Up       upstream.Adapter  // 默认上游（upstreama），用于下单/查单
	Manager  *upstream.Manager // v4：多上游管理器
	Store    *store.Store
	Premium  *domain.PremiumEngine
	Logger   *slog.Logger
	Limiter  *RateLimiter
	Pay      *payment.Manager
	Notifier *Notifier
	UserH    *UserHandler
	PayH     *PaymentHandler
}

// Register 注册所有路由
func Register(r *gin.Engine, cfg *config.Config, defaultUp upstream.Adapter, mgr *upstream.Manager, st *store.Store, logger *slog.Logger) {
	d := &Deps{
		Cfg:     cfg,
		Up:      defaultUp,
		Manager: mgr,
		Store:   st,
		Premium: domain.NewPremiumEngine(cfg.Pricing),
		Logger:  logger,
		Limiter: NewRateLimiter(cfg.RateLimit.PerIPPerMin),
	}
	// Notifier（含发货引擎）
	dispatcher := delivery.NewDispatcher(
		&delivery.ManualEngine{},
		&delivery.SecretEngine{Store: st},
		&delivery.UpstreamEngine{Up: defaultUp, Manager: mgr, Logger: logger},
		&delivery.EmailEngine{},
	)
	d.Notifier = &Notifier{Store: st, Up: defaultUp, Manager: mgr, Logger: logger, Dispatcher: dispatcher}
	// 支付管理
	d.Pay = payment.NewManager()
	// 余额支付适配器
	d.Pay.Register(payment.NewBalanceEngine(balanceAdapter{Store: st}, balanceAdapter{Store: st}, logger))
	// epay（仅在配置启用时注册；无配置时尝试从 settings 加载）
	if cfg.PayEpay.PID != "" && cfg.PayEpay.Key != "" {
		d.Pay.Register(payment.NewEpayEngine(payment.EpayConfig{
			PID: cfg.PayEpay.PID, Key: cfg.PayEpay.Key, GatewayURL: cfg.PayEpay.GatewayURL,
			NotifyURL: cfg.PayEpay.NotifyURL, ReturnURL: cfg.PayEpay.ReturnURL,
		}))
	}
	// USDT 占位（地址非空时启用）
	if cfg.PayUSDT.Address != "" {
		d.Pay.Register(payment.NewUSDTEngine(cfg.PayUSDT.Address, cfg.PayUSDT.APIURL))
	}
	// 用户 / 支付 handler
	d.UserH = &UserHandler{Store: st, Logger: logger}
	// VULN-013：业务维度限流 10/min/uid，独立桶（rate=10）
	d.PayH = &PaymentHandler{Store: st, Up: defaultUp, Pay: d.Pay, Logger: logger, Notify: d.Notifier, UserH: d.UserH, Limiter: NewRateLimiter(10)}

	// 公开 API（限流）
	api := r.Group("/api")
	api.Use(d.Limiter.Middleware())
	{
		// 公开配置（前端读取上游 base URL 等，无需登录）
		api.GET("/public/config", d.handlePublicConfig)
		api.GET("/categories", d.handleCategories)
		api.GET("/commodities", d.handleCommodities)
		api.GET("/commodities/:id", d.handleCommodityDetail)

		// 旧 alias（保留）
		api.GET("/orders/items/list", d.handleOrderableItems)

		// 用户（公开部分 + 受保护部分）
		d.UserH.RegisterRoutes(api, d.UserH.RequireUser())

		// 支付（包含 /orders 下单路由）
		d.PayH.RegisterRoutes(api, d.UserH.RequireUser())
	}

	// 健康检查
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true, "ts": strconv.FormatInt(upstream_unix(), 10)})
	})
}

// 兼容写法（避免重复 import）
func upstream_unix() int64 { return 0 }
