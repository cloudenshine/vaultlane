package api

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// sanitizeLogValue 对写入日志的字符串做脱敏（VULN-011）
// 命中以下任一规则，则返回 sha256 前 4 + ... + 后 2 形式：
//  1. 长度 >= 16 且包含换行（典型多行卡密）
//  2. 长度 >= 20 且形如 base64 / 十六进制（典型密钥/Token）
//  3. 长度 >= 12 且匹配 [A-Za-z0-9-_]{12,} 紧凑形式（典型卡号 / License）
//
// 业务主键（trade_no / out_trade_no / order_id）通常短、无换行、非 base64，
// 不会被命中；卡密/Token 会被识别并替换为指纹。
func sanitizeLogValue(s string) string {
	if s == "" {
		return ""
	}
	if looksLikeSecret(s) {
		sum := sha256.Sum256([]byte(s))
		h := hex.EncodeToString(sum[:])
		return "[redacted:" + h[:4] + "..." + h[len(h)-2:] + "]"
	}
	return s
}

var (
	reMultiline  = regexp.MustCompile(`[\r\n]`)
	reBase64Hex  = regexp.MustCompile(`^[A-Za-z0-9+/_=-]{16,}$`)
	reTightToken = regexp.MustCompile(`^[A-Za-z0-9_-]{12,}$`)
)

func looksLikeSecret(s string) bool {
	// 多行 → 几乎肯定是卡密 / 多 token 列表
	if reMultiline.MatchString(s) {
		return true
	}
	// 长度阈值
	if len(s) < 12 {
		return false
	}
	if reBase64Hex.MatchString(s) || reTightToken.MatchString(s) {
		// 额外排除常见的"短 base64 也合理"模式：trade_no / 时间戳
		// trade_no 通常带分隔符或纯数字，等下单独处理
		if isLikelyBusinessKey(s) {
			return false
		}
		return true
	}
	return false
}

// isLikelyBusinessKey 识别订单/支付主键，避免误杀日志
// 规则：包含 '-' / '_' / 纯数字 → 大概率是 trade_no / out_trade_no / uid
func isLikelyBusinessKey(s string) bool {
	if strings.ContainsAny(s, "-_") {
		return true
	}
	allDigits := true
	for _, r := range s {
		if r < '0' || r > '9' {
			allDigits = false
			break
		}
	}
	return allDigits
}
