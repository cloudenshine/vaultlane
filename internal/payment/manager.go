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

// Get 拿渠道
func (m *Manager) Get(method string) (Engine, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	e, ok := m.engines[method]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnsupported, method)
	}
	return e, nil
}

// Methods 列出已注册渠道
func (m *Manager) Methods() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.engines))
	for k := range m.engines {
		out = append(out, k)
	}
	return out
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
