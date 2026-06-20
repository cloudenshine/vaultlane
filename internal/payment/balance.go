package payment

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strconv"
)

// BalanceEngine 余额支付（同步）
type BalanceEngine struct {
	User   UserStore
	Order  OrderStore
	Logger *slog.Logger
}

// UserStore 用户相关存储依赖（避免反向依赖整个 store）
type UserStore interface {
	GetUserByID(id int64) (any, error)
	AdjustBalance(userID int64, amount float64, typ, note string, orderID int64) (float64, error)
}

// OrderStore 订单相关存储依赖
type OrderStore interface {
	GetOrderByTradeNo(tradeNo string) (any, error)
}

// NewBalanceEngine 构造余额支付
func NewBalanceEngine(u UserStore, o OrderStore, logger *slog.Logger) *BalanceEngine {
	if logger == nil {
		logger = slog.Default()
	}
	return &BalanceEngine{User: u, Order: o, Logger: logger}
}

func (b *BalanceEngine) Method() string { return "balance" }

// CreatePay 余额：先扣减，成功后返回成功标识（由回调路径触发发货）
func (b *BalanceEngine) CreatePay(ctx context.Context, req *CreateReq) (*CreateResp, error) {
	if req.UserID <= 0 {
		return nil, fmt.Errorf("balance: user_id required")
	}
	if req.Amount <= 0 {
		return nil, fmt.Errorf("balance: invalid amount")
	}
	// 余额扣减（负数）
	_, err := b.User.AdjustBalance(req.UserID, -req.Amount, "consume", "订单 "+req.TradeNo, 0)
	if err != nil {
		return nil, fmt.Errorf("balance: insufficient or db error: %w", err)
	}
	return &CreateResp{
		Method:   "balance",
		OutTrade: req.TradeNo,
	}, nil
}

func (b *BalanceEngine) VerifyNotify(ctx context.Context, raw []byte, headers map[string]string) (*Notify, error) {
	// 余额渠道是同步的，不走 HTTP 回调
	return nil, ErrVerifyFailed
}

func (b *BalanceEngine) QueryStatus(ctx context.Context, outTradeNo string) (*QueryResult, error) {
	return &QueryResult{Status: "success", OutTrade: outTradeNo}, nil
}

// GenerateTradeNo 生成本地流水号
func GenerateTradeNo() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	sum := sha256.Sum256(b)
	ts := strconv.FormatInt(nowUnixNano()/1e6, 36)
	return "FK" + ts + hex.EncodeToString(sum[:6])
}

func nowUnixNano() int64 { return timeNow() }
