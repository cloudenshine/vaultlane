package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	Listen         string   `yaml:"listen"`
	Mode           string   `yaml:"mode"`
	ReadTimeout    int      `yaml:"read_timeout"`
	WriteTimeout   int      `yaml:"write_timeout"`
	AllowedOrigins []string `yaml:"allowed_origins"` // VULN-018 / VULN-022：同站 origin 白名单（host 形式）
}

type UpstreamConfig struct {
	BaseURL   string `yaml:"base_url"`
	AppID     string `yaml:"app_id"`
	AppKey    string `yaml:"app_key"`
	Timeout   int    `yaml:"timeout"`
	RetryMax  int    `yaml:"retry_max"`
	UserAgent string `yaml:"user_agent"`
}

// UpstreamEntry 多上游配置项（v4：支持任意数量的上游网关）
// 通过 Name 区分（如 upstreama / custom / mock2）
// Type 决定使用哪个适配器
type UpstreamEntry struct {
	Name      string `yaml:"name"` // 唯一标识（用于 source 字段，如 upstream:upstreama）
	Type      string `yaml:"type"` // 适配器类型：upstreama / mock
	BaseURL   string `yaml:"base_url"`
	AppID     string `yaml:"app_id"`
	AppKey    string `yaml:"app_key"`
	Timeout   int    `yaml:"timeout"`
	RetryMax  int    `yaml:"retry_max"`
	UserAgent string `yaml:"user_agent"`
	Enabled   bool   `yaml:"enabled"`
	// mock 类型专用
	Prefix string `yaml:"prefix"`
	Count  int    `yaml:"count"`
}

type DownstreamBConfig struct {
	Enabled       bool   `yaml:"enabled"`
	AppKey        int64  `yaml:"app_key"`
	AppSecret     string `yaml:"app_secret"`
	SellerID      int64  `yaml:"seller_id"`
	BaseURL       string `yaml:"base_url"`
	CallbackToken string `yaml:"callback_token"`
	Timeout       int    `yaml:"timeout"`
	RetryMax      int    `yaml:"retry_max"`
	UserAgent     string `yaml:"user_agent"`
}

type CacheConfig struct {
	CategoryTTL        int `yaml:"category_ttl"`
	CommodityListTTL   int `yaml:"commodity_list_ttl"`
	CommodityDetailTTL int `yaml:"commodity_detail_ttl"`
	MaxEntries         int `yaml:"max_entries"`
}

type RateLimitConfig struct {
	PerIPPerMin int `yaml:"per_ip_per_min"`
}

type PremiumRule struct {
	Type   int     `yaml:"type"`   // 0=固定 1=百分比
	Amount float64 `yaml:"amount"` // 数值
}

type PricingConfig struct {
	Global             PremiumRule         `yaml:"global"`
	CategoryOverrides  map[int]PremiumRule `yaml:"category_overrides"`
	CommodityOverrides map[int]PremiumRule `yaml:"commodity_overrides"`
}

type AdminConfig struct {
	// 旧 Token 字段已删除（VULN-005）：改用 cookie session 唯一鉴权
	Username        string `yaml:"username"`
	PasswordHash    string `yaml:"password_hash"`  // bcrypt hash
	SessionSecret   string `yaml:"session_secret"` // cookie 签名
	CookieSecure    bool   `yaml:"cookie_secure"`  // 生产 true
	CookieMaxAgeSec int    `yaml:"cookie_max_age"` // 默认 8h
}

type MockConfig struct {
	Enabled bool   `yaml:"enabled"`
	Prefix  string `yaml:"prefix"`
	DelayMs int    `yaml:"delay_ms"`
}

// PayEpayConfig 易支付（彩虹聚合等）配置
type PayEpayConfig struct {
	PID        string `yaml:"pid"`
	Key        string `yaml:"key"`
	GatewayURL string `yaml:"gateway_url"`
	NotifyURL  string `yaml:"notify_url"`
	ReturnURL  string `yaml:"return_url"`
}

// PayUSDTConfig USDT 收款配置
type PayUSDTConfig struct {
	Address string `yaml:"address"`
	APIURL  string `yaml:"api_url"`
}

// SMTPConfig 邮件通知配置（email 模块启用后必填）
type SMTPConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	From     string `yaml:"from"`
	UseTLS   bool   `yaml:"use_tls"`
}

// ModulesConfig 可选模块开关（默认全部关闭，避免"半成品"暴露）
type ModulesConfig struct {
	Coupon             bool `yaml:"coupon"`              // 优惠券系统
	Referral           bool `yaml:"referral"`            // 邀请返佣
	Email              bool `yaml:"email"`               // 邮件通知
	UpstreamSync       bool `yaml:"upstream_sync"`       // 上游商品自动同步
	DownstreamCallback bool `yaml:"downstream_callback"` // 下游网关回调
	FinanceStats       bool `yaml:"finance_stats"`       // 财务统计
	ConfigCenter       bool `yaml:"config_center"`       // 配置中心
	Integrations       bool `yaml:"integrations"`        // 上下游对接面板
}

type Config struct {
	Server      ServerConfig      `yaml:"server"`
	Upstream    UpstreamConfig    `yaml:"upstream"`  // 兼容旧字段
	Upstreams   []UpstreamEntry   `yaml:"upstreams"` // v4：多上游
	DownstreamB DownstreamBConfig `yaml:"downstreamb"`
	Cache       CacheConfig       `yaml:"cache"`
	RateLimit   RateLimitConfig   `yaml:"ratelimit"`
	Pricing     PricingConfig     `yaml:"pricing"`
	Admin       AdminConfig       `yaml:"admin"`
	Mock        MockConfig        `yaml:"mock"`
	PayEpay     PayEpayConfig     `yaml:"pay_epay"`
	PayUSDT     PayUSDTConfig     `yaml:"pay_usdt"`
	SMTP        SMTPConfig        `yaml:"smtp"`
	Modules     ModulesConfig     `yaml:"modules"`
}

// SourceName 把 upstream name 转成 source 字段格式
func (e UpstreamEntry) SourceName() string {
	if e.Name == "" {
		return "upstream:unknown"
	}
	return "upstream:" + e.Name
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if c.Server.Listen == "" {
		c.Server.Listen = "127.0.0.1:8080"
	}
	if c.Server.Mode == "" {
		c.Server.Mode = "release"
	}
	if c.Upstream.UserAgent == "" {
		c.Upstream.UserAgent = "FakaGateway/1.0"
	}
	if c.Cache.MaxEntries == 0 {
		c.Cache.MaxEntries = 512
	}
	if c.DownstreamB.BaseURL == "" {
		c.DownstreamB.BaseURL = "https://downstream-api.example.com"
	}
	if c.DownstreamB.UserAgent == "" {
		c.DownstreamB.UserAgent = "FakaGateway/2.0"
	}
	if c.DownstreamB.Timeout == 0 {
		c.DownstreamB.Timeout = 20
	}
	if c.Admin.CookieMaxAgeSec == 0 {
		c.Admin.CookieMaxAgeSec = 8 * 3600
	}
	// VULN-017：cookie_secure 默认值由 false 改 true（HTTPS-only 部署）。
	// 开发环境（HTTP）请在 config.yaml 显式设 cookie_secure: false。
	// 此处不强制覆盖，保留用户配置；但会在 main 启动时检测 HTTP+secure=false 组合并警告。
	if c.Mock.Prefix == "" {
		c.Mock.Prefix = "MOCK-"
	}
	// v4：旧单 upstream 字段自动转成 upstreams 数组
	if len(c.Upstreams) == 0 && c.Upstream.BaseURL != "" {
		c.Upstreams = []UpstreamEntry{{
			Name:      "upstreama",
			Type:      "upstreama",
			BaseURL:   c.Upstream.BaseURL,
			AppID:     c.Upstream.AppID,
			AppKey:    c.Upstream.AppKey,
			Timeout:   c.Upstream.Timeout,
			RetryMax:  c.Upstream.RetryMax,
			UserAgent: c.Upstream.UserAgent,
			Enabled:   true,
		}}
	}
	// 至少加一个 mock 上游作为默认兜底（用于演示/对比）
	hasMock := false
	for _, u := range c.Upstreams {
		if u.Type == "mock" {
			hasMock = true
			break
		}
	}
	if !hasMock {
		c.Upstreams = append(c.Upstreams, UpstreamEntry{
			Name:    "mock",
			Type:    "mock",
			Enabled: true,
			Prefix:  c.Mock.Prefix,
			Count:   12,
		})
	}
	return &c, nil
}

// ApplyEnv 用环境变量覆盖敏感字段
func (c *Config) ApplyEnv() {
	if v := os.Getenv("UPSTREAM_APP_ID"); v != "" {
		c.Upstream.AppID = v
	}
	if v := os.Getenv("UPSTREAM_APP_KEY"); v != "" {
		c.Upstream.AppKey = v
	}
	if v := os.Getenv("UPSTREAM_BASE_URL"); v != "" {
		c.Upstream.BaseURL = v
	}
	if v := os.Getenv("LISTEN_ADDR"); v != "" {
		c.Server.Listen = v
	}
	if v := os.Getenv("GIN_MODE"); v != "" {
		c.Server.Mode = v
	}
	if v := os.Getenv("Downstream_APP_KEY"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			c.DownstreamB.AppKey = n
		}
	}
	if v := os.Getenv("Downstream_APP_SECRET"); v != "" {
		c.DownstreamB.AppSecret = v
	}
	if v := os.Getenv("ADMIN_USERNAME"); v != "" {
		c.Admin.Username = v
	}
	if v := os.Getenv("ADMIN_PASSWORD_HASH"); v != "" {
		c.Admin.PasswordHash = v
	}
	if v := os.Getenv("ADMIN_SESSION_SECRET"); v != "" {
		c.Admin.SessionSecret = v
	}
	if v := os.Getenv("MOCK_ENABLED"); v == "true" {
		c.Mock.Enabled = true
	}
}
