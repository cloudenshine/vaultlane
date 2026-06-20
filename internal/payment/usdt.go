package payment

import (
	"context"
	"fmt"
)

// USDTEngine USDT 支付骨架（地址 + 轮询）
// 真实链上轮询需要在生产环境接入 tron/eth 节点；这里只做接口合规。
type USDTEngine struct {
	Address string
	APIURL  string // 可选：第三方区块链浏览器 API
}

// NewUSDTEngine 构造
func NewUSDTEngine(address, apiURL string) *USDTEngine {
	return &USDTEngine{Address: address, APIURL: apiURL}
}

func (u *USDTEngine) Method() string { return "usdt" }

func (u *USDTEngine) CreatePay(ctx context.Context, req *CreateReq) (*CreateResp, error) {
	if u.Address == "" {
		return nil, ErrUnsupported
	}
	// 简化：返回收款地址 + 金额，UI 端生成二维码
	return &CreateResp{
		Method:   "usdt",
		PayURL:   fmt.Sprintf("usdt:%s?amount=%.2f", u.Address, req.Amount),
		OutTrade: req.TradeNo,
	}, nil
}

func (u *USDTEngine) VerifyNotify(ctx context.Context, raw []byte, headers map[string]string) (*Notify, error) {
	// 真实实现：调用区块链浏览器 API 查询 out_trade_no 对应金额/确认数
	return &Notify{Status: "pending", Raw: string(raw)}, nil
}

func (u *USDTEngine) QueryStatus(ctx context.Context, outTradeNo string) (*QueryResult, error) {
	// 真实实现：调用区块链浏览器
	return &QueryResult{Status: "pending", OutTrade: outTradeNo}, nil
}
