# Glipz

<p align="center">
  <strong>A modern social platform for independent communities & federation protocol</strong><br>
  <em>Built for communities that value privacy, control, and secure data synchronization</em>
</p>

---

## What is Glipz?

Glipz is a **social platform for independently operated communities** and a **high-performance federation protocol** (glipz-federation/2). 

Unlike generic protocols, Glipz is designed for speed, security (Ed25519), and optional gated post media (Unlock). It serves as both a full-featured social network and a reference implementation for the Glipz Federation Protocol.

### The Glipz Federation Protocol
This repository contains the official Go implementation of the Glipz Federation Protocol. Key features include:
- **High-Speed Sync:** Event-driven architecture for near-instant data propagation.
- **Strong Security:** Ed25519 signatures and mandatory nonce-based replay protection.
- **Gated post media:** Optional view password or membership-based unlock over federation.

### Who is Glipz for?

- **Community builders** who want a private, customizable social space
- **Creators** who want optional password- or membership-protected media on posts (Unlock; **Patreon** OAuth or **Gumroad** product license)
- **Developers** who need a flexible API for building custom frontends
- **Independent operators** who want to run their own instance or managed infrastructure

---

## Features

### Core Social Features

| Feature | Description |
|---------|-------------|
| **Timelines** | Home, local, and federated timelines |
| **Posts** | Text, media, polls, scheduled publishing; optional view password, Patreon / Gumroad membership gate, or PayPal subscription paywall on media |
| **Replies & Threads** | Full threaded conversations |
| **Reposts** | Share posts with optional commentary |
| **Reactions** | Emoji reactions on posts |
| **Bookmarks** | Save posts for later |
| **Visibility** | Public, logged-in-only, followers-only, and private posts |

### Direct Messages

- End-to-end encrypted identity setup
- File and media sharing
- Voice and video calls (P2P WebRTC with TURN)

### Customization

- Custom emoji support
- User badges and verification
- Theme-ready frontend

### Federation

- **Glipz Protocol**: Lightweight federation between Glipz instances
- Remote follow support
- Inbound federation timeline and federated direct messages (instance-to-instance)
- Delivery workers for reliable delivery
- Optional instance policy summary for operators (`FEDERATION_POLICY_SUMMARY`)

### Media

- Media storage in a local server folder or S3-compatible storage (Cloudflare R2, Wasabi, MinIO, AWS S3, etc.)
- Backend media proxy for privacy
- Post attachments: images (up to four per post), single video, or single audio; web UI uses custom video/audio players (theme-aware)

### Fan club (Patreon, Gumroad; optional)

- **Patreon:** link your campaign via OAuth; configure `PATREON_*` in `.env` (see [.env.example](.env.example)); callback path is documented there.
- **Gumroad:** lock a post to a [Gumroad](https://gumroad.com) product by ID; viewers unlock by entering a valid license key. The server verifies keys against [Gumroad’s license API](https://gumroad.com/api#licenses)—no `GUMROAD_*` secrets are required in `.env` (see comments in [.env.example](.env.example)).
- **Federation:** remote instances cannot mint `entitlement_jwt` for Patreon- or Gumroad-locked posts (returns `501` with `federation_membership_entitlement_unsupported`); unlock those memberships on the **origin** instance or with password-based unlock when applicable.
- Other membership platforms (e.g. SubscribeStar, Ko-fi, Fansly, Ci-en, pixiv FANBOX, Fantia) are not integrated: most lack a stable, third-party–safe API to verify a viewer’s subscription in real time, or are unsuitable for server-side checks under Glipz’s model.

### Payments (PayPal subscriptions; optional)

- Glipz also supports **user-to-user (non-custodial) paywalls** under `internal/payment/…`.
- **PayPal (subscriptions):** creators register a PayPal `plan_id` (created on PayPal) and can lock posts behind an active subscription. The server validates PayPal webhooks and mints short-lived unlock entitlements for viewers.
- Configure `PAYPAL_*` in `.env` (see [.env.example](.env.example)). PayPal approvals return through `{GLIPZ_PROTOCOL_PUBLIC_ORIGIN}/api/v1/payment/paypal/subscription/return`; webhooks use `{GLIPZ_PROTOCOL_PUBLIC_ORIGIN}/api/v1/payment/paypal/webhook`.

### Developer Features

- OAuth 2.0 client support
- Personal access tokens
- RESTful API (`/api/v1/…`)
- In-app OpenAPI reference (Scalar) for exploring endpoints

### Administration

- User moderation and reports
- Domain blocking
- Federation delivery monitoring
- Instance statistics

---

## Screenshots

![Home timeline](https://i.imgur.com/NxHY0rW.png)

---

## Tech Stack

| Layer | Technology |
|-------|------------|
| **Backend** | Go 1.22, Chi router, pgx, Redis |
| **Frontend** | Vue 3, TypeScript, Vite, Tailwind CSS, vue-i18n (en / ja) |
| **Database** | PostgreSQL 16 |
| **Cache** | Redis 7 |
| **Storage** | Local server folder or S3-compatible storage (Cloudflare R2, Wasabi, MinIO, etc.) |
| **Mobile (optional)** | Capacitor 7 (Android / iOS) |
| **Deployment** | Docker, Docker Compose (image builds Node 22 + Go 1.22) |

---

## Quick Start

### Prerequisites

- Docker & Docker Compose
- Node.js 22+ (for frontend development; matches `web/package.json` engines)
- Go 1.22+ (optional, for backend development outside Docker)
- Media storage: either a server-local folder or an S3-compatible bucket

### 1. Clone and configure

```bash
git clone https://github.com/miyagawayuu/glipz.git
cd glipz
cp .env.example .env
```

Edit `.env` with your settings. At minimum:

```env
JWT_SECRET=your-secure-random-secret

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

Cloudflare R2 uses `S3_ENDPOINT=https://<account-id>.r2.cloudflarestorage.com`, `S3_REGION=auto`, and path-style access. For direct media delivery, set `GLIPZ_PROTOCOL_MEDIA_PUBLIC_BASE` to your R2 custom public domain and use `GLIPZ_MEDIA_PROXY_MODE=direct`.

### 2. Start the stack

```bash
docker compose up --build
```

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

---

## Deployment

For production deployment, see [DEPLOY.md](DEPLOY.md).

Mailpit (started with the Docker stack) is for local development. In production, use Mailgun, SMTP, or another real mail provider. The linked guide covers a production-focused checklist and deployment flow.

### Production checklist

- [ ] Strong `JWT_SECRET`
- [ ] HTTPS via reverse proxy (Nginx, Caddy, Traefik)
- [ ] Media storage configured (`GLIPZ_STORAGE_MODE=local` or S3-compatible storage)
- [ ] `FRONTEND_ORIGIN` and (if federation) `GLIPZ_PROTOCOL_*` variables set
- [ ] Database and Redis secured
- [ ] Email provider configured (Mailgun, etc.)
- [ ] `GLIPZ_ADMIN_USER_IDS` set if you need built-in admin UIs
- [ ] Patreon fan club (if used): `PATREON_CLIENT_ID`, `PATREON_CLIENT_SECRET`, and matching redirect URI
- [ ] Gumroad (if used): no extra instance secrets—ensure creators know the product ID and that viewers use valid license keys
- [ ] PayPal payments (if used): `PAYPAL_CLIENT_ID`, `PAYPAL_CLIENT_SECRET`, `PAYPAL_WEBHOOK_ID`, and `PAYPAL_ENV`

---

## API

The backend exposes a REST API at `/api/v1/`. Use the in-app **API / OpenAPI** screen for an interactive catalog, or browse handlers under `backend/internal/httpserver/`.

### Authentication

- Email + password login
- JWT-based sessions
- Optional TOTP MFA

### Example: Get home timeline

```bash
curl -H "Authorization: Bearer $TOKEN" \
  https://your-instance.com/api/v1/posts/feed/home
```

### Example: Post unlock (password / membership entitlement)

Posts can carry a view password and/or a membership lock. **Unlock** reveals protected media/caption for that post:

- **Password unlock**: viewer enters a password.
- **Membership unlock (federation)**: viewer requests a short-lived, verifiable `entitlement_jwt` (JWS) from the origin instance and uses it to unlock.

#### Local post unlock (password)

```bash
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  https://your-instance.com/api/v1/posts/$POST_ID/unlock \
  -d '{"password":"your-password"}'
```

#### Local post unlock (Gumroad license)

```bash
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  https://your-instance.com/api/v1/fanclub/gumroad/entitlement \
  -d '{"post_id":"'"$POST_ID"'","license_key":"your-gumroad-license"}'
```

#### Federated incoming post unlock (membership, one-click)

If a federated incoming post is membership-locked and the lock type supports cross-instance `entitlement_jwt` minting, the web app can unlock it without a password.

Under the hood, the viewer instance does:

1. `POST {unlock_url_without_suffix}/entitlement` (federation-signed) to obtain `entitlement_jwt`
2. `POST unlock_url` with `entitlement_jwt`

**Patreon and Gumroad:** the origin will respond with `501` and `federation_membership_entitlement_unsupported`—external membership must be proven on the **origin** instance (e.g. Patreon connect flow or Gumroad license verification there), not via a remote node minting JWTs.

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

Membership entitlement over Glipz federation (`POST .../federation/posts/{postID}/entitlement`) is allowed for any caller that passes `verifyFederationRequest` (valid instance discovery + signature) and whose `ViewerAcct` host matches `X-Glipz-Instance`, **except** where the post is locked to Patreon or Gumroad (see above).

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
| `GLIPZ_PROTOCOL_*` | Federation / discovery / media URLs | Optional |
| `GLIPZ_METRICS_ENABLED` | Exposes lightweight expvar metrics at `/debug/vars` | Optional |
| `GLIPZ_ACCESS_LOG_ENABLED` | Enables per-request access logs; disabled by default for throughput | Optional |
| `GLIPZ_SLOW_REQUEST_LOG_MS` | Logs HTTP requests over this threshold in ms; `0` disables slow request logs | Optional |
| `GLIPZ_FEED_PAGE_SIZE` | Authenticated feed items returned per request; lower values reduce payload size under load | Optional |
| `GLIPZ_MEDIA_PROXY_MODE` | `proxy` streams media through the API; `direct` redirects to configured public media URLs | Optional |
| `GLIPZ_FEDERATION_DELIVERY_*` | Batch size, concurrency, and tick interval for outbound federation delivery | Optional |
| `GLIPZ_ADMIN_USER_IDS` | Comma-separated user UUIDs with site admin | Optional |
| `PATREON_*` | Patreon OAuth for fan club features | Optional |
| (none) | Gumroad: license checks use Gumroad’s public API; no Glipz env vars | — |
| `PAYPAL_*` | PayPal subscriptions for payment paywalls | Optional |
| `TURN_*` | WebRTC TURN credentials for DM calls | Optional |
| `WEB_PUSH_VAPID_*` | Web Push (VAPID) keys | Optional |

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

---

## Support

- Open an issue for bugs or feature requests
- Check SETUP.md for troubleshooting
- Review DEPLOY.md for production guidance
 

---

## License

GNU Affero General Public License v3.0 — see [LICENSE](LICENSE) file.
