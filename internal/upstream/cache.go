package upstream

import (
	"sync"
	"time"
)

// CacheItem 缓存项
type CacheItem struct {
	Value     any
	ExpiresAt time.Time
}

// TTLCache 简单内存 TTL 缓存（线程安全）
type TTLCache struct {
	mu         sync.RWMutex
	items      map[string]CacheItem
	maxEntries int
	hits       int64
	misses     int64
	evicted    int64
}

// NewTTLCache 创建缓存
func NewTTLCache(maxEntries int) *TTLCache {
	if maxEntries <= 0 {
		maxEntries = 256
	}
	return &TTLCache{
		items:      make(map[string]CacheItem),
		maxEntries: maxEntries,
	}
}

// Set 设置值（带 TTL）
func (c *TTLCache) Set(key string, value any, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 超过容量：粗略驱逐最早的
	if len(c.items) >= c.maxEntries {
		var oldestKey string
		var oldestTime time.Time
		first := true
		for k, v := range c.items {
			if first || v.ExpiresAt.Before(oldestTime) {
				oldestKey = k
				oldestTime = v.ExpiresAt
				first = false
			}
		}
		if oldestKey != "" {
			delete(c.items, oldestKey)
			c.evicted++
		}
	}

	c.items[key] = CacheItem{
		Value:     value,
		ExpiresAt: time.Now().Add(ttl),
	}
}

// Get 获取值（未命中/过期返回 nil）
func (c *TTLCache) Get(key string) (any, bool) {
	c.mu.RLock()
	item, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		c.misses++
		return nil, false
	}
	if time.Now().After(item.ExpiresAt) {
		c.mu.Lock()
		delete(c.items, key)
		c.mu.Unlock()
		c.misses++
		return nil, false
	}
	c.hits++
	return item.Value, true
}

// Clear 清空
func (c *TTLCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]CacheItem)
}

// Stats 缓存统计
type CacheStats struct {
	Entries    int
	MaxEntries int
	Hits       int64
	Misses     int64
	HitRate    float64
	Evicted    int64
}

// Stats 缓存统计
func (c *TTLCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	total := c.hits + c.misses
	var rate float64
	if total > 0 {
		rate = float64(c.hits) / float64(total)
	}
	return CacheStats{
		Entries:    len(c.items),
		MaxEntries: c.maxEntries,
		Hits:       c.hits,
		Misses:     c.misses,
		HitRate:    rate,
		Evicted:    c.evicted,
	}
}
