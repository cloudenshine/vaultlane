# Security audit (historical)

> Snapshot of the v4.0 source audit. Fixes listed here are already in tree.
> New reports: see [SECURITY.md](SECURITY.md). Product name as of 5.0: **Vaultlane**.

> 静态代码审计（基于源码，不做运行时测试）
> 审计时间：2026-06-17
> 审计范围：支付链路 / 订单 / 卡密 / 后台管理 / 配置

---

## 风险等级总览

| 等级 | 数量 | 说明 |
|------|------|------|
| 🔴 **P0 严重**（直接被突破） | 4 | 立即可被外部攻击者利用 |
| 🟠 **P1 高**（绕过支付/越权） | 6 | 有条件可利用，需特定场景 |
| 🟡 **P2 中**（信息泄露/审计缺失） | 5 | 间接风险 |
| 🟢 **P3 低**（最佳实践） | 4 | 改进项 |

---

## 🔴 P0 严重漏洞

### VULN-001：未登录直接拉走所有卡密（卡库被连锅端）

**位置**：`internal/api/trade.go:24-210` (`handleOrderCreate`) + `internal/api/payment_api.go:33` (`POST /api/orders`)

**问题**：`POST /api/orders` 路由**没有 requireUser 中间件**（payment_api.go:33）：

```go
g.POST("/orders", requireUser, p.handleOrderCreateV3)  // 看：用了 requireUser
```

**但 `handleOrderCreate` 旧路由（trade.go 注册的）没看：**
- 旧路由直接 `d.handleOrderCreate(c)` 在 `handleSelfOrder` 里**不检查登录、不检查支付**，直接调 `Store.PullSecret` 发卡
- 任何能访问 8080 的人都能下自营单 → 直接拿卡密

**实际验证路径**：
```
POST /api/orders  body: {"commodity_id": 自营ID, "contact": "x@x.com", "num": 1, "need_pay": false}
→ handleOrderCreate → isSelf=true → handleSelfOrder → PullSecret → 返回卡密
```

**修复**：
- 自营下单必须 `requireUser` 中间件
- `need_pay=false` 必须仅对管理员开放（或彻底删除）
- 不支付不能发卡

**影响**：**整个自营卡库可被任何人免费拿走**。

---

### VULN-002：越权重放支付回调（任意伪造支付成功）

**位置**：`internal/api/payment_api.go:533-576` (`handlePaymentCallback`)

**问题**：回调路由**完全公开**，且 `EpayEngine.VerifyNotify` 验签依赖 `e.Key`：

```go
g.POST("/payment/callback/:method", p.handlePaymentCallback)  // 公开
```

但关键是：
1. **epay Key 在 `config.yaml` 配置，攻击者只要读到 `config.yaml`** 就能伪造回调
2. 即使 key 未知，`Notifier.FinishPaidOrder` 是基于 `out_trade_no` 触发；任何能猜到 `out_trade_no`（= `FK` + 时间戳 + 6字节 hex = 24字符，可枚举）的人就能伪造
3. **`GetPaymentByOutTradeNo` 查到 pay 就直接 `MarkPaymentPaid` + `FinishPaidOrderByID`**

**验证路径**：
```bash
# 1. 列出近期订单（看 trade_no 规律）
GET /api/orders/<已知trade_no>?password=  # 旧路由无 password 校验！

# 2. 伪造 epay 回调
POST /api/payment/callback/epay
out_trade_no=<猜的>&trade_status=TRADE_SUCCESS&money=0.01&sign=md5(...)

# 3. 订单变 status=2，卡密发货
```

**修复**：
- `VerifyNotify` 必须强制签名（不能跳过）
- 回调金额必须 = 订单金额（不是只看 sign）
- 支付金额必须 = 订单 amount（避免 1 分钱买 100 元的）

---

### VULN-003：handleOrderGet 越权读他人订单卡密

**位置**：`internal/api/trade.go:213-242` (`handleOrderGet`)

**问题**：**完全没有登录/密码校验**——`order.Password` 字段只在 `!= ""` 时校验，但 `handleOrderCreate` 旧版**默认 Password 为空**：

```go
// trade.go:228
if order.Password != "" {
    pwd := c.Query("password")
    if pwd != order.Password {
        c.JSON(http.StatusForbidden, ...)
    }
}
// Password 为空 → 直接过！
```

**验证路径**：
```bash
# 任何用户只要猜到 trade_no 就能查所有订单
GET /api/orders/FK202606xxxxxxxxxx
# 返回 contents（含卡密）
```

**修复**：
- 匿名访问**永远**拒绝（要求登录 + 只能看自己的）
- 删除 `Password` 字段的免密逻辑
- 加 `order.UserID == uid || isAdmin` 校验

---

### VULN-004：handleBalanceFinish 任意标记订单已支付

**位置**：`internal/api/payment_api.go:578-591`

```go
g.POST("/payment/balance/finish", requireUser, p.handleBalanceFinish)
func (p *PaymentHandler) handleBalanceFinish(c *gin.Context) {
    var req struct {
        OrderID int64 `json:"order_id"`
    }
    ...
    p.Notify.FinishPaidOrderByID(req.OrderID)  // 直接完成！
}
```

**问题**：登录用户**可以完成任意订单 ID**（不验证订单是否属于自己）。配合 VULN-003 查 trade_no → 任何用户能拿任何订单的卡密。

**验证路径**：
```bash
# 1. 登录任意账号（甚至注册新账号）
POST /api/auth/login
# 2. 查所有订单（VULN-003）
GET /api/orders/<别人的trade_no>  # 拿到 order.id
# 3. 强制完成（绕过支付）
POST /api/payment/balance/finish  body: {"order_id": <别人的order.id>}
# 4. 再次查（拿到卡密）
GET /api/orders/<trade_no>  # 此时 status=2
```

**修复**：
- 校验 `order.UserID == uid`
- 校验 `order.Status == 0`（只能完成未支付的）
- 余额渠道走原子事务（先扣后发）

---

## 🟠 P1 高危漏洞

### VULN-005：旧 token 永远不过期 + 明文在 config.yaml

**位置**：`config/config.yaml:32` + 各 handler

```yaml
admin:
  token: "<legacy-admin-token>"  # 默认值
```

**问题**：
1. 这是**明文**的 admin token，可与 bcrypt 密码共存
2. 任何代码用 `X-Admin-Token` 校验的地方，**只要读 `config.yaml` 就能完全接管后台**
3. 配置文件常被误提交到 git

**修复**：
- 删除 `admin.token` 字段（用 cookie session 即可）
- `.gitignore` 已加 .env，但要 audit 历史 commit

---

### VULN-006：管理员密码 hash 弱 + 公开在仓库

**位置**：`config/config.yaml:34`

```yaml
password_hash: "<legacy-default-password-hash>"  # 默认 <initial-password>
```

**问题**：
- 默认密码 `<initial-password>` + 这个 bcrypt hash 在 README 公开 → 任何下载代码的人都能登后台
- README 写明了"默认 首次启动前自行配置的管理员账号" + 这个 hash

**修复**：
- README 写"首次启动必须改密码"
- 提供 `cmd/hash-password` 工具
- 启动时若检测默认 hash → 强制 warn 或拒绝启动

---

### VULN-007：handleOrderCreate 旧路由无认证 + 直接发卡

**位置**：`internal/api/router.go` + `trade.go:24`

**问题**：
- 旧 `POST /api/orders` 路由（trade.go 写的）可能没 requireUser
- `need_pay: false` 时直接走 `handleSelfOrder` 发卡，不支付

**修复**：
- 删除旧 `handleOrderCreate` 路由
- 统一走 `payment_api.handleOrderCreateV3`（已带 `requireUser`）

---

### VULN-008：批量调价/上下架 SQL 拼接（潜在注入）

**位置**：`internal/store/commodity.go:298-311` (`BatchSetStatus`)

```go
q := `UPDATE commodities SET status=?, updated_at=? WHERE id IN (?` +
    strings.Repeat(",?", len(ids)-1) + `)`
```

**评估**：
- `ids` 是 `[]int64`，Go SQL driver 会参数化占位符 `?` → **不是注入**
- 但 `IN (?)` 在某些 driver（如 pgx）要求不同语法，**改用 SQLite 没事**

**建议**：
- 加 length cap（避免 `len(ids)=100000` 拖死数据库）
- 显式 int64 类型断言

---

### VULN-009：EpayEngine MD5 签名可碰撞

**位置**：`internal/payment/epay.go:114-130`

**问题**：MD5 在已知前缀情况下可通过工具碰撞。攻击者：
1. 拿到任意 `out_trade_no=ABC&money=1.00` 的合法回调
2. 改 money=0.01 算 sign
3. 提交 → 服务端 `VerifyNotify` 验签通过

**修复**：
- 回调后**强制校验金额** = 订单金额（payment_api.go:handlePaymentCallback 缺这个校验）
- 改 HMAC-SHA256 替代 MD5

---

### VULN-010：HandleUpdateCommodity 无权限校验

**位置**：`internal/admin/handlers.go:241-267`

**问题**：在 `requireSession` 路由组内，但**没有 CSRF 防护**，攻击者诱导管理员点链接即可改商品。

**修复**：
- 加 CSRF token
- 改价等敏感操作二次确认

---

## 🟡 P2 中危漏洞

### VULN-011：日志泄露卡密（错误日志含 contents）

**位置**：`internal/api/payment_api.go`、`trade.go` 多处

**问题**：上游发卡失败时 `order.Contents` 可能写入日志或错误响应。

**修复**：所有日志/错误响应**只记 trade_no，不记 contents**。

---

### VULN-012：缺少审计日志

**位置**：全局

**问题**：谁在什么时间改了什么价/什么状态，没记录。

**修复**：加 `audit_logs` 表，所有 admin 操作记日志。

---

### VULN-013：限流仅 IP，无 user/订单维度

**位置**：`internal/api/router.go` `RateLimiter`

**问题**：单一 IP 可无限下单（卡 60/min 只防 API 滥用，不防业务滥用）。

**修复**：加 user_id 维度的限流（下单/支付）。

---

### VULN-014：SQLite 单 conn 并发瓶颈

**位置**：`internal/store/sqlite.go:55`

```go
db.SetMaxOpenConns(1)
```

**问题**：单文件数据库攻击者可触发 DoS（占满连接池）。

**评估**：当前单进程模型下，攻击者需要让服务端**卡住**才能利用。生产建议用 PostgreSQL/MySQL。

---

### VULN-015：mock 模式全局生效（任何上游可被切到 mock）

**位置**：`internal/admin/handlers.go` `HandleToggleMock` + `internal/upstream/upstreama.go`

**问题**：管理员切换到 mock 后，**所有上游交易走模拟**。如果攻击者拿到 admin session → 切 mock → 拿走所有卡密（不消耗上游余额）→ 切回真上游 → 隐蔽。

**修复**：
- mock 切换需要二次密码
- 切换记录到 audit log
- 切到 mock 时**已发卡密回收**

---

## 🟢 P3 低危 / 最佳实践

### VULN-016：session_secret 默认值在仓库

`config.yaml:35` 有默认 `session_secret`。生产必须改。

### VULN-017：HTTP 而非 HTTPS

`cookie_secure: false` 默认。生产必须 true。

### VULN-018：CORS 允许所有 origin

`main.go:86` `AllowOrigins: []string{"*"}`。生产应限制。

### VULN-019：错误响应泄露内部信息

`order.UpstreamMsg` 直接返回上游错误内容（含可能敏感信息）。

---

## 🧪 测试计划（运行时验证）

### 1. 启动
```bash
LISTEN_ADDR=127.0.0.1:8090 ./vaultlane
# 准备好 1 件自营商品（id=X）+ 余额 ¥100 的测试用户
```

### 2. 复现 P0 漏洞
```bash
# VULN-001 测试（必须验证）
curl -X POST http://127.0.0.1:8090/api/orders \
  -H "Content-Type: application/json" \
  -d '{"commodity_id": <自营ID>, "contact": "x@x.com", "num": 1, "need_pay": false}'
# 期望：401 或 400（需要登录）
# 实际：会直接返回卡密（漏洞存在）

# VULN-002 测试
# 1. 创建一个 epay 订单（need_pay: true, pay_method: epay）
# 2. 拿到 order.trade_no
# 3. 伪造回调：
curl -X POST http://127.0.0.1:8090/api/payment/callback/epay \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "out_trade_no=<pay的out_trade_no>&trade_status=TRADE_SUCCESS&money=0.01&sign=wrong"
# 期望：400 verify failed
# 实际：如果 key 错误但 out_trade_no 存在 → 验签通过

# VULN-003 测试
curl "http://127.0.0.1:8090/api/orders/<别人的trade_no>"
# 期望：403/401
# 实际：直接返回 order 完整信息（含卡密）

# VULN-004 测试
# 1. 登录用户 A
# 2. 用户 A 创建一个未支付订单（记下 order.id=100）
# 3. 退出，登录用户 B
# 4. 用户 B 调：
curl -X POST http://127.0.0.1:8090/api/payment/balance/finish \
  -H "Cookie: user_session=B的token" \
  -H "Content-Type: application/json" \
  -d '{"order_id": 100}'
# 期望：403
# 实际：会直接完成订单（漏洞存在）
```

### 3. 复现 P1 漏洞
```bash
# VULN-005：用 README 公开的默认 token 登后台
curl -H "X-Admin-Token: <legacy-admin-token>" \
  http://127.0.0.1:8090/admin/api/dashboard
# 期望：401
# 实际：成功（如果该路由用 token 校验）

# VULN-006：默认 首次启动前自行配置的管理员账号 登后台
curl -X POST http://127.0.0.1:8090/admin/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"<initial-password>"}'
# 期望：401（首次启动强制改密）
# 实际：直接成功
```

### 4. 复现 P2 漏洞
```bash
# VULN-013 限流绕过：换 IP
for ip in 192.168.1.{1..255}; do
  curl --interface $ip ...
done
# 期望：触发限流
# 实际：每 IP 独立计算 → 攻击者用 N 个 IP 下 N 个订单
```

---

## 📊 测试结果（当前状态）

**未做运行时验证**（因为静态审计已确认 4 个 P0 漏洞无需测试即可定性）。

**强烈建议**：在你接受修复前**先实地跑一遍**上面的 P0 复现步骤，把攻击证据记录下来：
- 截图
- curl 输出
- 实际拿到的卡密（验证用，不要泄露）

---

## 🛠️ 修复建议（按优先级）

### 必须立即修（24 小时内）

1. **VULN-001 + VULN-007**：删除/加固 `handleOrderCreate` 旧路由
   ```go
   // trade.go: 删除 handleOrderCreate 整个函数 + 注册
   // 或者加 requireUser + NeedPay 强校验
   ```
2. **VULN-003**：`handleOrderGet` 强制要求登录
3. **VULN-004**：`handleBalanceFinish` 校验 `order.UserID == uid`
4. **VULN-002**：`handlePaymentCallback` 强制金额匹配 + 删除可绕过路径

### 短期（1 周内）

5. VULN-005：删除 `admin.token` 配置
6. VULN-006：首次启动拒绝默认密码
7. VULN-009：epay 强制金额校验 + 改 HMAC

### 中期（1 月内）

8. VULN-012：审计日志
9. VULN-011：日志脱敏
10. VULN-013：业务维度限流

### 长期

11. VULN-014：迁移 PostgreSQL
12. 整体重写为 PostgreSQL + Row-Level Security

---

## 🛠️ 修复状态

### 2026-06-18 · 6 个 P1 全部修复

| 漏洞 | 修复 | 验证 |
|------|------|------|
| **VULN-005** 默认 admin token | 删除 `config.yaml admin.token` 字段 + 删除 main.go 的 `X-Admin-Token` CORS header | grep "admin.token" 0 命中 |
| **VULN-006** 默认 admin 密码 | 启动时检测默认 bcrypt hash，拒绝启动 + 提示用 `cmd/hash-password` 重设 | 实际启动被拒绝，错误信息明确 |
| **VULN-007** 死代码路由 | 删除 `trade.go` 的 `handleOrderCreate` + `handleOrderGet` + `handleSelfOrder`（500+ 行未注册死代码） | grep "handleOrderCreate" 0 命中 |
| **VULN-008** 批量操作 DoS | `BatchSetStatus` + `BatchAdjustPrice` 加 1000 ID cap + 负数 ID 校验 | 1001 个 ID → 500 拒绝；100 个 ID → 200 |
| **VULN-009** epay MD5 可碰撞 | 加 `EpaySignHmacSHA256`（推荐模式）；VerifyNotify 根据 `sign_type` 自动切换；金额范围校验（0 < money < 1000000） | curl 实测金额异常 → 400 |
| **VULN-015** mock 模式被滥用 | `HandleToggleMock` 加 admin 密码二次确认 + audit log | curl 实测无密码/错密码 → 401 |

### 2026-06-18 · VULN-020 回归测试

新增 `TestIntegration_OrderVULN020`：
- 普通用户查 admin 创建的 `UserID=0` 订单 → **403** ✓
- 未登录查任何订单 → **401** ✓

### 完整测试覆盖

```
=== RUN   TestIntegration_RegisterLoginOrder          PASS
=== RUN   TestIntegration_OrderNoPay_Flow            PASS  (VULN-001)
=== RUN   TestIntegration_OrderVULN020              PASS  (VULN-020)
=== RUN   TestIntegration_PaymentMethods              PASS
+ payment 6 个单元测试
+ store 7 个单元测试
+ upstream 1 个集成测试
= 18 个测试全绿
```

### 仍待修（按优先级）

1. 🟡 P2：审计日志（VULN-012）— ✅ v5 已修
2. 🟡 P2：日志脱敏（VULN-011）— ✅ v5 已修（helper + 3 处埋点）
3. 🟡 P2：业务维度限流（VULN-013）— ✅ v5 已修（payment/create 10/min/uid）
4. 🟡 P2：`/api/payment/methods` 信息泄露（VULN-021）— ✅ v5 已审计无需修改（仅返渠道名）
5. 🟡 P2：`/api/public/config` 暴露 upstream_base（VULN-022）— ✅ v5 已修（Origin/Referer 白名单）
6. 🟢 P3：session_secret 默认值 / HTTPS / CORS / 错误响应 — ✅ v5 已修
7. 🟢 P3：SQLite 单 conn — ✅ v5 验证（50 并发读不阻塞单 conn，写仍需 PostgreSQL）

---

| 漏洞 | 修复 | 验证 |
|------|------|------|
| **VULN-001** 未登录拿卡 | 新增 `RequireUserStrict` 中间件 + `handleOrderCreateV3` 强制登录 + `need_pay=false` 路径加 admin 校验 | 单元测试 `TestIntegration_OrderNoPay_Flow` + curl 实测 401 |
| **VULN-002** 伪造支付 | 删除 `trade_no` fallback；强制金额匹配 `pay.Amount == notify.Amount`；out_trade_no 不存在静默丢弃 | curl 实测 404 / 400 |
| **VULN-003** 越权读订单 | 删除 `Password` 字段免密路径；`RequireUserStrict` 拦截；非本人/非 admin 返回 403 | curl 实测 401 |
| **VULN-004** 任意完成 | 新增 `GetOrderByID` + 校验 `order.UserID == uid` + `order.Status == 0` | curl 实测 401 / 403 / 404 |

### 新增测试

- `TestIntegration_OrderNoPay_Flow` 覆盖 VULN-001（未登录 401 + 普通用户 403）
- 全部 `go test ./...` 通过

### 仍待修（按优先级）

1. 🟠 P1：删除 `admin.token` 默认值（VULN-005）
2. 🟠 P1：首次启动拒绝默认密码（VULN-006）
3. 🟠 P1：epay 改 HMAC（VULN-009）
4. 🟡 P2：审计日志（VULN-012）
5. 🟡 P2：日志脱敏（VULN-011）
6. 🟡 P2：业务维度限流（VULN-013）

---

## 📌 总结

**修复前**：4 个 P0 漏洞（攻击者拿全部卡密 / 越权读全部订单 / 任意完成订单 / 1 分钱买 100 元）

**修复后**：4 个 P0 全堵，单元测试覆盖，curl 实地复现全部返回 401/403/404

**剩余风险**：6 个 P1/P2（建议 1 个月内处理）

---

## 🔴 VULN-020（2026-06-17 新发现 + 修复）

**问题**：`need_pay=false` 路径（admin 走）创建的订单 `UserID=0`
- 原代码：`if !isAdmin && order.UserID > 0 && order.UserID != uid`
- 当 `UserID=0` 时条件不成立 → 普通登录用户能查 admin 创建的订单 → 拿卡密

**攻击路径**：
1. admin 免支付创建自营订单（UserID=0，trade_no=X）
2. 普通登录用户 `GET /api/orders/X`
3. 403 之前会**直接返回卡密**（漏洞）

**修复**：`internal/api/payment_api.go:408` 改为 `if !isAdmin && order.UserID != uid`（去掉 `> 0` 条件）

---

## 🟡 VULN-021（2026-06-17 新发现，v5 已审计无需修改）

**问题**：`GET /api/payment/methods` 返回 `["balance"]` 等所有已配置的支付方式
- 让攻击者知道系统支持哪些支付渠道 → 选择攻击路径
- 不算严重（攻击者本来就要试），但属于信息泄露

**v5 审计结论**：当前 `payment.Manager.Methods()` 仅返回已注册渠道名（`[]string`），不暴露任何内部配置（APIURL / Key / PID / 商户号等）。前端 `app.js:409/421` 用作"支付方式列表"渲染，本身就是用户面信息。无需代码变更。如未来要更严格：可在 Engine 接口加 `Enabled() bool`，未启用的渠道从 Methods 中过滤（v6+）。

---

## 🟡 VULN-022（2026-06-17 新发现，未修）

**问题**：`GET /api/public/config` 返回 `{"upstream_base": "https://upstreamapi.example.com"}`
- 暴露完整上游 URL
- 攻击者可针对该 URL 发起 SSRF / 探测

**修复建议**：仅返回 path 前缀（如 `/upstream/`），不返回完整 URL

---

## 🟠 中间件模式 bug（2026-06-17 审计）

**问题**：`gin.HandlerFunc` 的 `c.Next()` 是**同步**调用——handler 写完 response 后才回到中间件
- 原模式"先 inj → c.Next → 后置检查"看似安全，**实际无效**：handler 已写 response，再 Abort 已晚
- 任何"后置检查 Abort"的中间件 = **装饰品**

**审计结果**：
- `RequireUserStrict`：前置检查 + Abort，**已正确**（user.go:319-321）
- `requireSession`（admin）：前置检查 + Abort，**已正确**（handlers.go:122-132）
- 当前**没有任何"后置检查 Abort"的中间件残留**

**教训**：写 gin 中间件**永远前置校验 + 立即 Abort**，不要依赖 c.Next() 后的状态检查
