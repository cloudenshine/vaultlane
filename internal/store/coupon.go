package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Coupon 优惠券模型
type Coupon struct {
	ID         int64      `json:"id"`
	Code       string     `json:"code"`
	Name       string     `json:"name"`
	Type       int        `json:"type"`       // 0=固定立减金额, 1=百分比折扣 (0.1=9折, 0.2=8折)
	Discount   float64    `json:"discount"`   // 立减金额或折扣比例
	MinAmount  float64    `json:"min_amount"` // 最低消费使用门槛
	TotalLimit int        `json:"total_limit"`// 总发放量，0=不限制
	UsedCount  int        `json:"used_count"` // 已核销次数
	Status     int        `json:"status"`     // 1=启用 0=禁用
	ExpireAt   *time.Time `json:"expire_at"`  // 过期时间
	CreatedAt  time.Time  `json:"created_at"`
}

// CreateCoupon 创建优惠券
func (s *Store) CreateCoupon(c *Coupon) error {
	now := time.Now()
	c.CreatedAt = now
	if c.Status == 0 {
		c.Status = 1
	}
	res, err := s.db.Exec(`INSERT INTO coupons (code, name, type, discount, min_amount, total_limit, used_count, status, expire_at, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		c.Code, c.Name, c.Type, c.Discount, c.MinAmount, c.TotalLimit, c.UsedCount, c.Status, c.ExpireAt, c.CreatedAt)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	c.ID = id
	return nil
}

// GetCouponByCode 按券码查询
func (s *Store) GetCouponByCode(code string) (*Coupon, error) {
	row := s.db.QueryRow(`SELECT id, code, name, type, discount, min_amount, total_limit, used_count, status, expire_at, created_at
		FROM coupons WHERE code = ?`, code)
	var c Coupon
	var exp sql.NullTime
	if err := row.Scan(&c.ID, &c.Code, &c.Name, &c.Type, &c.Discount, &c.MinAmount, &c.TotalLimit, &c.UsedCount, &c.Status, &exp, &c.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if exp.Valid {
		c.ExpireAt = &exp.Time
	}
	return &c, nil
}

// ListCoupons 列出所有优惠券
func (s *Store) ListCoupons() ([]Coupon, error) {
	rows, err := s.db.Query(`SELECT id, code, name, type, discount, min_amount, total_limit, used_count, status, expire_at, created_at
		FROM coupons ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Coupon
	for rows.Next() {
		var c Coupon
		var exp sql.NullTime
		if err := rows.Scan(&c.ID, &c.Code, &c.Name, &c.Type, &c.Discount, &c.MinAmount, &c.TotalLimit, &c.UsedCount, &c.Status, &exp, &c.CreatedAt); err != nil {
			return nil, err
		}
		if exp.Valid {
			c.ExpireAt = &exp.Time
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ValidateCoupon 校验优惠券是否可用并计算减免金额
func (s *Store) ValidateCoupon(code string, orderAmount float64, userID int64) (*Coupon, float64, error) {
	c, err := s.GetCouponByCode(code)
	if err != nil {
		return nil, 0, err
	}
	if c == nil {
		return nil, 0, errors.New("优惠券不存在")
	}
	if c.Status != 1 {
		return nil, 0, errors.New("优惠券已停用")
	}
	if c.ExpireAt != nil && time.Now().After(*c.ExpireAt) {
		return nil, 0, errors.New("优惠券已过期")
	}
	if c.TotalLimit > 0 && c.UsedCount >= c.TotalLimit {
		return nil, 0, errors.New("优惠券已被抢光")
	}
	if orderAmount < c.MinAmount {
		return nil, 0, fmt.Errorf("未达到最低消费门槛 ¥%.2f", c.MinAmount)
	}
	// 用户是否已使用过
	if userID > 0 {
		var used int
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM coupon_usages WHERE coupon_id=? AND user_id=?`, c.ID, userID).Scan(&used)
		if used > 0 {
			return nil, 0, errors.New("您已使用过该优惠券")
		}
	}

	// 计算减免金额
	var discount float64
	if c.Type == 0 { // 固定金额
		discount = c.Discount
	} else if c.Type == 1 { // 百分比
		discount = orderAmount * c.Discount
	}
	if discount > orderAmount {
		discount = orderAmount
	}
	return c, discount, nil
}

// UseCoupon 核销优惠券
func (s *Store) UseCoupon(couponID, userID, orderID int64, discount float64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 增加已用次数
	if _, err := tx.Exec(`UPDATE coupons SET used_count=used_count+1 WHERE id=?`, couponID); err != nil {
		return err
	}
	// 记录使用明细
	if _, err := tx.Exec(`INSERT INTO coupon_usages (coupon_id, user_id, order_id, discount, created_at)
		VALUES (?,?,?,?,?)`, couponID, userID, orderID, discount, time.Now()); err != nil {
		return err
	}
	return tx.Commit()
}
