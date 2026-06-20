package upstream

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// HTTPDoer 通用 HTTP 调用器（带重试 + 指标记录）
type HTTPDoer struct {
	client *http.Client
	ua     string
}

// NewHTTPDoer 构造
func NewHTTPDoer(timeout time.Duration, userAgent string) *HTTPDoer {
	if timeout == 0 {
		timeout = 15 * time.Second
	}
	if userAgent == "" {
		userAgent = "FakaGateway/1.0"
	}
	return &HTTPDoer{
		client: &http.Client{Timeout: timeout},
		ua:     userAgent,
	}
}

// Do 单次请求
func (d *HTTPDoer) Do(ctx context.Context, method, urlStr string, body url.Values, headers map[string]string) ([]byte, int, error) {
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
	req.Header.Set("User-Agent", d.ua)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("http do: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read body: %w", err)
	}
	return data, resp.StatusCode, nil
}

// DoWithRetry 带重试；m 用于记录指标
func (d *HTTPDoer) DoWithRetry(ctx context.Context, method, urlStr string, body url.Values, maxRetries int, m *metricCounter) ([]byte, int, error) {
	if maxRetries < 0 {
		maxRetries = 0
	}
	var lastErr error
	var lastStatus int
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * 200 * time.Millisecond
			select {
			case <-ctx.Done():
				return nil, 0, ctx.Err()
			case <-time.After(backoff):
			}
		}
		start := time.Now()
		data, status, err := d.Do(ctx, method, urlStr, body, nil)
		duration := time.Since(start)
		success := err == nil && status >= 200 && status < 400
		if m != nil {
			m.recordCall(duration, success, urlStr, errMsg(err, status, data))
		}
		if err == nil && status < 500 {
			return data, status, nil
		}
		lastErr = err
		lastStatus = status
		if err == nil {
			lastErr = fmt.Errorf("HTTP %d", status)
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("HTTP %d", lastStatus)
	}
	return nil, lastStatus, fmt.Errorf("max retries exceeded: %w", lastErr)
}

func errMsg(err error, status int, body []byte) string {
	if err != nil {
		return err.Error()
	}
	if status >= 400 {
		return fmt.Sprintf("HTTP %d: %s", status, string(body)[:min2(200, len(body))])
	}
	return ""
}

func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// metricCounter 调用指标
type metricCounter struct {
	mu          sync.Mutex
	total       int64
	success     int64
	failed      int64
	latencySum  int64
	lastError   string
	lastErrorAt time.Time
	startedAt   time.Time
	baseURL     string
}

func newMetricCounter(baseURL string) *metricCounter {
	return &metricCounter{startedAt: time.Now(), baseURL: baseURL}
}

func (m *metricCounter) recordCall(duration time.Duration, success bool, urlStr, errMsgStr string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.total++
	m.latencySum += duration.Milliseconds()
	if success {
		m.success++
	} else {
		m.failed++
		if errMsgStr != "" {
			m.lastError = errMsgStr
			m.lastErrorAt = time.Now()
		}
	}
}

func (m *metricCounter) snapshot() Metric {
	m.mu.Lock()
	defer m.mu.Unlock()
	avg := int64(0)
	if m.total > 0 {
		avg = m.latencySum / m.total
	}
	return Metric{
		TotalCalls:   m.total,
		SuccessCalls: m.success,
		FailedCalls:  m.failed,
		AvgLatencyMs: avg,
		LastError:    m.lastError,
		LastErrorAt:  m.lastErrorAt,
		UpstreamURL:  m.baseURL,
		StartedAt:    m.startedAt,
	}
}
