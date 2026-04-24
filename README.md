# Glipz

<p align="center">
  <strong>A modern, self-hosted social platform & federation protocol</strong><br>
  <em>Built for communities that value privacy, control, and secure data synchronization</em>
</p>

---

## What is Glipz?

Glipz is a **self-hosted social platform** and a **high-performance federation protocol** (glipz-federation/2). 

Unlike generic protocols, Glipz is designed for speed, security (Ed25519), and advanced content control (Unlock feature). It serves as both a full-featured social network and a reference implementation for the Glipz Federation Protocol.

### The Glipz Federation Protocol
This repository contains the official Go implementation of the Glipz Federation Protocol. Key features include:
- **High-Speed Sync:** Event-driven architecture for near-instant data propagation.
- **Strong Security:** Ed25519 signatures and mandatory nonce-based replay protection.
- **Content Monetization:** Built-in "Unlock" flow for password-protected or gated content.

### Who is Glipz for?

- **Community builders** who want a private, customizable social space
- **Creators** who want to monetize content with password-protected or gated content (Unlock)
- **Developers** who need a flexible API for building custom frontends
- **Self-hosters** who prefer running their own infrastructure

---

## Features

### Core Social Features

| Feature | Description |
|---------|-------------|
| **Timelines** | Home, local, and federated timelines |
| **Posts** | Create posts with text, media, polls, and scheduled publishing |
| **Replies & Threads** | Full threaded conversations |
| **Reposts** | Share posts with optional commentary |
| **Reactions** | Emoji reactions on posts |
| **Bookmarks** | Save posts for later |
| **Visibility** | Public, logged-in-only, followers-only, and private posts |

### Notes (Premium Content)

- Create exclusive content for supporters
- Password-protected access control via the built-in Unlock flow
- Public / follower-only / private visibility

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

- S3-compatible storage (Wasabi, MinIO, AWS S3, etc.)
- Backend media proxy for privacy
- Image and video support

### Fan club (Patreon, optional)

- Creators can link Patreon via OAuth for membership-aware flows
- Configure `PATREON_*` in `.env` (see [.env.example](.env.example)); callback path is documented there

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
| **Storage** | S3-compatible (Wasabi, MinIO, etc.) |
| **Mobile (optional)** | Capacitor 7 (Android / iOS) |
| **Deployment** | Docker, Docker Compose (image builds Node 22 + Go 1.22) |

---

## Quick Start

### Prerequisites

- Docker & Docker Compose
- Node.js 22+ (for frontend development; matches `web/package.json` engines)
- Go 1.22+ (optional, for backend development outside Docker)
- S3-compatible storage bucket

### 1. Clone and configure

```bash
git clone https://github.com/miyagawayuu/glipz.git
cd glipz
cp .env.example .env
```

Edit `.env` with your settings. At minimum:

```env
JWT_SECRET=your-secure-random-secret
S3_ENDPOINT=https://s3.your-region.wasabisys.com
S3_PUBLIC_ENDPOINT=https://s3.your-region.wasabisys.com
S3_REGION=your-region
S3_ACCESS_KEY=your-access-key
S3_SECRET_KEY=your-secret-key
S3_BUCKET=your-bucket
S3_USE_PATH_STYLE=false
```

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
- [ ] S3-compatible storage configured
- [ ] `FRONTEND_ORIGIN` and (if federation) `GLIPZ_PROTOCOL_*` variables set
- [ ] Database and Redis secured
- [ ] Email provider configured (Mailgun, etc.)
- [ ] `GLIPZ_ADMIN_USER_IDS` set if you need built-in admin UIs
- [ ] Patreon fan club: `PATREON_CLIENT_ID`, `PATREON_CLIENT_SECRET`, and matching redirect URI

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

### Example: Unlock (password / membership entitlement)

Glipz supports "Unlock" for protected content:

- **Password unlock**: viewer enters a password.
- **Membership unlock (federation)**: viewer requests a short-lived, verifiable `entitlement_jwt` (JWS) from the origin instance and uses it to unlock.

#### Local post unlock (password)

```bash
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  https://your-instance.com/api/v1/posts/$POST_ID/unlock \
  -d '{"password":"your-password"}'
```

#### Federated incoming post unlock (membership, one-click)

If a federated incoming post is membership-locked, the web app can unlock it without a password.
Under the hood, the viewer instance does:

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

Membership entitlement over Glipz federation (`POST .../federation/posts/{postID}/entitlement`) is allowed for any caller that passes `verifyFederationRequest` (valid instance discovery + signature) and whose `ViewerAcct` host matches `X-Glipz-Instance`.

---

## Configuration

| Variable | Description | Required |
|----------|-------------|----------|
| `JWT_SECRET` | Secret for JWT signing | Yes |
| `DATABASE_URL` | PostgreSQL connection string | Provided by Compose when using Docker |
| `REDIS_URL` | Redis connection string | Provided by Compose when using Docker |
| `S3_*` | S3 storage configuration | Yes (local or cloud bucket) |
| `FRONTEND_ORIGIN` | Frontend origin(s) for CORS; comma-separated if apex + www | Recommended |
| `GLIPZ_PROTOCOL_*` | Federation / discovery / media URLs | Optional |
| `GLIPZ_ADMIN_USER_IDS` | Comma-separated user UUIDs with site admin | Optional |
| `PATREON_*` | Patreon OAuth for fan club features | Optional |
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
