# Security Follow-ups

このメモは、今回の高リスク修正では挙動を変えず、追加の仕様判断が必要な項目を残すためのものです。

## Public Media Objects

対象: `backend/internal/httpserver/public_media.go`

`/api/v1/media/object/*` は認証なしで object key を読み出す設計になっている。投稿本文側の認可を強化しても、object key が共有・推測・ログ露出した場合にメディア本体が読めるかは、key 空間と保存ポリシーに依存する。

確認すること:

- 投稿、プロフィール画像、DM 添付、identity transfer 用メディアが同じ公開 key 空間を共有していないか。
- 非公開投稿、宛先限定投稿、DM 添付に使う object key が公開 media object endpoint から読めない設計か。
- 公開メディアと非公開メディアで endpoint または署名付き URL を分ける必要があるか。

## Personal Access Tokens

対象: `backend/internal/httpserver/server.go`, `backend/internal/httpserver/oauth_handlers.go`

PAT は OAuth access token と同じ Bearer 経路で解決され、OAuth scope より広い権限を持つ。これは開発者向け機能としては自然だが、漏えい時の影響が大きい。

確認すること:

- PAT 作成 UI/API で、OAuth token より強い権限を持つことを明示しているか。
- PAT の scope 分割、最終使用日時、短期有効期限、失効導線が必要か。
- PAT で PAT 作成・削除や DM API まで呼べることが仕様として許容されるか。

## Identity Transfer

対象: `backend/internal/httpserver/identity_transfer_handlers.go`

転送セッションは token hash と target origin を検証しているため今回の最優先修正からは外した。今後は誤 token、誤 origin、試行上限、private/gated include フラグの回帰テストを増やす。
