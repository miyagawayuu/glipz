# Glipz Federation Protocol

English | [日本語](FEDERATION_PROTOCOL.ja.md)

This guide is for developers who want to integrate the Glipz Federation Protocol into a new social network, community application, or server implementation.

Glipz federation is a JSON-over-HTTP server-to-server protocol for public social posts, remote follows, reactions, polls, federated direct messages, and selected gated-media flows. The reference implementation in this repository uses `glipz-federation/2`.

For running a Glipz instance, start with [README.md](README.md) and [SETUP.md](SETUP.md). This document focuses on implementing or interoperating with the federation protocol.

---

## Overview

Glipz federation has four core pieces:

- **Discovery:** every instance publishes server metadata, its Ed25519 public key, and endpoint URLs at `/.well-known/glipz-federation`.
- **Public lookup:** remote software can fetch profile and public post documents for a local handle.
- **Signed events:** instances deliver JSON event envelopes to another instance's `/federation/events` inbox with Ed25519 signatures.
- **Remote follows:** a remote account subscribes to another account, then receives future public events through its inbox URL.

The protocol is not ActivityPub. It uses Glipz-specific JSON payloads and `X-Glipz-*` signature headers. Legacy ActivityPub-compatible shared inbox code is not the main federation path.

The reference server intentionally keeps the legacy ActivityPub-compatible shared
inbox disabled. Do not rely on HTTP Signature shared-inbox delivery for Glipz
interoperability; implement `X-Glipz-*` signed requests to `/federation/events`
instead. If ActivityPub compatibility is added in the future, it must include
complete HTTP Signature, digest, `keyId`/actor binding, and remote-key URL
validation before accepting inbound activities.

---

## When to Integrate

Use Glipz Federation Protocol if your software needs to:

- Let users on another Glipz-compatible instance follow your users.
- Deliver public posts, reposts, edits, deletes, likes, reactions, and poll updates across instances.
- Show remote public posts in a federated timeline.
- Support federated direct message events with published DM keys.
- Interoperate with Glipz's gated media unlock flow where supported.

Do not treat the protocol as a full ActivityPub replacement. It intentionally exposes a smaller surface that matches Glipz's social model and delivery worker design.

---

## Protocol Versioning

The current protocol version is:

```text
glipz-federation/2
```

The reference implementation advertises both `glipz-federation/1` and `glipz-federation/2` in discovery, but new integrations should implement version 2.

Version 2 requires:

- `X-Glipz-Nonce` on signed server-to-server requests.
- `event_id` on signed event and follow/unfollow payloads.
- Replay protection for nonces and event IDs.

Version 1 is retained only for compatibility with older Glipz deployments. New
servers should prefer version 2, and operators should plan to phase out version
1 peers because nonce-based replay protection is mandatory only in version 2.

Event envelopes use schema version `2` in the `v` field.

---

## Required Concepts

An **instance** is a server identified by a host such as `social.example`. The host appears in account names and in `X-Glipz-Instance`.

An **account** is represented as an acct string:

```text
alice@social.example
```

An **inbox** is the target URL for signed event delivery. In Glipz federation this is usually the remote instance's `events_url`, for example:

```text
https://social.example/federation/events
```

An **event** is a signed JSON envelope describing a post, repost, delete, like, reaction, poll update, or DM action.

---

## Discovery

Every compatible server should expose:

```http
GET /.well-known/glipz-federation
GET /.well-known/glipz-federation?resource=alice@social.example
```

The instance-level response contains a `server` object. If `resource` identifies a local account, the response also contains an `account` object.

Example:

```json
{
  "resource": "alice@social.example",
  "server": {
    "protocol_version": "glipz-federation/2",
    "supported_protocol_versions": [
      "glipz-federation/1",
      "glipz-federation/2"
    ],
    "server_software": "glipz",
    "server_version": "0.0.1",
    "event_schema_version": 2,
    "host": "social.example",
    "origin": "https://social.example",
    "key_id": "https://social.example/.well-known/glipz-federation#default",
    "public_key": "BASE64_ED25519_PUBLIC_KEY",
    "events_url": "https://social.example/federation/events",
    "follow_url": "https://social.example/federation/follow",
    "unfollow_url": "https://social.example/federation/unfollow",
    "dm_keys_url": "https://social.example/federation/dm-keys",
    "known_instances": ["remote.example"]
  },
  "account": {
    "acct": "alice@social.example",
    "handle": "alice",
    "domain": "social.example",
    "display_name": "Alice",
    "summary": "Profile text",
    "avatar_url": "https://social.example/api/v1/media/object/avatar",
    "profile_url": "https://social.example/@alice",
    "posts_url": "https://social.example/federation/posts/alice"
  }
}
```

Discovery is also used to verify signed requests. A receiver fetches the sender's discovery document, checks the advertised key, and verifies that signed endpoint URLs belong to the advertised origin.

For production federation, use HTTPS origins and stable hostnames. Local development may use different origins for testing, but public interoperability should assume HTTPS.

---

## Public HTTP Endpoints

A Glipz-compatible server should expose these public federation endpoints:

- `GET /.well-known/glipz-federation` for instance and account discovery.
- `GET /federation/profile/{handle}` for public profile JSON.
- `GET /federation/posts/{handle}?limit=20&cursor=...` for public posts.
- `GET /federation/dm-keys/{handle}` for a user's federated DM public keys.
- `POST /federation/follow` to accept a signed remote follow.
- `POST /federation/unfollow` to accept a signed remote unfollow.
- `POST /federation/events` to receive signed federation events.
- `POST /federation/posts/{postID}/unlock` to unlock gated post media where supported.
- `POST /federation/posts/{postID}/entitlement` for membership entitlement requests where supported.

The authenticated user-facing REST API under `/api/v1/...` is separate from the public federation surface. For example, a Glipz client starts a remote follow through its local API, and the server then sends the signed protocol request to the remote `follow_url`.

---

## Configuring a Glipz Instance

For the reference Glipz server, federation endpoints are mounted when a public federation origin is configured. In local development, the minimal federation configuration is:

```env
GLIPZ_PROTOCOL_PUBLIC_ORIGIN=http://localhost:8080
GLIPZ_PROTOCOL_HOST=localhost:8080
GLIPZ_PROTOCOL_MEDIA_PUBLIC_BASE=http://localhost:8080/api/v1/media/object
```

For production, use HTTPS values:

```env
GLIPZ_PROTOCOL_PUBLIC_ORIGIN=https://social.example
GLIPZ_PROTOCOL_HOST=social.example
GLIPZ_PROTOCOL_MEDIA_PUBLIC_BASE=https://social.example/api/v1/media/object
FEDERATION_POLICY_SUMMARY=Short text shown as your instance federation policy
```

The reference implementation derives its instance signing key from `JWT_SECRET`, so changing `JWT_SECRET` also changes the advertised federation public key and `key_id` trust relationship. Treat it as stable production configuration.

See [SETUP.md](SETUP.md) and [DEPLOY.md](DEPLOY.md) for the full environment file and deployment context.

---

## Signed Server-to-Server Requests

All mutating server-to-server requests should be signed with Ed25519. Version 2 requests include these headers:

```http
Content-Type: application/json
X-Glipz-Instance: social.example
X-Glipz-Key-Id: https://social.example/.well-known/glipz-federation#default
X-Glipz-Protocol-Version: glipz-federation/2
X-Glipz-App-Version: 0.0.1
X-Glipz-Timestamp: 2026-04-26T00:00:00Z
X-Glipz-Nonce: 550e8400-e29b-41d4-a716-446655440000
X-Glipz-Signature: BASE64_ED25519_SIGNATURE
```

The signature message is the UTF-8 bytes of:

```text
UPPERCASE_HTTP_METHOD
REQUEST_PATH
RFC3339_TIMESTAMP
NONCE
BASE64_SHA256_BODY
```

For example, a `POST /federation/events` body is signed over:

```text
POST
/federation/events
2026-04-26T00:00:00Z
550e8400-e29b-41d4-a716-446655440000
BASE64_SHA256_BODY
```

Receivers should:

- Require `X-Glipz-Instance`, `X-Glipz-Key-Id`, `X-Glipz-Protocol-Version`, `X-Glipz-Timestamp`, and `X-Glipz-Signature`.
- Require `X-Glipz-Nonce` for protocol version 2.
- Reject timestamps more than 10 minutes away from receiver time.
- Fetch `https://{X-Glipz-Instance}/.well-known/glipz-federation`.
- Verify that the discovery `key_id` matches `X-Glipz-Key-Id`.
- Verify the Ed25519 signature using the discovery `public_key`.
- Store each nonce long enough to reject replay attempts. The reference implementation uses a 15 minute nonce TTL.
- Store processed event IDs to avoid duplicate processing. The reference implementation keeps event IDs for 7 days.

---

## Event Envelope

Federation events use this envelope:

```json
{
  "event_id": "550e8400-e29b-41d4-a716-446655440001",
  "v": 2,
  "kind": "post_created",
  "author": {
    "acct": "alice@social.example",
    "handle": "alice",
    "domain": "social.example",
    "display_name": "Alice",
    "avatar_url": "https://social.example/api/v1/media/object/avatar",
    "profile_url": "https://social.example/@alice"
  },
  "post": {
    "id": "550e8400-e29b-41d4-a716-446655440002",
    "url": "https://social.example/posts/550e8400-e29b-41d4-a716-446655440002",
    "caption": "Hello from Glipz federation",
    "media_type": "image",
    "media_urls": ["https://social.example/api/v1/media/object/post-image"],
    "is_nsfw": false,
    "published_at": "2026-04-26T00:00:00Z",
    "like_count": 0
  }
}
```

Supported event kinds include:

- `post_created`
- `repost_created`
- `post_updated`
- `post_deleted`
- `post_liked`
- `post_unliked`
- `post_reaction_added`
- `post_reaction_removed`
- `poll_voted`
- `poll_tally_updated`
- `dm_*` events handled by the federated DM layer

`note_created`, `note_updated`, and `note_deleted` may be accepted for compatibility, but notes are no longer supported by the current Glipz social model.

Unknown event kinds should be rejected as unsupported.

---

## Public Post Documents

`GET /federation/posts/{handle}` returns public post documents. The reference implementation only exposes posts whose visibility is public.

Post fields can include:

- `id`, `url`, `caption`, `media_type`, `media_urls`, `is_nsfw`, and `published_at`.
- `like_count` for mirrored counts.
- `poll` for poll options and tallies.
- `reply_to_object_url` and `repost_of_object_url` for conversation and repost relationships.
- `has_view_password`, `view_password_scope`, and `unlock_url` for password-gated media.
- `has_membership_lock` and membership provider metadata in event payloads.

If you are building non-Glipz software, store remote object URLs as stable IDs. Glipz can fall back to a `glipz://{acct}/posts/{id}` object ID when a URL is missing, but HTTPS URLs are preferred for interoperability.

---

## Remote Follow Flow

A typical remote follow looks like this:

1. The local user enters a remote acct such as `bob@remote.example`.
2. The local server fetches the remote discovery document.
3. The local server resolves the remote `follow_url`.
4. The local server sends a signed `POST /federation/follow` request to the remote instance.
5. The remote instance stores the follower acct and inbox URL if moderation policy allows it.
6. Future public events are queued and delivered to the follower's inbox.

Follow request body:

```json
{
  "event_id": "550e8400-e29b-41d4-a716-446655440010",
  "follower_acct": "alice@social.example",
  "target_acct": "bob@remote.example",
  "inbox_url": "https://social.example/federation/events"
}
```

Unfollow uses the same shape against `unfollow_url`.

Receivers should check local block and moderation rules before accepting a follower.

---

## Delivery and Retry Model

The reference implementation stores outbound federation payloads in a delivery queue and processes them with a background worker.

Important behavior for implementers:

- Deliver to each subscriber's inbox URL with a signed JSON `POST`.
- Retry failed deliveries with exponential backoff starting at 30 seconds and capped at 1 hour.
- Stop retrying after 10 attempts.
- Treat domain-blocked inboxes as dead deliveries.
- Make event handling idempotent by using `event_id`.

If your server implementation does not use the same database schema, keep the same external behavior: durable queued delivery, signed POST, retries, and idempotent receivers.

---

## Direct Messages and DM Keys

Federated DMs are delivered as signed `dm_*` events through `/federation/events`.

Expose:

```http
GET /federation/dm-keys/{handle}
```

DM event payloads include:

- `thread_id`
- `message_id`
- `to_acct`
- `from_acct`
- `from_kid`
- sealed payload boxes
- optional encrypted attachments
- optional expiry and capability metadata

The public federation signature authenticates the sending instance. The DM payload layer handles message encryption material separately.

---

## Gated Media and Unlocks

Public post payloads can advertise that media is gated:

- Password-gated media can expose `has_view_password` and `unlock_url`.
- Membership-gated media can expose membership provider metadata.

Unlock requests use:

```http
POST /federation/posts/{postID}/unlock
```

Request body:

```json
{
  "event_id": "550e8400-e29b-41d4-a716-446655440020",
  "viewer_acct": "alice@remote.example",
  "password": "optional-view-password",
  "entitlement_jwt": "optional-entitlement-token"
}
```

Membership entitlement minting is intentionally limited. The origin-side `POST /federation/posts/{postID}/entitlement` endpoint refuses external providers that the origin cannot safely verify for a remote viewer, including Patreon, with `federation_membership_entitlement_unsupported`.

For Patreon-locked incoming posts, the current Glipz web/API flow can still unlock cross-instance when the viewer's home instance has Patreon enabled and the viewer has connected Patreon there. The viewer instance verifies the campaign/tier with Patreon locally, mints a short-lived `entitlement_jwt` whose issuer is the viewer instance, and sends that token to the origin `unlock_url`.

---

## Implementation Checklist

For a non-Glipz server, implement these pieces first:

1. Publish `/.well-known/glipz-federation` with a stable host, origin, Ed25519 public key, key ID, and endpoint URLs.
2. Generate and persist an Ed25519 signing key for the instance.
3. Sign every mutating server-to-server request with the `X-Glipz-*` headers.
4. Verify incoming signatures by resolving the sender's discovery document.
5. Store and reject replayed nonces.
6. Store and reject duplicate event IDs.
7. Expose public profile and public posts endpoints for local accounts.
8. Implement `POST /federation/follow` and `POST /federation/unfollow`.
9. Queue outbound events for each remote follower inbox.
10. Process incoming `/federation/events` idempotently.
11. Apply domain block, user block, and mute policy before displaying or accepting remote activity.
12. Add operational metrics and logs for delivery failures.

You can add DM keys, federated DMs, unlock flows, and richer interaction events after the basic follow and post delivery loop works.

---

## Testing Notes

Use these checks before announcing compatibility:

- Discovery returns the expected JSON for the instance and for a local account resource.
- `key_id`, `origin`, `events_url`, `follow_url`, and `unfollow_url` use the same HTTPS host in production.
- A receiver can fetch your discovery document and verify your signed request.
- Timestamp skew rejection works.
- Reusing the same nonce fails.
- Replaying the same event ID does not duplicate state.
- Remote follow and unfollow are idempotent.
- Public post fetches never expose non-public posts.
- Delivery retry behavior survives process restarts.

For Glipz deployment and scaling details, see [SETUP.md](SETUP.md), [DEPLOY.md](DEPLOY.md), and [SCALING.md](SCALING.md).

---

## Compatibility and Limitations

- Glipz Federation Protocol is not ActivityPub and does not require ActivityStreams documents.
- Public post federation is the primary content path.
- Notes are no longer supported by the current Glipz model.
- Origin-side remote membership entitlement minting for Patreon locks is not supported. Patreon cross-instance unlock is supported through the viewer-instance verification path described above.
- Production federation should use HTTPS and stable public origins.
- The authenticated `/api/v1/...` API is not part of the public server-to-server protocol, even when it starts federation actions locally.

---

## Related Documentation

- [README.md](README.md) for the project overview and feature list.
- [SETUP.md](SETUP.md) for local configuration, including optional federation environment variables.
- [DEPLOY.md](DEPLOY.md) for production deployment considerations.
- [SCALING.md](SCALING.md) for delivery workers, metrics, and scaling notes.
- [web/public/openapi.yaml](web/public/openapi.yaml) for the REST API reference.
