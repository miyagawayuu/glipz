# Fan club integrations — frontend implementation guidelines

This document explains how to add a **new fan club / membership provider** to the Glipz frontend.

The backend integration guidelines live at:
[`backend/internal/fanclub/kernel/IMPLEMENTATION_GUIDELINES.md`](../../../backend/internal/fanclub/kernel/IMPLEMENTATION_GUIDELINES.md)

## Current state (important)

- Backend APIs are currently **provider-specific** (e.g. Patreon uses `/api/v1/patreon/...`).
- There is **no** `GET /api/v1/fanclubs/providers` endpoint yet.
  - Because of that, the frontend starts with a **static provider registry**.
- `/api/v1/me` currently returns `me.patreon` (Patreon-only).
  - The frontend normalizes link status so it can support a future `me.fanclubs[providerId]` shape.

## Where things live

- Provider registry: `web/src/fanclub/registry.ts`
  - `fanclubProviderRegistry`: list of providers (start with Patreon only)
  - `enabledFanclubProviders()`: the list used by the Settings UI
- `/me` normalization: `web/src/fanclub/me.ts`
  - `getFanclubLinkStatus(me, providerId)` handles today’s `me.patreon` and a future `me.fanclubs`
- Provider actions (OAuth start / disconnect): `web/src/composables/useFanclubLinks.ts`
  - Currently implemented for `providerId === "patreon"`
- Settings UI: `web/src/components/SecuritySettingsPanel.vue`
  - Uses “selector + panel” when multiple providers exist
  - Provider panel component example: `web/src/components/fanclub/FanclubLinkPatreon.vue`
- Note editor paywall fields:
  - Wrapper and provider choice: `web/src/views/NoteEditView.vue`
  - Patreon fields component: `web/src/components/notes/NotePaywallPatreonFields.vue`

## Adding a new provider (step-by-step)

### 1) Add the provider to the registry

Edit `web/src/fanclub/registry.ts`:

- Add an entry to `fanclubProviderRegistry`:
  - `id`: stable lowercase id (e.g. `fantia`)
  - `labelKey` (preferred) or `label`
  - `returnQueryKey`: what the backend uses in the OAuth callback redirect query
  - `apiPrefix`: provider API prefix (usually `/api/v1/<provider>`)
  - `supportsMember` / `supportsCreator`
  - `enabled: true` to show it in the UI

### 2) Implement the provider’s Settings panel

Create `web/src/components/fanclub/FanclubLink<Provider>.vue`.

This panel should:

- Show a short provider-specific intro
- Show link status (`linked` / `not linked`)
- Provide buttons to connect/disconnect (member and/or creator), depending on capabilities

For link status, use:

- `getFanclubLinkStatus(me, "<providerId>")`

### 3) Add provider actions (OAuth start / disconnect)

Update `web/src/composables/useFanclubLinks.ts`:

- Add a new `providerId` branch
- Implement:
  - `connectMember` → GET `.../member/authorize-url` and redirect browser to `authorize_url`
  - `connectCreator` → GET `.../creator/authorize-url` and redirect
  - `disconnectMember` / `disconnectCreator` → POST disconnect endpoints

Keep provider-specific URLs inside the provider branch so we do not accidentally assume all providers follow Patreon’s API shapes.

### 4) Mount the new panel in `SecuritySettingsPanel`

In `web/src/components/SecuritySettingsPanel.vue`:

- Import your new panel component
- Add a `v-if` branch keyed by `selectedProviderId`

The header is already provider-agnostic (`views.settings.sections.fanclubLinks`).

### 5) (Optional) Add note editor fields

If the provider supports paywall requirements set per note:

- Add a provider option in `NoteEditView.vue`’s paywall selector.
- Create `web/src/components/notes/NotePaywall<Provider>Fields.vue` and mount it with a `v-if`.

Note: the backend still uses Patreon-specific fields (`patreon_campaign_id`, `patreon_required_reward_tier_id`). A future backend change will likely introduce a generic `paywall_spec` / `paywall_provider` shape. Keep provider-specific UI isolated so that migration is localized.

## OAuth callback return query

Today (Patreon): `?patreon=member_ok` / `?patreon=creator_ok`

The Settings flow is implemented so that we can support a future generic shape:

- `?fanclub=<providerId>&result=<code>`

The resolver currently lives in `web/src/composables/useSecuritySettings.ts`.

## i18n keys

- Settings header (generic): `views.settings.sections.fanclubLinks`
- Provider selector label: `views.settings.security.fanclubs.providerLabel`
- Note editor labels:
  - `views.noteEdit.paywallProviderLabel`
  - `views.noteEdit.paywallProviderNone`

Provider-specific strings should live under a provider namespace, e.g.:

- `views.settings.security.patreon.*`

## Checklist

- [ ] Add provider entry in `fanclubProviderRegistry`
- [ ] Add provider Settings panel component
- [ ] Add provider branch in `useFanclubLinks`
- [ ] Mount panel in `SecuritySettingsPanel`
- [ ] Add note editor fields component (if needed)
- [ ] Add i18n strings (ja/en)
- [ ] Ensure `npm run build` succeeds

