package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"faka-gateway/internal/admin"
	"faka-gateway/internal/api"
	"faka-gateway/internal/config"
	"faka-gateway/internal/crypto"
	"faka-gateway/internal/store"
	"faka-gateway/internal/upstream"
	"faka-gateway/internal/upstream/downstreamb"
	"faka-gateway/internal/web"
)

func main() {
	// 加载 .env（若存在）
	_ = godotenv.Load()

	cfg, err := config.Load("config/config.yaml")
	if err != nil {
		slog.Error("load config failed", "err", err)
		os.Exit(1)
	}

	// 环境变量覆盖敏感字段
	cfg.ApplyEnv()

	// VULN-006：检测 admin 密码 hash，启动时强制要求配置
	if err := checkAdminPasswordConfigured(cfg); err != nil {
		slog.Error("admin password 安全检查失败", "err", err)
		fmt.Fprintln(os.Stderr, "\n"+err.Error()+"\n")
		os.Exit(1)
	}

	// VULN-016：检测默认 session_secret（仓库内公开值，cookie 签名可被伪造）
	if err := checkDefaultSessionSecret(cfg); err != nil {
		slog.Error("admin session_secret 安全检查失败", "err", err)
		fmt.Fprintln(os.Stderr, "\n"+err.Error()+"\n")
		os.Exit(1)
	}

	// 日志
	logger := newLogger(cfg)
	slog.SetDefault(logger)

	slog.Info("vaultlane starting",
		"listen", cfg.Server.Listen,
		"upstreams", len(cfg.Upstreams),
	)

	// 初始化上游管理器（v4：多上游）
	mgr := upstream.NewManager(cfg)
	// 默认上游 = 配置里第一个 upstreama 类型（或第一个）
	var defaultUp upstream.Adapter
	for _, e := range cfg.Upstreams {
		if !e.Enabled {
			continue
		}
		if a, ok := mgr.Adapter(e.Name); ok {
			if defaultUp == nil {
				defaultUp = a
			}
		}
	}
	if defaultUp == nil {
		// 兜底：mock
		defaultUp, _ = mgr.Adapter("mock")
	}
	if defaultUp == nil {
		slog.Error("no upstream adapter available")
		os.Exit(1)
	}
	// 兼容老代码：保留 *upstream.Client 字段（旧 Mock 切换面板用）
	// 真实类型是 Adapter
	if cfg.Mock.Enabled {
		slog.Warn("MOCK MODE ENABLED (legacy)", "prefix", cfg.Mock.Prefix, "delay_ms", cfg.Mock.DelayMs)
	}

	// 初始化下游网关客户端（可选）
	var downstreamClient *downstreamb.Client
	if cfg.DownstreamB.Enabled && cfg.DownstreamB.AppKey > 0 && cfg.DownstreamB.AppSecret != "" {
		downstreamClient = downstreamb.New(downstreamb.Config{
			AppKey:     cfg.DownstreamB.AppKey,
			AppSecret:  cfg.DownstreamB.AppSecret,
			SellerID:   cfg.DownstreamB.SellerID,
			GatewayURL: cfg.DownstreamB.BaseURL,
			Timeout:    cfg.DownstreamB.Timeout,
			RetryMax:   cfg.DownstreamB.RetryMax,
			UserAgent:  cfg.DownstreamB.UserAgent,
		}, logger)
		slog.Info("downstreamb enabled", "app_key", cfg.DownstreamB.AppKey, "seller_id", cfg.DownstreamB.SellerID)
	} else {
		slog.Info("downstreamb disabled")
	}

	// 初始化存储
	ordStore, err := store.NewSQLite("data/gateway.db")
	if err != nil {
		slog.Error("init store failed", "err", err)
		os.Exit(1)
	}
	defer ordStore.Close()
	// 卡密 AES-256-GCM 加密（从 session_secret 派生）
	ordStore.Cipher = crypto.NewFromSecret(cfg.Admin.SessionSecret)

	// 后台上游自动同步与价格熔断器
	syncer := upstream.NewSyncer(mgr, ordStore, cfg, logger)
	if cfg.Modules.UpstreamSync {
		syncer.Start(15 * time.Minute)
		defer syncer.Stop()
	}

	// Gin
	gin.SetMode(cfg.Server.Mode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(api.LoggingMiddleware(logger))
	// VULN-018：同站 origin 白名单驱动 CORS。空列表拒绝所有跨域。
	allowedOrigins := cfg.Server.AllowedOrigins
	if len(allowedOrigins) == 0 {
		// 兜底：允许同源直访（无 Origin 头），不暴露 Access-Control-Allow-Origin
		allowedOrigins = []string{}
	}
	r.Use(cors.New(cors.Config{
		AllowOrigins:     allowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}))

	// 路由
	api.Register(r, cfg, defaultUp, mgr, ordStore, logger)

	// 下游网关订单回调（公开）
	downstreamCb := api.NewDownstreamCallbackHandler(ordStore, downstreamClient, cfg.DownstreamB.CallbackToken, logger)
	downstreamCb.RegisterRoutes(r.Group("/api"))

	// 后台管理（独立路径 + cookie session）
	adminH := &admin.Handlers{
		Cfg:        cfg,
		Store:      ordStore,
		Downstream: downstreamClient,
		Logger:     logger,
		Upstream:   upAdpSetter{defaultUp, mgr},
		Pool: &admin.PoolHandlers{
			Store:   ordStore,
			Manager: mgr,
			Logger:  logger,
		},
		UpstreamCalls: func() map[string]any {
			metrics := mgr.AllMetrics()
			out := map[string]any{"upstreams": map[string]any{}}
			if ups, ok := out["upstreams"].(map[string]any); ok {
				for n, m := range metrics {
					ups[n] = map[string]any{
						"total_calls":    m.TotalCalls,
						"success_calls":  m.SuccessCalls,
						"failed_calls":   m.FailedCalls,
						"avg_latency_ms": m.AvgLatencyMs,
						"last_error":     m.LastError,
					}
				}
			}
			return out
		},
		UpstreamCache: func() map[string]any {
			// 简化为空（v4 缓存由 adapter 内部 TTL 管理）
			return map[string]any{}
		},
	}
	admin.Register(r, adminH)

	// 静态资源（embed）
	web.Register(r)

	// 服务
	srv := &http.Server{
		Addr:         cfg.Server.Listen,
		Handler:      r,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeout) * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("listen failed", "err", err)
			os.Exit(1)
		}
	}()

	slog.Info("ready", "addr", cfg.Server.Listen)

	// 优雅退出
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func newLogger(cfg *config.Config) *slog.Logger {
	level := slog.LevelInfo
	switch cfg.Server.Mode {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	var handler slog.Handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(handler)
}

// upAdpSetter 包装 Adapter + Manager 实现 admin.UpstreamSetter
type upAdpSetter struct {
	up  upstream.Adapter
	mgr *upstream.Manager
}

func (s upAdpSetter) SetMock(enabled bool, prefix string, delayMs int) {
	if s.up != nil {
		s.up.SetMock(enabled, prefix, delayMs)
	}
}

func (s upAdpSetter) MockStatus() map[string]any {
	if s.up == nil {
		return map[string]any{}
	}
	return s.up.MockStatus()
}

// checkAdminPasswordConfigured 检测 admin 密码 hash 是否已配置（VULN-006）。
func checkAdminPasswordConfigured(cfg *config.Config) error {
	if strings.TrimSpace(cfg.Admin.PasswordHash) != "" {
		return nil
	}
	return fmt.Errorf(`admin.password_hash 未配置。
  生成新 hash: go run ./cmd/hash-password <你的新密码>
  然后把输出写到 config/config.yaml 的 admin.password_hash
  或通过环境变量 ADMIN_PASSWORD_HASH=... 覆盖`)
}

// checkDefaultSessionSecret 检测默认 session_secret（VULN-016）
// 默认值在仓库公开 = cookie 签名可被任意攻击者伪造。
func checkDefaultSessionSecret(cfg *config.Config) error {
	const defaultSecret = "ZGVmYXVsdC1zZWNyZXQtMzItYnl0ZS1jaGFuZ2UtbWU="
	if cfg.Admin.SessionSecret != defaultSecret {
		return nil
	}
	return fmt.Errorf(`检测到默认 admin.session_secret（仓库公开值，cookie 签名可被伪造）。
  生成新值: openssl rand -base64 32
  然后把输出写到 config/config.yaml 的 admin.session_secret
  或通过环境变量 ADMIN_SESSION_SECRET=... 覆盖`)
}
