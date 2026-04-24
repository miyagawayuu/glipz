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
| **Storage** | S3-compatible (Wasabi, MinIO, AWS S3) |
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

### 1.3 S3 Bucket

Create a bucket with:
- **Public access blocked** (Glipz uses the media proxy)
- **CORS enabled** for your domain

### 1.4 Domain & SSL

Point your domain to the server and obtain SSL certificates (Let's Encrypt recommended).

---

## Step 2: Configure Environment

Create a production `.env` file:

```env
# === Required ===

JWT_SECRET=your-very-long-random-secret

DATABASE_URL=postgres://glipz:password@db-host:5432/glipz?sslmode=disable
REDIS_URL=redis://redis-host:6379/0

S3_ENDPOINT=https://s3.ap-northeast-1.wasabisys.com
S3_PUBLIC_ENDPOINT=https://s3.ap-northeast-1.wasabisys.com
S3_REGION=ap-northeast-1
S3_ACCESS_KEY=your-access-key
S3_SECRET_KEY=your-secret-key
S3_BUCKET=your-bucket
S3_USE_PATH_STYLE=false

# === Frontend & Federation ===

FRONTEND_ORIGIN=https://your-domain.com
GLIPZ_PROTOCOL_PUBLIC_ORIGIN=https://your-domain.com
GLIPZ_PROTOCOL_HOST=your-domain.com
GLIPZ_PROTOCOL_MEDIA_PUBLIC_BASE=https://your-domain.com/api/v1/media/object

# === Email (Mailgun example) ===

MAILGUN_DOMAIN=your-domain.com
MAILGUN_API_KEY=key-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
MAIL_FROM_EMAIL=no-reply@your-domain.com
MAIL_FROM_NAME=Glipz
```

### Key Configuration Notes

| Variable | Description |
|----------|-------------|
| `JWT_SECRET` | Use a cryptographically secure random string (64+ characters) |
| `FRONTEND_ORIGIN` | Your public web app URL |
| `GLIPZ_PROTOCOL_PUBLIC_ORIGIN` | Backend API URL (can be same as frontend if behind same proxy) |
| `GLIPZ_PROTOCOL_MEDIA_PUBLIC_BASE` | Media proxy URL for federation |

---

## Step 3: Build the Image

```bash
docker build -f backend/Dockerfile -t glipz:latest .
```

This builds:
- The Go backend binary
- The Vue frontend
- Serves frontend from `/app/web/dist`

---

## Step 4: Run the Container

```bash
docker run -d \
  --name glipz \
  --restart unless-stopped \
  --env-file .env \
  -p 127.0.0.1:8080:8080 \
  glipz:latest
```

> **Important**: Only expose port 8080 to localhost. Access through your reverse proxy.

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

    # SSE endpoints - disable buffering
    location ~ ^/api/v1/(posts/feed/stream|notifications/stream)$ {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 1h;
        proxy_send_timeout 1h;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        add_header X-Accel-Buffering no;
    }

    # Default proxy
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

### Caddy

```caddy
your-domain.com {
    encode gzip zstd
    request_body {
        max_size 100MB
    }

    @feedStream path /api/v1/posts/feed/stream
    @notificationStream path /api/v1/notifications/stream

    reverse_proxy @feedStream 127.0.0.1:8080 {
        flush_interval -1
    }

    reverse_proxy @notificationStream 127.0.0.1:8080 {
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
| `/api/v1/posts/feed/stream` | SSE timeline |
| `/api/v1/notifications/stream` | SSE notifications |
| `/.well-known/*` | Federation discovery |
| `/ap/*` | Federation endpoints |

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
| **SkyWay Calls** | Set `SKYWAY_*` variables |
| **Web Push** | Set `WEB_PUSH_VAPID_*` variables |

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
