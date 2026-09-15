package store

import "time"

// RevenuePoint 收入曲线
type RevenuePoint struct {
	Date  string  `json:"date"`
	Count int     `json:"count"`
	Sum   float64 `json:"sum"`
}

// TopCommodity 销量排行
type TopCommodity struct {
	CommodityID   int64   `json:"commodity_id"`
	CommodityName string  `json:"commodity_name"`
	Sold          int     `json:"sold"`
	Revenue       float64 `json:"revenue"`
}

// OrderSummaryRange 使用 SQL 聚合统计区间订单量与有效收入
func (s *Store) OrderSummaryRange(from, to time.Time) (int, float64, error) {
	row := s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN status >= 1 THEN amount ELSE 0 END), 0)
		FROM orders WHERE created_at BETWEEN ? AND ?`, from, to)
	var count int
	var rev float64
	if err := row.Scan(&count, &rev); err != nil {
		return 0, 0, err
	}
	return count, rev, nil
}

// OrdersRange 区间订单
func (s *Store) OrdersRange(from, to time.Time) ([]Order, error) {
	rows, err := s.db.Query(`SELECT id, trade_no, request_no, commodity_id, commodity_name, shared_code,
		contact, num, race, password, amount, status, pay_status, contents,
		upstream_code, upstream_msg, source, category_id, user_id, payment_id, pay_method, coupon_id, discount, created_at, updated_at
		FROM orders WHERE created_at BETWEEN ? AND ? ORDER BY id DESC`, from, to)
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

// RevenueByDay 按天聚合
func (s *Store) RevenueByDay(from, to time.Time) ([]RevenuePoint, error) {
	rows, err := s.db.Query(`SELECT DATE(created_at) AS d, COUNT(*), COALESCE(SUM(amount),0)
		FROM orders WHERE status >= 1 AND created_at BETWEEN ? AND ?
		GROUP BY DATE(created_at) ORDER BY d ASC`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RevenuePoint
	for rows.Next() {
		var p RevenuePoint
		var d string
		if err := rows.Scan(&d, &p.Count, &p.Sum); err != nil {
			return nil, err
		}
		p.Date = d
		out = append(out, p)
	}
	return out, rows.Err()
}

// TopCommodities 销量排行
func (s *Store) TopCommodities(limit int) ([]TopCommodity, error) {
	rows, err := s.db.Query(`SELECT commodity_id, commodity_name, COUNT(*) AS sold, COALESCE(SUM(amount),0) AS rev
		FROM orders WHERE status >= 1
		GROUP BY commodity_id, commodity_name
		ORDER BY sold DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TopCommodity
	for rows.Next() {
		var t TopCommodity
		if err := rows.Scan(&t.CommodityID, &t.CommodityName, &t.Sold, &t.Revenue); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
