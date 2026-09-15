package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Order 本地订单（与上游 order 字段对齐，附加本地 metadata）
type Order struct {
	ID            int64     `json:"id"`
	TradeNo       string    `json:"trade_no"`
	RequestNo     string    `json:"request_no"`
	CommodityID   int       `json:"commodity_id"`
	CommodityName string    `json:"commodity_name"`
	SharedCode    string    `json:"shared_code"`
	Contact       string    `json:"contact"`
	Num           int       `json:"num"`
	Race          string    `json:"race"`
	Password      string    `json:"-"`
	Amount        float64   `json:"amount"`
	Status        int       `json:"status"`     // 0=待支付 1=已支付 2=已发货 3=已完成 4=退款中 5=已退款
	PayStatus     int       `json:"pay_status"` // 上游支付状态
	Contents      string    `json:"contents"`   // 卡密 / 发货信息
	UpstreamCode  int       `json:"upstream_code"`
	UpstreamMsg   string    `json:"upstream_msg"`
	Source        string    `json:"source"`      // self / upstream:upstreama / upstream:downstreamb
	CategoryID    int64     `json:"category_id"` // 本地分类 ID
	UserID        int64     `json:"user_id"`
	PaymentID     int64     `json:"payment_id"`
	PayMethod     string    `json:"pay_method"`
	CouponID      int64     `json:"coupon_id"`
	Discount      float64   `json:"discount"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Store 订单存储
type Store struct {
	db     *sql.DB
	Cipher interface {
		Encrypt(string) (string, error)
		Decrypt(string) (string, error)
	}
}

// NewSQLite 创建 SQLite 存储
func NewSQLite(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0o755)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite 单写
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	// 启用 WAL 模式与繁忙重试等待，大幅提升并发读写吞吐，防锁库
	_, _ = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; PRAGMA synchronous=NORMAL;`)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS orders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			trade_no TEXT UNIQUE NOT NULL,
			request_no TEXT NOT NULL,
			commodity_id INTEGER NOT NULL,
			commodity_name TEXT NOT NULL DEFAULT '',
			shared_code TEXT NOT NULL DEFAULT '',
			contact TEXT NOT NULL DEFAULT '',
			num INTEGER NOT NULL DEFAULT 1,
			race TEXT NOT NULL DEFAULT '',
			password TEXT NOT NULL DEFAULT '',
			amount REAL NOT NULL DEFAULT 0,
			status INTEGER NOT NULL DEFAULT 0,
			pay_status INTEGER NOT NULL DEFAULT 0,
			contents TEXT NOT NULL DEFAULT '',
			upstream_code INTEGER NOT NULL DEFAULT 0,
			upstream_msg TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_orders_trade_no ON orders(trade_no);
		CREATE INDEX IF NOT EXISTS idx_orders_created_at ON orders(created_at);
	`); err != nil {
		return err
	}
	// v2.0 增量迁移
	if err := s.migrateV2(); err != nil {
		return err
	}
	// v3.0 增量迁移：用户 / 支付 / 配置
	if err := s.migrateV3(); err != nil {
		return err
	}
	// v5.0 增量迁移：优惠券 / 分销返佣 / 订单折扣
	return s.migrateV5()
}

func (s *Store) migrateV5() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS coupons (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			code TEXT UNIQUE NOT NULL,
			name TEXT NOT NULL DEFAULT '',
			type INTEGER NOT NULL DEFAULT 0,
			discount REAL NOT NULL DEFAULT 0,
			min_amount REAL NOT NULL DEFAULT 0,
			total_limit INTEGER NOT NULL DEFAULT 0,
			used_count INTEGER NOT NULL DEFAULT 0,
			status INTEGER NOT NULL DEFAULT 1,
			expire_at DATETIME,
			created_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_coupons_code ON coupons(code)`,
		`CREATE TABLE IF NOT EXISTS coupon_usages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			coupon_id INTEGER NOT NULL,
			user_id INTEGER NOT NULL,
			order_id INTEGER NOT NULL,
			discount REAL NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_coupon_usages_uid ON coupon_usages(user_id, coupon_id)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	// 扩展列（试执行，忽略已存在错误）
	alterStmts := []string{
		`ALTER TABLE users ADD COLUMN invite_code TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN referrer_id INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE users ADD COLUMN commission_balance REAL NOT NULL DEFAULT 0`,
		`ALTER TABLE orders ADD COLUMN coupon_id INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE orders ADD COLUMN discount REAL NOT NULL DEFAULT 0`,
	}
	for _, q := range alterStmts {
		_, _ = s.db.Exec(q)
	}
	return nil
}

func (s *Store) migrateV3() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT UNIQUE NOT NULL,
			email TEXT NOT NULL DEFAULT '',
			password_hash TEXT NOT NULL,
			balance REAL NOT NULL DEFAULT 0,
			status INTEGER NOT NULL DEFAULT 1,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_users_email ON users(email)`,
		`CREATE TABLE IF NOT EXISTS user_sessions (
			token TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user ON user_sessions(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_expire ON user_sessions(expires_at)`,
		`CREATE TABLE IF NOT EXISTS balance_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			type TEXT NOT NULL,
			amount REAL NOT NULL,
			balance REAL NOT NULL,
			order_id INTEGER NOT NULL DEFAULT 0,
			note TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_balance_user ON balance_logs(user_id, created_at)`,
		`CREATE TABLE IF NOT EXISTS payments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			order_id INTEGER NOT NULL DEFAULT 0,
			user_id INTEGER NOT NULL DEFAULT 0,
			trade_no TEXT UNIQUE NOT NULL,
			amount REAL NOT NULL DEFAULT 0,
			method TEXT NOT NULL DEFAULT '',
			status INTEGER NOT NULL DEFAULT 0,
			out_trade_no TEXT NOT NULL DEFAULT '',
			notify_data TEXT NOT NULL DEFAULT '',
			pay_url TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			paid_at DATETIME,
			expire_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_pay_order ON payments(order_id)`,
		`CREATE INDEX IF NOT EXISTS idx_pay_status ON payments(status, method)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT '',
			type TEXT NOT NULL DEFAULT 'string',
			updated_at DATETIME NOT NULL
		)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) migrateV2() error {
	// 1. 新表（IF NOT EXISTS 安全）
	createStmts := []string{
		`CREATE TABLE IF NOT EXISTS categories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			icon TEXT NOT NULL DEFAULT '',
			sort INTEGER NOT NULL DEFAULT 0,
			parent_id INTEGER NOT NULL DEFAULT 0,
			source TEXT NOT NULL DEFAULT 'self',
			created_at DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS commodities (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			category_id INTEGER NOT NULL DEFAULT 0,
			name TEXT NOT NULL,
			cover TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			price REAL NOT NULL DEFAULT 0,
			cost_price REAL NOT NULL DEFAULT 0,
			delivery_way INTEGER NOT NULL DEFAULT 1,
			status INTEGER NOT NULL DEFAULT 1,
			sort INTEGER NOT NULL DEFAULT 0,
			shared_code TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT 'self',
			outer_id TEXT NOT NULL DEFAULT '',
			stock INTEGER NOT NULL DEFAULT 0,
			sold INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_comm_source ON commodities(source, status)`,
		`CREATE INDEX IF NOT EXISTS idx_comm_category ON commodities(category_id)`,
		`CREATE TABLE IF NOT EXISTS card_secrets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			commodity_id INTEGER NOT NULL,
			content TEXT NOT NULL,
			status INTEGER NOT NULL DEFAULT 0,
			order_id INTEGER NOT NULL DEFAULT 0,
			sold_at DATETIME,
			created_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_secrets_comm_status ON card_secrets(commodity_id, status)`,
		`CREATE TABLE IF NOT EXISTS upstream_runtime (
			name TEXT PRIMARY KEY,
			enabled INTEGER NOT NULL DEFAULT 1,
			last_sync_at DATETIME,
			last_status TEXT NOT NULL DEFAULT '',
			last_error TEXT NOT NULL DEFAULT '',
			synced_count INTEGER NOT NULL DEFAULT 0,
			updated_at DATETIME NOT NULL
		)`,
		// VULN-012：审计日志（admin 写操作的可追溯记录）
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			actor TEXT NOT NULL DEFAULT '',         -- 操作者（admin 用户名 / "system"）
			action TEXT NOT NULL DEFAULT '',         -- 事件类型：mock.toggle / commodity.update / ...
			target TEXT NOT NULL DEFAULT '',         -- 操作对象 ID / 名称
			detail TEXT NOT NULL DEFAULT '',         -- JSON 字符串，存变更前后值（不进卡密）
			ip TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_logs(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_action ON audit_logs(action)`,
	}
	for _, q := range createStmts {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	// 2. orders 加列（SQLite 缺 try-add-column，逐个试，失败即跳过）
	alterStmts := []string{
		`ALTER TABLE orders ADD COLUMN source TEXT NOT NULL DEFAULT 'upstream:upstreama'`,
		`ALTER TABLE orders ADD COLUMN category_id INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE orders ADD COLUMN user_id INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE orders ADD COLUMN payment_id INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE orders ADD COLUMN pay_method TEXT NOT NULL DEFAULT ''`,
		// 商品池扩展字段（v4）
		`ALTER TABLE commodities ADD COLUMN sale_price REAL NOT NULL DEFAULT 0`,
		`ALTER TABLE commodities ADD COLUMN stock_warning INTEGER NOT NULL DEFAULT 5`,
		`ALTER TABLE commodities ADD COLUMN last_seen_at DATETIME`,
		`ALTER TABLE commodities ADD COLUMN upstream_extra TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE commodities ADD COLUMN config TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE commodities ADD COLUMN tags TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE commodities ADD COLUMN minimum INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE commodities ADD COLUMN maximum INTEGER NOT NULL DEFAULT 1`,
	}
	for _, q := range alterStmts {
		_, _ = s.db.Exec(q) // 忽略 "duplicate column" 错误
	}
	// 把 price 同步到 sale_price（首次升级用）
	_, _ = s.db.Exec(`UPDATE commodities SET sale_price=price WHERE sale_price=0 AND price>0`)
	return nil
}

// Close 关闭
func (s *Store) Close() error {
	return s.db.Close()
}

// CreateOrder 创建订单
func (s *Store) CreateOrder(o *Order) error {
	now := time.Now()
	o.CreatedAt = now
	o.UpdatedAt = now
	if o.Source == "" {
		o.Source = "upstream:upstreama"
	}
	res, err := s.db.Exec(`
		INSERT INTO orders (trade_no, request_no, commodity_id, commodity_name, shared_code,
			contact, num, race, password, amount, status, pay_status, contents,
			upstream_code, upstream_msg, source, category_id, user_id, payment_id, pay_method, coupon_id, discount, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	`,
		o.TradeNo, o.RequestNo, o.CommodityID, o.CommodityName, o.SharedCode,
		o.Contact, o.Num, o.Race, o.Password, o.Amount, o.Status, o.PayStatus, o.Contents,
		o.UpstreamCode, o.UpstreamMsg, o.Source, o.CategoryID, o.UserID, o.PaymentID, o.PayMethod, o.CouponID, o.Discount, o.CreatedAt, o.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert order: %w", err)
	}
	id, _ := res.LastInsertId()
	o.ID = id
	return nil
}

// UpdateOrder 更新
func (s *Store) UpdateOrder(o *Order) error {
	o.UpdatedAt = time.Now()
	_, err := s.db.Exec(`
		UPDATE orders SET
			status=?, pay_status=?, contents=?,
			upstream_code=?, upstream_msg=?, payment_id=?, pay_method=?, updated_at=?
		WHERE trade_no=?
	`, o.Status, o.PayStatus, o.Contents,
		o.UpstreamCode, o.UpstreamMsg, o.PaymentID, o.PayMethod, o.UpdatedAt,
		o.TradeNo,
	)
	return err
}

// GetOrderByID 按 ID 查询
func (s *Store) GetOrderByID(id int64) (*Order, error) {
	row := s.db.QueryRow(`
		SELECT id, trade_no, request_no, commodity_id, commodity_name, shared_code,
			contact, num, race, password, amount, status, pay_status, contents,
			upstream_code, upstream_msg, source, category_id, user_id, payment_id, pay_method, coupon_id, discount, created_at, updated_at
		FROM orders WHERE id = ?
	`, id)
	var o Order
	err := row.Scan(&o.ID, &o.TradeNo, &o.RequestNo, &o.CommodityID, &o.CommodityName, &o.SharedCode,
		&o.Contact, &o.Num, &o.Race, &o.Password, &o.Amount, &o.Status, &o.PayStatus, &o.Contents,
		&o.UpstreamCode, &o.UpstreamMsg, &o.Source, &o.CategoryID, &o.UserID, &o.PaymentID, &o.PayMethod,
		&o.CouponID, &o.Discount, &o.CreatedAt, &o.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// GetOrderByTradeNo 查询
func (s *Store) GetOrderByTradeNo(tradeNo string) (*Order, error) {
	row := s.db.QueryRow(`
		SELECT id, trade_no, request_no, commodity_id, commodity_name, shared_code,
			contact, num, race, password, amount, status, pay_status, contents,
			upstream_code, upstream_msg, source, category_id, user_id, payment_id, pay_method, coupon_id, discount, created_at, updated_at
		FROM orders WHERE trade_no = ?
	`, tradeNo)
	var o Order
	err := row.Scan(&o.ID, &o.TradeNo, &o.RequestNo, &o.CommodityID, &o.CommodityName, &o.SharedCode,
		&o.Contact, &o.Num, &o.Race, &o.Password, &o.Amount, &o.Status, &o.PayStatus, &o.Contents,
		&o.UpstreamCode, &o.UpstreamMsg, &o.Source, &o.CategoryID, &o.UserID, &o.PaymentID, &o.PayMethod,
		&o.CouponID, &o.Discount, &o.CreatedAt, &o.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// ListOrders 最近订单
func (s *Store) ListOrders(limit int) ([]Order, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(`
		SELECT id, trade_no, request_no, commodity_id, commodity_name, shared_code,
			contact, num, race, password, amount, status, pay_status, contents,
			upstream_code, upstream_msg, source, category_id, user_id, payment_id, pay_method, coupon_id, discount, created_at, updated_at
		FROM orders ORDER BY id DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Order
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.ID, &o.TradeNo, &o.RequestNo, &o.CommodityID, &o.CommodityName, &o.SharedCode,
			&o.Contact, &o.Num, &o.Race, &o.Password, &o.Amount, &o.Status, &o.PayStatus, &o.Contents,
			&o.UpstreamCode, &o.UpstreamMsg, &o.Source, &o.CategoryID, &o.UserID, &o.PaymentID, &o.PayMethod,
			&o.CouponID, &o.Discount, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// ListOrdersByUser 用户的订单
func (s *Store) ListOrdersByUser(userID int64, limit int) ([]Order, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(`
		SELECT id, trade_no, request_no, commodity_id, commodity_name, shared_code,
			contact, num, race, password, amount, status, pay_status, contents,
			upstream_code, upstream_msg, source, category_id, user_id, payment_id, pay_method, coupon_id, discount, created_at, updated_at
		FROM orders WHERE user_id=? ORDER BY id DESC LIMIT ?
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Order
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.ID, &o.TradeNo, &o.RequestNo, &o.CommodityID, &o.CommodityName, &o.SharedCode,
			&o.Contact, &o.Num, &o.Race, &o.Password, &o.Amount, &o.Status, &o.PayStatus, &o.Contents,
			&o.UpstreamCode, &o.UpstreamMsg, &o.Source, &o.CategoryID, &o.UserID, &o.PaymentID, &o.PayMethod,
			&o.CouponID, &o.Discount, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// CountOrders 统计
func (s *Store) CountOrders() (int, error) {
	var n int
	err := s.db.QueryRow("SELECT COUNT(*) FROM orders").Scan(&n)
	return n, err
}
