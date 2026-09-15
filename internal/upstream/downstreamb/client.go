package downstreamb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultGatewayURL = "https://downstream-api.example.com"
	SDKVersion        = "downstream-go-v1.0"
)

// Config 下游网关客户端配置
type Config struct {
	AppKey     int64
	AppSecret  string
	SellerID   int64  // 0 = 自研模式
	GatewayURL string // 默认 https://downstream-api.example.com
	Timeout    int    // 秒
	RetryMax   int    // 重试次数
	UserAgent  string
}

// ApiError 业务错误
type ApiError struct {
	Code int32  `json:"code"`
	Msg  string `json:"msg"`
}

func (e *ApiError) Error() string {
	return fmt.Sprintf("downstream api error: code=%d, msg=%s", e.Code, e.Msg)
}

// Client 下游网关客户端
type Client struct {
	cfg    Config
	http   *http.Client
	logger *slog.Logger

	// 指标
	mu           sync.Mutex
	totalCalls   int64
	successCalls int64
	failedCalls  int64
	latencySum   int64
	lastError    string
	lastErrorAt  time.Time
}

// New 创建客户端
func New(cfg Config, logger *slog.Logger) *Client {
	if cfg.GatewayURL == "" {
		cfg.GatewayURL = DefaultGatewayURL
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 20
	}
	if cfg.RetryMax < 0 {
		cfg.RetryMax = 0
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "Vaultlane/5.0"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		cfg:    cfg,
		http:   &http.Client{Timeout: time.Duration(cfg.Timeout) * time.Second},
		logger: logger,
	}
}

// Metrics 简单指标
type Metrics struct {
	TotalCalls   int64     `json:"total_calls"`
	SuccessCalls int64     `json:"success_calls"`
	FailedCalls  int64     `json:"failed_calls"`
	AvgLatencyMs int64     `json:"avg_latency_ms"`
	LastError    string    `json:"last_error"`
	LastErrorAt  time.Time `json:"last_error_at"`
	AppKey       int64     `json:"app_key"`
	SellerID     int64     `json:"seller_id"`
	GatewayURL   string    `json:"gateway_url"`
}

func (c *Client) Metrics() Metrics {
	c.mu.Lock()
	defer c.mu.Unlock()
	avg := int64(0)
	if c.totalCalls > 0 {
		avg = c.latencySum / c.totalCalls
	}
	return Metrics{
		TotalCalls:   c.totalCalls,
		SuccessCalls: c.successCalls,
		FailedCalls:  c.failedCalls,
		AvgLatencyMs: avg,
		LastError:    c.lastError,
		LastErrorAt:  c.lastErrorAt,
		AppKey:       c.cfg.AppKey,
		SellerID:     c.cfg.SellerID,
		GatewayURL:   c.cfg.GatewayURL,
	}
}

func (c *Client) recordCall(duration time.Duration, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.totalCalls++
	c.latencySum += duration.Milliseconds()
	if ok {
		c.successCalls++
	} else {
		c.failedCalls++
	}
}

func (c *Client) recordErr(msg string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastError = msg
	c.lastErrorAt = time.Now()
}

// envelope 响应信封
type envelope struct {
	Code int32           `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

// execute 通用 HTTP 调用
func (c *Client) execute(ctx context.Context, apiPath string, body, result any) error {
	var bodyJSON string
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		bodyJSON = string(b)
	} else {
		bodyJSON = "{}"
	}

	ts := time.Now().Unix()
	var sign string
	if c.cfg.SellerID > 0 {
		sign = GenerateSignWithSeller(c.cfg.AppKey, c.cfg.AppSecret, bodyJSON, ts, c.cfg.SellerID)
	} else {
		sign = GenerateSign(c.cfg.AppKey, c.cfg.AppSecret, bodyJSON, ts)
	}

	q := url.Values{}
	q.Set("appid", strconv.FormatInt(c.cfg.AppKey, 10))
	q.Set("timestamp", strconv.FormatInt(ts, 10))
	if c.cfg.SellerID > 0 {
		q.Set("seller_id", strconv.FormatInt(c.cfg.SellerID, 10))
	}
	q.Set("sign", sign)

	fullURL := c.cfg.GatewayURL + apiPath + "?" + q.Encode()

	// 重试
	maxRetries := c.cfg.RetryMax
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * 200 * time.Millisecond
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}
		err := c.doOnce(ctx, fullURL, bodyJSON, result)
		if err == nil {
			return nil
		}
		lastErr = err
		// 业务错误（ApiError）不重试
		var apiErr *ApiError
		if errors.As(err, &apiErr) {
			return err
		}
		c.logger.Warn("downstream retry", "attempt", attempt+1, "err", err, "url", apiPath)
	}
	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

func (c *Client) doOnce(ctx context.Context, fullURL, bodyJSON string, result any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader([]byte(bodyJSON)))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json;charset=utf-8")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("sdk_version", SDKVersion)

	start := time.Now()
	resp, err := c.http.Do(req)
	duration := time.Since(start)
	if err != nil {
		c.recordCall(duration, false)
		c.recordErr(err.Error())
		return fmt.Errorf("http do: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		c.recordCall(duration, false)
		c.recordErr("read body: " + err.Error())
		return fmt.Errorf("read body: %w", err)
	}

	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		c.recordCall(duration, false)
		c.recordErr("unmarshal: " + string(data[:min(200, len(data))]))
		return fmt.Errorf("unmarshal envelope: %w (raw=%s)", err, truncate(string(data), 200))
	}

	if env.Code != 0 {
		c.recordCall(duration, false)
		apiErr := &ApiError{Code: env.Code, Msg: env.Msg}
		c.recordErr(apiErr.Error())
		return apiErr
	}

	if result != nil && len(env.Data) > 0 && !isNullJSON(env.Data) {
		if err := json.Unmarshal(env.Data, result); err != nil {
			c.recordCall(duration, false)
			c.recordErr("unmarshal data: " + err.Error())
			return fmt.Errorf("unmarshal data: %w", err)
		}
	}

	c.recordCall(duration, true)
	return nil
}

func isNullJSON(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	return s == "null" || s == "" || s == "{}"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
