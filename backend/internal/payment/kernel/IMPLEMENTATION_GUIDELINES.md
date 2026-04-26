# User-to-user payment integration — implementation guidelines

This document describes how to add **peer-style payment providers** (e.g. Stripe Connect, PayPal for merchants) to Glipz. The `internal/payment/kernel` package only contains **PSP-agnostic infrastructure** (Redis key naming, OAuth `state` helpers). All provider rules, API clients, and money semantics live under `internal/payment/<provider>/`.

`internal/fanclub/` is for **membership / entitlement** verification (Patreon, etc.) and is separate from this module.

## Non-custodial, user-to-user (no admin as payee)

Glipz payment integrations should be **non-custodial** with respect to the **server operator**:

- **Payees (sellers / creators)** connect **their own** merchant or Connect account (OAuth or onboarding links). Funds settle with the **PSP ↔ payee** relationship.
- The **instance** may use app credentials only to **orchestrate** API calls the payee has authorized (e.g. create Checkout on a connected account, verify webhooks). It must **not** present the operator as the merchant of record for user-to-user sales or pool user funds in an operator wallet that substitutes for the payee.
- **Payers (buyers)** pay the **PSP** in flows scoped to the **payee’s** account or IDs returned by the PSP, not a single global “instance Stripe account” that belongs to the admin.

Concretely: prefer **Connect / parallel / direct** charge models from each PSP’s documentation over a model where the site owner receives all pay-ins and redistributes manually. Document any unavoidable platform-fee or application-fee line item as provider-specific behavior in `internal/payment/<provider>/`, not in `kernel`.

## What belongs in `kernel`

- **Stable provider IDs** (lowercase) for namespacing Redis keys and database rows.
- **Redis key helpers** with prefix `payment:` (never `fanclub:`): `OAuthStateKey`, `WebhookEventDedupKey`, `IdempotencyKey`.
- **OAuth `state` helpers** — `RandomOAuthState`, `SaveOAuthState`, `GetDelOAuthState` — that do *not* interpret payloads.

Do **not** put here: API URLs, fee logic, KYC, dispute handling, or “which account ID goes on a PaymentIntent”.

## What belongs in `internal/payment/<provider>/`

- OAuth / Connect onboarding, account linkage (Glipz user → PSP account id), webhooks, and idempotent fulfillment.
- HTTP routes under `/api/v1/…` (register from `internal/httpserver/server.go` when shipping a provider).
- Persistence in `internal/migrate` + `internal/repo` for tokens, connection rows, and **never** store full PAN or CVV; follow PCI scope minimization (hosted fields, Checkout, etc.).

## Adding a new provider

1. Choose a short **provider ID** (e.g. `stripe`, `paypal`). Use it in all `kernel` key calls.
2. Create `internal/payment/<id>/` with config from env, API client, and webhook verification using provider signatures/secrets.
3. Use `kernel` keys:
   - `SaveOAuthState` / `GetDelOAuthState` for Connect-style OAuth.
   - `WebhookEventDedupKey` + `SET NX` (or equivalent) to deduplicate webhooks.
   - `IdempotencyKey` for user-scoped idempotent create calls if the PSP supports it and you need local namespacing.
4. **Redis compatibility:** renaming keys between deploys invalidates in-flight OAuth states; version strings in keys only when breaking.

## Quick checklist

- [ ] Provider package with all external API and settlement rules
- [ ] Provider ID used consistently; keys under `payment:` only
- [ ] DB + `repo` for account linkage (no admin piggy-bank)
- [ ] Webhook dedup and signature verification
- [ ] `.env.example` and operator docs for **application** keys only (not user card data)

## Federation

Cross-instance **money movement** is out of scope for this kernel. If a feature needs payment metadata across instances, keep it in explicit provider payloads in `internal/payment/<provider>/` and document trust boundaries; do not assume the fanclub entitlement JWT model applies to payments.
