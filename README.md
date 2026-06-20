# 码仓 · FakaGateway v4.0

> 上游发卡平台对接网关 · 多上游商品池 · 多支付方式
> AI 账号批发业务（各类 API 账号等）
> 完整重写于 2026-06-17

## 一句话

把任意数量上游发卡平台的商品**拉到本地**做成一个"商品池"，管理员能**选哪些上架、灵活加价、按成本保护**；用户在前台看到的是**你控制的池子**而不是上游原始数据。

---

## ✨ v4.0 核心能力

| 模块 | 能力 | 状态 |
|------|------|------|
| **商品池** | N 件商品实测可管理（上游 + mock） | ✅ |
| **多上游** | config.yaml 加项即可，无代码改动；upstreama + mock 双适配器 | ✅ |
| **灵活定价** | 批量改价：固定金额 / 百分比（+/-），自动防亏本 | ✅ |
| **上下架** | 单条 + 批量勾选上下架 | ✅ |
| **前台稳定** | 前台只查本地商品池（不再直连上游），改价立即生效 | ✅ |
| **多支付** | 余额 / 易支付（epay） / USDT 占位 | ✅ |
| **后台** | 仪表盘 + 财务统计 + 商品池 + 自营 + 订单 + 卡密 + downstream-b + 设置 | ✅ |
| **可选模块** | 优惠券/邀请返佣/邮件/同步/downstream-b回调 5 个开关 | ✅ |
| **极简白瓷风 UI** | 瓷白底 + 藏蓝点缀 + 朱砂红强调，4 级库存色阶 | ✅ |

---

## 🚀 5 分钟跑起来

### 前置
- Go 1.21+（已用 1.26.3 测试）
- 操作系统：Windows / Linux / macOS

### 1. 编译

```bash
cd 06-WEB/faka-gateway
go build -o faka-gateway.exe .
```

> 第一次 build 会下载依赖（modernc.org/sqlite, gin, bcrypt, slog, yaml），约 30 秒。

### 2. 看默认配置

打开 `config/config.yaml`，关键项：

```yaml
server:
  listen: "127.0.0.1:8080"      # 监听地址

upstreams:                       # v4：多上游（数组）
  - name: upstreama             # 唯一标识（用于 source 字段）
    type: upstreama             # 适配器类型
    base_url: "https://upstreamapi.example.com"
    app_id: "YOUR_APP_ID"
    app_key: "YOUR_APP_KEY"
    enabled: true
  - name: mock                   # 第二个上游：内置演示（无需外部配置）
    type: mock
    enabled: true
    prefix: "MOCK-"
    count: 12

admin:
  username: "admin"
  password_hash: ""      # 用 cmd/hash-password 生成
  session_secret: ""     # 用 openssl rand -base64 32 生成
```

### 3. 启动

```bash
./faka-gateway.exe
# 看到 "ready addr=127.0.0.1:8080" 即成功
```

### 4. 访问

| 入口 | URL | 账号 |
|------|-----|------|
| 用户端 | http://127.0.0.1:8080 | 无 |
| 后台 | http://127.0.0.1:8080/admin | 首次启动前自行配置 |

### 5. 把上游商品拉进池子

后台 → 商品池 → 点「⬇ 同步上游」。

完成后表格显示 N 件商品（上游 + mock）。

---

## 🔄 完整工作流（用户视角）

```
[上游 upstreama]   [上游 mock]
        ↓ 同步           ↓ 同步
       ┌──────────────────────┐
       │   本地商品池 (SQLite) │  ← N 件
       │  字段：售价/成本/上下架/标签/起售/限购  │
       └──────────────────────┘
                  ↓ 用户访问
       ┌──────────────────────┐
       │  前台 http://8080    │
       │  按 status=1 过滤     │
       └──────────────────────┘
                  ↓ 用户下单
       ┌──────────────────────┐
       │  source=self → 本地卡密 │
       │  source=upstream:* → 调上游 │
       └──────────────────────┘
```

---

## 🔑 关键操作

### 管理员：日常运营

1. **登录后台** → `http://127.0.0.1:8080/admin`
2. **进商品池 tab** → 看所有商品
3. **勾选不卖的** → 点「批量下架」
4. **想涨价** → 勾选 → 选"+10%" → 点「应用」
5. **加新上游** → 改 `config.yaml` 加一项 → 重启服务

### 管理员：财务对账

- 后台 → 财务统计 tab
- 今日/本月/累计 订单 + 收入
- 30 天收入曲线（SVG 柱状图）
- Top 10 热销商品
- 按支付方式筛选记录

### 管理员：自营商品

- 后台 → 自营 tab
- 点「+ 新建商品」
- 选「卡密」tab → 粘贴卡密 → 导入
- 立即可售（不依赖上游）

### 用户：购物

- `http://127.0.0.1:8080/`
- 左侧选分类 → 中间选商品 → 右侧点购买
- 填邮箱 / QQ / 手机 → 选支付方式 → 付款
- 查订单：`http://127.0.0.1:8080/#/orders`

---

## ⚙️ 配置文件详解

`config/config.yaml` 完整结构：

```yaml
server:
  listen: "127.0.0.1:8080"       # 监听地址
  mode: "release"                 # debug | release | test
  read_timeout: 15
  write_timeout: 15

upstream:                         # 兼容旧字段（会自动转成 upstreams[0]）
  base_url: "https://upstreamapi.example.com"
  app_id: "YOUR_APP_ID"
  app_key: "YOUR_APP_KEY"

upstreams:                        # v4 多上游
  - name: upstreama
    type: upstreama
    base_url: "https://upstreamapi.example.com"
    app_id: "YOUR_APP_ID"
    app_key: "YOUR_APP_KEY"
    timeout: 15
    retry_max: 2
    user_agent: "FakaGateway/1.0"
    enabled: true
  - name: mock                    # 内置演示上游（无需外部配置）
    type: mock
    enabled: true
    prefix: "MOCK-"
    count: 12

downstream-b:                      # downstream-b（独立体系，可选）
  enabled: false
  app_key: 0
  app_secret: ""

cache:
  category_ttl: 300               # 分类缓存秒数
  commodity_list_ttl: 60          # 列表缓存秒数
  commodity_detail_ttl: 30        # 详情缓存秒数
  max_entries: 512

ratelimit:
  per_ip_per_min: 60              # 单 IP 限流

pricing:
  global:
    type: 0                       # 0=固定 1=百分比
    amount: 0.0
  category_overrides: {}          # 按分类加价
  commodity_overrides: {}         # 按商品加价

admin:
  username: "admin"
  password_hash: ""                 # bcrypt，必填
  session_secret: ""                # cookie 签名密钥，必填
  cookie_secure: false
  cookie_max_age: 28800           # 8 小时

mock:
  enabled: false
  prefix: "MOCK-"
  delay_ms: 0

pay_epay:                         # 易支付
  pid: ""
  key: ""
  gateway_url: ""
  notify_url: ""
  return_url: ""

pay_usdt:
  address: ""
  api_url: ""

modules:                          # v4：可选模块开关
  coupon: false
  referral: false
  email: false
  upstream_sync: false
  downstream_callback: false
  finance_stats: true
  config_center: true
  integrations: true

smtp:                             # email 模块启用后必填
  host: ""
  port: 465
  username: ""
  password: ""
  from: ""
  use_tls: true
```

### 环境变量覆盖（敏感字段）

| 变量 | 覆盖字段 |
|------|----------|
| `UPSTREAM_APP_ID` | upstreams[].app_id |
| `UPSTREAM_APP_KEY` | upstreams[].app_key |
| `UPSTREAM_BASE_URL` | upstreams[].base_url |
| `LISTEN_ADDR` | server.listen |
| `ADMIN_USERNAME` | admin.username |
| `ADMIN_PASSWORD_HASH` | admin.password_hash |
| `ADMIN_SESSION_SECRET` | admin.session_secret |

---

## 🗄️ 数据库（SQLite）

文件：`data/gateway.db`

启动时自动迁移。当前 schema：

| 表 | 用途 |
|----|------|
| `commodities` | **商品池主表**（自营 + 上游映射） |
| `card_secrets` | 自营卡密 |
| `categories` | 分类 |
| `upstream_runtime` | 多上游同步状态 |
| `orders` | 订单 |
| `users` | 用户（前台登录） |
| `user_sessions` | 用户 session |
| `balance_logs` | 余额流水 |
| `payments` | 支付记录 |
| `settings` | 系统设置 KV |

### 商品池表 (`commodities`) 关键字段

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | 池 ID（前台用这个下单） |
| source | TEXT | `self` / `upstream:upstreama` / `upstream:mock` |
| outer_id | TEXT | 上游原始 ID（用于更新识别） |
| name | TEXT | 商品名 |
| cover | TEXT | 封面 URL（相对路径自动补上游 base） |
| description | TEXT | 详情（HTML 原文） |
| sale_price | REAL | **你定的售价**（用户看到） |
| cost_price | REAL | 上游成本（自动同步过来） |
| stock | INT | 库存 |
| status | INT | 0=下架 1=上架 |
| config | TEXT | 上游配置 JSON（规格/分类等） |
| tags | TEXT | 逗号分隔 |
| minimum | INT | 起售 |
| maximum | INT | 限购 |
| last_seen_at | DATETIME | 上次同步见到（用于下架超期商品） |
| created_at / updated_at | DATETIME | |

---

## 🛣️ API 速查

### 公开（限流 60 req/min/IP）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/public/config` | 上游 base URL 等 |
| GET | `/api/categories` | 全部分类 |
| GET | `/api/commodities?page&limit&category_id&keywords&source` | 商品列表（只显示 status=1） |
| GET | `/api/commodities/:id` | 商品详情 |
| GET | `/healthz` | 健康检查 |

### 前台用户（需登录）

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/auth/register` | 注册 |
| POST | `/api/auth/login` | 登录 |
| GET | `/api/auth/me` | 当前用户 |
| GET | `/api/balance` | 余额 |
| POST | `/api/orders` | 下单 |
| GET | `/api/orders/:trade_no` | 查订单 |

### 后台（需 admin cookie）

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/admin/api/login` | 登录 |
| POST | `/admin/api/logout` | 退出 |
| GET | `/admin/api/dashboard` | 仪表盘 |
| GET | `/admin/api/stats/overview` | 财务总览 |
| GET | `/admin/api/stats/revenue?days=30` | 收入曲线 |
| GET | `/admin/api/stats/top_commodities` | Top 10 商品 |
| GET | `/admin/api/stats/payments` | 支付记录 |
| GET | `/admin/api/commodities` | 自营商品 |
| POST | `/admin/api/commodities` | 新建自营 |
| PUT | `/admin/api/commodities/:id` | 改自营 |
| DELETE | `/admin/api/commodities/:id` | 删自营 |
| GET | `/admin/api/secrets?commodity_id&status` | 卡密列表 |
| POST | `/admin/api/secrets/import` | 批量导入卡密 |
| **GET** | **`/admin/api/pool`** | **商品池列表（含统计）** |
| **POST** | **`/admin/api/pool/sync`** | **同步上游到商品池** |
| **POST** | **`/admin/api/pool/status`** | **批量上下架** |
| **POST** | **`/admin/api/pool/price`** | **批量改价** |
| **PUT** | **`/admin/api/pool/:id`** | **改单条** |
| **DELETE** | **`/admin/api/pool/:id`** | **删单条** |
| **GET** | **`/admin/api/pool/upstreams`** | **上游状态** |
| GET | `/admin/api/settings` | 系统设置 |
| PUT | `/admin/api/settings` | 改设置（限流/Mock） |
| GET | `/admin/api/downstream/test` | downstream-b连通性 |

---

## 🔐 安全

1. **改默认密码**：编辑 `config/config.yaml` 的 `admin.password_hash`
   ```bash
   # 生成新 hash
   python3 -c "import bcrypt; print(bcrypt.hashpw(b'你的新密码', bcrypt.gensalt()).decode())"
   ```
2. **改 session_secret**：用 32 字节随机串
   ```bash
   python3 -c "import secrets,base64; print(base64.b64encode(secrets.token_bytes(32)).decode())"
   ```
3. **生产部署**：`cookie_secure: true`（需 HTTPS）+ nginx 反代 + 限流
4. **不要把 .env 提交到 git**（已在 .gitignore）

---

## 🧪 验证

### 编译验证
```bash
go build -o faka-gateway.exe .   # 必须无错
```

### 测试
```bash
go test ./...                     # 单元测试（应全绿）
```

### 实地验证清单（人工）
1. ✅ 启动后日志有 "ready"
2. ✅ 访问 `http://127.0.0.1:8080/` 看到首页
3. ✅ 进 `http://127.0.0.1:8080/admin` 用 `首次启动前自行配置的管理员账号` 登录
4. ✅ 进商品池 → 看到 N 件商品（上游 + mock）
5. ✅ 勾选 3 件 → 批量下架 → 列表中这 3 件显示"下架"
6. ✅ 进财务统计 → 看到 0 订单（首次无数据，正常）
7. ✅ 进设置 → 看到模块开关 / Mock / 限流 三个区块

---

## 📁 项目结构

```
faka-gateway/
├── main.go                       入口：构造 Manager + 启动服务
├── config/
│   └── config.yaml               所有配置（含多上游）
├── data/gateway.db               SQLite（运行时生成）
├── internal/
│   ├── config/                   YAML 加载
│   ├── store/                    SQLite 持久化
│   │   ├── sqlite.go             迁移
│   │   ├── commodity.go          商品池 CRUD
│   │   ├── user.go               用户
│   │   └── stats.go              财务统计
│   ├── upstream/                 上游适配器
│   │   ├── adapter.go            Adapter 接口
│   │   ├── upstreama.go         upstreama 适配器
│   │   ├── mock_adapter.go       mock 适配器
│   │   ├── manager.go            多上游管理器
│   │   ├── httpdoer.go           HTTP + 指标
│   │   ├── signature.go          MD5 签名
│   │   ├── cache.go              TTL 缓存
│   │   └── downstream-b/          downstream-b独立包
│   ├── domain/                   领域模型 + 加价引擎
│   ├── api/                      前台路由
│   │   ├── router.go             Register
│   │   ├── commodities.go        商品池 API
│   │   ├── trade.go              下单
│   │   ├── payment_api.go        支付
│   │   ├── public_config.go      公共配置
│   │   └── user.go               用户
│   ├── payment/                  多支付引擎
│   │   ├── engine.go             抽象
│   │   ├── balance.go            余额
│   │   ├── epay.go               易支付
│   │   └── usdt.go               USDT
│   ├── delivery/                 发货引擎
│   │   └── delivery.go           自营/上游/邮件
│   ├── admin/                    后台
│   │   ├── auth.go               cookie session
│   │   ├── handlers.go           路由
│   │   ├── pool.go               商品池 API（v4）
│   │   ├── stats.go              财务统计
│   │   └── static/               后台 SPA
│   └── web/                      前台
│       ├── embed.go              embed.FS
│       └── static/               前台 SPA
├── logs/gateway.log              运行日志
└── faka-gateway.exe              编译产物
```

---

## ❓ 常见问题

**Q: 我没真上游，能跑吗？**
A: 可以。`upstreams[].type: mock` 是内置演示上游，12 件固定商品，无需任何外部配置。

**Q: 同步后商品没出现？**
A: 看 `data/gateway.db` 是否能写入；查日志的 `pool sync failed` 行。

**Q: 改价不生效？**
A: 改了 sale_price 后，前台立即看到；改 cost_price 是上游同步时用，不影响销售。

**Q: 怎样清空商品池？**
A: `rm data/gateway.db` 重启（会重新迁移），但订单也会清。**生产别这么干**。

**Q: 可以接downstream-b（downstream-b）吗？**
A: 可以，在 `downstream-b` 段填 app_key/app_secret，UI 上"downstream-b"tab 会激活。

**Q: 端口被占？**
A: 改 `config.yaml` 的 `server.listen`，或设环境变量 `LISTEN_ADDR=127.0.0.1:9090`。

**Q: 怎么备份？**
A: `cp data/gateway.db backup/$(date +%F).db`。SQLite 单文件即可。

---

## 📞 部署到生产

```bash
# 1. 编译 Linux 版本
GOOS=linux GOARCH=amd64 go build -o faka-gateway-linux .

# 2. 上传
scp faka-gateway-linux config/config.yaml user@server:/opt/faka/

# 3. 在服务器创建 systemd 服务
cat > /etc/systemd/system/faka.service <<'EOF'
[Unit]
Description=FakaGateway
After=network.target

[Service]
Type=simple
User=faka
WorkingDirectory=/opt/faka
ExecStart=/opt/faka/faka-gateway-linux
Restart=always
RestartSec=5
Environment=LISTEN_ADDR=0.0.0.0:8080
Environment=ADMIN_PASSWORD_HASH=...

[Install]
WantedBy=multi-user.target
EOF

# 4. 启动
systemctl daemon-reload
systemctl enable --now faka

# 5. nginx 反代（80/443 + HTTPS）
```

---

## 🛣️ 路线图

- **v5**：前台用户中心、优惠券、邀请返佣
- **v5**：邮件通知（订单 / 发货）
- **v5**：自动定时同步上游
- **v5**：商品池导入导出（CSV）
- **v5**：更多上游适配器（自定义上游模板）

详细见 [ROADMAP.md](./ROADMAP.md)

---

## 📝 更新日志

### v4.0（2026-06-17）— 完整版
- **商品池**：所有商品落本地（`commodities` 表 9 个新字段）
- **多上游**：`config.Upstreams` 数组 + Adapter 接口
- **多上游管理**：后台 `/admin/api/pool/*` + UI
- **灵活定价**：批量改价（固定/百分比，+/-），防亏本
- **可选模块**：`config.yaml` 8 个开关（优惠券/邀请/邮件/同步/downstream-b回调/财务/配置中心/对接面板）
- **后台 SPA**：商品池 tab + 财务统计 tab + 模块开关可视化
- **前台稳定**：商品列表/详情只查本地（不再直连上游）
- **多支付**：余额 + 易支付 + USDT 占位
- **多支付引擎**：可插拔

### v3.0（2026-06-16）
- 用户/支付/余额/订单扩展
- 财务统计接口
- downstream-b对接包

### v2.0（2026-06-15）
- 自营商品 + 卡密
- 后台独立路径 + cookie session + bcrypt
- downstream-b开放平台对接

### v1.0（2026-06-14）
- 上游发卡 v3.4.9 签名 + 公共浏览 API
- 极简白瓷风 UI
