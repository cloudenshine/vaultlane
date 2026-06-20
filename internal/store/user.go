package store

import (
	"database/sql"
	"time"
)

// User 用户
type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Balance      float64   `json:"balance"`
	Status       int       `json:"status"` // 0=禁用 1=正常
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// UserSession 登录会话
type UserSession struct {
	Token     string    `json:"token"`
	UserID    int64     `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// BalanceLog 余额流水
type BalanceLog struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Type      string    `json:"type"` // recharge / consume / refund
	Amount    float64   `json:"amount"`
	Balance   float64   `json:"balance"` // 流水后余额
	OrderID   int64     `json:"order_id"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
}

// Payment 支付记录
type Payment struct {
	ID         int64      `json:"id"`
	OrderID    int64      `json:"order_id"`
	UserID     int64      `json:"user_id"`
	TradeNo    string     `json:"trade_no"`
	Amount     float64    `json:"amount"`
	Method     string     `json:"method"` // alipay/wxpay/usdt/balance/epay
	Status     int        `json:"status"` // 0=待支付 1=已支付 2=失败 3=已退款
	OutTradeNo string     `json:"out_trade_no"`
	NotifyData string     `json:"notify_data"`
	PayURL     string     `json:"pay_url"`
	CreatedAt  time.Time  `json:"created_at"`
	PaidAt     *time.Time `json:"paid_at"`
	ExpireAt   time.Time  `json:"expire_at"`
}

// Setting 运行时配置（覆盖 yaml）
type Setting struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	Type      string    `json:"type"` // string / json / int / bool
	UpdatedAt time.Time `json:"updated_at"`
}

// ----- Users -----

func (s *Store) CreateUser(u *User) error {
	now := time.Now()
	u.CreatedAt = now
	u.UpdatedAt = now
	if u.Status == 0 {
		u.Status = 1
	}
	res, err := s.db.Exec(`INSERT INTO users (username,email,password_hash,balance,status,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?)`,
		u.Username, u.Email, u.PasswordHash, u.Balance, u.Status, u.CreatedAt, u.UpdatedAt)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	u.ID = id
	return nil
}

func (s *Store) GetUserByID(id int64) (*User, error) {
	row := s.db.QueryRow(`SELECT id,username,email,password_hash,balance,status,created_at,updated_at
		FROM users WHERE id=?`, id)
	var u User
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Balance,
		&u.Status, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

func (s *Store) GetUserByLogin(login string) (*User, error) {
	// 同时匹配 username / email
	row := s.db.QueryRow(`SELECT id,username,email,password_hash,balance,status,created_at,updated_at
		FROM users WHERE username=? OR email=? LIMIT 1`, login, login)
	var u User
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Balance,
		&u.Status, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

// AdjustBalance 原子调整余额（amount 正加负减）并写流水
func (s *Store) AdjustBalance(userID int64, amount float64, typ, note string, orderID int64) (float64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var newBal float64
	if err := tx.QueryRow(`UPDATE users SET balance = balance + ?, updated_at=?
		WHERE id=? RETURNING balance`, amount, time.Now(), userID).Scan(&newBal); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`INSERT INTO balance_logs (user_id,type,amount,balance,order_id,note,created_at)
		VALUES (?,?,?,?,?,?,?)`, userID, typ, amount, newBal, orderID, note, time.Now()); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return newBal, nil
}

func (s *Store) ListBalanceLogs(userID int64, limit int) ([]BalanceLog, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id,user_id,type,amount,balance,order_id,note,created_at
		FROM balance_logs WHERE user_id=? ORDER BY id DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BalanceLog
	for rows.Next() {
		var l BalanceLog
		if err := rows.Scan(&l.ID, &l.UserID, &l.Type, &l.Amount, &l.Balance, &l.OrderID, &l.Note, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ----- Sessions -----

func (s *Store) CreateSession(sess *UserSession) error {
	sess.CreatedAt = time.Now()
	_, err := s.db.Exec(`INSERT INTO user_sessions (token,user_id,expires_at,created_at)
		VALUES (?,?,?,?)`, sess.Token, sess.UserID, sess.ExpiresAt, sess.CreatedAt)
	return err
}

func (s *Store) GetSession(token string) (*UserSession, error) {
	row := s.db.QueryRow(`SELECT token,user_id,expires_at,created_at FROM user_sessions WHERE token=?`, token)
	var sess UserSession
	if err := row.Scan(&sess.Token, &sess.UserID, &sess.ExpiresAt, &sess.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &sess, nil
}

func (s *Store) DeleteSession(token string) error {
	_, err := s.db.Exec(`DELETE FROM user_sessions WHERE token=?`, token)
	return err
}

func (s *Store) PurgeExpiredSessions() error {
	_, err := s.db.Exec(`DELETE FROM user_sessions WHERE expires_at < ?`, time.Now())
	return err
}

// ----- Payments -----

func (s *Store) CreatePayment(p *Payment) error {
	now := time.Now()
	p.CreatedAt = now
	if p.ExpireAt.IsZero() {
		p.ExpireAt = now.Add(15 * time.Minute)
	}
	res, err := s.db.Exec(`INSERT INTO payments (order_id,user_id,trade_no,amount,method,status,out_trade_no,pay_url,expire_at,created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		p.OrderID, p.UserID, p.TradeNo, p.Amount, p.Method, p.Status, p.OutTradeNo, p.PayURL, p.ExpireAt, p.CreatedAt)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	p.ID = id
	return nil
}

func (s *Store) GetPaymentByID(id int64) (*Payment, error) {
	row := s.db.QueryRow(`SELECT id,order_id,user_id,trade_no,amount,method,status,out_trade_no,notify_data,pay_url,created_at,paid_at,expire_at
		FROM payments WHERE id=?`, id)
	return scanPayment(row)
}

func (s *Store) GetPaymentByTradeNo(tradeNo string) (*Payment, error) {
	row := s.db.QueryRow(`SELECT id,order_id,user_id,trade_no,amount,method,status,out_trade_no,notify_data,pay_url,created_at,paid_at,expire_at
		FROM payments WHERE trade_no=?`, tradeNo)
	return scanPayment(row)
}

func (s *Store) GetPaymentByOutTradeNo(outTradeNo string) (*Payment, error) {
	row := s.db.QueryRow(`SELECT id,order_id,user_id,trade_no,amount,method,status,out_trade_no,notify_data,pay_url,created_at,paid_at,expire_at
		FROM payments WHERE out_trade_no=?`, outTradeNo)
	return scanPayment(row)
}

// MarkPaid 标记支付成功（idempotent）
func (s *Store) MarkPaymentPaid(id int64, outTradeNo, notifyData string) (bool, error) {
	now := time.Now()
	res, err := s.db.Exec(`UPDATE payments SET status=1, out_trade_no=COALESCE(NULLIF(?,''),out_trade_no),
		notify_data=?, paid_at=? WHERE id=? AND status=0`, outTradeNo, notifyData, now, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *Store) ListPayments(status int, method string, limit int) ([]Payment, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	args := []any{}
	where := `WHERE 1=1`
	if status >= 0 {
		where += ` AND status=?`
		args = append(args, status)
	}
	if method != "" {
		where += ` AND method=?`
		args = append(args, method)
	}
	args = append(args, limit)
	rows, err := s.db.Query(`SELECT id,order_id,user_id,trade_no,amount,method,status,out_trade_no,notify_data,pay_url,created_at,paid_at,expire_at
		FROM payments `+where+` ORDER BY id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Payment
	for rows.Next() {
		var p Payment
		if err := rows.Scan(&p.ID, &p.OrderID, &p.UserID, &p.TradeNo, &p.Amount,
			&p.Method, &p.Status, &p.OutTradeNo, &p.NotifyData, &p.PayURL, &p.CreatedAt, &p.PaidAt, &p.ExpireAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func scanPayment(row *sql.Row) (*Payment, error) {
	var p Payment
	if err := row.Scan(&p.ID, &p.OrderID, &p.UserID, &p.TradeNo, &p.Amount,
		&p.Method, &p.Status, &p.OutTradeNo, &p.NotifyData, &p.PayURL,
		&p.CreatedAt, &p.PaidAt, &p.ExpireAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

// ----- Settings -----

func (s *Store) GetSetting(key string) (*Setting, error) {
	row := s.db.QueryRow(`SELECT key,value,type,updated_at FROM settings WHERE key=?`, key)
	var st Setting
	if err := row.Scan(&st.Key, &st.Value, &st.Type, &st.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &st, nil
}

func (s *Store) UpsertSetting(key, value, typ string) error {
	_, err := s.db.Exec(`INSERT INTO settings (key,value,type,updated_at) VALUES (?,?,?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, type=excluded.type, updated_at=excluded.updated_at`,
		key, value, typ, time.Now())
	return err
}

func (s *Store) ListSettings() ([]Setting, error) {
	rows, err := s.db.Query(`SELECT key,value,type,updated_at FROM settings ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Setting
	for rows.Next() {
		var st Setting
		if err := rows.Scan(&st.Key, &st.Value, &st.Type, &st.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}
