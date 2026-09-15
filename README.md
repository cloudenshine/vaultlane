# Vaultlane · 码仓

[![CI](https://github.com/cloudenshine/faka-gateway/actions/workflows/ci.yml/badge.svg)](https://github.com/cloudenshine/faka-gateway/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-1F3A5F.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)](https://go.dev/)

Self-hosted **digital-goods commerce gateway**. Pull supplier catalogs into a local pool, set your own prices, sell with balance / Epay / USDT, and fulfill from self-operated secrets or upstream APIs.

> 中文品牌：**码仓**。Go module 仍为 `faka-gateway`（保持 import 稳定）。二进制名为 `vaultlane`。

---

## Why Vaultlane

Most card shops either proxy a single supplier (the storefront dies when the supplier dies) or dump SKUs into a spreadsheet. Vaultlane keeps a **local commodity pool**:

1. Adapters sync one or more upstreams into SQLite.
2. You decide sale price, listing status, and markup. Upstream cost updates never overwrite your price.
3. The public storefront reads **only** the local pool.
4. After payment, delivery routes by `source`: self-operated secrets, or the matching upstream adapter.

```
[Upstream A]  [Upstream B]  [Mock]  [Self-operated secrets]
        \         |         /              |
         \        |        /               |
          v       v       v                v
           ┌──────────────────┐            |
           │  Local catalog   │◄───────────┘
           │  sale_price /    │
           │  status / stock  │
           └────────┬─────────┘
                    │
     storefront ←───┴───→ payments → delivery
```

## Features (v5.0)

| Area | Status |
|------|--------|
| Multi-upstream adapters (`upstreama`, `mock`) | Done |
| Local catalog with protected sale price | Done |
| Balance / Epay (alipay · wxpay · qqpay) / USDT stub | Done |
| Coupons, referral commission, SMTP delivery | Done |
| Timed upstream sync + cost circuit breaker | Done |
| AES-256-GCM card-secret encryption | Done |
| Admin CSRF + optional TOTP | Done |
| SQLite WAL, SQL aggregate stats | Done |
| PostgreSQL / Redis / Vue 3 rewrite | Roadmap |

## Quick start

**Requirements:** Go 1.25+

```bash
git clone https://github.com/cloudenshine/faka-gateway.git
cd faka-gateway

# 1. Admin credentials (required — process refuses defaults)
go run ./cmd/hash-password
# paste the bcrypt hash into config/config.yaml → admin.password_hash
# openssl rand -base64 32  → admin.session_secret

# 2. Build & run
go test ./...
go build -o vaultlane .
./vaultlane
```

| Surface | URL |
|---------|-----|
| Storefront | http://127.0.0.1:8090 |
| Admin | http://127.0.0.1:8090/admin |
| Health | http://127.0.0.1:8090/healthz |

Default listen address is `127.0.0.1:8090` (`LISTEN_ADDR` overrides it).

A `type: mock` upstream is auto-added when no real supplier is configured, so you can click **Sync** in Admin → Catalog and get 12 demo SKUs.

## Configuration

All runtime options live in [`config/config.yaml`](config/config.yaml). Sensitive fields can be overridden by environment variables:

| Variable | Field |
|----------|--------|
| `LISTEN_ADDR` | `server.listen` |
| `ADMIN_USERNAME` | `admin.username` |
| `ADMIN_PASSWORD_HASH` | `admin.password_hash` |
| `ADMIN_SESSION_SECRET` | `admin.session_secret` |
| `UPSTREAM_APP_ID` / `UPSTREAM_APP_KEY` / `UPSTREAM_BASE_URL` | first upstream |

Never commit a filled `config.yaml` or `.env`. See [SECURITY.md](SECURITY.md).

### Modules

```yaml
modules:
  coupon: false
  referral: false
  email: false            # requires smtp.*
  upstream_sync: false    # 15-minute background sync + cost breaker
  finance_stats: true
  config_center: true
  integrations: true
```

## API (selected)

Public (IP rate-limited):

| Method | Path |
|--------|------|
| `GET` | `/api/public/config` |
| `GET` | `/api/categories` |
| `GET` | `/api/commodities` |
| `GET` | `/api/commodities/:id` |
| `GET` | `/healthz` |

Signed-in storefront:

| Method | Path |
|--------|------|
| `POST` | `/api/user/register` |
| `POST` | `/api/user/login` |
| `GET` | `/api/user/profile` |
| `GET` | `/api/user/referral` |
| `POST` | `/api/orders` |
| `GET` | `/api/orders/:trade_no` |
| `POST` | `/api/coupon/verify` |
| `GET` | `/api/payment/methods` |

Admin (cookie session + CSRF on writes; TOTP on high-risk writes when enabled):

| Method | Path |
|--------|------|
| `POST` | `/admin/api/login` |
| `GET` | `/admin/api/pool` |
| `POST` | `/admin/api/pool/sync` |
| `POST` | `/admin/api/coupons` |
| `POST` | `/admin/api/totp/setup` |

## Security

- Startup **exits** if `password_hash` or `session_secret` is empty / default.
- Orders require a signed-in user. Callbacks verify signature **and** amount.
- Card secrets are stored as `ENC:` + AES-256-GCM (key derived from `session_secret` via HKDF).
- Admin writes send `X-CSRF-Token`. Optional TOTP (`X-TOTP-Code`) on price/delete.
- Production: `admin.cookie_secure: true` behind HTTPS.

Historical findings: [SECURITY_AUDIT.md](SECURITY_AUDIT.md). How to report new issues: [SECURITY.md](SECURITY.md).

## Development

```bash
go test ./...
go vet ./...
go build -o vaultlane .
```

Layout:

```
.
├── main.go
├── config/config.yaml
├── internal/
│   ├── api/          storefront HTTP
│   ├── admin/        console HTTP + embedded SPA
│   ├── store/        SQLite
│   ├── upstream/     adapters + syncer
│   ├── payment/      engines
│   ├── delivery/     fulfillment + SMTP + queue
│   ├── crypto/       AES-GCM
│   ├── totp/         RFC 6238
│   └── web/          storefront SPA
└── cmd/hash-password
```

See [ARCHITECTURE.md](ARCHITECTURE.md), [CONTRIBUTING.md](CONTRIBUTING.md), and [ROADMAP.md](ROADMAP.md).

## Production sketch

```bash
GOOS=linux GOARCH=amd64 go build -o vaultlane .
# systemd: ExecStart=/opt/vaultlane/vaultlane
# nginx/caddy terminates TLS and reverse-proxies to 127.0.0.1:8090
```

Backup is a copy of `data/gateway.db` (SQLite, WAL mode).

## License

[MIT](LICENSE) © 2026 Vaultlane contributors.

This software is a **commerce gateway**. You are responsible for complying with payment-network rules, tax law, and the terms of every upstream you connect. Do not use it to sell unauthorized accounts, stolen credentials, or other illegal goods.
