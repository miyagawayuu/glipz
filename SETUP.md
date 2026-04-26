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

### Patreon fan club (Optional)

Patreon is disabled by default. Register an API client at Patreon, then enable it explicitly (redirect URI must match your deployment; see comments in [.env.example](.env.example)):

```env
PATREON_ENABLED=true
PATREON_CLIENT_ID=
PATREON_CLIENT_SECRET=
# PATREON_REDIRECT_URI=http://localhost:8080/api/v1/fanclub/patreon/callback
```

### Gumroad fan club locks (Optional)

Gumroad is disabled by default. It uses Gumroad's public license verification endpoint, so no API secret is required:

```env
GUMROAD_ENABLED=true
```

Creators enter their Gumroad Membership `product_id` in the composer; viewers enter a valid license key to unlock.

### PayPal subscriptions (Optional)

PayPal payment paywalls are disabled by default. Create a REST app in the PayPal Developer Dashboard, configure the webhook URLs shown in [.env.example](.env.example), then enable it explicitly:

```env
PAYPAL_ENABLED=true
PAYPAL_CLIENT_ID=
PAYPAL_CLIENT_SECRET=
PAYPAL_WEBHOOK_ID=
PAYPAL_ENV=sandbox
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
- Runs database migrations
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
- `/ap` → Federation endpoints

---

## Step 5: Verify Everything Works

| Check | URL | Expected |
|-------|-----|----------|
| Backend health | http://localhost:8080/health | `ok` |
| Frontend | http://localhost:5173 | App loads |
| Mailpit | http://localhost:8025 | Web UI |

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
| **Gumroad** | `GUMROAD_ENABLED=true` |
| **PayPal** | `PAYPAL_ENABLED=true`, `PAYPAL_CLIENT_ID`, `PAYPAL_CLIENT_SECRET`, `PAYPAL_WEBHOOK_ID`, `PAYPAL_ENV` |

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

### Media upload fails

- For local storage, the directory exists or can be created by the backend process
- For local storage in Docker Compose, `./data/media` is mounted into the backend container
- For S3 storage, credentials, bucket name, endpoint URL, and `S3_USE_PATH_STYLE` match your provider

### Federation not working

- `GLIPZ_PROTOCOL_PUBLIC_ORIGIN` is set
- `GLIPZ_PROTOCOL_HOST` is set
- `GLIPZ_PROTOCOL_MEDIA_PUBLIC_BASE` is set
- Reverse proxy buffering is disabled for SSE endpoints listed earlier in this guide

### Patreon OAuth errors after deploy

- `PATREON_ENABLED=true` is set
- Redirect URI in the Patreon developer console exactly matches `PATREON_REDIRECT_URI` or `{GLIPZ_PROTOCOL_PUBLIC_ORIGIN}/api/v1/fanclub/patreon/callback`
- `GLIPZ_PROTOCOL_PUBLIC_ORIGIN` uses HTTPS in production

### Patreon / Gumroad / PayPal UI is missing

- The related provider flag is enabled: `PATREON_ENABLED=true`, `GUMROAD_ENABLED=true`, or `PAYPAL_ENABLED=true`
- Credential-backed providers also have credentials set (`PATREON_*` or `PAYPAL_*`)
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
- [ ] Database and Redis secured
- [ ] Real email provider configured (Mailgun, etc.)
- [ ] Optional providers explicitly enabled only when configured (`PATREON_ENABLED`, `GUMROAD_ENABLED`, `PAYPAL_ENABLED`)
- [ ] License file added (see LICENSE)

---

## Related Documentation

- [README.md](README.md) — Project overview
- [DEPLOY.md](DEPLOY.md) — Production deployment
- [.env.example](.env.example) — All configuration options
- [LICENSE](LICENSE) — AGPLv3 license

---

### Federation Setup (Optional)
To enable federation between instances, configure these variables in your `.env`:

| Variable | Description |
|----------|-------------|
| `GLIPZ_PROTOCOL_PUBLIC_ORIGIN` | Your instance's public API URL (e.g., https://api.social.com) |
| `GLIPZ_PROTOCOL_HOST` | Your display hostname (e.g., social.com) |
| `GLIPZ_PROTOCOL_MEDIA_PUBLIC_BASE` | Base URL for serving media assets |

> **Note:** Federation between public instances expects HTTPS and stable hostnames. Signing keys are managed by the server (see federation code under `backend/internal/`).

For **adding another fan-club provider** (not Patreon-specific), follow [backend/internal/fanclub/kernel/IMPLEMENTATION_GUIDELINES.md](backend/internal/fanclub/kernel/IMPLEMENTATION_GUIDELINES.md).