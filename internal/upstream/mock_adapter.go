package upstream

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"faka-gateway/internal/config"
)

// MockAdapter 内置 mock 上游：用于演示/无真实上游时填充商品池
// 始终返回固定的几件商品，价格区间 9.9 ~ 19.9
type MockAdapter struct {
	name   string
	prefix string
	count  int
	cfg    config.UpstreamConfig

	metric *metricCounter
	seq    atomic.Int64
}

func NewMockAdapter(name, prefix string, count int, cfg config.UpstreamConfig) *MockAdapter {
	if name == "" {
		name = "mock"
	}
	if prefix == "" {
		prefix = "MOCK-"
	}
	if count <= 0 {
		count = 12
	}
	return &MockAdapter{
		name:   name,
		prefix: prefix,
		count:  count,
		cfg:    cfg,
		metric: newMetricCounter("mock://" + name),
	}
}

func (m *MockAdapter) Name() string    { return m.name }
func (m *MockAdapter) Type() string    { return "mock" }
func (m *MockAdapter) Enabled() bool   { return true }
func (m *MockAdapter) Metrics() Metric { return m.metric.snapshot() }
func (m *MockAdapter) SetMock(enabled bool, prefix string, delayMs int) {
	// mock 适配器本身就在 mock 模式
}
func (m *MockAdapter) MockStatus() map[string]any {
	return map[string]any{"enabled": true, "prefix": m.prefix}
}

func (m *MockAdapter) FetchCategories(ctx context.Context) ([]Category, error) {
	m.metric.recordCall(10*time.Millisecond, true, "", "")
	return []Category{
		{ID: 1, Name: "演示·示例A 系列", Icon: "", Sort: 0, Status: 1, CommodityCount: 4},
		{ID: 2, Name: "演示·示例B 系列", Icon: "", Sort: 1, Status: 1, CommodityCount: 3},
		{ID: 3, Name: "演示·示例E 系列", Icon: "", Sort: 2, Status: 1, CommodityCount: 2},
	}, nil
}

func (m *MockAdapter) FetchCommodities(ctx context.Context, page, limit, categoryID int, keyword string) ([]Commodity, int, error) {
	m.metric.recordCall(15*time.Millisecond, true, "", "")
	if page < 1 {
		page = 1
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	startID := (page-1)*limit + 1
	items := []Commodity{}
	for i := 0; i < limit && startID+i <= m.count; i++ {
		id := startID + i
		catID := ((id - 1) % 3) + 1
		if categoryID > 0 && catID != categoryID {
			continue
		}
		name := fmt.Sprintf("【mock】演示商品#%d", id)
		if keyword != "" {
			name = fmt.Sprintf("【mock】%s #%d", keyword, id)
		}
		items = append(items, Commodity{
			ID:          id,
			Name:        name,
			Cover:       "",
			Tags:        "mock,演示",
			TagsList:    []string{"mock", "演示"},
			Price:       9.9 + float64(id%5),
			UserPrice:   8.8 + float64(id%5),
			Stock:       "999",
			StockState:  3,
			DeliveryWay: 1,
			CategoryID:  catID,
			OrderSold:   id * 7,
		})
	}
	return items, m.count, nil
}

func (m *MockAdapter) FetchCommodityDetail(ctx context.Context, id int) (*CommodityDetail, error) {
	m.metric.recordCall(8*time.Millisecond, true, "", "")
	return &CommodityDetail{
		Commodity: Commodity{
			ID:          id,
			Name:        fmt.Sprintf("【mock】演示商品 #%d", id),
			Cover:       "",
			Tags:        "mock,演示",
			Price:       9.9 + float64(id%5),
			UserPrice:   8.8 + float64(id%5),
			Stock:       "999",
			StockState:  3,
			DeliveryWay: 1,
			CategoryID:  ((id - 1) % 3) + 1,
		},
		Description:    "<h2>Mock 演示商品</h2><p>这是 mock 上游返回的演示商品，用于演示「商品池」全链路。</p><ul><li>即时发货</li><li>支持多规格</li><li>多账号可用</li></ul>",
		Minimum:        1,
		Maximum:        10,
		ContactType:    0,
		PasswordStatus: 0,
		Config:         map[string]any{"category": map[string]float64{"日卡": 1.0, "周卡": 5.0, "月卡": 18.0}},
	}, nil
}

func (m *MockAdapter) SubmitOrder(ctx context.Context, sharedCode string, num int, contact string) (*TradeResult, error) {
	m.metric.recordCall(5*time.Millisecond, true, "", "")
	lines := make([]string, 0, num)
	for i := 1; i <= num; i++ {
		lines = append(lines, fmt.Sprintf("%s%s-%04d", m.prefix, sharedCode, i))
	}
	seq := m.seq.Add(1)
	return &TradeResult{
		Code:     200,
		Msg:      "success (mock upstream)",
		TradeNo:  fmt.Sprintf("MOCK%d%04d", time.Now().Unix()%1000000, seq),
		Contents: strings.Join(lines, "\n"),
	}, nil
}

func (m *MockAdapter) QueryOrder(ctx context.Context, tradeNo string) (*TradeResult, error) {
	m.metric.recordCall(5*time.Millisecond, true, "", "")
	return &TradeResult{
		Code:     200,
		Msg:      "ok",
		TradeNo:  tradeNo,
		Contents: m.prefix + "QUERY-001",
	}, nil
}
