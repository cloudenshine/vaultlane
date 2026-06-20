package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"faka-gateway/internal/config"
	"faka-gateway/internal/payment"
	"faka-gateway/internal/store"
	"faka-gateway/internal/upstream"
	"faka-gateway/internal/web"
)

func newTestServer(t *testing.T) (*gin.Engine, *store.Store, upstream.Adapter) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{
		Server:    config.ServerConfig{Listen: ":0", Mode: "test", ReadTimeout: 5, WriteTimeout: 5},
		RateLimit: config.RateLimitConfig{PerIPPerMin: 10000},
	}
	st, err := store.NewSQLite(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	// mock 上游（v4：用 Adapter 接口）
	up := upstream.NewMockAdapter("mock", "T-", 12, cfg.Upstream)
	mgr := upstream.NewManager(cfg)
	_ = mgr // 暂未用，保留接口

	r := gin.New()
	r.Use(gin.Recovery())
	Register(r, cfg, up, mgr, st, slogNew())

	// web
	web.Register(r)

	return r, st, up
}

func TestIntegration_RegisterLoginOrder(t *testing.T) {
	r, st, _ := newTestServer(t)
	// 准备：插入一个自营商品 + 3 张卡密
	com := &store.Commodity{
		CategoryID: 1, Name: "测试商品", Price: 10, CostPrice: 5,
		DeliveryWay: 1, Status: 1, Source: "self", Stock: 0,
	}
	_ = st.CreateCommodity(com)
	_, _ = st.ImportSecrets(com.ID, []string{"card-001", "card-002", "card-003"})
	com2, _ := st.GetCommodityByID(com.ID)
	if com2.Stock != 3 {
		t.Fatalf("stock = %d, want 3", com2.Stock)
	}

	// 1) 注册
	w := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]any{"username": "tester", "email": "t@x.com", "password": "abc12345"})
	req := httptest.NewRequest("POST", "/api/user/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("register: %d %s", w.Code, w.Body.String())
	}
	// 拿到 cookie
	var sessionCookie string
	for _, c := range w.Result().Cookies() {
		if c.Name == "user_session" {
			sessionCookie = c.Value
		}
	}
	if sessionCookie == "" {
		t.Fatal("no session cookie")
	}
	t.Logf("session cookie: %s", sessionCookie)

	// 给用户充 100 元
	u, _ := st.GetUserByLogin("tester")
	_, _ = st.AdjustBalance(u.ID, 100, "recharge", "init", 0)

	// 2) 余额下单
	w = httptest.NewRecorder()
	body, _ = json.Marshal(map[string]any{
		"commodity_id": com.ID, "contact": "tester@x.com", "num": 2,
		"pay_method": "balance",
	})
	req = httptest.NewRequest("POST", "/api/orders", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "user_session", Value: sessionCookie})
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("order: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Order struct {
				ID        int64   `json:"id"`
				TradeNo   string  `json:"trade_no"`
				Status    int     `json:"status"`
				Amount    float64 `json:"amount"`
				PayMethod string  `json:"pay_method"`
				Contents  string  `json:"contents"`
			} `json:"order"`
			Secrets []string `json:"secrets"`
			Balance float64  `json:"balance"`
			NeedPay bool     `json:"need_pay"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != 200 {
		t.Fatalf("order resp code = %d", resp.Code)
	}
	if resp.Data.Order.Status != 2 {
		t.Errorf("order status = %d, want 2 (delivered)", resp.Data.Order.Status)
	}
	if len(resp.Data.Secrets) != 2 {
		t.Errorf("secrets = %d, want 2", len(resp.Data.Secrets))
	}
	if resp.Data.Balance != 80 {
		t.Errorf("balance = %v, want 80", resp.Data.Balance)
	}
	if resp.Data.NeedPay {
		t.Error("need_pay should be false (balance paid)")
	}

	// 3) 库存已扣
	com3, _ := st.GetCommodityByID(com.ID)
	if com3.Stock != 1 {
		t.Errorf("stock after = %d, want 1", com3.Stock)
	}
	if com3.Sold != 2 {
		t.Errorf("sold after = %d, want 2", com3.Sold)
	}

	// 4) 登录
	w = httptest.NewRecorder()
	body, _ = json.Marshal(map[string]any{"account": "tester", "password": "abc12345"})
	req = httptest.NewRequest("POST", "/api/user/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}

	// 5) 个人中心
	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/user/profile", nil)
	for _, c := range w.Result().Cookies() {
		if c.Name == "user_session" {
			req.AddCookie(c)
		}
	}
	// 用登录返回的 cookie
	var loginCookie string
	for _, c := range w.Result().Cookies() {
		_ = c
	}
	_ = loginCookie
	// 简单：直接用之前 session
	req2 := httptest.NewRequest("GET", "/api/user/profile", nil)
	req2.AddCookie(&http.Cookie{Name: "user_session", Value: sessionCookie})
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("profile: %d %s", w2.Code, w2.Body.String())
	}
}

func TestIntegration_OrderNoPay_Flow(t *testing.T) {
	r, st, _ := newTestServer(t)
	com := &store.Commodity{
		CategoryID: 1, Name: "测试商品2", Price: 20, CostPrice: 10,
		DeliveryWay: 1, Status: 1, Source: "self",
	}
	_ = st.CreateCommodity(com)
	_, _ = st.ImportSecrets(com.ID, []string{"c1", "c2"})

	// 1) 未登录直接下单 → 必须 401（VULN-001 修复验证）
	w1 := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]any{
		"commodity_id": com.ID, "contact": "a@x.com", "num": 1,
	})
	req1 := httptest.NewRequest("POST", "/api/orders", bytes.NewReader(body))
	req1.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w1, req1)
	if w1.Code != 401 {
		t.Fatalf("未登录应 401，实际: %d %s", w1.Code, w1.Body.String())
	}

	// 2) 登录普通用户免支付 → 必须 403（VULN-001 修复验证）
	u := &store.User{Username: "alice_test", Email: "a@x.com", PasswordHash: "x", Balance: 100}
	_ = st.CreateUser(u)
	sess := &store.UserSession{Token: "test-sess-alice", UserID: u.ID, ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now()}
	_ = st.CreateSession(sess)

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/api/orders", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.AddCookie(&http.Cookie{Name: "user_session", Value: sess.Token})
	r.ServeHTTP(w2, req2)
	if w2.Code != 403 {
		t.Fatalf("普通用户免支付应 403，实际: %d %s", w2.Code, w2.Body.String())
	}
}

// TestIntegration_OrderVULN020 VULN-020 回归测试
// admin 免支付下单（UserID=0）后，普通用户不能查
func TestIntegration_OrderVULN020(t *testing.T) {
	r, st, _ := newTestServer(t)
	com := &store.Commodity{
		CategoryID: 1, Name: "VULN020测试商品", Price: 50, CostPrice: 30,
		DeliveryWay: 1, Status: 1, Source: "self",
	}
	_ = st.CreateCommodity(com)
	_, _ = st.ImportSecrets(com.ID, []string{"VULN020-CARD-001", "VULN020-CARD-002"})

	// 1) 模拟 admin 免支付下单：直接写 order（UserID=0，Status=2 已发卡）
	adminOrder := &store.Order{
		TradeNo:     "ADMIN-TEST-001",
		RequestNo:   "ADMIN-TEST-001",
		CommodityID: int(com.ID),
		Num:         1,
		Amount:      50,
		Status:      2, // 已发卡
		Source:      "self",
		Contents:    "VULN020-CARD-001",
	}
	if err := st.CreateOrder(adminOrder); err != nil {
		t.Fatalf("admin order 创建失败: %v", err)
	}

	// 2) 普通登录用户尝试查这个 admin 下的订单 → 必须 403
	u := &store.User{Username: "bob", Email: "b@x.com", PasswordHash: "x", Balance: 0}
	_ = st.CreateUser(u)
	sess := &store.UserSession{Token: "test-sess-bob", UserID: u.ID, ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now()}
	_ = st.CreateSession(sess)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/orders/ADMIN-TEST-001", nil)
	req.AddCookie(&http.Cookie{Name: "user_session", Value: sess.Token})
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("VULN-020: 普通用户查 user_id=0 订单应 403，实际: %d %s", w.Code, w.Body.String())
	}

	// 3) 未登录必须 401
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("GET", "/api/orders/ADMIN-TEST-001", nil)
	r.ServeHTTP(w2, req2)
	if w2.Code != 401 {
		t.Fatalf("未登录查订单应 401，实际: %d %s", w2.Code, w2.Body.String())
	}
}

func TestIntegration_PaymentMethods(t *testing.T) {
	r, _, _ := newTestServer(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/payment/methods", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("methods: %d", w.Code)
	}
	var resp struct {
		Code int      `json:"code"`
		Data []string `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != 200 {
		t.Errorf("code = %d", resp.Code)
	}
	has := false
	for _, m := range resp.Data {
		if m == "balance" {
			has = true
		}
	}
	if !has {
		t.Errorf("expected balance in methods: %v", resp.Data)
	}
}

// 避免导入抖动
var (
	_ = payment.ErrUnsupported
	_ = bcrypt.GenerateFromPassword
)
