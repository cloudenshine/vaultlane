// Package admin 后台管理（独立路由 + cookie session + 不暴露给用户端）
package admin

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Session 会话（自实现，不依赖第三方 session 包）
// 结构：base64(payloadJSON).base64(hmac_sha256(payloadJSON, secret))
type Session struct {
	User      string    `json:"user"`
	IssuedAt  time.Time `json:"iat"`
	ExpiresAt time.Time `json:"exp"`
}

// cookieName cookie 名
const cookieName = "faka_admin_session"

// EncodeSession 编码
func EncodeSession(s *Session, secret string) (string, error) {
	if s.User == "" {
		return "", errors.New("empty user")
	}
	payload, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payloadB64))
	sigB64 := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payloadB64 + "." + sigB64, nil
}

// DecodeSession 解码 + 验签
func DecodeSession(token, secret string) (*Session, error) {
	if token == "" {
		return nil, errors.New("empty token")
	}
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return nil, errors.New("malformed session")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0]))
	wantSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(wantSig), []byte(parts[1])) {
		return nil, errors.New("bad signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(payload, &s); err != nil {
		return nil, err
	}
	if time.Now().After(s.ExpiresAt) {
		return nil, errors.New("session expired")
	}
	return &s, nil
}

// NewSessionID 生成新会话
func NewSessionID() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
