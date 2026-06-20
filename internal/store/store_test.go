package store

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	s, err := NewSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestMigrateV3_CreatesAllTables(t *testing.T) {
	s := newTestStore(t)
	tables := []string{"users", "user_sessions", "balance_logs", "payments", "settings"}
	for _, tname := range tables {
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, tname).Scan(&n); err != nil {
			t.Fatalf("query %s: %v", tname, err)
		}
		if n != 1 {
			t.Errorf("table %s not created", tname)
		}
	}
}

func TestUserCRUD(t *testing.T) {
	s := newTestStore(t)
	u := &User{Username: "alice", Email: "a@x.com", PasswordHash: "hash"}
	if err := s.CreateUser(u); err != nil {
		t.Fatal(err)
	}
	if u.ID <= 0 {
		t.Fatal("id not set")
	}
	got, err := s.GetUserByLogin("alice")
	if err != nil || got == nil {
		t.Fatal(err)
	}
	if got.Email != "a@x.com" {
		t.Errorf("email = %s", got.Email)
	}
	got2, err := s.GetUserByLogin("a@x.com")
	if err != nil || got2 == nil || got2.ID != got.ID {
		t.Errorf("login by email failed: %v %v", err, got2)
	}
}

func TestUserUniqueConstraint(t *testing.T) {
	s := newTestStore(t)
	a := &User{Username: "bob", Email: "b@x.com", PasswordHash: "h1"}
	if err := s.CreateUser(a); err != nil {
		t.Fatal(err)
	}
	b := &User{Username: "bob", Email: "b2@x.com", PasswordHash: "h2"}
	if err := s.CreateUser(b); err == nil {
		t.Fatal("expected unique constraint on username")
	}
}

func TestAdjustBalance(t *testing.T) {
	s := newTestStore(t)
	u := &User{Username: "u1", Email: "u1@x.com", PasswordHash: "h"}
	_ = s.CreateUser(u)
	// 充值
	if _, err := s.AdjustBalance(u.ID, 100, "recharge", "test", 0); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetUserByID(u.ID)
	if got.Balance != 100 {
		t.Errorf("balance = %v", got.Balance)
	}
	// 消费
	if _, err := s.AdjustBalance(u.ID, -30, "consume", "buy", 0); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetUserByID(u.ID)
	if got.Balance != 70 {
		t.Errorf("balance = %v", got.Balance)
	}
	logs, _ := s.ListBalanceLogs(u.ID, 10)
	if len(logs) != 2 {
		t.Errorf("logs = %d", len(logs))
	}
}

func TestPaymentLifecycle(t *testing.T) {
	s := newTestStore(t)
	p := &Payment{TradeNo: "FK001", Amount: 9.9, Method: "epay"}
	if err := s.CreatePayment(p); err != nil {
		t.Fatal(err)
	}
	if p.ID <= 0 {
		t.Fatal("id not set")
	}
	got, _ := s.GetPaymentByID(p.ID)
	if got == nil {
		t.Fatal("not found")
	}
	// mark paid
	ok, err := s.MarkPaymentPaid(p.ID, "OUT001", "raw")
	if err != nil || !ok {
		t.Fatalf("mark: %v %v", ok, err)
	}
	// 二次标记应返回 false（幂等）
	ok2, _ := s.MarkPaymentPaid(p.ID, "OUT001", "raw2")
	if ok2 {
		t.Error("expected not ok on second mark")
	}
}

func TestSessionCRUD(t *testing.T) {
	s := newTestStore(t)
	u := &User{Username: "u2", Email: "u2@x.com", PasswordHash: "h"}
	_ = s.CreateUser(u)
	sess := &UserSession{Token: "tok-1", UserID: u.ID, ExpiresAt: time.Now().Add(time.Hour)}
	if err := s.CreateSession(sess); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetSession("tok-1")
	if got == nil || got.UserID != u.ID {
		t.Fatal("session not found")
	}
	_ = s.DeleteSession("tok-1")
	got2, _ := s.GetSession("tok-1")
	if got2 != nil {
		t.Error("session not deleted")
	}
}

func TestSettings(t *testing.T) {
	s := newTestStore(t)
	_ = s.UpsertSetting("site_name", "码仓", "string")
	got, _ := s.GetSetting("site_name")
	if got == nil || got.Value != "码仓" {
		t.Errorf("setting = %v", got)
	}
	_ = s.UpsertSetting("site_name", "码仓2", "string")
	got, _ = s.GetSetting("site_name")
	if got.Value != "码仓2" {
		t.Errorf("after update = %s", got.Value)
	}
	all, _ := s.ListSettings()
	if len(all) != 1 {
		t.Errorf("settings = %d", len(all))
	}
}
