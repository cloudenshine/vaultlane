# Security Policy

## Supported versions

| Version | Supported |
|---|---|
| 5.x | Yes |
| 4.x | Security fixes only |
| < 4.0 | No |

## Reporting a vulnerability

**Do not open a public GitHub issue for security reports.**

Please email the maintainers (or use GitHub Security Advisories on this repository) with:

- A short description of the issue
- Affected version / commit
- Steps to reproduce
- Impact (data exposure, payment bypass, privilege escalation, etc.)

We aim to acknowledge reports within 72 hours and to provide a status update within 7 days.

## Scope

In scope:

- Authentication, session cookies, CSRF, TOTP
- Payment callbacks, order ownership, balance adjustments
- Card-secret storage and delivery
- Admin authorization and audit logs
- Configuration secrets (`password_hash`, `session_secret`, upstream keys)

Out of scope unless they lead to a security impact:

- Denial of service via legitimate high traffic
- Issues that require an already-compromised admin session
- Theoretical attacks without a working proof of concept

## Handling secrets

Never commit:

- `config.yaml` copies with real `password_hash` / `session_secret` / payment keys
- SQLite files under `data/`
- `.env` files

Startup refuses empty or default admin credentials. Production deployments must set:

```yaml
admin:
  cookie_secure: true
```

and terminate TLS in front of the process (nginx, Caddy, or a cloud load balancer).
