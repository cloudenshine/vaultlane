# Roadmap

Vaultlane 5.0 landed the local catalog, multi-upstream adapters, coupons, referral, SMTP delivery, CSRF, TOTP, and AES-GCM card-secret storage. The items below are the next production milestones.

## Near term

- [ ] PostgreSQL / MySQL storage engine behind the existing `store.Store` API
- [ ] Redis-backed rate limit, session, and delivery queue
- [ ] Replace the in-process delivery queue with Redis Stream or NATS
- [ ] Persist runtime settings (`HandleSettingsPut`) back to disk or a settings table
- [ ] Stock-warning notifications (email / Telegram / webhook)

## Product

- [ ] SKU / multi-spec catalog
- [ ] Official WeChat Pay v3 and Alipay certificate engines
- [ ] Real USDT TRC-20 confirmation (not a stub)
- [ ] Wire `downstreamb` listings into the local catalog
- [ ] CSV import / export for the commodity pool

## Experience

- [ ] Vue 3 storefront (replace the embedded vanilla SPA)
- [ ] Admin console on a component library with RBAC
- [ ] OpenAPI 3 documentation generated from handlers

Completed in 5.0:

- [x] Multi-upstream fulfillment routing
- [x] Idempotent payment callbacks
- [x] Coupon and referral engines
- [x] SMTP card delivery
- [x] Timed upstream sync and cost circuit breaker
- [x] AES-256-GCM card-secret encryption
- [x] Admin CSRF + optional TOTP
