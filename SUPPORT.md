# Support

Vaultlane is an open-source project. Please use the following channels:

| Need | Where |
|---|---|
| Bug report | GitHub Issues, with logs (redact secrets) and reproduction steps |
| Feature request | GitHub Issues, labeled `enhancement` |
| Security report | [SECURITY.md](SECURITY.md) |
| Usage questions | GitHub Discussions (if enabled) or Issues labeled `question` |

Before opening an issue, confirm:

1. You generated a unique `admin.password_hash` and `admin.session_secret`.
2. `go test ./...` passes on your machine.
3. The problem is not already listed in `ARCHITECTURE.md` § known limits.

Maintainers cannot provide private merchant onboarding, payment-channel account setup, or custom reseller deployments through GitHub.
