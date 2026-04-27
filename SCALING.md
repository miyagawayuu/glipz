# Scaling and Load Testing

This note captures the first production-readiness checks for running Glipz beyond a small instance.

## Runtime Metrics

Set `GLIPZ_METRICS_ENABLED=true` to expose Go `expvar` metrics at `/debug/vars`.
Keep this endpoint behind a trusted network or a reverse-proxy allowlist.

Useful counters include:

- `glipz_http_requests_total`
- `glipz_http_request_duration_ms_total`
- `glipz_operation_total`
- `glipz_operation_duration_ms_total`
- `glipz_sse_active`
- `glipz_media_proxy_bytes_total`
- `glipz_federation_delivery_total`
- `glipz_rate_limit_errors_total`

Slow HTTP requests and slow DB-facing operations are also logged with duration fields.

## Federation Delivery Scaling

Glipz Federation Protocol delivery is queue-backed. Outbound public events are
sent as signed JSON `POST` requests to each remote subscriber's inbox, normally
the peer's `/federation/events` URL discovered from `/.well-known/glipz-federation`.
The protocol path is not ActivityPub shared-inbox delivery, so scale and monitor
the `/federation/*` handlers directly.

Important behavior to preserve when increasing throughput:

- Select the highest mutually supported `glipz-federation/{major}` version from peer discovery; new peers should use `glipz-federation/3`.
- Keep `X-Glipz-Nonce` replay protection durable enough for the 15 minute nonce TTL used by the reference implementation.
- Keep processed `event_id` records long enough to reject duplicate event delivery; the reference implementation keeps them for 7 days.
- Retry transient delivery failures with exponential backoff starting at 30 seconds, capped at 1 hour, and stop after 10 attempts.
- Treat domain-blocked inboxes as dead deliveries instead of retrying them indefinitely.

Tune `GLIPZ_FEDERATION_DELIVERY_*` values gradually. Watch delivery success and
failure counts, queue age, Redis latency, remote 429/5xx rates, and database
write pressure from nonce/event dedupe tables before adding more workers.

## Feed Query Profiling

Run these against production-like data before and after changing indexes. Use `EXPLAIN (ANALYZE, BUFFERS)` and compare planning time, execution time, shared buffer reads, and row estimates.

```sql
-- Replace with a real viewer id.
\set viewer_id '00000000-0000-0000-0000-000000000000'

EXPLAIN (ANALYZE, BUFFERS)
WITH candidate AS (
  SELECT p.id
  FROM posts p
  WHERE p.reply_to_id IS NULL
    AND COALESCE(btrim(p.reply_to_remote_object_iri), '') = ''
    AND p.visible_at <= NOW()
    AND p.group_id IS NULL
    AND (p.visibility = 'public' OR p.user_id = :'viewer_id'::uuid)
  ORDER BY p.visible_at DESC, p.id DESC
  LIMIT 50
)
SELECT p.id
FROM posts p
JOIN candidate c ON c.id = p.id
LEFT JOIN (
  SELECT post_id, COUNT(*)::bigint AS like_count
  FROM post_likes
  WHERE post_id IN (SELECT id FROM candidate)
  GROUP BY post_id
) lk ON lk.post_id = p.id
ORDER BY p.visible_at DESC, p.id DESC;

EXPLAIN (ANALYZE, BUFFERS)
WITH candidate_reposts AS (
  SELECT rr.user_id, rr.post_id, rr.created_at
  FROM post_reposts rr
  JOIN posts p ON p.id = rr.post_id
  WHERE p.reply_to_id IS NULL
    AND COALESCE(btrim(p.reply_to_remote_object_iri), '') = ''
    AND p.visible_at <= NOW()
    AND p.group_id IS NULL
  ORDER BY rr.created_at DESC, rr.user_id DESC, rr.post_id DESC
  LIMIT 50
)
SELECT *
FROM candidate_reposts;

EXPLAIN (ANALYZE, BUFFERS)
SELECT id, actor_iri, published_at
FROM federation_incoming_posts
WHERE deleted_at IS NULL
  AND recipient_user_id IS NULL
  AND COALESCE(btrim(reply_to_object_iri), '') = ''
ORDER BY published_at DESC, id DESC
LIMIT 50;
```

## Media Delivery

The default `GLIPZ_MEDIA_PROXY_MODE=proxy` streams stored media through the API. This preserves one stable media endpoint, but large image/video traffic consumes backend bandwidth.

When `GLIPZ_STORAGE_MODE=local`, media is stored under `GLIPZ_LOCAL_STORAGE_PATH` and served by the backend through the same media endpoint. This is simple for a single VPS, but you must back up that folder and keep it on shared storage before running multiple backend instances.

For larger deployments, prefer:

```env
GLIPZ_MEDIA_PROXY_MODE=direct
GLIPZ_PROTOCOL_MEDIA_PUBLIC_BASE=https://media.example.com
```

For Cloudflare R2, keep `S3_ENDPOINT` / `S3_PUBLIC_ENDPOINT` on the R2 S3 API endpoint for signed uploads, then point `GLIPZ_PROTOCOL_MEDIA_PUBLIC_BASE` at the R2 custom public domain. R2 path-style access is auto-enabled for `*.r2.cloudflarestorage.com`.

For local storage with `direct`, serve `GLIPZ_LOCAL_STORAGE_PATH` from your reverse proxy or CDN and set `GLIPZ_PROTOCOL_MEDIA_PUBLIC_BASE` to that public URL. If no public media base is configured, local storage falls back to proxy mode.

Keep the proxy path available when you need private media behavior or remote media SSRF protection. Remote media continues to use the backend proxy.

## SSE and Reverse Proxy Notes

SSE endpoints set `X-Accel-Buffering: no`; make sure your reverse proxy also disables buffering for:

- `/api/v1/posts/feed/stream`
- `/api/v1/public/posts/feed/stream`
- `/api/v1/notifications/stream`
- `/api/v1/dm/stream`
- `/api/v1/public/federation/incoming/stream`

DM realtime traffic is message and invite notification traffic only. WebRTC audio/video calls and TURN relay support have been removed, so there is no media relay bandwidth to size for in the backend.

Watch `glipz_sse_active` and Redis client count. If SSE connections grow into the thousands, split stream-serving instances from general API instances.

The server also caps SSE connection count to reduce accidental fan-out: public streams allow up to 3 concurrent connections per client IP, authenticated streams allow up to 12 per IP and 4 per user, and each stream is rotated after 30 minutes. These limits use local process counters plus Redis-backed shared counters when Redis is available. If `GLIPZ_AUTH_RATE_LIMIT_FAIL_CLOSED=true`, Redis errors during shared SSE slot acquisition reject new streams instead of failing open.

## Load Test

Use `k6-load-test.js` for a basic staged test:

```bash
k6 run -e BASE_URL=https://staging.example -e ALLOW_NON_LOCAL_LOAD_TEST=true -e TOKEN="$TOKEN" k6-load-test.js
```

The script refuses non-local `BASE_URL` values unless
`ALLOW_NON_LOCAL_LOAD_TEST=true` is set. Use that flag only for staging or a
production-like environment you are allowed to test; do not point the first run
at a live production instance.

When testing the local Docker Compose stack, run k6 on the Compose network so traffic goes directly to the backend container instead of through Docker Desktop's host port forwarding:

```powershell
$env:GLIPZ_LOAD_TOKEN="glpat_..."
docker run --rm --network glipz_default -v "D:\glipz:/scripts" `
  -e BASE_URL=http://backend:8080 `
  -e ALLOW_NON_LOCAL_LOAD_TEST=true `
  -e TOKEN=$env:GLIPZ_LOAD_TOKEN `
  -e SLEEP_SECONDS=1 `
  -e NOTIFICATIONS_EVERY=5 `
  -e DM_THREADS_EVERY=5 `
  grafana/k6 run --vus 100 --duration 2m /scripts/k6-load-test.js
Remove-Item Env:\GLIPZ_LOAD_TOKEN
```

`NOTIFICATIONS_EVERY` and `DM_THREADS_EVERY` keep the worst-case script available while allowing more realistic tests where feed loads happen more often than notification and DM thread list refreshes.

Use `GLIPZ_FEED_PAGE_SIZE=30` as the default authenticated feed size for load tests. Raising it to `50` increases timeline payload size and is useful for worst-case checks.

Start with 100 VUs, then 500, then 1000. Record API p95, DB CPU, slow query logs, Redis connected clients, and outbound bandwidth for each run.
