package admin

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"faka-gateway/internal/totp"
)

// HandleTOTPSetup 生成新的 TOTP 密钥，返回 otpauth:// URI 供管理员扫描
func (h *Handlers) HandleTOTPSetup(c *gin.Context) {
	secret, err := totp.GenerateSecret()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "生成密钥失败"})
		return
	}
	issuer := "Vaultlane"
	account := h.Cfg.Admin.Username
	if account == "" {
		account = "admin"
	}
	// 临时存入 settings，等用户扫码确认后再正式激活
	_ = h.Store.SetSetting("totp_pending_secret", secret)
	c.JSON(http.StatusOK, gin.H{
		"code": 200, "msg": "success",
		"data": gin.H{
			"secret":  secret,
			"otp_url": totp.OTPAuthURL(secret, issuer, account),
			"hint":    "请用 Google Authenticator / Authy 扫描，然后调用 /admin/api/totp/confirm 验证激活",
		},
	})
}

// HandleTOTPConfirm 验证一次性代码并正式激活 TOTP
func (h *Handlers) HandleTOTPConfirm(c *gin.Context) {
	var req struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	pending := h.Store.GetSettingValue("totp_pending_secret")
	if pending == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "请先调用 /totp/setup 生成密钥"})
		return
	}
	if !totp.Verify(pending, strings.TrimSpace(req.Code), time.Now()) {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "验证码错误，请重试"})
		return
	}
	// 激活：将 pending 写入正式 totp_secret
	_ = h.Store.SetSetting("totp_secret", pending)
	_ = h.Store.SetSetting("totp_pending_secret", "")
	h.audit(c, "totp.activate", "admin", nil)
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "TOTP 双因子认证已激活"})
}

// HandleTOTPDisable 禁用 TOTP（需要一次正确的当前 TOTP 代码）
func (h *Handlers) HandleTOTPDisable(c *gin.Context) {
	var req struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	secret := h.Store.GetSettingValue("totp_secret")
	if secret == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "TOTP 未启用"})
		return
	}
	if !totp.Verify(secret, strings.TrimSpace(req.Code), time.Now()) {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "验证码错误"})
		return
	}
	_ = h.Store.SetSetting("totp_secret", "")
	h.audit(c, "totp.disable", "admin", nil)
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "TOTP 已禁用"})
}

// RequireTOTP 中间件：若已启用 TOTP，则强制校验请求头 X-TOTP-Code
func (h *Handlers) RequireTOTP() gin.HandlerFunc {
	return func(c *gin.Context) {
		secret := h.Store.GetSettingValue("totp_secret")
		if secret == "" {
			c.Next()
			return
		}
		code := strings.TrimSpace(c.GetHeader("X-TOTP-Code"))
		if code == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code": 401, "msg": "此操作需要 TOTP 双因子验证，请在 X-TOTP-Code 请求头携带当前 6 位验证码",
			})
			return
		}
		if !totp.Verify(secret, code, time.Now()) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code": 401, "msg": "TOTP 验证码错误或已过期",
			})
			return
		}
		c.Next()
	}
}
