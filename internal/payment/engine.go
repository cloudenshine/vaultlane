// Package payment 提供统一的支付接口与多种实现。
//
// 设计原则：所有支付渠道实现 Engine 接口；回调统一走 callback 入口验签。
// 不依赖第三方 SDK（保持二进制小、依赖少），真实接入时新增实现即可。
package payment

import (
	"context"
	"errors"
)

// Engine 支付渠道统一接口
type Engine interface {
	// Method 渠道标识（alipay/wxpay/usdt/balance/epay）
	Method() string
	// CreatePay 创建支付（返回跳转 URL 或表单参数；balance 返回成功/失败）
	CreatePay(ctx context.Context, req *CreateReq) (*CreateResp, error)
	// VerifyNotify 验证并解析回调（HTTP 调用）
	VerifyNotify(ctx context.Context, raw []byte, headers map[string]string) (*Notify, error)
	// QueryStatus 主动查询订单状态（用于轮询；USDT 用到）
	QueryStatus(ctx context.Context, outTradeNo string) (*QueryResult, error)
}

// CreateReq 创建支付请求
type CreateReq struct {
	TradeNo   string  // 本地流水号
	Amount    float64 // 金额（元）
	Subject   string  // 商品名
	NotifyURL string  // 回调地址
	ReturnURL string  // 同步跳转
	UserID    int64
	ExpireSec int // 过期秒数，0 = 默认
}

// CreateResp 创建支付响应
type CreateResp struct {
	Method   string // 渠道
	PayURL   string // 跳转 URL（表单提交 / 二维码 / 空）
	FormHTML string // 自渲染 HTML 表单（epay 用）
	OutTrade string // 渠道订单号（balance 等同步渠道直接 = TradeNo）
	ExpireAt int64  // 过期时间戳
}

// Notify 回调统一结构
type Notify struct {
	OutTradeNo string // 渠道订单号
	TradeNo    string // 本地流水号
	Amount     float64
	Status     string // success / failed
	Raw        string
}

// QueryResult 查询结果
type QueryResult struct {
	Status   string // success / pending / failed
	Amount   float64
	OutTrade string
}

// ErrUnsupported 渠道未启用
var ErrUnsupported = errors.New("payment: engine not supported")

// ErrVerifyFailed 验签失败
var ErrVerifyFailed = errors.New("payment: verify failed")
