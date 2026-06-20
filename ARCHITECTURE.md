# 架构说明 · ARCHITECTURE

> 写给想改代码的人。讲清 v4 的设计、各模块关系、扩展点。

## 一、设计目标

| 目标 | 做法 |
|------|------|
| **多上游** | Adapter 抽象接口，每个上游一个实现（`upstreama.go` / `mock_adapter.go`），未来新增自定义上游时不动其他代码 |
| **商品池统一** | 前台用户只查本地 `commodities` 表，不直连上游（防止上游挂掉前台挂） |
| **灵活定价** | `cost_price`（上游成本） vs `sale_price`（你定的售价）两个字段独立 |
| **可选模块** | `config.Modules` 8 个 bool 开关，编译时路由就按开关挂载 |
| **小而清晰** | 包内文件 < 500 行，单一职责 |

## 二、整体架构图

```
┌─────────────────────────────────────────────────────────────────┐
│                          main.go                                │
│  1. 加载 config.yaml                                            │
│  2. 构造 upstream.Manager（根据 upstreams 数组）                  │
│  3. 构造 store.Store（SQLite 迁移）                              │
│  4. 构造 admin.Handlers（含 PoolHandlers）                       │
│  5. api.Register(r, cfg, defaultUp, mgr, st, logger)            │
│  6. admin.Register(r, adminH)                                    │
│  7. web.Register(r)  ← embed.FS                                 │
│  8. http.Server.ListenAndServe                                  │
└─────────────────────────────────────────────────────────────────┘
                                  │
        ┌─────────────────────────┼─────────────────────────┐
        ▼                         ▼                         ▼
  ┌──────────┐              ┌──────────┐              ┌──────────┐
  │  /api/*  │              │ /admin/* │              │ /* (SPA) │
  │  前台    │              │  后台    │              │  embed   │
  └────┬─────┘              └────┬─────┘              └──────────┘
       │                         │
       ▼                         ▼
  api.Deps                  admin.Handlers
  {Cfg, Up, Manager,        {Cfg, Store, Downstream, Logger,
   Store, Premium, ...}      Upstream, Pool, ...}
       │                         │
       ├─→ store                 ├─→ store
       ├─→ upstream.Manager      ├─→ admin.PoolHandlers
       └─→ payment.Manager      └─→ upstream.Manager
                                   └─→ downstream-b.Client
```

## 三、关键数据结构

### 1. Commodity（商品池主表）

```go
type Commodity struct {
    ID            int64
    CategoryID    int64
    Name          string
    Cover         string
    Description   string
    Price         float64  // 兼容旧字段 = SalePrice
    SalePrice     float64  // ★ 你定的售价
    CostPrice     float64  // ★ 上游成本
    Stock         int
    Sold          int
    StockWarning  int
    DeliveryWay   int      // 0=手动 1=自营卡密 2=上游
    Status        int      // 0=下架 1=上架
    Sort          int
    SharedCode    string
    Source        string   // self / upstream:upstreama / upstream:mock
    OuterID       string   // 上游原始 ID
    Config        string   // JSON
    Tags          string
    Minimum       int
    Maximum       int
    UpstreamExtra string
    LastSeenAt    *time.Time
    CreatedAt     time.Time
    UpdatedAt     time.Time
}
```

**关键**：`Source` 字段决定**这张记录从哪来**。`OuterID` 是上游原始 ID（用于更新识别）。

### 2. Config.Upstreams

```go
type UpstreamEntry struct {
    Name      string  // 唯一标识
    Type      string  // 适配器类型：upstreama / mock
    BaseURL   string
    AppID     string
    AppKey    string
    Timeout   int
    RetryMax  int
    UserAgent string
    Enabled   bool
    Prefix    string  // mock 专用
    Count     int     // mock 专用
}
```

### 3. upstream.Adapter 接口

```go
type Adapter interface {
    Name() string
    Type() string
    Enabled() bool
    FetchCategories(ctx) ([]Category, error)
    FetchCommodities(ctx, page, limit, categoryID int, keyword string) ([]Commodity, int, error)
    FetchCommodityDetail(ctx, id int) (*CommodityDetail, error)
    SubmitOrder(ctx, sharedCode string, num int, contact string) (*TradeResult, error)
    QueryOrder(ctx, tradeNo string) (*TradeResult, error)
    Metrics() Metric
    SetMock(enabled bool, prefix string, delayMs int)
    MockStatus() map[string]any
}
```

**加新上游只需 3 步**：
1. 写 `internal/upstream/custom.go` 实现 `Adapter` 接口
2. 在 `adapter.go` 的 `BuildAdapter` 加 switch case
3. `config.yaml` 加 `upstreams: - type: custom ...`

### 4. Config.Modules

```go
type ModulesConfig struct {
    Coupon       bool  // 优惠券系统（v5）
    Referral     bool  // 邀请返佣（v5）
    Email        bool  // 邮件通知
    UpstreamSync bool  // 上游商品自动同步（定时）
    DownstreamCallback  bool  // downstream-b回调
    FinanceStats bool  // 财务统计
    ConfigCenter bool  // 配置中心
    Integrations bool  // 上下游对接面板
}
```

**怎么生效**：
- Go 端：`api.Register` / `admin.Register` 根据 `Cfg.Modules.X` 决定挂不挂载路由
- 前端：`window.MODULES`（由后端注入）决定 UI 显隐
- 加新模块：在 struct 加字段，路由处加 if，UI 加 disabled 判断

## 四、数据流

### 用户前台浏览

```
浏览器 GET /api/commodities
  → handleCommodities (api/commodities.go)
    → store.ListCommodities (status=1)
    → toDisplayPool (domain.DisplayCommodity)
    → JSON 返回
```

**关键**：**不调上游**。所有数据来自本地 SQLite。前台稳定不挂。

### 同步上游

```
后台点"⬇ 同步上游"
  → POST /admin/api/pool/sync
    → admin.PoolHandlers.HandlePoolSync
      → upstream.FetchOneSnapshot(adapter, 3 pages)
        → adapter.FetchCategories()
        → adapter.FetchCommodities() × 3 页
      → store.UpsertFromUpstream(每件商品)
        → 存在：UPDATE（保留 sale_price）
        → 不存在：INSERT（默认 sale_price = cost × 1.1）
```

### 批量改价

```
后台勾选 + 点"应用"
  → POST /admin/api/pool/price {ids, type, op, amount}
    → admin.PoolHandlers.HandlePoolBatchPrice
      → store.BatchAdjustPrice
        → 逐件：sale_price ± (固定 | 百分比)
        → if newSale < cost_price: newSale = cost_price  // 防亏本
        → UPDATE
```

### 用户下单

```
浏览器 POST /api/orders {commodity_id, contact, num}
  → api/trade.go handleOrder
    → store.GetCommodityByID(commodity_id)
    → if com.Source == "self": handleSelfOrder (本地卡密)
    → else: upstream.Trade (调上游 SubmitOrder)
      → adapter.SubmitOrder
      → 落库到 orders 表
```

## 五、并发模型

- `upstream.Manager` 用 `sync.RWMutex` 保护 adapters map
- 同步上游时多 adapter 并发（每 adapter 独立 goroutine + `sync.WaitGroup`）
- HTTP 请求每 adapter 独立 `*http.Client`，可独立超时
- SQLite 单写（`db.SetMaxOpenConns(1)`）

## 六、关键设计决策

### 1. 为什么商品池用 `commodities` 表而不是新表 `commodity_pool`？

**答**：避免数据迁移噩梦。`commodities` 表是 v2.0 创建的，加 9 个字段就够。改名 = 改一堆代码 + 改 SQL = 错。

### 2. 为什么 `sale_price` 和 `price` 两个字段？

**答**：`price` 是历史遗留字段（v2.0 自营商品用），保持兼容。`sale_price` 是 v4 引入的实际售价。同步时 `price = sale_price`。

### 3. 为什么 mock 适配器是内置而不是放外部 sample？

**答**：用户"无上游也能跑"是基本体验。`type: mock` 适配器内置，零配置出 12 件演示商品。

### 4. 为什么同步时保留旧 `sale_price`？

**答**：管理员已经设过价，再同步上游不能让上游的成本变化影响售价。`UpsertFromUpstream` 只更新 `cost_price/stock/description/cover/config/tags/last_seen_at`，**不动** `sale_price` 和 `status`。

### 5. 为什么 8080 端口可能冲突？

**答**：开发机常被其他服务占。`LISTEN_ADDR=127.0.0.1:9090 ./faka-gateway.exe` 即可。

## 七、扩展点

### 加新上游适配器

```go
// internal/upstream/custom.go
package upstream

type CustomAdapter struct {
    name string
    cfg  config.UpstreamConfig
    // ...
}

func NewCustomAdapter(name string, cfg config.UpstreamConfig) *CustomAdapter { ... }
func (m *CustomAdapter) Name() string { return m.name }
func (m *CustomAdapter) Type() string { return "custom" }
// ... 实现 Adapter 接口其他方法
```

```go
// internal/upstream/adapter.go 的 BuildAdapter 加 case
case "custom":
    return NewCustomAdapter(e.Name, entryCfg)
```

```yaml
# config.yaml
upstreams:
  - name: custom
    type: custom
    base_url: "https://provider-api.example.com"
    api_key: "..."
    enabled: true
```

### 加新支付方式

```go
// internal/payment/custom.go
type CustomEngine struct { ... }
func (m *CustomEngine) Name() string { return "custom" }
func (m *CustomEngine) Pay(...) { ... }
```

```go
// main.go 注册
d.Pay.Register(payment.NewCustomEngine(...))
```

### 加新可选模块

```go
// internal/config/config.go
type ModulesConfig struct {
    CustomModule bool `yaml:"custom_module"`
}
```

```go
// main.go 注册路由
if cfg.Modules.CustomModule {
    api.GET("/api/custom", h.CustomHandler)
}
```

```yaml
# config.yaml
modules:
  custom_module: true
```

## 八、测试

- 单元测试：`go test ./...`（覆盖 store / payment / upstream）
- 集成测试：`go test -tags integration ./internal/upstream/downstream-b/...`
- 实地验证：见 README 的"实地验证清单"

## 九、已知限制

1. **没做定时同步**：需要手动点"⬇ 同步上游"。`UpstreamSync` 模块开关已留位，未实现
2. **没做库存预警通知**：`stock_warning` 字段已存，UI 未展示
3. **没做自动下架超期商品**：`MarkUpstreamMissing` 已实现，未挂定时任务
4. **downstream-b 30+ 接口已写但未与商品池打通**：需 v5 改造
5. **前台用户中心简陋**：v5 重做

## 十、文件变更记录（v4.0）

```
新增：
  internal/upstream/adapter.go          Adapter 接口 + BuildAdapter
  internal/upstream/upstreama.go       独立 upstreama 适配器
  internal/upstream/mock_adapter.go     mock 适配器
  internal/upstream/manager.go          多上游管理器
  internal/upstream/httpdoer.go         HTTP + 指标
  internal/admin/pool.go                商品池 REST API

改动：
  internal/store/commodity.go           +9 字段, +4 函数
  internal/store/sqlite.go              v4 迁移
  internal/config/config.go             Upstreams 数组
  internal/api/commodities.go           前台改用商品池
  internal/api/trade.go                 下单用商品池 ID
  internal/api/payment_api.go           同上
  internal/api/router.go                Deps 加 Manager
  internal/admin/handlers.go            注册 pool 路由
  internal/admin/auth.go                nav 加"商品池"
  internal/admin/static/app.js          render_pool + 7 个批量操作
  internal/admin/static/style.css       pool 表格样式
  main.go                               构造 Manager + PoolHandlers
  internal/delivery/delivery.go         UpstreamEngine 用 Adapter
  internal/api/api_integration_test.go  适配新签名
  config/config.yaml                    upstreams 段
```
