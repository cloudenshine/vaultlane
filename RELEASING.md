# Releasing Vaultlane

## Versioning

Follow [SemVer](https://semver.org/):

- **MAJOR** — breaking HTTP or config changes
- **MINOR** — features that stay compatible
- **PATCH** — fixes

Update in the same commit:

1. `CHANGELOG.md` (move unreleased notes into the new version)
2. `CITATION.cff` `version` and `date-released`
3. Default User-Agent strings (`Vaultlane/x.y`) only on MAJOR/MINOR

## Tag and GitHub Release

```bash
git tag -a v5.0.0 -m "Vaultlane 5.0.0"
git push origin v5.0.0
```

Then create a GitHub Release from the tag:

- Title: `v5.0.0 — catalog, coupons, CSRF/TOTP`
- Body: paste the matching `CHANGELOG.md` section
- Attach binaries if you build them:

```bash
GOOS=linux   GOARCH=amd64 go build -ldflags="-s -w" -o vaultlane-linux-amd64 .
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o vaultlane-windows-amd64.exe .
GOOS=darwin  GOARCH=amd64 go build -ldflags="-s -w" -o vaultlane-darwin-amd64 .
```

Do **not** attach `data/gateway.db`, filled `config.yaml`, or `.env`.

## GitHub repository settings (manual)

After the first push of this tree:

1. **About**: Description = `Self-hosted digital-goods commerce gateway (码仓)`. Topics = `golang`, `ecommerce`, `payment`, `sqlite`, `self-hosted`.
2. **General**: Enable Issues, Discussions (optional), and “Automatically delete head branches”.
3. **Security**: Enable Dependabot alerts + GitHub Advisory reports (matches `SECURITY.md`).
4. Repository is already named `vaultlane`. Keep the Go module path `faka-gateway` until a dedicated major bump.

## Community health files

GitHub’s Community Standards checklist expects:

| File | Status |
|------|--------|
| `README.md` | present |
| `LICENSE` | MIT |
| `CODE_OF_CONDUCT.md` | present |
| `CONTRIBUTING.md` | present |
| `SECURITY.md` | present |
| `SUPPORT.md` | present |
| Issue / PR templates | under `.github/` |
| CI | `.github/workflows/ci.yml` |
