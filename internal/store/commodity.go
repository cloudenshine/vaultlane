package store

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"
)

// ceil2 向上取整到 0.01
func ceil2(v float64) float64 {
	return math.Ceil(v*100) / 100
}

// Category 自营/上游分类
type Category struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Icon      string    `json:"icon"`
	Sort      int       `json:"sort"`
	ParentID  int64     `json:"parent_id"`
	Source    string    `json:"source"` // self / upstream:upstreama
	CreatedAt time.Time `json:"created_at"`
}

// Commodity 商品池记录（自营 + 多上游映射）
type Commodity struct {
	ID            int64      `json:"id"`
	CategoryID    int64      `json:"category_id"`
	Name          string     `json:"name"`
	Cover         string     `json:"cover"`
	Description   string     `json:"description"`
	Price         float64    `json:"price"`      // 兼容旧字段 = sale_price
	SalePrice     float64    `json:"sale_price"` // 实际售价（管理员设定）
	CostPrice     float64    `json:"cost_price"` // 成本价（来自上游）
	Stock         int        `json:"stock"`
	Sold          int        `json:"sold"`
	StockWarning  int        `json:"stock_warning"` // 库存预警阈值
	DeliveryWay   int        `json:"delivery_way"`  // 0=手动 1=自动卡密
	Status        int        `json:"status"`        // 0=下架 1=上架
	Sort          int        `json:"sort"`
	SharedCode    string     `json:"shared_code"`    // 上游 code
	Source        string     `json:"source"`         // self / upstream:upstreama / upstream:mock2
	OuterID       string     `json:"outer_id"`       // 外部 ID
	Config        string     `json:"config"`         // 上游配置（JSON 字符串：category/spec）
	Tags          string     `json:"tags"`           // 逗号分隔
	Minimum       int        `json:"minimum"`        // 起售
	Maximum       int        `json:"maximum"`        // 限购
	UpstreamExtra string     `json:"upstream_extra"` // 上游原始数据（JSON）
	LastSeenAt    *time.Time `json:"last_seen_at"`   // 上次同步出现时间
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// CardSecret 卡密
type CardSecret struct {
	ID          int64      `json:"id"`
	CommodityID int64      `json:"commodity_id"`
	Content     string     `json:"content"`
	Status      int        `json:"status"` // 0=未售 1=已售 2=锁定 3=禁用
	OrderID     int64      `json:"order_id"`
	SoldAt      *time.Time `json:"sold_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

// ----- Categories -----

func (s *Store) CreateCategory(c *Category) error {
	now := time.Now()
	c.CreatedAt = now
	res, err := s.db.Exec(`INSERT INTO categories (name, icon, sort, parent_id, source, created_at)
		VALUES (?,?,?,?,?,?)`,
		c.Name, c.Icon, c.Sort, c.ParentID, c.Source, c.CreatedAt)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	c.ID = id
	return nil
}

func (s *Store) ListCategories() ([]Category, error) {
	rows, err := s.db.Query(`SELECT id, name, icon, sort, parent_id, source, created_at
		FROM categories ORDER BY sort ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Category
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Name, &c.Icon, &c.Sort, &c.ParentID, &c.Source, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ----- Commodities -----

// commodityCols 公共列
const commodityCols = `id, category_id, name, cover, description, price, sale_price, cost_price,
		stock, sold, stock_warning, delivery_way, status, sort,
		shared_code, source, outer_id, config, tags, minimum, maximum,
		upstream_extra, last_seen_at, created_at, updated_at`

func (s *Store) CreateCommodity(c *Commodity) error {
	now := time.Now()
	c.CreatedAt = now
	c.UpdatedAt = now
	if c.Source == "" {
		c.Source = "self"
	}
	if c.SalePrice == 0 && c.Price > 0 {
		c.SalePrice = c.Price
	}
	if c.Status == 0 {
		c.Status = 1
	}
	res, err := s.db.Exec(`INSERT INTO commodities
		(category_id, name, cover, description, price, sale_price, cost_price,
		 stock, sold, stock_warning, delivery_way, status, sort,
		 shared_code, source, outer_id, config, tags, minimum, maximum,
		 upstream_extra, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.CategoryID, c.Name, c.Cover, c.Description, c.Price, c.SalePrice, c.CostPrice,
		c.Stock, c.Sold, c.StockWarning, c.DeliveryWay, c.Status, c.Sort,
		c.SharedCode, c.Source, c.OuterID, c.Config, c.Tags, c.Minimum, c.Maximum,
		c.UpstreamExtra, c.CreatedAt, c.UpdatedAt)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	c.ID = id
	return nil
}

func (s *Store) UpdateCommodity(c *Commodity) error {
	c.UpdatedAt = time.Now()
	_, err := s.db.Exec(`UPDATE commodities SET
		category_id=?, name=?, cover=?, description=?, price=?, sale_price=?, cost_price=?,
		stock=?, sold=?, stock_warning=?, delivery_way=?, status=?, sort=?,
		shared_code=?, source=?, outer_id=?, config=?, tags=?, minimum=?, maximum=?,
		upstream_extra=?, updated_at=?
		WHERE id=?`,
		c.CategoryID, c.Name, c.Cover, c.Description, c.Price, c.SalePrice, c.CostPrice,
		c.Stock, c.Sold, c.StockWarning, c.DeliveryWay, c.Status, c.Sort,
		c.SharedCode, c.Source, c.OuterID, c.Config, c.Tags, c.Minimum, c.Maximum,
		c.UpstreamExtra, c.UpdatedAt, c.ID)
	return err
}

func (s *Store) DeleteCommodity(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM card_secrets WHERE commodity_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM commodities WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) GetCommodityByID(id int64) (*Commodity, error) {
	row := s.db.QueryRow(`SELECT `+commodityCols+` FROM commodities WHERE id = ?`, id)
	var c Commodity
	var ls sql.NullTime
	err := row.Scan(&c.ID, &c.CategoryID, &c.Name, &c.Cover, &c.Description,
		&c.Price, &c.SalePrice, &c.CostPrice,
		&c.Stock, &c.Sold, &c.StockWarning, &c.DeliveryWay, &c.Status, &c.Sort,
		&c.SharedCode, &c.Source, &c.OuterID, &c.Config, &c.Tags, &c.Minimum, &c.Maximum,
		&c.UpstreamExtra, &ls, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if ls.Valid {
		c.LastSeenAt = &ls.Time
	}
	return &c, nil
}

// scanCommodity 通用行扫描
func scanCommodity(rows *sql.Rows) (Commodity, error) {
	var c Commodity
	var ls sql.NullTime
	if err := rows.Scan(&c.ID, &c.CategoryID, &c.Name, &c.Cover, &c.Description,
		&c.Price, &c.SalePrice, &c.CostPrice,
		&c.Stock, &c.Sold, &c.StockWarning, &c.DeliveryWay, &c.Status, &c.Sort,
		&c.SharedCode, &c.Source, &c.OuterID, &c.Config, &c.Tags, &c.Minimum, &c.Maximum,
		&c.UpstreamExtra, &ls, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return c, err
	}
	if ls.Valid {
		c.LastSeenAt = &ls.Time
	}
	return c, nil
}

// ListCommodities 分页 + 分类筛选（用户端）
func (s *Store) ListCommodities(categoryID int64, source string, page, limit int) ([]Commodity, int, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 24
	}
	offset := (page - 1) * limit

	where := `WHERE status=1`
	args := []any{}
	if categoryID > 0 {
		where += ` AND category_id=?`
		args = append(args, categoryID)
	}
	if source != "" {
		where += ` AND source=?`
		args = append(args, source)
	}

	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM commodities `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args2 := append(args, limit, offset)
	rows, err := s.db.Query(`SELECT `+commodityCols+` FROM commodities `+where+
		` ORDER BY sort ASC, id DESC LIMIT ? OFFSET ?`, args2...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Commodity
	for rows.Next() {
		c, err := scanCommodity(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, c)
	}
	return out, total, rows.Err()
}

// ListAllCommodities 后台用（含下架，支持关键字/来源筛选）
func (s *Store) ListAllCommodities(source, keyword string, page, limit int) ([]Commodity, int, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 500 {
		limit = 100
	}
	offset := (page - 1) * limit

	where := `WHERE 1=1`
	args := []any{}
	if source != "" {
		where += ` AND source=?`
		args = append(args, source)
	}
	if keyword != "" {
		where += ` AND name LIKE ?`
		args = append(args, "%"+keyword+"%")
	}

	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM commodities `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args2 := append(args, limit, offset)
	rows, err := s.db.Query(`SELECT `+commodityCols+` FROM commodities `+where+
		` ORDER BY id DESC LIMIT ? OFFSET ?`, args2...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Commodity
	for rows.Next() {
		c, err := scanCommodity(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, c)
	}
	return out, total, rows.Err()
}

// batchMaxIDs 单次批量操作最大 ID 数量（VULN-008：防 DoS）
const batchMaxIDs = 1000

// BatchSetStatus 批量上下架
func (s *Store) BatchSetStatus(ids []int64, status int) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	if len(ids) > batchMaxIDs {
		return 0, fmt.Errorf("batch: too many ids (%d > %d)", len(ids), batchMaxIDs)
	}
	q := `UPDATE commodities SET status=?, updated_at=? WHERE id IN (?` +
		strings.Repeat(",?", len(ids)-1) + `)`
	args := []any{status, time.Now()}
	for _, id := range ids {
		if id <= 0 {
			return 0, fmt.Errorf("batch: invalid id %d", id)
		}
		args = append(args, id)
	}
	res, err := s.db.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// BatchAdjustPrice 批量调价：type 0=固定 1=百分比；op +/-
func (s *Store) BatchAdjustPrice(ids []int64, opType int, op string, amount float64) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	if len(ids) > batchMaxIDs {
		return 0, fmt.Errorf("batch: too many ids (%d > %d)", len(ids), batchMaxIDs)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	now := time.Now()
	n := 0
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		row := tx.QueryRow(`SELECT sale_price, cost_price FROM commodities WHERE id=?`, id)
		var sale, cost float64
		if err := row.Scan(&sale, &cost); err != nil {
			continue
		}
		var newSale float64
		if opType == 0 { // 固定
			if op == "+" {
				newSale = sale + amount
			} else {
				newSale = sale - amount
			}
		} else { // 百分比
			if op == "+" {
				newSale = sale * (1 + amount/100)
			} else {
				newSale = sale * (1 - amount/100)
			}
		}
		if newSale < 0 {
			newSale = 0
		}
		if newSale < cost {
			newSale = cost // 不允许亏本
		}
		if _, err := tx.Exec(`UPDATE commodities SET sale_price=?, price=?, updated_at=? WHERE id=?`,
			newSale, newSale, now, id); err != nil {
			continue
		}
		n++
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
}

// UpsertFromUpstream 上游同步：按 (source, outer_id) 唯一，存在则更新，不存在则插入
func (s *Store) UpsertFromUpstream(c *Commodity) (id int64, isNew bool, err error) {
	now := time.Now()
	// 查
	row := s.db.QueryRow(`SELECT id, sale_price FROM commodities WHERE source=? AND outer_id=?`,
		c.Source, c.OuterID)
	var existingID int64
	var existingPrice float64
	scanErr := row.Scan(&existingID, &existingPrice)
	if scanErr == nil {
		// 存在：更新成本/库存/描述/封面/config/tags/最后见到时间
		// 售价不动（保留管理员设定）
		c.ID = existingID
		c.UpdatedAt = now
		_, err = s.db.Exec(`UPDATE commodities SET
			name=?, cover=?, description=?, cost_price=?, stock=?, config=?, tags=?,
			category_id=?, upstream_extra=?, last_seen_at=?, updated_at=?
			WHERE id=?`,
			c.Name, c.Cover, c.Description, c.CostPrice, c.Stock, c.Config, c.Tags,
			c.CategoryID, c.UpstreamExtra, now, now, existingID)
		return existingID, false, err
	}
	// 新建：默认售价 = 成本 × 1.1（向上取整 0.01），默认上架
	if c.SalePrice <= 0 {
		c.SalePrice = ceil2(c.CostPrice * 1.1)
	}
	c.Price = c.SalePrice
	c.Status = 1
	c.CreatedAt = now
	c.UpdatedAt = now
	res, ierr := s.db.Exec(`INSERT INTO commodities
		(category_id, name, cover, description, price, sale_price, cost_price,
		 stock, sold, stock_warning, delivery_way, status, sort,
		 shared_code, source, outer_id, config, tags, minimum, maximum,
		 upstream_extra, last_seen_at, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.CategoryID, c.Name, c.Cover, c.Description, c.Price, c.SalePrice, c.CostPrice,
		c.Stock, c.Sold, c.StockWarning, c.DeliveryWay, c.Status, c.Sort,
		c.SharedCode, c.Source, c.OuterID, c.Config, c.Tags, c.Minimum, c.Maximum,
		c.UpstreamExtra, now, c.CreatedAt, c.UpdatedAt)
	if ierr != nil {
		return 0, false, ierr
	}
	id, _ = res.LastInsertId()
	return id, true, nil
}

// MarkUpstreamMissing 把一段时间没见到的上游商品标为下架
func (s *Store) MarkUpstreamMissing(source string, olderThan time.Time) (int, error) {
	res, err := s.db.Exec(`UPDATE commodities SET status=0, updated_at=?
		WHERE source=? AND last_seen_at IS NOT NULL AND last_seen_at < ?`,
		time.Now(), source, olderThan)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (s *Store) CountCommodities() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM commodities WHERE status=1`).Scan(&n)
	return n, err
}

// CountCommoditiesBySource 按上游统计
func (s *Store) CountCommoditiesBySource() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT source, COUNT(*) FROM commodities GROUP BY source`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var src string
		var c int
		if err := rows.Scan(&src, &c); err != nil {
			return nil, err
		}
		out[src] = c
	}
	return out, rows.Err()
}

// ----- Upstream Runtime -----

// UpstreamRuntime 同步状态
type UpstreamRuntime struct {
	Name        string     `json:"name"`
	Enabled     bool       `json:"enabled"`
	LastSyncAt  *time.Time `json:"last_sync_at"`
	LastStatus  string     `json:"last_status"`
	LastError   string     `json:"last_error"`
	SyncedCount int        `json:"synced_count"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (s *Store) GetUpstreamRuntime(name string) (*UpstreamRuntime, error) {
	row := s.db.QueryRow(`SELECT name, enabled, last_sync_at, last_status, last_error, synced_count, updated_at
		FROM upstream_runtime WHERE name=?`, name)
	var r UpstreamRuntime
	var enabled int
	var ls sql.NullTime
	if err := row.Scan(&r.Name, &enabled, &ls, &r.LastStatus, &r.LastError, &r.SyncedCount, &r.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	r.Enabled = enabled == 1
	if ls.Valid {
		r.LastSyncAt = &ls.Time
	}
	return &r, nil
}

func (s *Store) ListUpstreamRuntimes() ([]UpstreamRuntime, error) {
	rows, err := s.db.Query(`SELECT name, enabled, last_sync_at, last_status, last_error, synced_count, updated_at
		FROM upstream_runtime ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UpstreamRuntime
	for rows.Next() {
		var r UpstreamRuntime
		var enabled int
		var ls sql.NullTime
		if err := rows.Scan(&r.Name, &enabled, &ls, &r.LastStatus, &r.LastError, &r.SyncedCount, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) UpsertUpstreamRuntime(r *UpstreamRuntime) error {
	now := time.Now()
	r.UpdatedAt = now
	enabled := 0
	if r.Enabled {
		enabled = 1
	}
	_, err := s.db.Exec(`INSERT INTO upstream_runtime
		(name, enabled, last_sync_at, last_status, last_error, synced_count, updated_at)
		VALUES (?,?,?,?,?,?,?)
		ON CONFLICT(name) DO UPDATE SET
			enabled=excluded.enabled, last_sync_at=excluded.last_sync_at,
			last_status=excluded.last_status, last_error=excluded.last_error,
			synced_count=excluded.synced_count, updated_at=excluded.updated_at`,
		r.Name, enabled, r.LastSyncAt, r.LastStatus, r.LastError, r.SyncedCount, r.UpdatedAt)
	return err
}

// ----- Card Secrets -----

// ImportSecrets 批量导入
func (s *Store) ImportSecrets(commodityID int64, contents []string) (int, error) {
	if len(contents) == 0 {
		return 0, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := time.Now()
	stmt, err := tx.Prepare(`INSERT INTO card_secrets (commodity_id, content, status, order_id, created_at)
		VALUES (?,?,0,0,?)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	n := 0
	for _, c := range contents {
		if c == "" {
			continue
		}
		if _, err := stmt.Exec(commodityID, c, now); err != nil {
			return n, err
		}
		n++
	}
	// 刷新库存
	if _, err := tx.Exec(`UPDATE commodities SET stock=(SELECT COUNT(*) FROM card_secrets
		WHERE commodity_id=? AND status=0), updated_at=? WHERE id=?`,
		commodityID, now, commodityID); err != nil {
		return n, err
	}
	if err := tx.Commit(); err != nil {
		return n, err
	}
	return n, nil
}

func (s *Store) ListSecrets(commodityID int64, status int, limit int) ([]CardSecret, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	args := []any{}
	where := `WHERE 1=1`
	if commodityID > 0 {
		where += ` AND commodity_id=?`
		args = append(args, commodityID)
	}
	if status >= 0 {
		where += ` AND status=?`
		args = append(args, status)
	}
	args = append(args, limit)
	rows, err := s.db.Query(`SELECT id, commodity_id, content, status, order_id, sold_at, created_at
		FROM card_secrets `+where+` ORDER BY id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CardSecret
	for rows.Next() {
		var c CardSecret
		if err := rows.Scan(&c.ID, &c.CommodityID, &c.Content, &c.Status, &c.OrderID, &c.SoldAt, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// PullSecret 自营发货：原子取一张未售卡密
func (s *Store) PullSecret(commodityID, orderID int64) (*CardSecret, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// 找一张未售卡密并锁定
	row := tx.QueryRow(`SELECT id, commodity_id, content, status, order_id, sold_at, created_at
		FROM card_secrets WHERE commodity_id=? AND status=0 ORDER BY id ASC LIMIT 1`, commodityID)
	var c CardSecret
	if err := row.Scan(&c.ID, &c.CommodityID, &c.Content, &c.Status, &c.OrderID, &c.SoldAt, &c.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	now := time.Now()
	if _, err := tx.Exec(`UPDATE card_secrets SET status=1, order_id=?, sold_at=? WHERE id=?`,
		orderID, now, c.ID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE commodities SET stock=(SELECT COUNT(*) FROM card_secrets
		WHERE commodity_id=? AND status=0), sold=sold+1, updated_at=? WHERE id=?`,
		commodityID, now, commodityID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	c.Status = 1
	c.OrderID = orderID
	c.SoldAt = &now
	return &c, nil
}
