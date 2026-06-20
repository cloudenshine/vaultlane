// Package downstreamb 下游网关开放平台（自研系统-进销存类型）客户端
//
// 鉴权算法（自研签名）：
//   sign = md5("{appKey},{bodyMd5},{timestamp},{appSecret}")
//   业务参数：POST JSON Body
//   签名参数：URL Query (?appid=&timestamp=&sign=)
package downstreamb

import (
	"crypto/md5"
	"encoding/hex"
	"strconv"
)

// GenerateSign 计算自研签名（不含 SellerID）
//   bodyJSON: 已序列化的 JSON 字符串（保持原始顺序，不做排序）
//   ts:       秒级时间戳
func GenerateSign(appKey int64, appSecret, bodyJSON string, ts int64) string {
	bodyMd5 := md5Hex(bodyJSON)
	raw := strconv.FormatInt(appKey, 10) + "," +
		bodyMd5 + "," +
		strconv.FormatInt(ts, 10) + "," +
		appSecret
	return md5Hex(raw)
}

// GenerateSignWithSeller 计算商务对接签名（含 SellerID）
func GenerateSignWithSeller(appKey int64, appSecret, bodyJSON string, ts, sellerID int64) string {
	bodyMd5 := md5Hex(bodyJSON)
	raw := strconv.FormatInt(appKey, 10) + "," +
		bodyMd5 + "," +
		strconv.FormatInt(ts, 10) + "," +
		strconv.FormatInt(sellerID, 10) + "," +
		appSecret
	return md5Hex(raw)
}

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}
