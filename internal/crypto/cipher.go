// Package crypto 提供 AES-256-GCM 加密/解密工具，用于保护卡密等敏感数据。
//
// 设计原则：
// - 每次加密随机生成 12 字节 nonce，防止 nonce 复用攻击。
// - 密钥从 session_secret 派生（HKDF-SHA256），无需额外配置。
// - 密文格式：base64(nonce || ciphertext || tag)，明文可安全存入 SQLite。
// - 若加密密钥为空（未配置），则透明透传（不加密），保持向后兼容。
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"

	"golang.org/x/crypto/hkdf"
)

const encPrefix = "ENC:"

// Cipher AES-256-GCM 加解密器
type Cipher struct {
	key []byte // 32 字节派生密钥
}

// NewFromSecret 从任意长度 secret 字节通过 HKDF-SHA256 派生 32 字节 AES 密钥
func NewFromSecret(secret string) *Cipher {
	if secret == "" {
		return &Cipher{key: nil}
	}
	r := hkdf.New(sha256.New, []byte(secret), []byte("faka-gateway-card-secret-v1"), nil)
	key := make([]byte, 32)
	if _, err := io.ReadFull(r, key); err != nil {
		return &Cipher{key: nil}
	}
	return &Cipher{key: key}
}

// Encrypt 加密明文，返回 "ENC:<base64>" 格式密文；密钥为空时透传明文
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	if len(c.key) == 0 || plaintext == "" {
		return plaintext, nil
	}
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return encPrefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt 解密 "ENC:<base64>" 格式密文，返回明文；若输入不含前缀则视作明文直接返回（兼容旧数据）
func (c *Cipher) Decrypt(ciphertext string) (string, error) {
	if len(c.key) == 0 {
		return ciphertext, nil
	}
	if len(ciphertext) < len(encPrefix) || ciphertext[:len(encPrefix)] != encPrefix {
		// 旧明文数据，直接返回（向后兼容）
		return ciphertext, nil
	}
	data, err := base64.StdEncoding.DecodeString(ciphertext[len(encPrefix):])
	if err != nil {
		return "", errors.New("crypto: invalid base64")
	}
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", errors.New("crypto: ciphertext too short")
	}
	nonce, cipherBytes := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plainBytes, err := gcm.Open(nil, nonce, cipherBytes, nil)
	if err != nil {
		return "", errors.New("crypto: decrypt failed (tampered or wrong key)")
	}
	return string(plainBytes), nil
}

// IsEncrypted 判断是否是加密格式
func IsEncrypted(s string) bool {
	return len(s) >= len(encPrefix) && s[:len(encPrefix)] == encPrefix
}
