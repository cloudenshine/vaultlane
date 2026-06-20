package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"faka-gateway/internal/store"
)

// UserHandler 用户端处理器
type UserHandler struct {
	Store  *store.Store
	Logger *slog.Logger
}

// RegisterRoutes 注册用户端路由
func (h *UserHandler) RegisterRoutes(g *gin.RouterGroup, require gin.HandlerFunc) {
	g.POST("/user/register", h.handleRegister)
	g.POST("/user/login", h.handleLogin)
	g.POST("/user/logout", h.handleLogout)
	g.GET("/user/profile", require, h.handleProfile)
	g.GET("/user/orders", require, h.handleMyOrders)
	g.GET("/user/balance/logs", require, h.handleBalanceLogs)
}

// 入口结构
type registerReq struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *UserHandler) handleRegister(c *gin.Context) {
	var req registerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if !validUsername(req.Username) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "用户名需为 3-20 位字母/数字/下划线"})
		return
	}
	if !validEmail(req.Email) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "邮箱格式不正确"})
		return
	}
	if len(req.Password) < 6 || len(req.Password) > 64 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "密码长度需 6-64 位"})
		return
	}
	if existing, _ := h.Store.GetUserByLogin(req.Username); existing != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "用户名已存在"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "密码哈希失败"})
		return
	}
	u := &store.User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: string(hash),
		Balance:      0,
		Status:       1,
	}
	if err := h.Store.CreateUser(u); err != nil {
		// 用户名/邮箱 unique 冲突
		if strings.Contains(err.Error(), "UNIQUE") {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "用户名或邮箱已存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	// 登录态
	if err := h.issueSession(c, u); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 200, "msg": "注册成功",
		"data": gin.H{
			"id":       u.ID,
			"username": u.Username,
			"email":    u.Email,
			"balance":  u.Balance,
		},
	})
}

type loginReq struct {
	Account  string `json:"account"`  // username or email
	Username string `json:"username"` // 兼容旧字段
	Password string `json:"password"`
}

func (h *UserHandler) handleLogin(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	login := strings.TrimSpace(req.Account)
	if login == "" {
		login = strings.TrimSpace(req.Username)
	}
	if login == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "账号密码必填"})
		return
	}
	u, err := h.Store.GetUserByLogin(login)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	if u == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "账号或密码错误"})
		return
	}
	if u.Status != 1 {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "账号已被禁用"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "账号或密码错误"})
		return
	}
	if err := h.issueSession(c, u); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 200, "msg": "登录成功",
		"data": gin.H{
			"id":       u.ID,
			"username": u.Username,
			"email":    u.Email,
			"balance":  u.Balance,
		},
	})
}

func (h *UserHandler) handleLogout(c *gin.Context) {
	if tok, _ := c.Cookie("user_session"); tok != "" {
		_ = h.Store.DeleteSession(tok)
	}
	clearUserCookie(c)
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已退出"})
}

func (h *UserHandler) handleProfile(c *gin.Context) {
	uid, _ := c.Get("user_id")
	u, err := h.Store.GetUserByID(toInt64(uid))
	if err != nil || u == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "会话失效"})
		return
	}
	orders, _ := h.Store.ListOrdersByUser(u.ID, 5)
	c.JSON(http.StatusOK, gin.H{
		"code": 200, "msg": "success",
		"data": gin.H{
			"id":            u.ID,
			"username":      u.Username,
			"email":         u.Email,
			"balance":       u.Balance,
			"status":        u.Status,
			"created_at":    u.CreatedAt,
			"recent_orders": stripOrderSecrets(orders),
		},
	})
}

func (h *UserHandler) handleMyOrders(c *gin.Context) {
	uid, _ := c.Get("user_id")
	orders, err := h.Store.ListOrdersByUser(toInt64(uid), 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 200, "msg": "success",
		"data": stripOrderSecrets(orders),
	})
}

func (h *UserHandler) handleBalanceLogs(c *gin.Context) {
	uid, _ := c.Get("user_id")
	logs, err := h.Store.ListBalanceLogs(toInt64(uid), 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success", "data": logs})
}

// issueSession 颁发 cookie session
func (h *UserHandler) issueSession(c *gin.Context, u *store.User) error {
	tok, err := randomTokenHex(32)
	if err != nil {
		return err
	}
	exp := time.Now().Add(7 * 24 * time.Hour)
	if err := h.Store.CreateSession(&store.UserSession{
		Token: tok, UserID: u.ID, ExpiresAt: exp,
	}); err != nil {
		return err
	}
	setUserCookie(c, tok, 7*24*3600)
	return nil
}

func setUserCookie(c *gin.Context, token string, maxAgeSec int) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("user_session", token, maxAgeSec, "/", "", false, true)
}

func clearUserCookie(c *gin.Context) {
	c.SetCookie("user_session", "", -1, "/", "", false, true)
}

// 校验工具
func validUsername(s string) bool {
	if len(s) < 3 || len(s) > 20 {
		return false
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return true
}

func validEmail(s string) bool {
	at := strings.IndexByte(s, '@')
	if at <= 0 || at == len(s)-1 {
		return false
	}
	if !strings.Contains(s[at+1:], ".") {
		return false
	}
	return true
}

func randomTokenHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// stripOrderSecrets 清掉敏感字段 + 卡密（仅展示用）
func stripOrderSecrets(orders []store.Order) []store.Order {
	out := make([]store.Order, len(orders))
	for i, o := range orders {
		o.Password = ""
		// 卡密只对 status>=2 暴露简短摘要
		out[i] = o
	}
	return out
}

func toInt64(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	}
	return 0
}

// AuthMiddleware 用户态校验（已废弃，仅类型占位）
type AuthMiddleware = gin.HandlerFunc

// InjectUser 中间件：检查 cookie 里的 user_session，注入但不拦截
// 未登录时 c.Get("user_id") 拿不到值；handlers 自行决定如何处理
func (h *UserHandler) InjectUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		tok, _ := c.Cookie("user_session")
		if tok == "" {
			c.Next()
			return
		}
		sess, err := h.Store.GetSession(tok)
		if err != nil || sess == nil || time.Now().After(sess.ExpiresAt) {
			c.Next()
			return
		}
		c.Set("user_id", sess.UserID)
		c.Set("user_session", sess.Token)
		c.Next()
	}
}

// RequireUserStrict 中间件：必须登录，否则 401
// 用于：下单、查订单、查余额、改密等需要明确用户身份的场景
// 注意：不能简单调用 InjectUser（c.Next() 同步执行 handler，那时再 abort 已晚）。
// 必须自己读 cookie + 自己 abort。
func (h *UserHandler) RequireUserStrict() gin.HandlerFunc {
	return func(c *gin.Context) {
		tok, _ := c.Cookie("user_session")
		if tok == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "请先登录"})
			return
		}
		sess, err := h.Store.GetSession(tok)
		if err != nil || sess == nil || time.Now().After(sess.ExpiresAt) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "会话已过期"})
			return
		}
		c.Set("user_id", sess.UserID)
		c.Set("user_session", sess.Token)
		c.Next()
	}
}

// RequireUser 兼容旧名（=InjectUser，可选登录）
// 保留是为了不破坏其它可能的引用；新代码请用 InjectUser 或 RequireUserStrict
func (h *UserHandler) RequireUser() gin.HandlerFunc {
	return h.InjectUser()
}

// 避免 import 抖动
var _ = errors.New
