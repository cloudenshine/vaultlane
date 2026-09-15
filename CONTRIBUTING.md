# Contributing to Vaultlane

Thanks for helping improve Vaultlane（码仓）. Please read this guide before opening a pull request.

## Before you start

1. Search existing issues and pull requests.
2. For security issues, **do not** open a public issue. Follow [SECURITY.md](SECURITY.md).
3. Keep pull requests focused. One concern per PR.

## Development setup

Requirements:

- Go 1.25 or newer
- Git

```bash
git clone https://github.com/<owner>/vaultlane.git
cd vaultlane
cp config/config.yaml config/config.local.yaml   # optional local copy
go test ./...
go build -o vaultlane .
```

Do not commit secrets. Generate a bcrypt hash and session secret locally:

```bash
go run ./cmd/hash-password
# openssl rand -base64 32
```

## Project conventions

- Keep Go packages small and single-purpose.
- Prefer adding an adapter or engine instead of branching existing ones.
- Do not break existing HTTP paths without a compatibility note in `CHANGELOG.md`.
- Database changes belong in incremental migrations (`migrateVn` in `internal/store`).
- Frontend assets live under `internal/web/static` (storefront) and `internal/admin/static` (console). They are embedded; no separate build step.

## Tests

```bash
go test ./...
go vet ./...
```

Add or update tests when you change:

- payment / delivery / order status
- store queries and migrations
- authentication, CSRF, TOTP, or cookie behavior

## Pull request checklist

- [ ] `go test ./...` and `go vet ./...` pass
- [ ] `CHANGELOG.md` updated if users can observe the change
- [ ] Docs updated when APIs, config keys, or modules change
- [ ] No secrets, live credentials, or production databases committed
- [ ] New files include a license header only when required by the surrounding package

## Naming

| Surface | Name |
|---|---|
| Product | Vaultlane |
| Chinese brand | 码仓 |
| Go module (keep stable) | `faka-gateway` |
| Binary | `vaultlane` |

Do not rename the Go module in a drive-by PR; that is a breaking change for importers.
