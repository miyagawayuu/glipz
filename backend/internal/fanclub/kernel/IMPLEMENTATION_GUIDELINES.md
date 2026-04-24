# Fan club integration — implementation guidelines

This document describes how to add a **new third-party fan club / membership site** to Glipz. The `kernel` package only contains **site-agnostic infrastructure** (key naming, OAuth state helpers). All provider-specific behavior lives under `internal/fanclub/<provider>/`.

**Reference implementation:** Patreon lives in `internal/fanclub/patreon/` (OAuth, token handling, UI-facing settings). Operator-facing env vars are documented in the repository root [.env.example](../../../../.env.example) (search for `PATREON_`). Routes are registered from `internal/httpserver/server.go` (e.g. `/api/v1/fanclub/patreon/...`, callback under `/api/v1/fanclub/patreon/callback`).

## What belongs in `kernel`

- **Stable provider IDs** (string constants, e.g. `example`) for namespacing Redis keys and, later, database rows.
- **Redis key helpers** such as `OAuthStateKey` and `EntitledCacheKey` so every provider uses the same prefix and layout: `fanclub:oauth:<provider>:…`, `fanclub:entitled:<provider>:…`.
- **OAuth state helpers** — `RandomOAuthState`, `SaveOAuthState`, `GetDelOAuthState` — that do *not* interpret payloads; the provider’s HTTP handlers own the JSON shape.

Do **not** put here: authorize URLs, token endpoints, API clients, “tier vs plan” semantics, or any business rules.

## What belongs in `internal/fanclub/<provider>/`

- OAuth (or API keys, webhooks, etc.) **as required by that site’s documentation**.
- HTTP-facing logic can stay in `internal/fanclub/<provider>/` *or* thin wrappers in `internal/httpserver/`, but the **site-specific rules must not leak into `kernel`**.

Add future providers by reusing this kernel package (keys + OAuth state helpers) and keeping all provider-specific rules inside `internal/fanclub/<provider>/`.

## Adding a new provider

1. **Choose a short, stable provider ID** (lowercase, no spaces), e.g. `example`. Add a constant in `kernel` or define `ProviderID` in the provider package and use it whenever calling `kernel` helpers.
2. **Create `internal/fanclub/<id>/`** with:
   - Configuration struct loaded from env (provider-specific env vars).
   - Token exchange / refresh (if applicable).
   - **Creator** flows: whatever the author must pick in the UI (e.g. campaigns, plans, store IDs) — your types, your API calls.
   - **Member** flows: check whether a viewer’s credentials satisfy the author’s required rule for a note. This may look nothing like Patreon’s “campaign + reward tier” model; that is fine.
3. **Persistence**: either extend the schema (provider-specific columns, a generic `user_fanclub_connections` table, or JSONB) in `internal/migrate` and `internal/repo`. Keep token columns out of public JSON in API responses.
4. **HTTP routes**: mount routes under `/api/v1/…`. Reuse `kernel` for OAuth `state` storage by passing your provider ID into `SaveOAuthState` / `GetDelOAuthState`.
5. **Note paywall**: in `notePremiumProjection` (or equivalent), branch on the note’s (and author’s) paywall fields. A new provider adds its own entitlement check (or a single dispatcher reading `paywall_spec`).
6. **Cache**: use `kernel.EntitledCacheKey(provider, viewerUUID, authorUUID, scopeID, tierID)` (or repurpose the last two segments as opaque IDs if your site’s model differs; document the meaning in the provider package).
7. **Config & deployment**: add env vars, document in `.env.example` / operator docs, and register routes in `internal/httpserver/server.go`.
8. **OpenAPI & web**: add paths and UI only for the new provider; avoid hard-coding a single global “fan club shape” in the client.

## Redis compatibility

When changing key layouts, expect in-flight OAuth flows to fail with a bad state if keys are renamed between deploys. Entitlement cache keys can be versioned in the string if you ever need a breaking change.

## Quick checklist

- [ ] Provider package under `internal/fanclub/<id>/` with all external API and domain rules
- [ ] Provider ID used consistently in `kernel` key helpers
- [ ] DB + `repo` methods for tokens and author defaults
- [ ] HTTP handlers + `server.go` registration
- [ ] Note premium visibility branch + entitlement caching
- [ ] Env, OpenAPI, and frontend (if user-facing) updated

## Federation note

For federated / remote content, “forced” tiered access is a separate concern (e.g. origin-side authorization). Local fan club integrations in this module apply to **this instance’s** notes and user linking.
