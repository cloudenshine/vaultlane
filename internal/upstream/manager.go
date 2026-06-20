package upstream

import (
	"context"
	"faka-gateway/internal/config"
	"fmt"
	"sync"
)

// Manager 多上游管理器
// 负责：
//  1. 根据 config.Upstreams 构造所有 Adapter
//  2. 并行拉取所有上游的商品
//  3. 路由：按 source 字段找到对应 Adapter
type Manager struct {
	cfg      *config.Config
	adapters map[string]Adapter // key = name
	mu       sync.RWMutex
}

// NewManager 构造
func NewManager(cfg *config.Config) *Manager {
	m := &Manager{
		cfg:      cfg,
		adapters: map[string]Adapter{},
	}
	for _, entry := range cfg.Upstreams {
		if !entry.Enabled {
			continue
		}
		adp := BuildAdapter(entry)
		m.adapters[entry.Name] = adp
	}
	return m
}

// Adapters 列出所有 Adapter
func (m *Manager) Adapters() map[string]Adapter {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]Adapter, len(m.adapters))
	for k, v := range m.adapters {
		out[k] = v
	}
	return out
}

// Adapter 按 name 取
func (m *Manager) Adapter(name string) (Adapter, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.adapters[name]
	return a, ok
}

// AllMetrics 所有 Adapter 的指标
func (m *Manager) AllMetrics() map[string]Metric {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]Metric, len(m.adapters))
	for n, a := range m.adapters {
		out[n] = a.Metrics()
	}
	return out
}

// SyncAllCommodities 并行同步所有上游的商品到本地池（由 store 完成落库）
// 返回每条上游同步结果：{上游名, 新增数, 更新数, 错误}
type SyncReport struct {
	Upstream string `json:"upstream"`
	Added    int    `json:"added"`
	Updated  int    `json:"updated"`
	Error    string `json:"error"`
}

// UpstreamCommodity 上游商品（适配器无关的中间格式）
type UpstreamCommodity struct {
	Source       string
	OuterID      string
	Name         string
	Cover        string
	Description  string
	Price        float64
	Stock        int
	CategoryID   int64
	CategoryName string
	Tags         string
	Config       string
	Minimum      int
	Maximum      int
	Extra        string
}

// UpstreamSnapshot 单个上游拉到的所有商品（page 0 = 全量）
type UpstreamSnapshot struct {
	Upstream   string
	Categories []UpstreamCategory
	Commodity  []UpstreamCommodity
}

type UpstreamCategory struct {
	ID   int64
	Name string
	Icon string
}

// FetchAllSnapshots 并行拉所有上游的"全量商品快照"
// 1) 拉分类
// 2) 拉商品（多页）
// 失败的上游不影响其他
func (m *Manager) FetchAllSnapshots(ctx context.Context, perUpstreamPages int) []UpstreamSnapshot {
	m.mu.RLock()
	adapters := make([]Adapter, 0, len(m.adapters))
	for _, a := range m.adapters {
		adapters = append(adapters, a)
	}
	m.mu.RUnlock()

	var wg sync.WaitGroup
	out := make([]UpstreamSnapshot, len(adapters))
	for i, a := range adapters {
		wg.Add(1)
		go func(i int, a Adapter) {
			defer wg.Done()
			snap, err := fetchOneSnapshot(ctx, a, perUpstreamPages)
			if err != nil {
				snap.Upstream = a.Name()
			}
			out[i] = snap
		}(i, a)
	}
	wg.Wait()
	return out
}

// FetchOneSnapshot 拉单个上游的快照
func FetchOneSnapshot(ctx context.Context, a Adapter, pages int) (UpstreamSnapshot, error) {
	return fetchOneSnapshot(ctx, a, pages)
}

func fetchOneSnapshot(ctx context.Context, a Adapter, pages int) (UpstreamSnapshot, error) {
	snap := UpstreamSnapshot{Upstream: a.Name()}
	// 1) 分类
	cats, err := a.FetchCategories(ctx)
	if err != nil {
		return snap, fmt.Errorf("categories: %w", err)
	}
	catByID := map[int]string{}
	for _, c := range cats {
		catByID[c.ID] = c.Name
		snap.Categories = append(snap.Categories, UpstreamCategory{
			ID:   int64(c.ID),
			Name: c.Name,
			Icon: c.Icon,
		})
	}
	// 2) 商品（分页）
	if pages <= 0 {
		pages = 3
	}
	for page := 1; page <= pages; page++ {
		items, total, err := a.FetchCommodities(ctx, page, 50, 0, "")
		if err != nil {
			return snap, fmt.Errorf("commodities p%d: %w", page, err)
		}
		for _, it := range items {
			cfgJSON := configToJSON(it)
			stock := parseStockInt(it.Stock)
			uc := UpstreamCommodity{
				Source:       "upstream:" + a.Name(),
				OuterID:      strconvI(it.ID),
				Name:         it.Name,
				Cover:        resolveCover(a, it.Cover),
				Price:        pickPrice(it),
				Stock:        stock,
				CategoryID:   int64(it.CategoryID),
				CategoryName: catByID[it.CategoryID],
				Tags:         it.Tags,
				Config:       cfgJSON,
			}
			snap.Commodity = append(snap.Commodity, uc)
		}
		_ = total
		// 如果本页不满，break
		if len(items) < 50 {
			break
		}
	}
	return snap, nil
}

func pickPrice(c Commodity) float64 {
	if c.UserPrice > 0 {
		return c.UserPrice
	}
	return c.Price
}

func parseStockInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case string:
		n, _ := strconvAtoi(x)
		return n
	}
	return 0
}

func resolveCover(a Adapter, raw string) string {
	if raw == "" {
		return ""
	}
	if a.Type() == "upstreama" {
		// upstreama 封面是相对路径，需要补 base
		// 但因为同步时不知道 base，先存原值；展示时再补
		return raw
	}
	return raw
}

func configToJSON(c Commodity) string {
	// 我们没有 c.Config 字段（公开 API 返回的 Comodity 没这字段）
	// 详情才能拿 config。这里留空，详情阶段在展示页补。
	_ = c
	return ""
}

func strconvI(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func strconvAtoi(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("invalid")
		}
		n = n*10 + int(ch-'0')
	}
	return n, nil
}
