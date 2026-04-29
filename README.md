# Glipz

<p align="center">
  <strong>A modern social platform for independent communities & federation protocol</strong><br>
  <em>Built for communities that value privacy, control, and secure data synchronization</em>
</p>

---

## What is Glipz?

Glipz is a **social platform for independently operated communities** and a **high-performance federation protocol** (glipz-federation/3). 

Unlike generic protocols, Glipz is designed for speed, security (Ed25519), and optional gated post media (Unlock). It serves as both a full-featured social network and a reference implementation for the Glipz Federation Protocol.

### The Glipz Federation Protocol
This repository contains the reference Go implementation of the Glipz Federation Protocol. It is a JSON-over-HTTP server-to-server protocol, not ActivityPub. Compatible peers discover each other at `/.well-known/glipz-federation` and deliver signed events to `/federation/events`.

Key features include:
- **Discovery-driven capability negotiation:** peers advertise `glipz-federation/3`, endpoint URLs, schema version, DM key lookup support, and known-instance hints.
- **Strong security:** Ed25519 `X-Glipz-*` signatures with nonce and event ID replay protection.
- **Public social delivery:** remote follows, public posts, reposts, edits, deletes, likes, reactions, poll updates, and federated timeline ingestion.
- **ID portability:** stable portable account IDs, account move events, and stable federated object IDs.
- **Federated DMs and gated media:** signed `dm_*` events, DM public-key lookup, and optional password or membership unlock flows.

### Who is Glipz for?

- **Independent communities** that want a customizable social space under their own domain and rules
- **SNS operators** who want to launch a community-focused social network without depending on a centralized platform
- **Creators and fan communities** that need optional gated posts and memberships
- **Developers** who want a modern Go/Vue social app with REST APIs, OAuth, PATs, and federation hooks
- **Independent operators** who want to run a single-server instance with local media storage or scale out with S3-compatible storage/CDN delivery

---

## Features

### Core Social Features

| Feature | Description |
|---------|-------------|
| **Timelines** | Home, local, and federated timelines |
| **Posts** | Text, media, polls, scheduled publishing; optional view password or Patreon membership gate |
| **Replies & Threads** | Full threaded conversations |
| **Reposts** | Share posts with optional commentary |
| **Reactions** | Emoji reactions on local and federated posts |
| **Bookmarks** | Save posts for later |
| **Visibility** | Public, logged-in-only, followers-only, and private posts |
| **Profile pins** | Pin one top-level profile post so it stays at the top of the profile timeline |

### Communities

- Public community directory with search by name, description, or UUID.
- Community pages include recommended, latest, media-grid, and details tabs.
- Community owners can edit the name, short description, longer details/rules, icon, and header image.
- Join requests are owner-approved; approved members can post to the community from the compose flow.
- Community timelines stay separate from the main/profile timelines via `posts.group_id`; community posts are not shown as normal profile posts.
- Community media uses the same square tile grid as profile media, including locked-media placeholders.
- The community header shows up to five overlapping approved-member avatars and a compact member count.

### Direct Messages

- End-to-end encrypted identity setup with client-side encrypted message bodies
  and attachments
- File and media sharing
- Participant-submitted DM reports for moderation, with optional explicit
  plaintext inclusion by the reporting participant

### Customization

- Custom emoji support
- Site-wide custom emoji management for instance administrators
- User badges and verification, managed from the admin user page
- Theme-ready frontend

### Federation

- **Glipz Federation Protocol v3**: JSON-over-HTTP federation between Glipz-compatible instances
- Discovery at `/.well-known/glipz-federation` with public Ed25519 key, endpoint URLs, protocol versions, and optional DM key endpoint metadata
- Signed server-to-server requests using `X-Glipz-*` headers, nonces, and idempotent `event_id` values
- Remote follow/unfollow support and outbound public event delivery to `/federation/events`
- Inbound federation timeline and federated direct messages (`dm_*` events, instance-to-instance)
- ID portability support with encrypted migration bundles, transfer-token protected data import jobs, account move declarations, portable account IDs, and stable federated object IDs
- Delivery workers for reliable delivery with retries and duplicate-event protection
- Admin-managed federation delivery monitoring, domain blocks, and known instances; known instances are discovery hints, not automatic trust grants
- Database-backed instance settings, including public server metadata and federation policy summary
- Operator-editable Markdown legal pages (`LEGAL_DOCS_DIR`) or admin-configured external legal document URLs
- Law enforcement request policy document support, including
  `law-enforcement.md` in `LEGAL_DOCS_DIR`

### Media

- Media storage in a local server folder or S3-compatible storage (Cloudflare R2, Wasabi, MinIO, AWS S3, etc.)
- Backend media proxy for privacy
- Post attachments: images (up to four per post), single video, or single audio; web UI uses custom video/audio players (theme-aware)

### Fan club (Patreon; optional)

- **Disabled by default:** set `PATREON_ENABLED=true` to expose the related UI and API behavior. Disabled providers are hidden in settings/composer/unlock UI and rejected server-side.
- **Patreon:** link your campaign via OAuth; configure `PATREON_ENABLED=true` plus `PATREON_CLIENT_ID` / `PATREON_CLIENT_SECRET` in `.env` (see [.env.example](.env.example)); callback path is documented there.
- **Federation:** Patreon-locked federated posts can be unlocked from the viewer instance when that instance has Patreon enabled and the viewer has connected Patreon there. The viewer instance verifies the campaign/tier with Patreon and sends a short-lived `entitlement_jwt` to the origin unlock endpoint.
- Other membership platforms (e.g. SubscribeStar, Ko-fi, Fansly, Ci-en, pixiv FANBOX, Fantia) are not integrated: most lack a stable, third-party–safe API to verify a viewer’s subscription in real time, or are unsuitable for server-side checks under Glipz’s model.

### Developer Features

- OAuth 2.0 client support
- Personal access tokens
- RESTful API (`/api/v1/…`)
- In-app OpenAPI reference (Scalar) for exploring endpoints

### Administration

- Dedicated `/admin` control panel with its own fixed side menu and admin-only access
- Dashboard with instance statistics, open reports, and federation queue status
- User search, suspension/unsuspension, and user badge assignment from the user management page
- Local, federated post, and DM report review with legal/safety categories for
  priority triage
- Federation delivery monitoring, domain blocking, and known-instance management
- Law enforcement request tracking, legal preservation holds, manifest-hashed
  disclosure package export, and sensitive admin audit logging
- Site custom emoji management
- Runtime instance settings stored in PostgreSQL (`site_settings`), including:
  - registrations enabled/disabled
  - minimum account creation age
  - server name and server description
  - administrator name and email address
  - Terms of Service, Privacy Policy, and NSFW Guidelines external URLs
  - federation policy summary
  - operator announcements shown in the normal UI

---

## Screenshots

![Home timeline](https://i.imgur.com/NxHY0rW.png)

---

## Tech Stack

| Layer | Technology |
|-------|------------|
| **Backend** | Go 1.26.2, Chi router, pgx, Redis |
| **Frontend** | Vue 3, TypeScript, Vite, Tailwind CSS, vue-i18n (en / ja) |
| **Database** | PostgreSQL 16 |
| **Cache** | Redis 7 |
| **Storage** | Local server folder or S3-compatible storage (Cloudflare R2, Wasabi, MinIO, etc.) |
| **Mobile (optional)** | Capacitor 7 (Android / iOS) |
| **Deployment** | Docker, Docker Compose (image builds Node 22 + Go 1.26.2) |

---

## Quick Start

### Prerequisites

- Docker & Docker Compose
- Node.js 22+ (for frontend development; matches `web/package.json` engines)
- Go 1.26.2+ (optional, for backend development outside Docker)
- Media storage: either a server-local folder or an S3-compatible bucket

### 1. Clone and configure

```bash
git clone https://github.com/miyagawayuu/glipz.git
cd glipz
cp .env.example .env
```

Edit `.env` with your settings. At minimum:

```env
# Generate with: openssl rand -base64 48
JWT_SECRET=

# Simplest single-server setup:
GLIPZ_STORAGE_MODE=local
GLIPZ_LOCAL_STORAGE_PATH=./data/media
```

For S3-compatible storage instead:

```env
GLIPZ_STORAGE_MODE=s3
S3_ENDPOINT=https://s3.your-provider.example
S3_PUBLIC_ENDPOINT=https://s3.your-provider.example
S3_REGION=your-region
S3_ACCESS_KEY=your-access-key
S3_SECRET_KEY=your-secret-key
S3_BUCKET=your-bucket
S3_USE_PATH_STYLE=true-or-false-for-your-provider
```

In local mode, the backend stores uploaded files on disk and serves them from `/api/v1/media/object/*`. With Docker Compose, `./data/media` is mounted into the backend container so uploads survive container rebuilds.

Cloudflare R2 uses `S3_ENDPOINT=https://<account-id>.r2.cloudflarestorage.com`, `S3_REGION=auto`, and path-style access. For direct media delivery, set `GLIPZ_PROTOCOL_MEDIA_PUBLIC_BASE` to your R2 custom public domain and use `GLIPZ_MEDIA_PROXY_MODE=direct`. Direct media endpoints must reject or download active content types such as SVG, HTML, XML, and JavaScript with `Content-Disposition: attachment` and `X-Content-Type-Options: nosniff`; the backend proxy applies this automatically.

### 2. Start the stack

```bash
docker compose up --build
```

This compose stack is for local development only. It uses fixed development
credentials and localhost-bound ports; use [DEPLOY.md](DEPLOY.md) for production.

Services started:
- **Backend API**: http://localhost:8080
- **PostgreSQL**: localhost:5432
- **Redis**: localhost:6379
- **Mailpit** (dev email): http://localhost:8025

### 3. Start the frontend

```bash
cd web
npm install
npm run dev
```

Frontend: http://localhost:5173

The Vite dev server listens on `127.0.0.1` by default. For intentional LAN
testing, set `VITE_DEV_HOST=0.0.0.0` on a trusted network.

---

## Deployment

For production deployment, see [DEPLOY.md](DEPLOY.md).

Mailpit (started with the Docker stack) is for local development. In production, use Mailgun, SMTP, or another real mail provider. The linked guide covers a production-focused checklist and deployment flow.

### Production checklist

- [ ] Strong `JWT_SECRET`
- [ ] HTTPS via reverse proxy (Nginx, Caddy, Traefik)
- [ ] Media storage configured (`GLIPZ_STORAGE_MODE=local` or S3-compatible storage)
- [ ] `FRONTEND_ORIGIN` and (if federation) `GLIPZ_PROTOCOL_*` variables set
- [ ] If trusting proxy headers, backend is private and proxy overwrites `X-Real-IP`, `X-Forwarded-For`, and `X-Forwarded-Proto`
- [ ] Database and Redis secured
- [ ] Email provider configured (Mailgun, SMTP, etc.)
- [ ] `GLIPZ_ADMIN_USER_IDS` set for site administrators who can access `/admin`
- [ ] Patreon fan club (if used): `PATREON_ENABLED=true`, `PATREON_CLIENT_ID`, `PATREON_CLIENT_SECRET`, and matching redirect URI

---

## API

The backend exposes a REST API at `/api/v1/`. Use the in-app **API / OpenAPI** screen for an interactive catalog, or browse handlers under `backend/internal/httpserver/`.

### Authentication

- Email + password login
- JWT-based sessions
- Optional TOTP MFA

OAuth client redirect URIs must be absolute `https://` URLs in production. `http://` is accepted only for `localhost` or loopback IPs during development. Redirect URIs may not include userinfo, fragments, spaces, or control characters.

### Identity portability

Authenticated users can use the migration wizard to move their portable account
identity to another instance, import their profile, post/media history, follow
relationships, and bookmarks, and declare that their account moved to a new
acct.

The migration wizard creates an encrypted identity bundle v2 using a migration
passphrase, issues a short-lived transfer token on the source instance, and
starts a background import job on the target instance. The target instance pulls
the source manifest, profile, post batches, follow graph, bookmarks, and
authorized media sequentially through transfer-token protected endpoints.
Imported historical posts are restored as profile history and are not fanned out
as new `post_created` federation events; once the user confirms the move, the
old instance sends the normal `account_moved` event.

Migrated data:
- Portable identity keys and profile fields (`display_name`, `bio`,
  `also_known_as`, avatar/header media when present)
- Eligible posts, polls, and attached media
- Local and remote following rows that can be resolved safely
- Remote follower/subscriber rows used for Glipz Protocol delivery
- Local bookmarks for migrated posts and federated bookmarks that can be
  resolved on the target

Not migrated:
- Password hashes, TOTP secrets, OAuth clients/tokens, personal access tokens,
  DM threads/messages, notification history, raw IP data, and unrelated account
  security material

Migration security details:
- The encrypted bundle v2 stores the account private key under Argon2id +
  AES-GCM. The passphrase is required on the target instance to import the
  identity.
- Transfer tokens are short-lived, stored server-side only as hashes, encrypted
  when saved in import jobs, and sent to source transfer endpoints as
  `X-Glipz-Transfer-Token`.
- Source transfer endpoints require `X-Glipz-Target-Origin` to match the
  `target_origin` authorized when the source transfer session was created. If
  the wizard creates a session for `http://localhost:5173` but the import job
  runs from `https://example.com`, the source returns `401 Unauthorized`.
- `http://` origins are accepted only for `localhost` or loopback IPs during
  development. Public instances should use `https://` origins. Origins must not
  include path, query, fragment, or userinfo.
- Remote source URLs are checked before fetches to reduce SSRF risk; private,
  loopback, link-local, unspecified, and multicast remote addresses are
  rejected unless the origin is explicitly local development.
- Follow and bookmark imports are best-effort and idempotent. Rows that cannot
  be resolved or safely revalidated on the target are skipped and reflected in
  import job `stats`.
- Import job progress includes both legacy `total_posts` / `imported_posts` and
  aggregate `total_items` / `imported_items` plus category `stats` for profile,
  posts, following, followers, and bookmarks.

Secure migration wizard APIs:

```bash
# Source instance: create an encrypted bundle for the target origin.
curl -X POST -H "Authorization: Bearer $SOURCE_TOKEN" \
  -H "Content-Type: application/json" \
  https://your-old-instance.com/api/v1/me/identity/export-secure \
  -d '{"passphrase":"long migration passphrase","target_origin":"https://your-new-instance.com"}'

# Source instance: create a data transfer session and one-time transfer token.
curl -X POST -H "Authorization: Bearer $SOURCE_TOKEN" \
  -H "Content-Type: application/json" \
  https://your-old-instance.com/api/v1/me/identity/transfer-sessions \
  -d '{"target_origin":"https://your-new-instance.com","include_private":false,"include_gated":false}'

# Target instance: import the encrypted identity bundle.
curl -X PUT -H "Authorization: Bearer $TARGET_TOKEN" \
  -H "Content-Type: application/json" \
  https://your-new-instance.com/api/v1/me/identity/import-secure \
  -d '{"bundle":{...},"passphrase":"long migration passphrase"}'

# Target instance: start the background data import job.
curl -X POST -H "Authorization: Bearer $TARGET_TOKEN" \
  -H "Content-Type: application/json" \
  https://your-new-instance.com/api/v1/me/identity/import-jobs \
  -d '{"source_origin":"https://your-old-instance.com","target_origin":"https://your-new-instance.com","source_session_id":"...","token":"...","include_private":false,"include_gated":false}'
```

### Example: Get home timeline

```bash
curl -H "Authorization: Bearer $TOKEN" \
  https://your-instance.com/api/v1/posts/feed
```

### Example: Communities

Community reads are public. Mutations require authentication.

```bash
# List communities.
curl https://your-instance.com/api/v1/communities

# Read a community timeline.
curl https://your-instance.com/api/v1/communities/$COMMUNITY_ID/posts

# Read profile-style media tiles for a community.
curl https://your-instance.com/api/v1/communities/$COMMUNITY_ID/post-media-tiles

# Create a community. The creator becomes the owner.
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  https://your-instance.com/api/v1/communities \
  -d '{"name":"Artists","description":"Short intro","details":"Rules and links"}'

# Request to join.
curl -X POST -H "Authorization: Bearer $TOKEN" \
  https://your-instance.com/api/v1/communities/$COMMUNITY_ID/join-requests
```

Community posts use the normal post-create API with `community_id` in the JSON
body. The backend requires approved membership before accepting that field.

### Example: Post unlock (password / membership entitlement)

Posts can carry a view password or membership lock. **Unlock** reveals the protected media/caption for that post:

- **Password unlock**: viewer enters a password.
- **Membership unlock (federation)**: viewer obtains a short-lived, verifiable `entitlement_jwt` through the supported provider path and uses it to unlock.

#### Local post unlock (password)

```bash
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  https://your-instance.com/api/v1/posts/$POST_ID/unlock \
  -d '{"password":"your-password"}'
```

#### Federated incoming post unlock (membership)

If a federated incoming post is membership-locked, the viewer instance tries to obtain an `entitlement_jwt` and then calls the origin post's `unlock_url`.

For Patreon, the viewer instance does not ask the origin to verify Patreon directly. Instead, when Patreon is enabled and the viewer connected Patreon on the viewer instance, it verifies the viewer's Patreon campaign/tier locally against Patreon's API and mints an `entitlement_jwt` for the origin post.

For other federation membership providers, the viewer instance may ask the origin for an entitlement token only when the origin can safely verify the remote viewer's membership:

1. `POST {unlock_url_without_suffix}/entitlement` (federation-signed) to obtain `entitlement_jwt`
2. `POST unlock_url` with `entitlement_jwt`

From a client, you can simply call the viewer-instance API:

```bash
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  https://your-viewer-instance.com/api/v1/federation/posts/$INCOMING_ID/unlock \
  -d '{}'
```

#### (PoC) Issue an entitlement JWT as site admin

For debugging/PoC you can mint `entitlement_jwt` on the origin instance as a site admin:

```bash
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  https://your-origin-instance.com/api/v1/admin/federation/entitlements \
  -d '{"post_id":"'"$POST_ID"'","viewer_acct":"alice@viewer.example"}'
```

Membership entitlement over Glipz federation (`POST .../federation/posts/{postID}/entitlement`) requires a valid Glipz federation signature, a `ViewerAcct` host that matches `X-Glipz-Instance`, and origin policy approval. If the origin cannot safely verify a remote viewer's external membership, it should reject entitlement minting. Patreon uses the viewer-instance verification path described above and is not minted by the origin entitlement endpoint.

---

## Configuration

| Variable | Description | Required |
|----------|-------------|----------|
| `JWT_SECRET` | Secret for JWT signing | Yes |
| `DATABASE_URL` | PostgreSQL connection string | Provided by Compose when using Docker |
| `REDIS_URL` | Redis connection string | Provided by Compose when using Docker |
| `GLIPZ_STORAGE_MODE` | `s3` for S3-compatible storage or `local` for server-local folder storage | Optional |
| `GLIPZ_LOCAL_STORAGE_PATH` | Folder used when `GLIPZ_STORAGE_MODE=local`; default is `data/media` | Required for local storage |
| `S3_*` | S3 storage configuration | Required when `GLIPZ_STORAGE_MODE=s3` |
| `FRONTEND_ORIGIN` | Frontend origin(s) for CORS; comma-separated if apex + www | Recommended |
| `GLIPZ_PROTOCOL_PUBLIC_ORIGIN` | Public API/federation origin advertised in discovery; falls back to `FRONTEND_ORIGIN` when empty | Recommended for federation |
| `GLIPZ_PROTOCOL_HOST` | Stable federation host advertised to peers | Recommended for federation |
| `GLIPZ_PROTOCOL_MEDIA_PUBLIC_BASE` | Public base URL for federated media URLs | Recommended for federation |
| `GLIPZ_METRICS_ENABLED` | Exposes lightweight expvar metrics at `/debug/vars` | Optional |
| `GLIPZ_ACCESS_LOG_ENABLED` | Enables per-request access logs; disabled by default for throughput | Optional |
| `GLIPZ_SLOW_REQUEST_LOG_MS` | Logs HTTP requests over this threshold in ms; `0` disables slow request logs | Optional |
| `GLIPZ_TRUST_PROXY_HEADERS` | Trusts reverse-proxy client IP / scheme headers; enable only behind a proxy that overwrites them | Optional |
| `GLIPZ_AUTH_RATE_LIMIT_FAIL_CLOSED` | Rejects login/MFA attempts when Redis-backed auth/SSE rate limit checks fail | Optional |
| `GLIPZ_FEED_PAGE_SIZE` | Authenticated feed items returned per request; lower values reduce payload size under load | Optional |
| `GLIPZ_MEDIA_PROXY_MODE` | `proxy` streams media through the API and applies media safety headers; `direct` redirects safe media to configured public media URLs | Optional |
| `GLIPZ_REMOTE_MEDIA_PROXY_MAX_BYTES` | Maximum bytes streamed by the public remote-media proxy; default is 50 MiB | Optional |
| `GLIPZ_REMOTE_MEDIA_PROXY_RATE_LIMIT_MAX` | Public remote-media proxy requests allowed per IP per 15 minutes; default is 120 | Optional |
| `GLIPZ_REMOTE_MEDIA_PROXY_RATE_LIMIT_FAIL_CLOSED` | Rejects remote-media proxy requests when Redis-backed rate limit writes fail | Optional |
| `GLIPZ_LINK_PREVIEW_RATE_LIMIT_MAX` | Public link-preview requests allowed per IP/user per 15 minutes; default is 60 | Optional |
| `GLIPZ_LINK_PREVIEW_RATE_LIMIT_FAIL_CLOSED` | Rejects link-preview requests when Redis-backed rate limit writes fail | Optional |
| `GLIPZ_FEDERATION_INBOX_RATE_LIMIT_FAIL_CLOSED` | Rejects federation inbox POSTs when Redis-backed rate limit writes fail | Optional |
| `GLIPZ_FEDERATION_DM_ATTACHMENT_MAX_BYTES` | Maximum bytes streamed by the federated DM attachment proxy; default is 50 MiB | Optional |
| `GLIPZ_FEDERATION_DELIVERY_*` | Batch size, concurrency, and tick interval for outbound federation delivery | Optional |
| `GLIPZ_ADMIN_USER_IDS` | Comma-separated user UUIDs with site admin access to `/admin` | Optional |
| `VITE_ALLOWED_MEDIA_BASE_URLS` | Frontend allowlist for CDN/direct media URL prefixes used in rich media rendering | Optional |
| `VITE_ALLOWED_DM_ATTACHMENT_BASE_URLS` | Frontend allowlist for CDN/direct encrypted DM attachment URL prefixes | Optional |
| `PATREON_ENABLED` | Enables Patreon UI/routes; defaults to disabled | Optional |
| `PATREON_*` | Patreon OAuth credentials for fan club features | Required when Patreon is enabled |
| `MAILGUN_*` / `SMTP_*` | Mail delivery for registration verification emails | Optional |
| `WEB_PUSH_VAPID_*` | Web Push (VAPID) keys | Optional |

Most operational instance settings are editable at runtime from `/admin/instance-settings` and are stored in PostgreSQL (`site_settings`). Environment variables such as `FEDERATION_POLICY_SUMMARY` are still useful as initial/default configuration, but the admin-saved database value is what operators should manage after deployment.

See [.env.example](.env.example) for every variable, including legacy aliases and mail (`MAILGUN_*`, `SMTP_*`).

---

## Development

### Run backend tests

```bash
cd backend
go test ./...
```

### Build frontend

```bash
cd web
npm run build
```

### Serve built frontend from backend

Set `STATIC_WEB_ROOT=../web/dist` in your environment and restart the backend.

### Customize legal documents

Set `LEGAL_DOCS_DIR` to a directory containing `terms.md`, `privacy.md`, and
`nsfw-guidelines.md`. Locale-specific files such as `terms.ja.md` or
`terms.en.md` are used first. See `legal-docs.example/` for starter files.

Alternatively, site administrators can set external Terms of Service, Privacy
Policy, and NSFW Guidelines URLs in `/admin/instance-settings`. When a URL is
configured, the app opens that external document in a new tab; when it is empty,
the built-in `/legal/...` pages continue to be used.

---

## Support

- Open an issue for bugs or feature requests
- Check SETUP.md for troubleshooting
- Review DEPLOY.md for production guidance
 

---

## License

GNU Affero General Public License v3.0 — see [LICENSE](LICENSE) file.
