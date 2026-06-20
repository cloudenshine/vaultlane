package api

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
)

// ctxWithTimeout 辅助：从 gin context 派生带超时的 context
func ctxWithTimeout(c *gin.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(c.Request.Context(), d)
}
