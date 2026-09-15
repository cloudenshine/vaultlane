# Changelog

All notable user-facing changes are listed here. Format inspired by [Keep a Changelog](https://keepachangelog.com/).

---

## v5.0.0 · 2026-04-10 · Vaultlane

Product rename: **Vaultlane** (码仓). The Go module path stays `faka-gateway`.

### Added

- Coupon engine (`/api/coupon/verify`, admin coupon CRUD)
- Referral invite codes and 5% commission on paid orders
- SMTP card-delivery mailer
- Timed upstream sync + cost circuit breaker (`modules.upstream_sync`)
- AES-256-GCM encryption for card secrets
- Admin CSRF (`X-CSRF-Token`) and optional TOTP 2FA
- In-process delivery queue with backoff
- Epay channel aliases (`alipay` / `wxpay` / `qqpay`)
- GitHub community files: LICENSE, SECURITY, CONTRIBUTING, Code of Conduct, CI

### Fixed

- Multi-upstream fulfillment now routes by `order.Source`
- Orders prefer the local catalog instead of punching through to the default adapter
- Payment callbacks are idempotent (`MarkPaymentPaid` must succeed before delivery)
- `FinishPaidOrderByID` uses `GetOrderByID` instead of scanning 200 rows
- Category sync upserts instead of duplicating rows
- Dashboard stats use SQL aggregates
- Balance checkout creates the order before deducting and refunds on delivery failure

### Security

- User session cookies honor `admin.cookie_secure`
- Card contents are encrypted at rest when `session_secret` is set

---

## v4.0 · 2026-06-17 · Catalog pool + multi-upstream + hardening

### 用户能感知到的变化

1. **后台多了"商品池"标签页**（不是原"商品"）
   - 旧"商品" → 改名"自营"（仅自营商品）
   - 新"商品池" → 管理所有商品（自营 + 上游）
2. **前台价格不再依赖上游**——管理员改价后立即生效
3. **多上游支持**——`config.yaml` 加项即可，无代码改动
4. **可选模块开关**——8 个模块按需启用
5. **财务统计独立 tab**——今日/本月/30 天曲线/Top 10/支付记录
6. **首次启动自动建 mock 上游**——无真上游也能跑（12 件演示商品）
7. **安全加固**：4 个 P0 漏洞修复（详见 `SECURITY_AUDIT.md`）

### 关键文件

| 文件 | 变化 |
|------|------|
| `internal/upstream/adapter.go` | 新增 Adapter 接口 |
| `internal/upstream/upstreama.go` | 新增 upstreama 适配器 |
| `internal/upstream/mock_adapter.go` | 新增 mock 适配器 |
| `internal/upstream/manager.go` | 新增多上游管理器 |
| `internal/admin/pool.go` | 新增商品池 REST API |
| `internal/store/commodity.go` | 扩 9 字段，+4 函数 |
| `internal/store/sqlite.go` | v4 迁移 |
| `internal/config/config.go` | Upstreams 数组 |
| `internal/api/commodities.go` | 前台改用商品池 |
| `main.go` | 构造 Manager + PoolHandlers |
| `config/config.yaml` | upstreams 段 |
| `internal/admin/static/app.js` | render_pool + 7 个批量操作 |
| `internal/api/payment_api.go` | VULN-001/002/003/004 安全修复 + VULN-020 user_id=0 订单 |
| `internal/api/user.go` | RequireUserStrict 中间件（前置检查模式）|
| `internal/store/sqlite.go` | GetOrderByID（VULN-004 修复用）|
| `internal/api/api_integration_test.go` | 加 VULN-001 反向测试 |
| `README.md` | **完全重写** |
| `ARCHITECTURE.md` | **新增** |
| `CHANGELOG.md` | **新增**（本文件） |
| `SECURITY_AUDIT.md` | **新增** |

### 数据库迁移

启动时自动应用 v4 迁移（`ALTER TABLE commodities ADD COLUMN ...`）。SQLite 缺 try-add-column 能力，所以用"试执行，duplicate column 错误忽略"。

新增字段：
- `sale_price REAL NOT NULL DEFAULT 0`（售价）
- `stock_warning INTEGER NOT NULL DEFAULT 5`（库存预警）
- `last_seen_at DATETIME`（上游同步见到时间）
- `upstream_extra TEXT NOT NULL DEFAULT ''`（上游原始 JSON）
- `config TEXT NOT NULL DEFAULT ''`（规格 JSON）
- `tags TEXT NOT NULL DEFAULT ''`（逗号分隔）
- `minimum INTEGER NOT NULL DEFAULT 1`（起售）
- `maximum INTEGER NOT NULL DEFAULT 1`（限购）

首次升级自动 `UPDATE commodities SET sale_price=price WHERE sale_price=0` 把旧数据迁移好。

### 兼容性

| 情况 | 行为 |
|------|------|
| 旧 `upstream: { base_url, app_id, app_key }` 段 | 自动转成 `upstreams[0]` |
| 旧 `commodities` 表的现有数据 | 自动加新字段，price 当 sale_price |
| 旧 `d.Up.CommodityDetail` 之类的旧方法 | 改为 `d.Up.FetchCommodityDetail` |
| 旧 `d.Up.Trade` | 改为 `upstream.Trade(d.Up, ctx, params)` |
| 测试 `Register(r, cfg, up, st, log)` | 改为 `Register(r, cfg, up, mgr, st, log)` |

### 配置示例（v4）

```yaml
upstreams:                    # 新字段（可选）
  - name: upstreama
    type: upstreama
    base_url: "https://upstream-api.example.com"
    app_id: "YOUR_APP_ID"
    app_key: "YOUR_APP_KEY"
    enabled: true
  - name: mock               # 自动添加（无上游时演示用）
    type: mock
    enabled: true
    prefix: "MOCK-"
    count: 12

modules:                      # 新字段
  finance_stats: true
  config_center: true
  integrations: true
  # 其它默认 false
```

### API 变化

| 旧 | 新 |
|----|----|
| `GET /api/commodities` 直连上游 | `GET /api/commodities` 查本地商品池 |
| `GET /admin/api/commodities` 仅自营 | `GET /admin/api/commodities` 仍仅自营<br>**新** `GET /admin/api/pool` 全部商品 |
| （无） | `POST /admin/api/pool/sync` |
| （无） | `POST /admin/api/pool/status` |
| （无） | `POST /admin/api/pool/price` |
| （无） | `GET /admin/api/pool/upstreams` |

### 已知问题

1. **窗口 hash change 路由偶尔跳**：SPA 路由未用 pushState，长 hash URL 直接跳
2. **cookie 在 Chrome 重启后丢失**：默认 secure=false
3. **mock 同步有 1.5 秒延迟**：mock 适配器设了 `delay_ms: 800`
4. **没有定时同步**：必须手动点
5. **没有图片懒加载**：upstreama 远程图片可能慢

---

## v3.0 · 2026-06-16

- 用户注册/登录 + 余额
- 支付引擎抽象（balance / epay / usdt）
- 财务统计接口
- 下游网关对接包

## v2.0 · 2026-06-15

- 自营商品 + 卡密体系
- 后台独立路径 `/admin/*`
- cookie session + bcrypt
- 下游网关开放平台对接包（30+ 接口）

## v1.0 · 2026-06-14

- 上游发卡 v3.4.9 签名算法（字节级一致）
- 公共浏览 API 对接（无需登录）
- 极简白瓷风 UI
- 单二进制 ~13MB

---

## 约定

- **每次发布必更新 CHANGELOG.md**（用户能感知的）
- **不删旧 API**（保持兼容），新功能用新端点
- **配置变更在 README + config.yaml 注释里同步**
- **数据库迁移写 `migrateVn.go`**，自动应用
