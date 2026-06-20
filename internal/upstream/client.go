package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"faka-gateway/internal/config"
)

// Client 上游 API 客户端
type Client struct {
	cfg   config.UpstreamConfig
	http  *http.Client
	cache *TTLCache

	// Mock 模式
	mockEnabled bool
	mockPrefix  string
	mockDelayMs int

	// 指标
	metricMu     sync.Mutex
	totalCalls   int64
	successCalls int64
	failedCalls  int64
	latencySum   int64
	lastError    string
	lastErrorAt  time.Time
	recentErrors []errorRecord
}

type errorRecord struct {
	At      time.Time
	URL     string
	Message string
}

// New 创建客户端
func New(cfg config.UpstreamConfig) *Client {
	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout == 0 {
		timeout = 15 * time.Second
	}
	return &Client{
		cfg:   cfg,
		http:  &http.Client{Timeout: timeout},
		cache: NewTTLCache(512),
	}
}

// NewWithMock 创建带 mock 的客户端（admin 切换）
func NewWithMock(cfg config.UpstreamConfig, mockEnabled bool, mockPrefix string, mockDelayMs int) *Client {
	c := New(cfg)
	c.SetMock(mockEnabled, mockPrefix, mockDelayMs)
	return c
}

// Cache 获取缓存引用（用于管理面板）
func (c *Client) Cache() *TTLCache {
	return c.cache
}

// SetMock 设置 Mock 模式（admin API 调用）
func (c *Client) SetMock(enabled bool, prefix string, delayMs int) {
	c.mockEnabled = enabled
	if prefix != "" {
		c.mockPrefix = prefix
	} else {
		c.mockPrefix = "MOCK-"
	}
	if delayMs > 0 {
		c.mockDelayMs = delayMs
	} else if delayMs == 0 {
		c.mockDelayMs = 800
	}
}

// MockStatus 返回当前 mock 状态
func (c *Client) MockStatus() map[string]any {
	return map[string]any{
		"enabled":  c.mockEnabled,
		"prefix":   c.mockPrefix,
		"delay_ms": c.mockDelayMs,
	}
}

// Metrics 获取指标
func (c *Client) Metrics() Metric {
	c.metricMu.Lock()
	defer c.metricMu.Unlock()
	avg := int64(0)
	if c.totalCalls > 0 {
		avg = c.latencySum / c.totalCalls
	}
	return Metric{
		TotalCalls:   c.totalCalls,
		SuccessCalls: c.successCalls,
		FailedCalls:  c.failedCalls,
		CacheHits:    c.cache.hits,
		CacheMisses:  c.cache.misses,
		AvgLatencyMs: avg,
		LastError:    c.lastError,
		LastErrorAt:  c.lastErrorAt,
		UpstreamURL:  c.cfg.BaseURL,
		StartedAt:    time.Now(),
	}
}

// RecentErrors 最近的错误
func (c *Client) RecentErrors(limit int) []errorRecord {
	c.metricMu.Lock()
	defer c.metricMu.Unlock()
	if limit > len(c.recentErrors) {
		limit = len(c.recentErrors)
	}
	out := make([]errorRecord, limit)
	copy(out, c.recentErrors[len(c.recentErrors)-limit:])
	return out
}

func (c *Client) recordError(urlStr, msg string) {
	c.metricMu.Lock()
	defer c.metricMu.Unlock()
	c.lastError = msg
	c.lastErrorAt = time.Now()
	c.recentErrors = append(c.recentErrors, errorRecord{
		At:      time.Now(),
		URL:     urlStr,
		Message: msg,
	})
	if len(c.recentErrors) > 50 {
		c.recentErrors = c.recentErrors[len(c.recentErrors)-50:]
	}
}

func (c *Client) recordCall(duration time.Duration, success bool) {
	c.metricMu.Lock()
	defer c.metricMu.Unlock()
	c.totalCalls++
	c.latencySum += duration.Milliseconds()
	if success {
		c.successCalls++
	} else {
		c.failedCalls++
	}
}

// doRequest 通用 HTTP 调用
func (c *Client) doRequest(ctx context.Context, method, urlStr string, body url.Values, headers map[string]string) ([]byte, int, error) {
	var reqBody io.Reader
	if body != nil {
		reqBody = strings.NewReader(body.Encode())
	}

	req, err := http.NewRequestWithContext(ctx, method, urlStr, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("new request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	start := time.Now()
	resp, err := c.http.Do(req)
	duration := time.Since(start)
	if err != nil {
		c.recordCall(duration, false)
		c.recordError(urlStr, err.Error())
		return nil, 0, fmt.Errorf("http do: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.recordCall(duration, false)
		c.recordError(urlStr, "read body: "+err.Error())
		return nil, resp.StatusCode, fmt.Errorf("read body: %w", err)
	}

	success := resp.StatusCode >= 200 && resp.StatusCode < 400
	c.recordCall(duration, success)
	if !success {
		msg := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(respBody)[:min(200, len(respBody))])
		c.recordError(urlStr, msg)
	}

	return respBody, resp.StatusCode, nil
}

// doRequestWithRetry 带重试
func (c *Client) doRequestWithRetry(ctx context.Context, method, urlStr string, body url.Values, headers map[string]string) ([]byte, int, error) {
	maxRetries := c.cfg.RetryMax
	if maxRetries < 0 {
		maxRetries = 0
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// 指数退避
			backoff := time.Duration(1<<uint(attempt-1)) * 200 * time.Millisecond
			select {
			case <-ctx.Done():
				return nil, 0, ctx.Err()
			case <-time.After(backoff):
			}
		}

		data, status, err := c.doRequest(ctx, method, urlStr, body, headers)
		if err == nil && status < 500 {
			return data, status, nil
		}
		lastErr = err
		if err == nil {
			lastErr = fmt.Errorf("HTTP %d", status)
		}
		slog.Warn("upstream retry",
			"attempt", attempt+1,
			"err", lastErr,
			"url", urlStr,
		)
	}
	return nil, 0, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// encodeJSON 用于 debug 接口的辅助
func encodeJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// min 辅助
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// formatFloat 数字格式化
func formatFloat(f float64, p int) string {
	return strconv.FormatFloat(f, 'f', p, 64)
}

// readJSON 解码上游 JSON 响应到 dst
func readJSON(data []byte, dst any) error {
	if !bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
		return fmt.Errorf("not json object: %s", string(data)[:min(100, len(data))])
	}
	return json.Unmarshal(data, dst)
}
