package payment

import (
	"context"
	"fmt"
	"sync"
)

// Manager 渠道路由
type Manager struct {
	mu      sync.RWMutex
	engines map[string]Engine
}

// NewManager 构造
func NewManager() *Manager {
	return &Manager{engines: map[string]Engine{}}
}

// Register 注册渠道
func (m *Manager) Register(e Engine) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.engines[e.Method()] = e
}

// Get 拿渠道。支持 epay:wxpay / wxpay 等别名，统一路由到已注册引擎。
func (m *Manager) Get(method string) (Engine, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if e, ok := m.engines[method]; ok {
		return e, nil
	}
	if e, ok := m.engines[canonicalPayMethod(method)]; ok {
		return e, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrUnsupported, method)
}

// Methods 列出已注册渠道（易支付展开 alipay/wxpay/qqpay）
func (m *Manager) Methods() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.engines)+3)
	for k := range m.engines {
		out = append(out, k)
		if k == "epay" {
			out = append(out, "alipay", "wxpay", "qqpay")
		}
	}
	return out
}

func canonicalPayMethod(method string) string {
	switch method {
	case "alipay", "wxpay", "qqpay", "bank":
		return "epay"
	default:
		if len(method) > 5 && method[:5] == "epay:" {
			return "epay"
		}
		return method
	}
}

// EngineExists 是否启用某渠道
func (m *Manager) EngineExists(method string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.engines[method]
	return ok
}

// _ context 包占位
var _ = context.Background
