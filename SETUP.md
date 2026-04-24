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
| **Node.js 18+** | Yes (frontend) | For running the Vue dev server |
| **npm** | Yes (frontend) | Comes with Node.js |
| **Go 1.22+** | No | Only if running backend outside Docker |
| **S3-compatible storage** | Yes | Wasabi, MinIO, AWS S3, etc. |

---

## Step 1: Clone the Repository

```bash
git clone https://github.com/your-repo/glipz.git
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
JWT_SECRET=your-secure-random-secret

S3_ENDPOINT=https://s3.your-region.wasabisys.com
S3_PUBLIC_ENDPOINT=https://s3.your-region.wasabisys.com
S3_REGION=your-region
S3_ACCESS_KEY=your-access-key
S3_SECRET_KEY=your-secret-key
S3_BUCKET=your-bucket
S3_USE_PATH_STYLE=false
```

### Recommended Variables

```env
FRONTEND_ORIGIN=http://localhost:5173
GLIPZ_VERSION=dev
```

### Federation (Optional)

```env
GLIPZ_PROTOCOL_PUBLIC_ORIGIN=http://localhost:8080
GLIPZ_PROTOCOL_HOST=localhost:8080
GLIPZ_PROTOCOL_MEDIA_PUBLIC_BASE=http://localhost:8080/api/v1/media/object
```

---

## Step 3: Start the Backend Stack

```bash
docker compose up --build
```

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
- Initializes the S3 client
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

Vite proxies these routes to the backend:
- `/api` → Backend API
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
- All S3 variables

---

## Optional Integrations

These are disabled unless configured:

| Feature | Environment Variables |
|---------|----------------------|
| **SkyWay (calls)** | `SKYWAY_APP_ID`, `SKYWAY_SECRET_KEY` |
| **Web Push** | `WEB_PUSH_VAPID_*` |
| **Federation** | `GLIPZ_PROTOCOL_*` |

See `.env.example` for all options.

---

## Troubleshooting

### Backend exits immediately

- `.env` file exists
- `JWT_SECRET` is set
- S3 variables are set
- S3 bucket is reachable from Docker

### Frontend loads but API fails

- Backend is running on port 8080
- `docker compose up` completed without errors
- No custom `VITE_PROXY_TARGET` overriding the default

### Media upload fails

- S3 credentials and bucket name are correct
- `S3_USE_PATH_STYLE` matches your provider
- Endpoint URL is correct for your region

### Federation not working

- `GLIPZ_PROTOCOL_PUBLIC_ORIGIN` is set
- `GLIPZ_PROTOCOL_HOST` is set
- `GLIPZ_PROTOCOL_MEDIA_PUBLIC_BASE` is set

### Emails not sending

- Open Mailpit at http://localhost:8025
- Local development uses Mailpit, not a real mail provider

---

## Production Checklist

Before going live:

- [ ] Strong `JWT_SECRET` (use a random generator)
- [ ] HTTPS configured (reverse proxy)
- [ ] S3 credentials configured
- [ ] `FRONTEND_ORIGIN` set to your public URL
- [ ] `GLIPZ_PROTOCOL_PUBLIC_ORIGIN` set (if using federation)
- [ ] Database and Redis secured
- [ ] Real email provider configured (Mailgun, etc.)
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

> **Note:** Federation requires a valid HTTPS endpoint and an Ed25519 key pair (generated automatically on first boot).