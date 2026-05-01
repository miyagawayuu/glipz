# Glipz Federation Protocol

English | [日本語](FEDERATION_PROTOCOL.ja.md)

This guide is for developers who want to integrate the Glipz Federation Protocol into a new social network, community application, or server implementation.

Glipz federation is a JSON-over-HTTP server-to-server protocol for public social posts, remote follows, reactions, polls, federated direct messages, and selected gated-media flows. The reference implementation in this repository uses `glipz-federation/3`.

For running a Glipz instance, start with [README.md](README.md) and [SETUP.md](SETUP.md). This document focuses on implementing or interoperating with the federation protocol.

---

## Status of This Document

This document is a protocol specification for Glipz-compatible federation peers. It is not an IETF RFC, but it uses RFC-style terminology so independent implementations can reason about interoperability, security requirements, and optional extensions.

The reference implementation is the Glipz server in this repository. Where this document says "the reference implementation", it describes current Glipz behavior rather than a new requirement for all compatible software.

---

## Conformance Keywords

The key words **MUST**, **MUST NOT**, **REQUIRED**, **SHOULD**, **SHOULD NOT**, **RECOMMENDED**, **MAY**, and **OPTIONAL** are to be interpreted as described in RFC 2119 and RFC 8174 when, and only when, they appear in all capitals.

Implementations MAY expose additional fields, endpoints, or policy metadata. Receivers MUST ignore unknown JSON fields unless this document explicitly says otherwise.

---

## Terminology

- **Instance:** a server identified by a stable host such as `social.example`.
- **Origin:** the HTTPS scheme and authority used to dereference federation endpoints, such as `https://social.example`.
- **Account:** a user identity represented by an acct string such as `alice@social.example`.
- **Portable ID:** a stable account identifier such as `glipz:id:<public-key-fingerprint>` that can survive an account move.
- **Inbox:** the target URL for signed event delivery, usually the peer's `events_url`.
- **Event:** a signed JSON envelope delivered to `/federation/events`.
- **Capability:** a feature or protocol surface advertised directly or inferred from discovery metadata.
- **Policy:** an operator-controlled decision layer, such as domain blocks, remote follow acceptance, or gated media restrictions.

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
glipz-federation/3
```

The reference implementation advertises `glipz-federation/2` and `glipz-federation/3` in discovery, but new integrations should implement version 3.

Version 3 adds optional ID portability fields:

- Account and event author documents can include `id`, a stable portable identity such as `glipz:id:<public-key-fingerprint>`.
- Account documents can include `public_key`, `also_known_as`, and `moved_to`.
- Post documents and event post payloads can include `object_id`, a stable object identifier separate from the current HTTP URL.
- Instances can send an `account_moved` event when a user declares a new home account.

Version 2 requires:

- `X-Glipz-Nonce` on signed server-to-server requests.
- `event_id` on signed event and follow/unfollow payloads.
- Replay protection for nonces and event IDs.

Version 1 is not advertised by current discovery responses. Operators should
plan to phase out version 1 peers because nonce-based replay protection is
mandatory only in version 2 and later.

Event envelopes use schema version `3` in the `v` field.

---

## Identifiers and Addressing

This section summarizes the identifiers most often used on the wire. The normative definitions are in [Terminology](#terminology).

An account is represented as an acct string:

```text
alice@social.example
```

An inbox is the target URL for signed event delivery. In Glipz federation this is usually the remote instance's `events_url`, for example:

```text
https://social.example/federation/events
```

An event is a signed JSON envelope describing a post, repost, delete, like, reaction, poll update, or DM action. Implementations SHOULD persist remote object IDs and event IDs separately: object IDs identify content, while event IDs identify delivery attempts and replay protection state.

---

## Discovery

Every compatible server SHOULD expose:

```http
GET /.well-known/glipz-federation
GET /.well-known/glipz-federation?resource=alice@social.example
```

The instance-level response contains a `server` object. If `resource` identifies a local account, the response also contains an `account` object. `resource=instance@{host}` is accepted as an instance-level lookup and returns only the server object.

Example:

```json
{
  "resource": "alice@social.example",
  "server": {
    "protocol_version": "glipz-federation/3",
    "supported_protocol_versions": [
      "glipz-federation/2",
      "glipz-federation/3"
    ],
    "server_software": "glipz",
    "server_version": "0.0.3",
    "event_schema_version": 3,
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
    "id": "glipz:id:BASE64URL_ACCOUNT_KEY_FINGERPRINT",
    "acct": "alice@social.example",
    "handle": "alice",
    "domain": "social.example",
    "display_name": "Alice",
    "summary": "Profile text",
    "avatar_url": "https://social.example/api/v1/media/object/avatar",
    "header_url": "https://social.example/api/v1/media/object/header",
    "profile_url": "https://social.example/@alice",
    "posts_url": "https://social.example/federation/posts/alice",
    "public_key": "BASE64URL_ACCOUNT_PUBLIC_KEY",
    "also_known_as": ["alice@old.example"],
    "moved_to": ""
  }
}
```

Discovery is also used to verify signed requests. A receiver fetches the sender's discovery document, checks the advertised key, and verifies that signed endpoint URLs belong to the advertised origin.

`known_instances` is an operational hint that can help peers discover trusted or recently seen instances. It is not an automatic allowlist. `dm_keys_url`, when present, is the base URL for DM key lookup; append the local handle path segment, for example `/federation/dm-keys/alice`.

For production federation, use HTTPS origins and stable hostnames. Local development may use different origins for testing, but public interoperability should assume HTTPS.

---

## Capability Negotiation

Capability negotiation is discovery-driven. A sender MUST fetch the peer discovery document before sending mutating federation requests to a new peer or after a cached capability record expires.

The current discovery document does not require a separate `capabilities` object. Instead, peers negotiate support from the following fields:

| Discovery field | Negotiated meaning |
| --- | --- |
| `protocol_version` | Preferred protocol version advertised by the peer. |
| `supported_protocol_versions` | Complete set of protocol versions the peer is willing to receive. |
| `event_schema_version` | Highest event envelope schema version the peer expects. |
| `events_url` | Peer can receive signed event envelopes. |
| `follow_url` / `unfollow_url` | Peer can receive remote follow and unfollow requests. |
| `dm_keys_url` | Peer advertises federated DM key lookup support. |
| `known_instances` | Operational discovery hint; not an authorization decision. |

When selecting a protocol version, a sender MUST choose the highest version that both peers support. If `supported_protocol_versions` is absent, the sender MAY treat `protocol_version` as the peer's only advertised version. New integrations SHOULD prefer `glipz-federation/3`.

A sender SHOULD apply this negotiation procedure:

1. Fetch `/.well-known/glipz-federation` for the peer host.
2. Validate that `origin`, `events_url`, `follow_url`, and `unfollow_url` are HTTPS URLs under the advertised host for production federation.
3. Select the highest mutually supported `glipz-federation/{major}` version.
4. Require `X-Glipz-Nonce` and `event_id` when the selected version is 2 or later.
5. Use schema version `3` for new event envelopes when the peer advertises `event_schema_version >= 3` or `glipz-federation/3`.
6. Enable optional protocol surfaces only when their endpoint metadata is present. For example, federated DM clients SHOULD require `dm_keys_url` before sending `dm_*` events.

Receivers MUST reject unsupported protocol versions. Receivers SHOULD treat missing optional endpoints as "capability not advertised" rather than as a hard discovery failure.

---

## Public HTTP Endpoints

A Glipz-compatible server SHOULD expose these public federation endpoints:

- `GET /.well-known/glipz-federation` for instance and account discovery.
- `GET /federation/profile/{handle}` for public profile JSON.
- `GET /federation/posts/{handle}?limit=30&cursor=...` for public posts. The reference implementation defaults to 30 and accepts up to 100.
- `GET /federation/dm-keys/{handle}` for a user's federated DM public keys.
- `POST /federation/follow` to accept a signed remote follow.
- `POST /federation/unfollow` to accept a signed remote unfollow.
- `POST /federation/events` to receive signed federation events.
- `POST /federation/posts/{postID}/unlock` to unlock gated post media where supported.
- `POST /federation/posts/{postID}/entitlement` for membership entitlement requests where supported.

The authenticated user-facing REST API under `/api/v1/...` is separate from the public federation surface. For example, a Glipz client starts a remote follow through its local API, and the server then sends the signed protocol request to the remote `follow_url`.

---

## Configuring a Glipz Instance

For the reference Glipz server, federation endpoints are mounted when a public federation origin is available. `GLIPZ_PROTOCOL_PUBLIC_ORIGIN` is preferred; if it is empty, `FRONTEND_ORIGIN` is used as a fallback. In production, set `GLIPZ_PROTOCOL_PUBLIC_ORIGIN` explicitly when the API/federation origin differs from the frontend origin.

In local development, the minimal federation configuration is:

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

## Federation Policy

Federation policy is intentionally separate from protocol capability. A peer can be technically compatible and still be rejected by local operator policy.

The reference implementation exposes a short policy summary through `FEDERATION_POLICY_SUMMARY` for human-readable instance guidance. Implementations MAY publish equivalent policy text in their UI or discovery-adjacent documentation, but protocol peers MUST NOT treat policy text as machine-enforceable authorization.

Operators and implementations SHOULD define policy for at least these decisions:

- **Domain blocks:** requests or deliveries involving blocked domains SHOULD be rejected or marked as dead delivery before user-visible state is changed.
- **Remote follow acceptance:** `POST /federation/follow` MAY be rejected based on local moderation, account privacy, user blocks, or instance policy.
- **Known instances:** `known_instances` MAY be used as a discovery hint, but MUST NOT grant trust automatically.
- **User privacy:** user-level blocks, mutes, and privacy settings SHOULD be applied before displaying remote activity or accepting subscriptions.
- **Gated media:** password-gated and membership-gated media MAY be advertised, but unlock requests MUST still be evaluated by the origin's policy.
- **External entitlement providers:** if the origin cannot verify a remote viewer's external membership safely, it SHOULD reject entitlement minting rather than trust the remote claim.

Policy failures SHOULD use stable JSON error codes where possible. A sender MUST treat policy rejection as final unless the error clearly indicates a transient delivery failure.

---

## Signed Server-to-Server Requests

All mutating server-to-server requests MUST be signed with Ed25519. Version 3 uses the same nonce-protected signature base as version 2 and includes these headers:

```http
Content-Type: application/json
X-Glipz-Instance: social.example
X-Glipz-Key-Id: https://social.example/.well-known/glipz-federation#default
X-Glipz-Protocol-Version: glipz-federation/3
X-Glipz-App-Version: 0.0.3
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

Receivers MUST:

- Require `X-Glipz-Instance`, `X-Glipz-Key-Id`, `X-Glipz-Protocol-Version`, `X-Glipz-Timestamp`, and `X-Glipz-Signature`.
- Require `X-Glipz-Nonce` for protocol version 2 and later.
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
  "v": 3,
  "kind": "post_created",
  "author": {
    "id": "glipz:id:BASE64URL_ACCOUNT_KEY_FINGERPRINT",
    "acct": "alice@social.example",
    "handle": "alice",
    "domain": "social.example",
    "display_name": "Alice",
    "avatar_url": "https://social.example/api/v1/media/object/avatar",
    "profile_url": "https://social.example/@alice"
  },
  "post": {
    "id": "550e8400-e29b-41d4-a716-446655440002",
    "object_id": "glipz://550e8400-e29b-41d4-a716-446655440002",
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

- `account_moved`
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

Unknown event kinds SHOULD be rejected as unsupported.

---

## Public Post Documents

`GET /federation/posts/{handle}` returns public post documents. The reference implementation only exposes posts whose visibility is public.

Post fields can include:

- `id`, `object_id`, `url`, `caption`, `media_type`, `media_urls`, `is_nsfw`, and `published_at`.
- `like_count` for mirrored counts.
- `poll` for poll options and tallies.
- `reply_to_object_url`, `repost_of_object_url`, and `repost_comment` for conversation and repost relationships.
- `has_view_password`, `view_password_scope`, `view_password_text_ranges`, and `unlock_url` for password-gated media.
- `has_membership_lock`, `membership_provider`, `membership_creator_id`, and `membership_tier_id` in event payloads.

If you are building non-Glipz software, store remote object URLs as stable IDs. Glipz can fall back to a `glipz://{acct}/posts/{id}` object ID when a URL is missing, but HTTPS URLs are preferred for interoperability.

For protocol version 3, prefer `object_id` as the stable storage key when it is present and keep `url` as the current dereferenceable URL.

---

## ID Portability

Protocol version 3 separates a portable account identity from the current account address:

- `account.id` and `author.id` identify the same account across instance moves.
- `acct` remains the current display and delivery address.
- `public_key` lets receivers remember account-level key material for future move verification.
- `moved_to` advertises that the account has declared a new home account.

An account move is delivered as a signed event:

```json
{
  "event_id": "550e8400-e29b-41d4-a716-446655440030",
  "v": 3,
  "kind": "account_moved",
  "author": {
    "id": "glipz:id:BASE64URL_ACCOUNT_KEY_FINGERPRINT",
    "acct": "alice@old.example",
    "handle": "alice",
    "domain": "old.example",
    "display_name": "Alice",
    "public_key": "BASE64URL_ACCOUNT_PUBLIC_KEY"
  },
  "move": {
    "portable_id": "glipz:id:BASE64URL_ACCOUNT_KEY_FINGERPRINT",
    "old_acct": "alice@old.example",
    "new_acct": "alice@new.example",
    "inbox_url": "https://new.example/federation/events",
    "public_key": "BASE64URL_ACCOUNT_PUBLIC_KEY"
  }
}
```

Receivers SHOULD keep `acct` compatibility for older peers. If `id` is missing, store the actor as `legacy:{acct}` and do not merge it with a later portable account unless a verified move or account-key proof is available.

The reference implementation can also expose a user-facing migration wizard.
This wizard is an authenticated REST helper flow, not a federation event. The
source instance creates an identity bundle v2 encrypted with a migration
passphrase and issues a short-lived transfer token pinned to the target origin.
The target instance uses that token to pull the manifest, profile, post batches,
follow graph, bookmarks, and authorized media in a background import job with
progress and retry tracking. Imported historical posts are not fanned out as new
`post_created` events; after the user confirms the move, the source instance
sends the normal `account_moved` event.

Implementations SHOULD hash transfer tokens at rest, support expiry and
revocation, verify the target origin, prevent SSRF when fetching a source URL,
reject object keys not owned by the transfer session, and cap media size. The
reference import treats follow graph and bookmark restoration as best-effort:
remote inboxes should be revalidated where possible, unresolved rows are
skipped, and progress is reported through per-category import stats. DM history,
notification history, passwords, OAuth/PAT credentials, TOTP secrets, raw IP
data, and poll votes are not transferred by this helper flow.

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

Receivers SHOULD check local block and moderation rules before accepting a follower.

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

In discovery, `dm_keys_url` is advertised without the handle as a base endpoint. Clients append the escaped handle before fetching keys.

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
- Capability negotiation selects the highest mutually supported protocol version.
- Missing optional endpoints, such as `dm_keys_url`, disable only that optional capability.
- Policy rejection is surfaced as a stable final error rather than retried indefinitely.
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
- Capability negotiation currently uses discovery fields rather than a dedicated `capabilities` object.
- Federation policy is operator-controlled and can reject otherwise compatible peers.
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
