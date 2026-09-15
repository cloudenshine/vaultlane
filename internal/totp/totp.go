// Package totp 实现 RFC 6238 基于时间的一次性密码（TOTP）。
// 仅依赖标准库与已有的 golang.org/x/crypto，无需额外引入第三方包。
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"
)

const digits = 6
const period = 30 // 秒

// GenerateSecret 生成随机 Base32 TOTP 密钥（160 bit）
func GenerateSecret() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw), nil
}

// TOTP 对指定密钥和当前时间生成 6 位验证码
func TOTP(secret string, t time.Time) (string, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(
		strings.ToUpper(strings.ReplaceAll(secret, " ", "")),
	)
	if err != nil {
		return "", fmt.Errorf("totp: invalid secret: %w", err)
	}
	counter := uint64(t.Unix()) / period
	return hotp(key, counter), nil
}

// Verify 验证码在 ±1 步容忍窗口内是否有效
func Verify(secret, code string, t time.Time) bool {
	for offset := int64(-1); offset <= 1; offset++ {
		adjusted := t.Add(time.Duration(offset*period) * time.Second)
		expected, err := TOTP(secret, adjusted)
		if err != nil {
			return false
		}
		if hmac.Equal([]byte(expected), []byte(code)) {
			return true
		}
	}
	return false
}

// OTPAuthURL 生成可扫描的 otpauth:// URI（供生成二维码）
func OTPAuthURL(secret, issuer, account string) string {
	return fmt.Sprintf(
		"otpauth://totp/%s:%s?secret=%s&issuer=%s&algorithm=SHA1&digits=6&period=30",
		issuer, account, secret, issuer,
	)
}

func hotp(key []byte, counter uint64) string {
	msg := make([]byte, 8)
	binary.BigEndian.PutUint64(msg, counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(msg)
	h := mac.Sum(nil)
	offset := h[len(h)-1] & 0x0f
	code := binary.BigEndian.Uint32(h[offset:offset+4]) & 0x7fffffff
	otp := int(code) % int(math.Pow10(digits))
	return fmt.Sprintf("%06d", otp)
}
