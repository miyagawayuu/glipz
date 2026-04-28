# Setup Guide

This guide walks you through setting up Glipz locally — from environment configuration to running both backend and frontend.

For a quick overview, see [README.md](README.md).
For production deployment, see [DEPLOY.md](DEPLOY.md).

---

## Prerequisites

Install these before starting:

| Tool | Required | Notes |
|------|----------|-------|
| **Docker & Docker Compose** | Yes | Runs PostgreSQL, Redis, Mailpit, Backend |
| **Node.js 22+** | Yes (frontend) | Matches `web/package.json` `engines`; Docker image uses Node 22 |
| **npm** | Yes (frontend) | Comes with Node.js |
| **Go 1.26.2+** | No | Only if running backend outside Docker |
| **Media storage** | Yes | Server-local folder or S3-compatible storage (Cloudflare R2, Wasabi, MinIO, AWS S3, etc.) |

---

## Step 1: Clone the Repository

```bash
git clone https://github.com/miyagawayuu/glipz.git
cd glipz
```

---

## Step 2: Configure Environment

Copy the template and edit it:

```bash
# macOS / Linux
cp .env.example .env

# Windows PowerShell
Copy-Item .env.example .env
```

### Required Variables

At minimum, set these in `.env`:

```env
# Generate with: openssl rand -base64 48
JWT_SECRET=

# Either use a server-local folder:
GLIPZ_STORAGE_MODE=local
GLIPZ_LOCAL_STORAGE_PATH=./data/media

# Or use S3-compatible storage:
# GLIPZ_STORAGE_MODE=s3
# S3_ENDPOINT=https://s3.your-region.wasabisys.com
# S3_PUBLIC_ENDPOINT=https://s3.your-region.wasabisys.com
# S3_REGION=your-region
# S3_ACCESS_KEY=your-access-key
# S3_SECRET_KEY=your-secret-key
# S3_BUCKET=your-bucket
# S3_USE_PATH_STYLE=false
```

### Recommended Variables

```env
FRONTEND_ORIGIN=http://localhost:5173
GLIPZ_VERSION=dev
```

`GLIPZ_VERSION` is optional. When it is omitted, federation metadata exposes the
app version synced from `web/package.json`; set it only when you want a runtime
override such as `dev`, a release tag, or a short Git SHA.

### Admin users (optional)

Comma-separated user UUIDs for built-in admin and moderation UIs:

```env
GLIPZ_ADMIN_USER_IDS=
```

Users listed here can open `/admin`. Runtime instance settings edited in that
panel are stored in the database; environment variables such as
`FEDERATION_POLICY_SUMMARY` still provide the initial/default value.

### Federation (Optional)

```env
GLIPZ_PROTOCOL_PUBLIC_ORIGIN=http://localhost:8080
GLIPZ_PROTOCOL_HOST=localhost:8080
GLIPZ_PROTOCOL_MEDIA_PUBLIC_BASE=http://localhost:8080/api/v1/media/object
# FEDERATION_POLICY_SUMMARY=Short text shown as your instance federation policy
```

These values enable Glipz Federation Protocol discovery at
`/.well-known/glipz-federation` and advertise the local `/federation/*`
endpoints. The reference server uses `glipz-federation/3` for new peers while
retaining older protocol versions for compatibility. Mutating federation
requests are signed with Ed25519 `X-Glipz-*` headers and use nonces plus
`event_id` values for replay protection.

In production, use HTTPS origins and a stable public host. Set
`GLIPZ_PROTOCOL_PUBLIC_ORIGIN` explicitly when the API/federation origin differs
from the frontend origin. The instance signing key is derived from `JWT_SECRET`,
so treat `JWT_SECRET` as stable production configuration after federation is
enabled.

For production, prefer the backend media proxy (`GLIPZ_MEDIA_PROXY_MODE=proxy`).
It forces active content types such as SVG, HTML, XML, and JavaScript to download
instead of rendering inline. If you switch to direct object-storage or CDN media
delivery, configure that endpoint to apply equivalent `Content-Disposition: attachment`
and `X-Content-Type-Options: nosniff` behavior for those types.

### Patreon fan club (Optional)

Patreon is disabled by default. Register an API client at Patreon, then enable it explicitly (redirect URI must match your deployment; see comments in [.env.example](.env.example)):

```env
PATREON_ENABLED=true
PATREON_CLIENT_ID=
PATREON_CLIENT_SECRET=
# PATREON_REDIRECT_URI=http://localhost:8080/api/v1/fanclub/patreon/callback
```

### Production web build

For `npm run build`, copy [web/.env.production.example](web/.env.production.example) to `web/.env.production` when you need `VITE_API_URL` (cross-origin API or Capacitor). Same-origin deployments can usually omit it.

---

## Step 3: Start the Backend Stack

```bash
docker compose up --build
```

This compose stack is for local development only. It uses fixed development
credentials and localhost-bound ports; do not reuse it as-is for production.

This starts:

| Service | URL |
|---------|-----|
| **Backend API** | http://localhost:8080 |
| **PostgreSQL** | localhost:5432 |
| **Redis** | localhost:6379 |
| **Mailpit (SMTP)** | localhost:1025 |
| **Mailpit (Web)** | http://localhost:8025 |

On first startup, the backend:
- Connects to PostgreSQL and Redis
- Runs database migrations, including ID portability transfer tables and
  bookmark/follow portability support, community tables / `posts.group_id`, and
  profile pinned-post support for existing databases
- Initializes the configured media store (`local` folder or S3-compatible storage)
- Starts the HTTP server

---

## Step 4: Start the Frontend

Open a new terminal:

```bash
cd web
npm install
npm run dev
```

Frontend: http://localhost:5173

The Vite dev server binds to `127.0.0.1` by default. If you intentionally need
LAN access from another device, start it with `VITE_DEV_HOST=0.0.0.0 npm run dev`
and only do so on a trusted network.

Vite proxies these routes to the backend (override backend host with `VITE_PROXY_TARGET` if needed, for example when the API runs only inside Docker):

- `/api` → Backend API (including SSE: feed, notifications, DMs)
- `/.well-known` → Federation discovery
- `/federation` → Glipz Federation endpoints

---

## Step 5: Verify Everything Works

| Check | URL | Expected |
|-------|-----|----------|
| Backend health | http://localhost:8080/health | `ok` |
| Frontend | http://localhost:5173 | App loads |
| Mailpit | http://localhost:8025 | Web UI |
| Communities | http://localhost:5173/communities | Community directory loads |

---

## Running Tests

**Backend:**

```bash
cd backend
go test ./...
```

**Frontend:**

```bash
cd web
npm test
npm run build
```

---

## Optional: Serve Frontend from Backend

Instead of running the dev server, serve the built frontend directly:

1. Build the frontend:
   ```bash
   cd web
   npm install
   npm run build
   ```

2. Add to `.env`:
   ```env
   STATIC_WEB_ROOT=../web/dist
   ```

3. Restart the backend.

---

## Optional: Customize Legal Documents

To let the server operator update public policy pages without rebuilding the
frontend, set a Markdown document directory:

```env
LEGAL_DOCS_DIR=./data/legal-docs
```

Supported files are `terms.md`, `privacy.md`, and `nsfw-guidelines.md`.
Locale-specific files such as `terms.ja.md` or `terms.en.md` take precedence.
Use `legal-docs.example/` as starter content. Missing files fall back to the
built-in Glipz policy text.

---

## Running Backend Without Docker

If you have PostgreSQL and Redis already running:

```bash
cd backend
go run ./cmd/server
```

Ensure these are set:
- `DATABASE_URL`
- `REDIS_URL`
- `JWT_SECRET`
- `GLIPZ_STORAGE_MODE=local` with a writable `GLIPZ_LOCAL_STORAGE_PATH`, or `GLIPZ_STORAGE_MODE=s3` with the required `S3_*` variables

---

## Optional Integrations

These are disabled unless configured:

| Feature | Environment Variables |
|---------|----------------------|
| **Web Push** | `WEB_PUSH_VAPID_*` |
| **Federation** | `GLIPZ_PROTOCOL_*`, optional `FEDERATION_POLICY_SUMMARY` |
| **Legal documents** | `LEGAL_DOCS_DIR` |
| **Patreon** | `PATREON_ENABLED=true`, `PATREON_CLIENT_ID`, `PATREON_CLIENT_SECRET`, optional `PATREON_REDIRECT_URI` |

See [.env.example](.env.example) for all options.

---

## Troubleshooting

### Backend exits immediately

- `.env` file exists
- `JWT_SECRET` is set
- `GLIPZ_STORAGE_MODE` is either `local` or `s3`
- For local storage, `GLIPZ_LOCAL_STORAGE_PATH` is writable by the backend
- For S3 storage, S3 variables are set and the bucket is reachable from Docker

### Frontend loads but API fails

- Backend is running on port 8080
- `docker compose up` completed without errors
- No custom `VITE_PROXY_TARGET` overriding the default

### Docker build sees missing Go methods or old files

- Re-run `docker compose build backend` after confirming `go test ./internal/...`
  passes locally.
- If using Jujutsu (`jj`), make sure the `main` bookmark points at the commit
  containing all related files, not an older partial commit. `jj status` should
  be clean before building an image.
- Build context should include the repository files. If Docker reports a tiny
  context and compile errors for missing methods, check `.dockerignore` and the
  current `jj` / Git checkout.

### Media upload fails

- For local storage, the directory exists or can be created by the backend process
- For local storage in Docker Compose, `./data/media` is mounted into the backend container
- For S3 storage, credentials, bucket name, endpoint URL, and `S3_USE_PATH_STYLE` match your provider

### Federation not working

- `GLIPZ_PROTOCOL_PUBLIC_ORIGIN` is set
- `GLIPZ_PROTOCOL_HOST` is set
- `GLIPZ_PROTOCOL_MEDIA_PUBLIC_BASE` is set
- `/.well-known/glipz-federation` returns a discovery document with `glipz-federation/3`, `events_url`, `follow_url`, and `unfollow_url`
- `/federation/events`, `/federation/follow`, and `/federation/unfollow` are reachable through the reverse proxy
- Public federation uses HTTPS and stable hostnames outside local development
- Reverse proxy buffering is disabled for SSE endpoints listed earlier in this guide

### Patreon OAuth errors after deploy

- `PATREON_ENABLED=true` is set
- Redirect URI in the Patreon developer console exactly matches `PATREON_REDIRECT_URI` or `{GLIPZ_PROTOCOL_PUBLIC_ORIGIN}/api/v1/fanclub/patreon/callback`
- `GLIPZ_PROTOCOL_PUBLIC_ORIGIN` uses HTTPS in production

### Patreon UI is missing

- The related provider flag is enabled: `PATREON_ENABLED=true`
- Credential-backed providers also have credentials set (`PATREON_*`)
- Restart the backend after changing `.env`, then reload the frontend

### Emails not sending

- Open Mailpit at http://localhost:8025
- Local development uses Mailpit, not a real mail provider

---

## Production Checklist

Before going live:

- [ ] Strong `JWT_SECRET` (use a random generator)
- [ ] HTTPS configured (reverse proxy)
- [ ] Media storage configured (`GLIPZ_STORAGE_MODE=local` with a backed-up folder, or `GLIPZ_STORAGE_MODE=s3` with valid S3 credentials)
- [ ] `FRONTEND_ORIGIN` set to your public URL
- [ ] `GLIPZ_PROTOCOL_PUBLIC_ORIGIN` set (if using federation)
- [ ] `GLIPZ_PROTOCOL_HOST` and `GLIPZ_PROTOCOL_MEDIA_PUBLIC_BASE` set (if using federation)
- [ ] Database and Redis secured
- [ ] Real email provider configured (Mailgun, etc.)
- [ ] Optional providers explicitly enabled only when configured (`PATREON_ENABLED`)
- [ ] License file added (see LICENSE)

---

## Related Documentation

- [README.md](README.md) — Project overview
- [DEPLOY.md](DEPLOY.md) — Production deployment
- [.env.example](.env.example) — All configuration options
- [LICENSE](LICENSE) — AGPLv3 license

---

### Federation Setup (Optional)
To enable Glipz Federation Protocol between instances, configure these variables
in your `.env`:

| Variable | Description |
|----------|-------------|
| `GLIPZ_PROTOCOL_PUBLIC_ORIGIN` | Public API/federation origin used for discovery and endpoint URLs (for example, `https://api.social.example`) |
| `GLIPZ_PROTOCOL_HOST` | Stable federation host advertised to peers (for example, `social.example`) |
| `GLIPZ_PROTOCOL_MEDIA_PUBLIC_BASE` | Base URL for federated media assets, usually ending in `/api/v1/media/object` unless using direct CDN delivery |

> **Note:** Federation between public instances expects HTTPS and stable
> hostnames. Glipz federation is not ActivityPub; proxy `/.well-known/glipz-federation`
> and `/federation/*`, and do not rely on `/ap` shared-inbox delivery for
> interoperability. Signing keys are managed by the server and are tied to
> stable production configuration.

For **adding another fan-club provider** (not Patreon-specific), follow [backend/internal/fanclub/kernel/IMPLEMENTATION_GUIDELINES.md](backend/internal/fanclub/kernel/IMPLEMENTATION_GUIDELINES.md).