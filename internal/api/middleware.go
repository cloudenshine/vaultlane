package api

import (
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// LoggingMiddleware 结构化访问日志
func LoggingMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		latency := time.Since(start)
		logger.Info("http",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"size", c.Writer.Size(),
			"latency_ms", latency.Milliseconds(),
			"ip", c.ClientIP(),
			"ua", c.Request.UserAgent(),
		)
	}
}

// RateLimiter 简单令牌桶限流（IP 维度）
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    int
}

type bucket struct {
	tokens   int
	lastFill time.Time
}

func NewRateLimiter(perMin int) *RateLimiter {
	return &RateLimiter{
		buckets: make(map[string]*bucket),
		rate:    perMin,
	}
}

func (rl *RateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	b, ok := rl.buckets[key]
	if !ok {
		b = &bucket{tokens: rl.rate, lastFill: now}
		rl.buckets[key] = b
	}
	// 补充 token（按秒）
	elapsed := now.Sub(b.lastFill).Seconds()
	addBack := int(elapsed * float64(rl.rate) / 60.0)
	if addBack > 0 {
		b.tokens += addBack
		if b.tokens > rl.rate {
			b.tokens = rl.rate
		}
		b.lastFill = now
	}
	if b.tokens <= 0 {
		return false
	}
	b.tokens--
	return true
}

func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !rl.allow(c.ClientIP()) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"code": 429,
				"msg":  "请求过于频繁，请稍后再试",
			})
			return
		}
		c.Next()
	}
}

// BusinessKey 业务维度限流 key（VULN-013）
// key 形如 "route:uid"，独立于 IP 限流，单用户跨 IP 不能绕过。
type BusinessKey struct {
	Route string
	UID   int64
}

func (k BusinessKey) String() string {
	return k.Route + ":" + strconv.FormatInt(k.UID, 10)
}

// BusinessLimit 按业务 key 做令牌桶限流（VULN-013）
func (rl *RateLimiter) BusinessLimit(key BusinessKey) bool {
	return rl.allow(key.String())
}
