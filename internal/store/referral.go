package store

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"time"
)

// ReferralStats 邀请返佣统计
type ReferralStats struct {
	InviteCode      string  `json:"invite_code"`
	TotalInvites    int     `json:"total_invites"`
	CommissionBal   float64 `json:"commission_balance"`
	TotalCommission float64 `json:"total_commission"`
}

// EnsureUserInviteCode 确保用户拥有邀请码
func (s *Store) EnsureUserInviteCode(userID int64) (string, error) {
	var code string
	err := s.db.QueryRow(`SELECT invite_code FROM users WHERE id=?`, userID).Scan(&code)
	if err != nil {
		return "", err
	}
	if code != "" {
		return code, nil
	}
	code = fmt.Sprintf("INV%04d%s", userID, randomString(3))
	_, err = s.db.Exec(`UPDATE users SET invite_code=? WHERE id=?`, code, userID)
	return code, err
}

// BindReferrer 绑定推荐人
func (s *Store) BindReferrer(userID int64, inviteCode string) (bool, error) {
	if inviteCode == "" {
		return false, nil
	}
	var referrerID int64
	err := s.db.QueryRow(`SELECT id FROM users WHERE invite_code=? AND id!=?`, inviteCode, userID).Scan(&referrerID)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, err = s.db.Exec(`UPDATE users SET referrer_id=? WHERE id=? AND referrer_id=0`, referrerID, userID)
	return err == nil, err
}

// ProcessOrderCommission 订单完成触发分销返佣
func (s *Store) ProcessOrderCommission(orderID, buyerID int64, amount float64, rate float64) error {
	if buyerID <= 0 || amount <= 0 || rate <= 0 {
		return nil
	}
	var referrerID int64
	err := s.db.QueryRow(`SELECT referrer_id FROM users WHERE id=?`, buyerID).Scan(&referrerID)
	if err != nil || referrerID <= 0 {
		return nil
	}

	commission := amount * rate
	if commission < 0.01 {
		return nil
	}

	note := fmt.Sprintf("邀请返佣: 订单 #%d (消费 ¥%.2f × %.1f%%)", orderID, amount, rate*100)
	_, err = s.AdjustBalance(referrerID, commission, "referral", note, orderID)
	if err != nil {
		return err
	}

	// 累加佣金账户
	_, _ = s.db.Exec(`UPDATE users SET commission_balance=commission_balance+? WHERE id=?`, commission, referrerID)
	return nil
}

// GetUserReferralStats 获取分销推广统计
func (s *Store) GetUserReferralStats(userID int64) (*ReferralStats, error) {
	code, err := s.EnsureUserInviteCode(userID)
	if err != nil {
		return nil, err
	}

	var count int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE referrer_id=?`, userID).Scan(&count)

	var commBal float64
	_ = s.db.QueryRow(`SELECT commission_balance FROM users WHERE id=?`, userID).Scan(&commBal)

	var totalEarned float64
	_ = s.db.QueryRow(`SELECT COALESCE(SUM(amount), 0) FROM balance_logs WHERE user_id=? AND type='referral'`, userID).Scan(&totalEarned)

	return &ReferralStats{
		InviteCode:      code,
		TotalInvites:    count,
		CommissionBal:   commBal,
		TotalCommission: totalEarned,
	}, nil
}

func randomString(n int) string {
	const letters = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	raw := make([]byte, n)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano()%1000)
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[int(raw[i])%len(letters)]
	}
	return string(b)
}
