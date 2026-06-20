package store

import (
	"database/sql"
	"encoding/json"
	"time"
)

// AuditLog 审计日志条目（VULN-012）
type AuditLog struct {
	ID        int64     `json:"id"`
	Actor     string    `json:"actor"`
	Action    string    `json:"action"`
	Target    string    `json:"target"`
	Detail    string    `json:"detail"`
	IP        string    `json:"ip"`
	CreatedAt time.Time `json:"created_at"`
}

// WriteAudit 写入一条审计日志（best-effort：失败不阻塞主流程）
func (s *Store) WriteAudit(actor, action, target string, detail any, ip string) {
	detailJSON := ""
	if detail != nil {
		if b, err := json.Marshal(detail); err == nil {
			detailJSON = string(b)
		} else {
			detailJSON = `{"_marshal_error":"` + err.Error() + `"}`
		}
	}
	_, _ = s.db.Exec(
		`INSERT INTO audit_logs (actor, action, target, detail, ip, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		actor, action, target, detailJSON, ip, time.Now(),
	)
}

// ListAudit 倒序拉取审计日志（带分页）
func (s *Store) ListAudit(limit, offset int) ([]AuditLog, error) {
	rows, err := s.db.Query(
		`SELECT id, actor, action, target, detail, ip, created_at
		   FROM audit_logs ORDER BY id DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditLog
	for rows.Next() {
		var a AuditLog
		var detail sql.NullString
		var ip sql.NullString
		if err := rows.Scan(&a.ID, &a.Actor, &a.Action, &a.Target, &detail, &ip, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.Detail = detail.String
		a.IP = ip.String
		out = append(out, a)
	}
	return out, rows.Err()
}

// CountAudit 统计审计日志总条数（admin 查询页用）
func (s *Store) CountAudit() (int64, error) {
	var n int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM audit_logs`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
