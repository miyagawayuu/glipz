# Deployment Guide

This guide covers deploying Glipz to production — from infrastructure preparation to verification and security hardening.

For local development, see [SETUP.md](SETUP.md).
For project overview, see [README.md](README.md).

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                     Reverse Proxy                           │
│                  (Nginx / Caddy / Traefik)                  │
│                      HTTPS (443)                            │
└─────────────────────────┬───────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│                     Glipz Container                         │
│   ┌─────────────────┐    ┌─────────────────────────────┐    │
│   │   Go Backend    │    │      Vue Frontend           │    │
│   │   (Port 8080)   │    │      (static files)         │    │
│   └────────┬────────┘    └─────────────────────────────┘    │
│            │                                                │
│            ▼                                                │
│   ┌─────────────────┐    ┌─────────────────────────────┐    │
│   │   PostgreSQL    │    │         Redis               │    │
│   │   (external)    │    │       (external)            │    │
│   └─────────────────┘    └─────────────────────────────┘    │
└─────────────────────────┬───────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│                   S3-Compatible Storage                     │
│                  (Wasabi / MinIO / S3)                      │
└─────────────────────────────────────────────────────────────┘
```

---

## Prerequisites

| Component | Recommendation |
|-----------|----------------|
| **Host** | Linux server (Ubuntu 22.04 LTS recommended) |
| **Container Runtime** | Docker |
| **Database** | PostgreSQL 16+ (managed or self-hosted) |
| **Cache** | Redis 7+ (managed or self-hosted) |
| **Storage** | Local server folder or S3-compatible storage (Cloudflare R2, Wasabi, MinIO, AWS S3) |
| **Reverse Proxy** | Nginx, Caddy, or Traefik |
| **Domain** | Public domain with DNS configured |

---

## Step 1: Prepare Infrastructure

### 1.1 Database

Create a PostgreSQL database:

```sql
CREATE USER glipz WITH PASSWORD 'your-secure-password';
CREATE DATABASE glipz OWNER glipz;
```

### 1.2 Redis

Start Redis or use a managed service (Redis Cloud, etc.).

### 1.3 Media Storage

Choose one storage mode:

- `GLIPZ_STORAGE_MODE=local`: store media under a server folder such as `/var/lib/glipz/media`.
- `GLIPZ_STORAGE_MODE=s3`: store media in an S3-compatible bucket.

For S3-compatible storage, create a bucket with:
- **Public access blocked** (Glipz uses the media proxy)
- **CORS enabled** for your domain

Keep `GLIPZ_MEDIA_PROXY_MODE=proxy` unless your CDN or object-storage public
endpoint enforces equivalent media safety headers. The backend proxy serves
active content types such as SVG, HTML, XML, and JavaScript as downloads with
`Content-Type: application/octet-stream`, `Content-Disposition: attachment`,
and `X-Content-Type-Options: nosniff`.

### 1.4 Domain & SSL

Point your domain to the server and obtain SSL certificates (Let's Encrypt recommended).

---

## Step 2: Configure Environment

Create a production `.env` file:

```env
# === Required ===

# Generate with: openssl rand -base64 48
JWT_SECRET=

DATABASE_URL=postgres://glipz:password@db-host:5432/glipz?sslmode=require
REDIS_URL=redis://redis-host:6379/0

GLIPZ_STORAGE_MODE=local
GLIPZ_LOCAL_STORAGE_PATH=/app/data/media
LEGAL_DOCS_DIR=/app/data/legal-docs

# Or, for S3-compatible storage:
# GLIPZ_STORAGE_MODE=s3
# S3_ENDPOINT=https://s3.ap-northeast-1.wasabisys.com
# S3_PUBLIC_ENDPOINT=https://s3.ap-northeast-1.wasabisys.com
# S3_REGION=ap-northeast-1
# S3_ACCESS_KEY=your-access-key
# S3_SECRET_KEY=your-secret-key
# S3_BUCKET=your-bucket
# S3_USE_PATH_STYLE=false

# === Frontend & Federation ===

FRONTEND_ORIGIN=https://your-domain.com
GLIPZ_PROTOCOL_PUBLIC_ORIGIN=https://your-domain.com
GLIPZ_PROTOCOL_HOST=your-domain.com
GLIPZ_PROTOCOL_MEDIA_PUBLIC_BASE=https://your-domain.com/api/v1/media/object

# If you serve the API on a separate subdomain, use values like:
# FRONTEND_ORIGIN=https://your-domain.com
# GLIPZ_PROTOCOL_PUBLIC_ORIGIN=https://api.your-domain.com
# GLIPZ_PROTOCOL_HOST=your-domain.com
# GLIPZ_PROTOCOL_MEDIA_PUBLIC_BASE=https://api.your-domain.com/api/v1/media/object

# === Email (Mailgun example) ===

MAILGUN_DOMAIN=your-domain.com
MAILGUN_API_KEY=key-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
# MAILGUN_API_BASE=https://api.eu.mailgun.net
MAIL_FROM_EMAIL=no-reply@your-domain.com
MAIL_FROM_NAME=Glipz

# === Optional: site admin + provider integrations ===

# GLIPZ_ADMIN_USER_IDS=uuid-of-admin-user
# PATREON_ENABLED=true
# PATREON_CLIENT_ID=...
# PATREON_CLIENT_SECRET=...
# PATREON_REDIRECT_URI=https://your-domain.com/api/v1/fanclub/patreon/callback
# GUMROAD_ENABLED=true
# PAYPAL_ENABLED=true
# PAYPAL_CLIENT_ID=...
# PAYPAL_CLIENT_SECRET=...
# PAYPAL_WEBHOOK_ID=...
# PAYPAL_ENV=live
```

### Key Configuration Notes

| Variable | Description |
|----------|-------------|
| `JWT_SECRET` | Use a cryptographically secure random string (64+ characters) |
| `FRONTEND_ORIGIN` | Your public web app URL |
| `GLIPZ_PROTOCOL_PUBLIC_ORIGIN` | Backend API URL (can be same as frontend if behind same proxy) |
| `GLIPZ_PROTOCOL_MEDIA_PUBLIC_BASE` | Media proxy URL for federation |
| `GLIPZ_STORAGE_MODE` | `local` stores media on the server; `s3` uses S3-compatible storage |
| `GLIPZ_LOCAL_STORAGE_PATH` | Local media directory; back it up if using `GLIPZ_STORAGE_MODE=local` |
| `GLIPZ_ADMIN_USER_IDS` | Built-in moderation / admin API access and `/admin` control panel access |
| `LEGAL_DOCS_DIR` | Optional directory for editable `terms.md`, `privacy.md`, and `nsfw-guidelines.md` |
| `GLIPZ_TRUST_PROXY_HEADERS` | Set to `true` only when your reverse proxy always overwrites `X-Real-IP` / `X-Forwarded-For` and the backend cannot be reached directly |
| `GLIPZ_AUTH_RATE_LIMIT_FAIL_CLOSED` | Optional stricter mode that rejects login/MFA attempts when Redis rate limit checks fail |
| `GLIPZ_REMOTE_MEDIA_PROXY_RATE_LIMIT_MAX` | Public remote-media proxy requests allowed per IP per 15 minutes; defaults to `120` |
| `GLIPZ_REMOTE_MEDIA_PROXY_RATE_LIMIT_FAIL_CLOSED` | Optional stricter mode that rejects public remote-media proxy requests when Redis rate limit writes fail |
| `GLIPZ_REMOTE_MEDIA_PROXY_MAX_BYTES` | Maximum bytes streamed by the public remote-media proxy; defaults to `52428800` |
| `GLIPZ_FEDERATION_DM_ATTACHMENT_MAX_BYTES` | Maximum bytes streamed by the authenticated federated DM attachment proxy; defaults to `52428800` |
| `GLIPZ_LINK_PREVIEW_RATE_LIMIT_MAX` | Public link-preview requests allowed per IP/user per 15 minutes; defaults to `60` |
| `GLIPZ_LINK_PREVIEW_RATE_LIMIT_FAIL_CLOSED` | Optional stricter mode that rejects link-preview requests when Redis rate limit writes fail |
| `GLIPZ_FEDERATION_INBOX_RATE_LIMIT_FAIL_CLOSED` | Optional stricter mode that rejects federation inbox POSTs when Redis rate limit writes fail |
| `GLIPZ_FEDERATION_DELIVERY_*` | Outbound federation delivery batch size, worker concurrency, and tick interval |
| `PATREON_ENABLED` | Enables Patreon UI/routes; defaults to disabled |
| `PATREON_*` | Patreon OAuth credentials; required when Patreon is enabled, and redirect URI must match your public API origin |
| `GUMROAD_ENABLED` | Enables Gumroad license-key locks; defaults to disabled and requires no server secret |
| `PAYPAL_ENABLED` | Enables PayPal payment UI/routes; defaults to disabled |
| `PAYPAL_*` | PayPal REST app and webhook credentials; required when PayPal is enabled |
| `MAILGUN_API_BASE` | Optional Mailgun regional API base, for example `https://api.eu.mailgun.net` |

Use `sslmode=require` or stronger for production PostgreSQL connections unless
the database connection is protected by an equivalent private TLS tunnel. Keep
`sslmode=disable` for local development only.

Rate limit checks use Redis. The default fail-open behavior preserves
availability during Redis outages, but public internet deployments that prefer
abuse resistance should consider enabling:

```env
GLIPZ_AUTH_RATE_LIMIT_FAIL_CLOSED=true
GLIPZ_REMOTE_MEDIA_PROXY_RATE_LIMIT_FAIL_CLOSED=true
GLIPZ_LINK_PREVIEW_RATE_LIMIT_FAIL_CLOSED=true
GLIPZ_FEDERATION_INBOX_RATE_LIMIT_FAIL_CLOSED=true
```

Enable fail-closed mode together with Redis health checks and alerting, because
Redis outages will reject the protected flows instead of allowing them through.

The production image is built from the **repository root** with `backend/Dockerfile` (see [docker-compose.yml](docker-compose.yml)): it runs `npm ci` / `npm run build` in `web/` on **Node 22**, then compiles the Go server with **Go 1.26.2**, and sets `STATIC_WEB_ROOT=/app/web/dist` by default.

When using CDN or direct object-storage media URLs, also set the frontend build-time allowlists in `web/.env.production`: `VITE_ALLOWED_MEDIA_BASE_URLS` for rendered media and `VITE_ALLOWED_DM_ATTACHMENT_BASE_URLS` for encrypted DM attachments. Use exact HTTPS path prefixes such as `https://cdn.example.com/media/`; root origins are rejected by the frontend safety checks. Configure the CDN/storage endpoint to reject or download active content types (`image/svg+xml`, `text/html`, XML, and JavaScript types) with `Content-Disposition: attachment` and `X-Content-Type-Options: nosniff`.

Provider callback URLs use the API public origin, not necessarily the frontend origin. For example, Patreon callbacks and PayPal return/webhook URLs should be based on `GLIPZ_PROTOCOL_PUBLIC_ORIGIN`.

Mailgun's default API base works for the US region. Set `MAILGUN_API_BASE` when your Mailgun domain uses a regional API endpoint such as the EU region.

---

## Step 3: Build the Image

```bash
docker build -f backend/Dockerfile -t glipz:latest .
```

This builds:
- The Go backend binary
- The Vue frontend
- Serves frontend from `/app/web/dist`

For production releases, pin and verify base image digests in CI instead of
relying only on mutable tags. For example:

```bash
docker buildx imagetools inspect node:22-alpine
docker buildx imagetools inspect golang:1.26.2-alpine
docker buildx imagetools inspect alpine:3.20
docker buildx imagetools inspect postgres:16-alpine
docker buildx imagetools inspect redis:7-alpine
```

Then use the reviewed `@sha256:...` references in the release Dockerfile or
release compose overlay. Keep `web/Dockerfile` out of production publishing; it
is a development-only Vite server image.

---

## Step 4: Run the Container

Prepare writable host directories for the non-root `glipz` user used inside the
container:

```bash
sudo mkdir -p /var/lib/glipz/media /var/lib/glipz/legal-docs
sudo chown -R 10001:10001 /var/lib/glipz/media
sudo chown -R 10001:10001 /var/lib/glipz/legal-docs
```

On shared hosts, prefer ownership or a dedicated read-only group for
`legal-docs`. `chmod -R a+rX /var/lib/glipz/legal-docs` is only a simple
single-purpose-host fallback.

```bash
docker run -d \
  --name glipz \
  --restart unless-stopped \
  --env-file .env \
  -v /var/lib/glipz/media:/app/data/media \
  -v /var/lib/glipz/legal-docs:/app/data/legal-docs:ro \
  -p 127.0.0.1:8080:8080 \
  glipz:latest
```

> **Important**: Only expose port 8080 to localhost. Access through your reverse proxy.
> If you use `GLIPZ_STORAGE_MODE=s3`, the media volume mount is not required.

For editable legal pages, set `LEGAL_DOCS_DIR=/app/data/legal-docs` and
place `terms.md`, `privacy.md`, and `nsfw-guidelines.md` in that directory.
Locale-specific files such as `terms.ja.md` or `terms.en.md` take precedence.

The `/admin` control panel stores runtime instance settings in Postgres. Back up
the database before changing registration policy or operator announcements there.

---

## Step 5: Configure Reverse Proxy

### Nginx

```nginx
server {
    listen 80;
    server_name your-domain.com;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl http2;
    server_name your-domain.com;

    ssl_certificate /etc/letsencrypt/live/your-domain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/your-domain.com/privkey.pem;

    client_max_body_size 100m;

    add_header Content-Security-Policy "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob: https:; media-src 'self' blob: https:; connect-src 'self'; frame-src https://www.youtube-nocookie.com https://player.vimeo.com https://www.dailymotion.com https://www.loom.com https://streamable.com https://fast.wistia.net https://player.bilibili.com https://www.tiktok.com https://store.steampowered.com; frame-ancestors 'none'; base-uri 'self'; form-action 'self'" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;
    add_header Permissions-Policy "camera=(), microphone=(), geolocation=(), payment=()" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;

    # SSE endpoints - disable buffering (authenticated + public streams)
    location ~ ^/api/v1/(posts/feed/stream|notifications/stream|dm/stream|public/posts/feed/stream|public/federation/incoming/stream)$ {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 1h;
        proxy_send_timeout 1h;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $remote_addr;
        proxy_set_header X-Forwarded-Proto $scheme;
        add_header X-Accel-Buffering no;
    }

    # Default proxy
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $remote_addr;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

If you enable `GLIPZ_TRUST_PROXY_HEADERS=true`, keep the backend bound to
`127.0.0.1` or a private network and make sure the proxy overwrites
`X-Real-IP` and `X-Forwarded-For` as shown above. Do not pass through
client-supplied forwarding headers.

The backend sets security headers when they are not already present, including
`Content-Security-Policy`, `Referrer-Policy`, `Permissions-Policy`,
`X-Content-Type-Options`, and `X-Frame-Options`. The examples above pin them at
the proxy so operators can audit and adjust policy in one place.

### Caddy

```caddy
your-domain.com {
    encode gzip zstd
    request_body {
        max_size 100MB
    }

    header {
        Content-Security-Policy "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob: https:; media-src 'self' blob: https:; connect-src 'self'; frame-src https://www.youtube-nocookie.com https://player.vimeo.com https://www.dailymotion.com https://www.loom.com https://streamable.com https://fast.wistia.net https://player.bilibili.com https://www.tiktok.com https://store.steampowered.com; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"
        Referrer-Policy "strict-origin-when-cross-origin"
        Permissions-Policy "camera=(), microphone=(), geolocation=(), payment=()"
        X-Content-Type-Options "nosniff"
        X-Frame-Options "DENY"
        Strict-Transport-Security "max-age=31536000; includeSubDomains"
    }

    @sse path_regexp ^/api/v1/(posts/feed/stream|notifications/stream|dm/stream|public/posts/feed/stream|public/federation/incoming/stream)$

    reverse_proxy @sse 127.0.0.1:8080 {
        flush_interval -1
    }

    reverse_proxy 127.0.0.1:8080
}
```

### Important Paths

Ensure these paths are proxied correctly:

| Path | Description |
|------|-------------|
| `/api/*` | REST API |
| `/api/v1/posts/feed/stream` | SSE home / feed timeline |
| `/api/v1/notifications/stream` | SSE notifications |
| `/api/v1/dm/stream` | SSE direct messages |
| `/api/v1/public/posts/feed/stream` | Public SSE feed (no auth; configure caching carefully) |
| `/api/v1/public/federation/incoming/stream` | Public SSE federated incoming stream |
| `/.well-known/glipz-federation` | Glipz Federation discovery |
| `/federation/*` | Glipz Federation endpoints |

---

## Step 6: Verify Deployment

Run these checks after deployment:

| Check | Command/URL | Expected |
|-------|-------------|----------|
| Health | `curl https://your-domain.com/health` | `ok` |
| Frontend | Open https://your-domain.com | App loads |
| Login | Try registering a test account | Email received |
| Media Upload | Upload an image | URL returned |
| API | `curl -H "Authorization: Bearer $TOKEN" https://your-domain.com/api/v1/users/me` | User data |

---

## Step 7: Security Hardening

### Pre-Deployment Checklist

- [ ] `JWT_SECRET` is a strong random string (64+ chars)
- [ ] `.env` is not committed to Git
- [ ] Database is not exposed to the internet
- [ ] Redis is not exposed to the internet
- [ ] HTTPS is enabled and working
- [ ] `FRONTEND_ORIGIN` matches your actual domain
- [ ] S3 bucket blocks public access
- [ ] Federation settings reviewed (if enabled)

### Optional Features

| Feature | Enable With |
|---------|-------------|
| **Federation** | Set `GLIPZ_PROTOCOL_*` variables |
| **Web Push** | Set `WEB_PUSH_VAPID_*` variables |
| **Patreon fan club** | Set `PATREON_ENABLED=true`, `PATREON_CLIENT_ID`, `PATREON_CLIENT_SECRET`, and `PATREON_REDIRECT_URI` (or rely on default derived from `GLIPZ_PROTOCOL_PUBLIC_ORIGIN`) |
| **Gumroad fan club locks** | Set `GUMROAD_ENABLED=true`; no server secret is required |
| **PayPal subscriptions** | Set `PAYPAL_ENABLED=true`, `PAYPAL_CLIENT_ID`, `PAYPAL_CLIENT_SECRET`, `PAYPAL_WEBHOOK_ID`, and `PAYPAL_ENV` |

---

## Step 8: Maintenance

### Logs

```bash
docker logs -f glipz
```

### Updates

```bash
docker pull glipz:latest
docker restart glipz
```

### Backup

- PostgreSQL database
- S3 bucket contents
- `.env` file (keep secure)

---

## Related Documentation

- [README.md](README.md) — Project overview
- [SETUP.md](SETUP.md) — Local development
- [.env.example](.env.example) — All configuration options
- [LICENSE](LICENSE) — AGPLv3 license
