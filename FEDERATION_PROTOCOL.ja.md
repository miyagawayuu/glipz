# Glipz Federation Protocol

[English](FEDERATION_PROTOCOL.md) | 日本語

この資料は、Glipz Federation Protocol を新しいSNS、コミュニティアプリ、またはサーバソフトウェアへ組み込む開発者向けの導入ガイドです。

Glipz federation は、公開投稿、リモートフォロー、リアクション、投票、連合DM、一部のゲート付きメディアのフローを扱う JSON over HTTP のサーバ間プロトコルです。このリポジトリの参照実装は `glipz-federation/3` を使用します。

Glipz インスタンスを動かす手順は [README.md](README.md) と [SETUP.md](SETUP.md) を先に参照してください。この資料では、プロトコルの実装や相互接続に必要な情報を中心に説明します。

---

## 概要

Glipz federation は主に4つの要素で構成されます。

- **Discovery:** 各インスタンスは `/.well-known/glipz-federation` でサーバメタデータ、Ed25519公開鍵、エンドポイントURLを公開します。
- **Public lookup:** リモートソフトウェアは、ローカルハンドルのプロフィールと公開投稿ドキュメントを取得できます。
- **Signed events:** インスタンスは Ed25519 署名付きの JSON event envelope を相手の `/federation/events` inbox へ配送します。
- **Remote follows:** リモートアカウントが別インスタンスのアカウントを購読し、その後の公開イベントを自分の inbox URL で受け取ります。

このプロトコルは ActivityPub ではありません。Glipz 専用の JSON ペイロードと `X-Glipz-*` 署名ヘッダを使用します。ActivityPub 互換の legacy shared inbox コードは、現在の主経路ではありません。

参照サーバでは、ActivityPub 互換の legacy shared inbox は意図的に無効化されています。Glipz の相互運用では HTTP Signature shared inbox 配送に依存せず、`/federation/events` への `X-Glipz-*` 署名付きリクエストを実装してください。将来 ActivityPub 互換を追加する場合は、受信を許可する前に HTTP Signature、digest、`keyId` と actor の結び付け、リモート鍵 URL の検証を完全に実装する必要があります。

---

## どのような場合に組み込むか

次のような機能が必要な場合に Glipz Federation Protocol を組み込みます。

- 他の Glipz 互換インスタンスのユーザーが、自分のサーバのユーザーをフォローできるようにする。
- 公開投稿、リポスト、編集、削除、いいね、リアクション、投票更新をインスタンス間で配送する。
- リモートの公開投稿を federated timeline に表示する。
- 公開DM鍵を使った連合DMイベントをサポートする。
- Glipz のゲート付きメディア unlock flow と、対応している範囲で相互運用する。

このプロトコルを ActivityPub の完全な代替として扱わないでください。Glipz のソーシャルモデルと delivery worker 設計に合わせ、意図的に小さなサーフェスに絞っています。

---

## プロトコルバージョン

現在のプロトコルバージョンは次の通りです。

```text
glipz-federation/3
```

参照実装は discovery で `glipz-federation/1`、`glipz-federation/2`、`glipz-federation/3` を広告しますが、新規実装では version 3 を実装してください。

Version 3 では ID ポータビリティ用の optional field が追加されます。

- Account と event author は、`glipz:id:<public-key-fingerprint>` のような安定した portable identity を `id` として含められます。
- Account は `public_key`、`also_known_as`、`moved_to` を含められます。
- Post document と event post payload は、現在の HTTP URL とは別の安定IDとして `object_id` を含められます。
- ユーザーが新しいホームアカウントを宣言したとき、インスタンスは `account_moved` event を送信できます。

Version 2 では次が必須です。

- 署名付きサーバ間リクエストに `X-Glipz-Nonce` を含める。
- 署名付き event と follow/unfollow ペイロードに `event_id` を含める。
- nonce と event ID による replay protection を行う。

Version 1 は古い Glipz デプロイとの互換目的でのみ残されています。新規サーバは version 3 を優先し、運用者は version 1 ピアを段階的に外す計画を立ててください。nonce による replay protection が必須なのは version 2 以降です。

Event envelope の `v` フィールドには schema version `3` を使います。

---

## 基本概念

**Instance** は `social.example` のようなホストで識別されるサーバです。このホストはアカウント名や `X-Glipz-Instance` に現れます。

**Account** は acct 文字列で表します。

```text
alice@social.example
```

**Inbox** は署名付きイベント配送の送信先URLです。Glipz federation では通常、リモートインスタンスの `events_url` が inbox になります。

```text
https://social.example/federation/events
```

**Event** は、投稿、リポスト、削除、いいね、リアクション、投票更新、DMアクションを表す署名付き JSON envelope です。

---

## Discovery（検出）

互換サーバは次のエンドポイントを公開します。

```http
GET /.well-known/glipz-federation
GET /.well-known/glipz-federation?resource=alice@social.example
```

インスタンス単位のレスポンスには `server` オブジェクトを含めます。`resource` がローカルアカウントを指す場合は、`account` オブジェクトも含めます。

例:

```json
{
  "resource": "alice@social.example",
  "server": {
    "protocol_version": "glipz-federation/3",
    "supported_protocol_versions": [
      "glipz-federation/1",
      "glipz-federation/2",
      "glipz-federation/3"
    ],
    "server_software": "glipz",
    "server_version": "0.0.1",
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
    "profile_url": "https://social.example/@alice",
    "posts_url": "https://social.example/federation/posts/alice",
    "public_key": "BASE64URL_ACCOUNT_PUBLIC_KEY",
    "also_known_as": ["alice@old.example"],
    "moved_to": ""
  }
}
```

Discovery は署名付きリクエストの検証にも使われます。受信側は送信元の discovery document を取得し、広告されている鍵を確認し、署名対象のエンドポイントURLが広告された origin に属していることを検証します。

本番 federation では HTTPS origin と安定したホスト名を使用してください。ローカル開発ではテスト用に異なる origin を使う場合がありますが、公開相互運用では HTTPS を前提にします。

---

## 公開HTTPエンドポイント

Glipz 互換サーバは、次の公開 federation endpoint を提供します。

- `GET /.well-known/glipz-federation`: インスタンスとアカウントの discovery。
- `GET /federation/profile/{handle}`: 公開プロフィール JSON。
- `GET /federation/posts/{handle}?limit=20&cursor=...`: 公開投稿一覧。
- `GET /federation/dm-keys/{handle}`: ユーザーの連合DM公開鍵。
- `POST /federation/follow`: 署名付き remote follow の受信。
- `POST /federation/unfollow`: 署名付き remote unfollow の受信。
- `POST /federation/events`: 署名付き federation event の受信。
- `POST /federation/posts/{postID}/unlock`: 対応している場合のゲート付き投稿メディアの unlock。
- `POST /federation/posts/{postID}/entitlement`: 対応している場合の membership entitlement request。

`/api/v1/...` 以下の認証済みユーザー向けREST APIは、公開 federation surface とは別です。たとえば Glipz クライアントがローカルAPI経由で remote follow を開始し、サーバが相手の `follow_url` へ署名付きプロトコルリクエストを送ります。

---

## Glipz インスタンス設定

参照実装の Glipz サーバでは、public federation origin が設定されている場合に federation endpoint がマウントされます。ローカル開発での最小構成は次の通りです。

```env
GLIPZ_PROTOCOL_PUBLIC_ORIGIN=http://localhost:8080
GLIPZ_PROTOCOL_HOST=localhost:8080
GLIPZ_PROTOCOL_MEDIA_PUBLIC_BASE=http://localhost:8080/api/v1/media/object
```

本番では HTTPS の値を使用します。

```env
GLIPZ_PROTOCOL_PUBLIC_ORIGIN=https://social.example
GLIPZ_PROTOCOL_HOST=social.example
GLIPZ_PROTOCOL_MEDIA_PUBLIC_BASE=https://social.example/api/v1/media/object
FEDERATION_POLICY_SUMMARY=Short text shown as your instance federation policy
```

参照実装は instance signing key を `JWT_SECRET` から導出します。そのため、`JWT_SECRET` を変更すると discovery で広告される federation public key と `key_id` の信頼関係も変わります。本番では安定した設定として扱ってください。

完全な環境変数とデプロイ文脈は [SETUP.md](SETUP.md) と [DEPLOY.md](DEPLOY.md) を参照してください。

---

## 署名付きサーバ間リクエスト

状態を変更するサーバ間リクエストは Ed25519 で署名します。Version 3 のリクエストには次のヘッダを含めます。

```http
Content-Type: application/json
X-Glipz-Instance: social.example
X-Glipz-Key-Id: https://social.example/.well-known/glipz-federation#default
X-Glipz-Protocol-Version: glipz-federation/3
X-Glipz-App-Version: 0.0.1
X-Glipz-Timestamp: 2026-04-26T00:00:00Z
X-Glipz-Nonce: 550e8400-e29b-41d4-a716-446655440000
X-Glipz-Signature: BASE64_ED25519_SIGNATURE
```

署名メッセージは、次の内容を改行で連結した UTF-8 bytes です。

```text
UPPERCASE_HTTP_METHOD
REQUEST_PATH
RFC3339_TIMESTAMP
NONCE
BASE64_SHA256_BODY
```

たとえば `POST /federation/events` の body は次の文字列に対して署名します。

```text
POST
/federation/events
2026-04-26T00:00:00Z
550e8400-e29b-41d4-a716-446655440000
BASE64_SHA256_BODY
```

受信側は次を行います。

- `X-Glipz-Instance`、`X-Glipz-Key-Id`、`X-Glipz-Protocol-Version`、`X-Glipz-Timestamp`、`X-Glipz-Signature` を必須にする。
- Protocol version 2 以降では `X-Glipz-Nonce` を必須にする。
- 受信側時刻から10分を超えてずれた timestamp を拒否する。
- `https://{X-Glipz-Instance}/.well-known/glipz-federation` を取得する。
- Discovery の `key_id` が `X-Glipz-Key-Id` と一致することを検証する。
- Discovery の `public_key` で Ed25519 署名を検証する。
- Nonce を一定時間保存し、replay を拒否する。参照実装では nonce TTL は15分です。
- 処理済み event ID を保存し、重複処理を防ぐ。参照実装では event ID を7日保持します。

---

## Event Envelope（イベント形式）

Federation event は次の envelope を使用します。

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
    "object_id": "glipz://glipz:id:BASE64URL_ACCOUNT_KEY_FINGERPRINT/posts/550e8400-e29b-41d4-a716-446655440002",
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

対応している event kind には次があります。

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
- 連合DMレイヤで処理される `dm_*` event

`note_created`、`note_updated`、`note_deleted` は互換性のため受理される場合がありますが、現在の Glipz social model では notes はサポートされていません。

未知の event kind は unsupported として拒否してください。

---

## 公開投稿ドキュメント

`GET /federation/posts/{handle}` は公開投稿ドキュメントを返します。参照実装は visibility が public の投稿だけを公開します。

投稿フィールドには次のような値を含められます。

- `id`、`object_id`、`url`、`caption`、`media_type`、`media_urls`、`is_nsfw`、`published_at`。
- ミラーされたカウントとしての `like_count`。
- 投票オプションと集計を表す `poll`。
- 会話やリポスト関係を表す `reply_to_object_url` と `repost_of_object_url`。
- パスワード付きメディア用の `has_view_password`、`view_password_scope`、`unlock_url`。
- Event payload 内の `has_membership_lock` と membership provider metadata。

非 Glipz ソフトウェアを実装する場合、version 3 では `object_id` があればそれを安定した保存キーとして優先し、`url` は現在の参照可能 URL として保持してください。`object_id` がない相手では、従来通りリモート object URL を stable ID として保存します。Glipz は URL がない場合に `glipz://{acct}/posts/{id}` 形式の object ID にフォールバックできます。

---

## ID ポータビリティ

Protocol version 3 では、portable account identity と現在の account address を分離します。

- `account.id` と `author.id` は、インスタンス移転後も同じアカウントを表します。
- `acct` は現在の表示・配送アドレスとして残ります。
- `public_key` は、将来の move verification のために receiver が記憶できます。
- `moved_to` は、そのアカウントが新しいホームアカウントを宣言したことを示します。

アカウント移転は署名付き event として配送されます。

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

Receiver は古い peer との互換性のため `acct` を引き続き保存してください。`id` がない場合は `legacy:{acct}` として扱い、検証済み move または account-key proof がない限り、あとから現れた portable account と自動的に統合しないでください。

---

## Remote Follow Flow（リモートフォロー）

典型的な remote follow は次の流れです。

1. ローカルユーザーが `bob@remote.example` のような remote acct を入力する。
2. ローカルサーバがリモート discovery document を取得する。
3. ローカルサーバがリモートの `follow_url` を解決する。
4. ローカルサーバがリモートインスタンスへ署名付き `POST /federation/follow` を送る。
5. リモートインスタンスが moderation policy に基づいて follower acct と inbox URL を保存する。
6. 以後の公開イベントが follower の inbox へキューイングされ、配送される。

Follow request body:

```json
{
  "event_id": "550e8400-e29b-41d4-a716-446655440010",
  "follower_acct": "alice@social.example",
  "target_acct": "bob@remote.example",
  "inbox_url": "https://social.example/federation/events"
}
```

Unfollow は同じ形の body を `unfollow_url` へ送ります。

受信側は follower を受け入れる前に、ローカルの block や moderation rule を確認してください。

---

## Delivery and Retry Model（配送と再試行）

参照実装は、送信する federation payload を delivery queue に保存し、background worker で処理します。

実装時に重要な挙動は次の通りです。

- 各 subscriber の inbox URL へ署名付き JSON `POST` で配送する。
- 失敗した配送は30秒から始まる exponential backoff で再試行し、最大1時間に抑える。
- 10回失敗したら再試行を止める。
- Domain-block された inbox は dead delivery として扱う。
- `event_id` を使って event handling を idempotent にする。

同じデータベーススキーマを使わない実装でも、外部挙動としては durable queued delivery、signed POST、retry、idempotent receiver を保ってください。

---

## Direct Messages and DM Keys（DMと公開鍵）

連合DMは、署名付き `dm_*` event として `/federation/events` 経由で配送されます。

次のエンドポイントを公開します。

```http
GET /federation/dm-keys/{handle}
```

DM event payload には次のような値が含まれます。

- `thread_id`
- `message_id`
- `to_acct`
- `from_acct`
- `from_kid`
- sealed payload boxes
- 任意の encrypted attachments
- 任意の expiry と capability metadata

公開 federation 署名は送信インスタンスを認証します。DM payload layer はメッセージ暗号化の材料を別に扱います。

---

## Gated Media and Unlocks（ゲート付きメディア）

公開投稿 payload は、メディアが gate されていることを広告できます。

- パスワード付きメディアでは `has_view_password` と `unlock_url` を公開できます。
- Membership-gated media では membership provider metadata を公開できます。

Unlock request は次のエンドポイントを使います。

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

Membership entitlement の発行は意図的に制限されています。Origin 側の `POST /federation/posts/{postID}/entitlement` は、origin がリモート閲覧者の外部会員状態を安全に検証できない provider（Patreon など）に対して `federation_membership_entitlement_unsupported` を返します。

Patreon locked incoming post については、現在の Glipz web/API flow では、閲覧者の home instance で Patreon が有効化され、閲覧者がそこで Patreon 連携済みであれば cross-instance unlock が可能です。Viewer instance が Patreon API で campaign/tier をローカル検証し、viewer instance を issuer とする短命の `entitlement_jwt` を発行して、origin の `unlock_url` に送ります。

---

## 実装チェックリスト

非 Glipz サーバでは、まず次を実装してください。

1. `/.well-known/glipz-federation` で、安定した host、origin、Ed25519公開鍵、key ID、endpoint URL を公開する。
2. インスタンス用の Ed25519 signing key を生成し、永続化する。
3. 状態を変更する全てのサーバ間リクエストに `X-Glipz-*` ヘッダで署名する。
4. 送信元の discovery document を解決して incoming signature を検証する。
5. Nonce を保存し、replay を拒否する。
6. Event ID を保存し、重複 event を拒否または無害化する。
7. ローカルアカウント用の public profile と public posts endpoint を公開する。
8. `POST /federation/follow` と `POST /federation/unfollow` を実装する。
9. 各 remote follower inbox に対して outbound event を queue する。
10. Incoming `/federation/events` を idempotent に処理する。
11. Remote activity の表示や受け入れ前に、domain block、user block、mute policy を適用する。
12. Delivery failure の metrics と log を追加する。

基本的な follow と post delivery loop が動いた後に、DM keys、連合DM、unlock flow、より豊富な interaction event を追加できます。

---

## テスト観点

互換性を案内する前に、次を確認してください。

- Discovery がインスタンス単位とローカルアカウント resource の両方で期待通りの JSON を返す。
- 本番では `key_id`、`origin`、`events_url`、`follow_url`、`unfollow_url` が同じ HTTPS host を使っている。
- 受信側が discovery document を取得し、署名付きリクエストを検証できる。
- Timestamp skew の拒否が機能する。
- 同じ nonce の再利用が失敗する。
- 同じ event ID の replay で状態が重複しない。
- Remote follow と unfollow が idempotent に動作する。
- Public post fetch が非公開投稿を露出しない。
- Delivery retry がプロセス再起動後も継続できる。

Glipz のデプロイやスケーリングについては [SETUP.md](SETUP.md)、[DEPLOY.md](DEPLOY.md)、[SCALING.md](SCALING.md) も参照してください。

---

## 互換性と制限

- Glipz Federation Protocol は ActivityPub ではなく、ActivityStreams documents を必要としません。
- 公開投稿 federation が主要な content path です。
- Notes は現在の Glipz model ではサポートされていません。
- Patreon lock に対する origin-side remote membership entitlement minting はサポートされていません。Patreon の cross-instance unlock は上記の viewer-instance verification path でサポートされます。
- 本番 federation では HTTPS と安定した public origin を使用してください。
- 認証済み `/api/v1/...` API は、ローカルで federation action を開始する場合でも、公開 server-to-server protocol の一部ではありません。

---

## 関連資料

- [README.md](README.md): プロジェクト概要と機能一覧。
- [SETUP.md](SETUP.md): federation の任意環境変数を含むローカル設定。
- [DEPLOY.md](DEPLOY.md): 本番デプロイ時の考慮事項。
- [SCALING.md](SCALING.md): delivery worker、metrics、スケーリングの補足。
- [web/public/openapi.yaml](web/public/openapi.yaml): REST API reference。
