CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    handle TEXT NOT NULL,
    birth_date DATE,
    display_name TEXT NOT NULL DEFAULT '',
    bio TEXT NOT NULL DEFAULT '',
    avatar_object_key TEXT,
    header_object_key TEXT,
    portable_id TEXT,
    account_public_key TEXT NOT NULL DEFAULT '',
    account_private_key_encrypted TEXT NOT NULL DEFAULT '',
    moved_to_acct TEXT NOT NULL DEFAULT '',
    moved_at TIMESTAMPTZ,
    also_known_as TEXT[] NOT NULL DEFAULT '{}',
    totp_secret TEXT,
    totp_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS users_handle_lower ON users (lower(handle));
CREATE UNIQUE INDEX IF NOT EXISTS users_portable_id_unique ON users (portable_id) WHERE portable_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS pending_user_registrations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    handle TEXT NOT NULL DEFAULT '',
    birth_date DATE,
    token_sha256 TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    verified_user_id UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_pending_user_registrations_expires_at
    ON pending_user_registrations (expires_at);

CREATE TABLE IF NOT EXISTS posts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    reply_to_id UUID REFERENCES posts (id) ON DELETE CASCADE,
    reply_to_remote_object_iri TEXT NOT NULL DEFAULT '',
    caption TEXT,
    media_type TEXT NOT NULL CHECK (media_type IN ('image', 'video', 'audio', 'none')),
    object_keys TEXT[] NOT NULL,
    is_nsfw BOOLEAN NOT NULL DEFAULT FALSE,
    visibility TEXT NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'logged_in', 'followers', 'private')),
    view_password_hash TEXT,
    view_password_scope INTEGER NOT NULL DEFAULT 0,
    view_password_text_ranges JSONB NOT NULL DEFAULT '[]'::jsonb,
    membership_provider TEXT NOT NULL DEFAULT '',
    membership_creator_id TEXT NOT NULL DEFAULT '',
    membership_tier_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    visible_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    feed_broadcast_done BOOLEAN NOT NULL DEFAULT TRUE,
    group_id UUID,
    CONSTRAINT posts_media_object_keys CHECK (
        (media_type = 'none' AND cardinality(object_keys) = 0)
        OR (media_type = 'image' AND cardinality(object_keys) BETWEEN 1 AND 4)
        OR (media_type = 'video' AND cardinality(object_keys) = 1)
        OR (media_type = 'audio' AND cardinality(object_keys) = 1)
    )
);

CREATE INDEX IF NOT EXISTS idx_posts_created_at ON posts (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_posts_user_id ON posts (user_id);
CREATE INDEX IF NOT EXISTS idx_posts_reply_to ON posts (reply_to_id) WHERE reply_to_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_posts_visible_at ON posts (visible_at DESC);
CREATE INDEX IF NOT EXISTS idx_posts_user_visible_at ON posts (user_id, visible_at DESC);
CREATE INDEX IF NOT EXISTS idx_posts_feed_visible_top
    ON posts (visible_at DESC, id DESC)
    WHERE reply_to_id IS NULL
      AND COALESCE(btrim(reply_to_remote_object_iri), '') = ''
      AND group_id IS NULL;
CREATE INDEX IF NOT EXISTS idx_posts_user_feed_visible_top
    ON posts (user_id, visible_at DESC, id DESC)
    WHERE reply_to_id IS NULL
      AND COALESCE(btrim(reply_to_remote_object_iri), '') = ''
      AND group_id IS NULL;

CREATE TABLE IF NOT EXISTS hashtags (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tag TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT hashtags_tag_not_blank CHECK (btrim(tag) <> '')
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_hashtags_tag ON hashtags (tag);

CREATE TABLE IF NOT EXISTS post_hashtags (
    post_id UUID NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
    hashtag_id UUID NOT NULL REFERENCES hashtags (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (post_id, hashtag_id)
);
CREATE INDEX IF NOT EXISTS idx_post_hashtags_hashtag ON post_hashtags (hashtag_id, post_id);

CREATE TABLE IF NOT EXISTS post_polls (
    post_id UUID PRIMARY KEY REFERENCES posts (id) ON DELETE CASCADE,
    ends_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE IF NOT EXISTS post_poll_options (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    post_id UUID NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
    position SMALLINT NOT NULL,
    label TEXT NOT NULL,
    UNIQUE (post_id, position)
);
CREATE INDEX IF NOT EXISTS idx_post_poll_options_post ON post_poll_options(post_id);
CREATE TABLE IF NOT EXISTS post_poll_votes (
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    post_id UUID NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
    option_id UUID NOT NULL REFERENCES post_poll_options(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, post_id)
);
CREATE INDEX IF NOT EXISTS idx_post_poll_votes_post ON post_poll_votes(post_id);
CREATE INDEX IF NOT EXISTS idx_post_poll_votes_option ON post_poll_votes(option_id);

CREATE TABLE IF NOT EXISTS post_likes (
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    post_id UUID NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, post_id)
);
CREATE INDEX IF NOT EXISTS idx_post_likes_post_id ON post_likes (post_id);
CREATE INDEX IF NOT EXISTS idx_post_likes_user_created_at ON post_likes (user_id, created_at DESC);
CREATE TABLE IF NOT EXISTS post_reactions (
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    post_id UUID NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
    emoji TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, post_id, emoji),
    CONSTRAINT post_reactions_emoji_non_empty CHECK (char_length(btrim(emoji)) > 0)
);
CREATE INDEX IF NOT EXISTS idx_post_reactions_post_id ON post_reactions (post_id);
CREATE INDEX IF NOT EXISTS idx_post_reactions_user_created_at ON post_reactions (user_id, created_at DESC);
CREATE TABLE IF NOT EXISTS custom_emojis (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shortcode_name TEXT NOT NULL,
    owner_user_id UUID REFERENCES users (id) ON DELETE CASCADE,
    domain TEXT NOT NULL DEFAULT '',
    object_key TEXT NOT NULL,
    is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT custom_emojis_shortcode_name_non_empty CHECK (char_length(btrim(shortcode_name)) > 0),
    CONSTRAINT custom_emojis_object_key_non_empty CHECK (char_length(btrim(object_key)) > 0)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_custom_emojis_site_shortcode
ON custom_emojis (lower(shortcode_name))
WHERE owner_user_id IS NULL AND COALESCE(btrim(domain), '') = '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_custom_emojis_user_shortcode
ON custom_emojis (owner_user_id, lower(shortcode_name))
WHERE owner_user_id IS NOT NULL AND COALESCE(btrim(domain), '') = '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_custom_emojis_remote_shortcode
ON custom_emojis (lower(shortcode_name), lower(domain))
WHERE COALESCE(btrim(domain), '') <> '';
CREATE INDEX IF NOT EXISTS idx_custom_emojis_enabled ON custom_emojis (is_enabled, created_at DESC);
INSERT INTO post_reactions (user_id, post_id, emoji, created_at)
SELECT pl.user_id, pl.post_id, '❤️', pl.created_at
FROM post_likes pl
ON CONFLICT (user_id, post_id, emoji) DO NOTHING;

CREATE TABLE IF NOT EXISTS post_reposts (
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    post_id UUID NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, post_id)
);
CREATE INDEX IF NOT EXISTS idx_post_reposts_post_id ON post_reposts (post_id);
CREATE INDEX IF NOT EXISTS idx_post_reposts_created_at ON post_reposts (created_at);
CREATE INDEX IF NOT EXISTS idx_post_reposts_created_desc ON post_reposts (created_at DESC, user_id, post_id);
CREATE INDEX IF NOT EXISTS idx_post_reposts_user_created_at ON post_reposts (user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS post_bookmarks (
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    post_id UUID NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, post_id)
);
CREATE INDEX IF NOT EXISTS idx_post_bookmarks_created_at ON post_bookmarks (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_post_bookmarks_user_created_at ON post_bookmarks (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_post_bookmarks_post_id ON post_bookmarks (post_id);

CREATE TABLE IF NOT EXISTS user_follows (
    follower_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    followee_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (follower_id, followee_id),
    CONSTRAINT user_follows_no_self CHECK (follower_id <> followee_id)
);
CREATE INDEX IF NOT EXISTS idx_user_follows_followee ON user_follows (followee_id);
CREATE INDEX IF NOT EXISTS idx_user_follows_follower ON user_follows (follower_id);

CREATE TABLE IF NOT EXISTS notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    recipient_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    actor_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('reply', 'like', 'repost', 'follow', 'dm_invite')),
    subject_post_id UUID REFERENCES posts (id) ON DELETE SET NULL,
    actor_post_id UUID REFERENCES posts (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    read_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_notifications_recipient_created ON notifications (recipient_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notifications_recipient_unread ON notifications (recipient_id) WHERE read_at IS NULL;

-- Keep the allowed notification kinds in sync with backend migrations.
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_kind_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check
    CHECK (kind IN ('reply', 'like', 'repost', 'follow', 'dm_invite'));

-- Glipz Protocol tables used by federation follow + remote follower counts.
CREATE TABLE IF NOT EXISTS federation_remote_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    portable_id TEXT NOT NULL,
    current_acct TEXT NOT NULL DEFAULT '',
    profile_url TEXT NOT NULL DEFAULT '',
    posts_url TEXT NOT NULL DEFAULT '',
    inbox_url TEXT NOT NULL DEFAULT '',
    public_key TEXT NOT NULL DEFAULT '',
    moved_to TEXT NOT NULL DEFAULT '',
    moved_from TEXT NOT NULL DEFAULT '',
    also_known_as TEXT[] NOT NULL DEFAULT '{}',
    last_verified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT federation_remote_accounts_portable_nonempty CHECK (char_length(btrim(portable_id)) > 0)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_federation_remote_accounts_portable
    ON federation_remote_accounts (portable_id);
CREATE INDEX IF NOT EXISTS idx_federation_remote_accounts_current_acct
    ON federation_remote_accounts (lower(current_acct)) WHERE COALESCE(btrim(current_acct), '') <> '';

CREATE TABLE IF NOT EXISTS user_glipz_protocol_keys (
    user_id UUID PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    private_key_pem TEXT NOT NULL,
    public_key_pem TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS glipz_protocol_remote_followers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    local_user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    remote_actor_id TEXT NOT NULL,
    remote_inbox TEXT NOT NULL,
    remote_account_id UUID REFERENCES federation_remote_accounts (id) ON DELETE SET NULL,
    remote_current_acct TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (local_user_id, remote_actor_id)
);
CREATE INDEX IF NOT EXISTS idx_glipz_protocol_remote_followers_local ON glipz_protocol_remote_followers (local_user_id);

CREATE TABLE IF NOT EXISTS glipz_protocol_outbox_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    author_user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    post_id UUID NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('create', 'update', 'delete', 'announce', 'post_created', 'post_updated', 'post_deleted', 'repost_created', 'post_liked', 'post_unliked', 'post_reaction_added', 'post_reaction_removed', 'poll_voted', 'poll_tally_updated', 'dm_invite', 'dm_accept', 'dm_reject', 'dm_message', 'account_moved')),
    inbox_url TEXT NOT NULL,
    payload JSONB NOT NULL,
    attempt_count INT NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    locked_until TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'completed', 'dead')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_glipz_protocol_outbox_pending
    ON glipz_protocol_outbox_deliveries (next_attempt_at)
    WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_glipz_protocol_outbox_author
    ON glipz_protocol_outbox_deliveries (author_user_id);
CREATE INDEX IF NOT EXISTS idx_glipz_protocol_outbox_status_created
    ON glipz_protocol_outbox_deliveries (status, created_at DESC);

-- Idempotent Glipz-to-remote follow state table (for /api/v1/federation/remote-follow).
CREATE TABLE IF NOT EXISTS federation_remote_follows (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    local_user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    remote_actor_id TEXT NOT NULL,
    remote_inbox TEXT NOT NULL,
    remote_account_id UUID REFERENCES federation_remote_accounts (id) ON DELETE SET NULL,
    remote_current_acct TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL CHECK (state IN ('pending', 'accepted')),
    follow_activity_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (local_user_id, remote_actor_id)
);
CREATE INDEX IF NOT EXISTS idx_federation_remote_follows_local ON federation_remote_follows (local_user_id);
CREATE INDEX IF NOT EXISTS idx_federation_remote_follows_actor_accepted
    ON federation_remote_follows (remote_actor_id, local_user_id)
    WHERE state = 'accepted';

CREATE TABLE IF NOT EXISTS notes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    title TEXT NOT NULL DEFAULT '',
    body_md TEXT NOT NULL DEFAULT '',
    body_premium_md TEXT NOT NULL DEFAULT '',
    editor_mode TEXT NOT NULL DEFAULT 'markdown' CHECK (editor_mode IN ('markdown', 'richtext')),
    status TEXT NOT NULL DEFAULT 'published' CHECK (status IN ('draft', 'published')),
    visibility TEXT NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'followers', 'private')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_notes_user_updated ON notes (user_id, updated_at DESC);

-- OAuth and personal API tokens (same as migrate.RunOAuthAPI; for first-time init when used instead of migrations)
CREATE TABLE IF NOT EXISTS oauth_clients (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    client_secret_hash TEXT NOT NULL,
    redirect_uris TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_oauth_clients_user ON oauth_clients (user_id);
CREATE TABLE IF NOT EXISTS oauth_authorization_codes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code_sha256 TEXT NOT NULL UNIQUE,
    client_id UUID NOT NULL REFERENCES oauth_clients (id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    redirect_uri TEXT NOT NULL,
    scope TEXT NOT NULL DEFAULT 'posts',
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_oauth_codes_expires ON oauth_authorization_codes (expires_at);
CREATE TABLE IF NOT EXISTS api_personal_access_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    label TEXT NOT NULL DEFAULT '',
    token_prefix TEXT NOT NULL,
    secret_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_api_pat_user ON api_personal_access_tokens (user_id);

CREATE TABLE IF NOT EXISTS identity_transfer_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    portable_id TEXT NOT NULL DEFAULT '',
    token_hash TEXT NOT NULL,
    token_nonce TEXT NOT NULL DEFAULT '',
    allowed_target_origin TEXT NOT NULL DEFAULT '',
    include_private BOOLEAN NOT NULL DEFAULT FALSE,
    include_gated BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_ip_hash TEXT NOT NULL DEFAULT '',
    last_used_ip_hash TEXT NOT NULL DEFAULT '',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT identity_transfer_sessions_token_hash_nonempty CHECK (char_length(btrim(token_hash)) > 0)
);
CREATE INDEX IF NOT EXISTS idx_identity_transfer_sessions_user
    ON identity_transfer_sessions (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_identity_transfer_sessions_active
    ON identity_transfer_sessions (id, expires_at)
    WHERE revoked_at IS NULL AND used_at IS NULL;

CREATE TABLE IF NOT EXISTS identity_transfer_import_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    source_origin TEXT NOT NULL,
    target_origin TEXT NOT NULL DEFAULT '',
    source_session_id UUID NOT NULL,
    source_token_encrypted TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
    total_posts INTEGER NOT NULL DEFAULT 0,
    imported_posts INTEGER NOT NULL DEFAULT 0,
    failed_posts INTEGER NOT NULL DEFAULT 0,
    total_items INTEGER NOT NULL DEFAULT 0,
    imported_items INTEGER NOT NULL DEFAULT 0,
    stats JSONB NOT NULL DEFAULT '{}'::jsonb,
    next_cursor TEXT NOT NULL DEFAULT '',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    locked_until TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    include_private BOOLEAN NOT NULL DEFAULT FALSE,
    include_gated BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_identity_transfer_import_jobs_claim
    ON identity_transfer_import_jobs (next_attempt_at, created_at)
    WHERE status IN ('pending', 'running');
CREATE INDEX IF NOT EXISTS idx_identity_transfer_import_jobs_user
    ON identity_transfer_import_jobs (user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS identity_transfer_post_mappings (
    job_id UUID NOT NULL REFERENCES identity_transfer_import_jobs (id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    source_post_id TEXT NOT NULL,
    original_object_id TEXT NOT NULL,
    new_post_id UUID REFERENCES posts (id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'imported', 'failed', 'skipped')),
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (job_id, original_object_id)
);
CREATE INDEX IF NOT EXISTS idx_identity_transfer_post_mappings_user
    ON identity_transfer_post_mappings (user_id, created_at DESC);
