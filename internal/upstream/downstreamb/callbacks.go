// Package downstreamb callbacks.go
// 接收下游网关推送（验签 + 解析）
package downstreamb

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
)

// VerifyFuluOrderSign 验签（福禄订单回调）
// 签名规则（参考福禄文档）：按字段排序拼接 + secret md5
func VerifyFuluOrderSign(notify *FuluOrderNotifyReq, secret string) bool {
	if notify.Sign == "" {
		return false
	}
	// 收集非空字段
	parts := []string{
		notify.XyOrderNo, notify.ChargeFinishTime, notify.OutOrderNo,
		notify.OrderStatus, notify.RechargeDescription, notify.ProductID,
		notify.Price, notify.BuyNum, notify.OperatorSerialNumber,
	}
	var nonEmpty []string
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	sort.Strings(nonEmpty)
	raw := strings.Join(nonEmpty, "&") + secret
	sum := md5.Sum([]byte(raw))
	return hex.EncodeToString(sum[:]) == notify.Sign
}

// ValidateNotify 基础校验
func ValidateNotify(n *VirtualOrderNotifyReq) error {
	if n.OrderNo == "" {
		return errors.New("order_no empty")
	}
	if n.AppID == 0 {
		return errors.New("app_id empty")
	}
	return nil
}
