package admin

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const csrfCookieName = "admin_csrf"
const csrfHeaderName = "X-CSRF-Token"
const csrfFormField = "_csrf"

// CSRFMiddleware 对 POST/PUT/DELETE 请求验证 CSRF Token。
// Token 存在 admin_csrf cookie 中，请求方需在 X-CSRF-Token 或 _csrf 表单字段提交相同值。
// GET/HEAD/OPTIONS 请求会自动颁发 / 刷新 cookie。
func CSRFMiddleware(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		method := strings.ToUpper(c.Request.Method)

		// 对写操作进行 CSRF 校验
		if method == "POST" || method == "PUT" || method == "PATCH" || method == "DELETE" {
			cookieTok, err := c.Cookie(csrfCookieName)
			if err != nil || cookieTok == "" {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "msg": "CSRF token missing"})
				return
			}
			headerTok := c.GetHeader(csrfHeaderName)
			if headerTok == "" {
				headerTok = c.PostForm(csrfFormField)
			}
			if !hmac.Equal([]byte(cookieTok), []byte(headerTok)) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "msg": "CSRF token invalid"})
				return
			}
		}

		// GET 等安全方法：颁发 cookie（如尚未颁发）
		if _, err := c.Cookie(csrfCookieName); err != nil {
			tok := generateCSRFToken(secret)
			secure := c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
			c.SetCookie(csrfCookieName, tok, 8*3600, "/admin", "", secure, false)
		}

		c.Next()
	}
}

// generateCSRFToken 生成 HMAC-SHA256 签名的随机 token
func generateCSRFToken(secret string) string {
	raw := make([]byte, 16)
	_, _ = rand.Read(raw)
	nonce := hex.EncodeToString(raw)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(nonce))
	sig := hex.EncodeToString(mac.Sum(nil))
	return nonce + "." + sig
}
