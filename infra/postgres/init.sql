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
    totp_secret TEXT,
    totp_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    patreon_creator_access_token TEXT,
    patreon_creator_refresh_token TEXT,
    patreon_creator_token_expires_at TIMESTAMPTZ,
    patreon_campaign_id TEXT,
    patreon_required_reward_tier_id TEXT,
    patreon_member_access_token TEXT,
    patreon_member_refresh_token TEXT,
    patreon_member_token_expires_at TIMESTAMPTZ,
    patreon_member_user_id TEXT
);
CREATE UNIQUE INDEX IF NOT EXISTS users_handle_lower ON users (lower(handle));

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
    caption TEXT,
    media_type TEXT NOT NULL CHECK (media_type IN ('image', 'video', 'none')),
    object_keys TEXT[] NOT NULL,
    is_nsfw BOOLEAN NOT NULL DEFAULT FALSE,
    visibility TEXT NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'logged_in', 'followers', 'private')),
    view_password_hash TEXT,
    view_password_scope INTEGER NOT NULL DEFAULT 0,
    view_password_text_ranges JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    visible_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    feed_broadcast_done BOOLEAN NOT NULL DEFAULT TRUE,
    CONSTRAINT posts_media_object_keys CHECK (
        (media_type = 'none' AND cardinality(object_keys) = 0)
        OR (media_type = 'image' AND cardinality(object_keys) BETWEEN 1 AND 4)
        OR (media_type = 'video' AND cardinality(object_keys) = 1)
    )
);

CREATE INDEX IF NOT EXISTS idx_posts_created_at ON posts (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_posts_user_id ON posts (user_id);
CREATE INDEX IF NOT EXISTS idx_posts_reply_to ON posts (reply_to_id) WHERE reply_to_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_posts_visible_at ON posts (visible_at DESC);
CREATE INDEX IF NOT EXISTS idx_posts_user_visible_at ON posts (user_id, visible_at DESC);

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
CREATE INDEX IF NOT EXISTS idx_post_reposts_user_created_at ON post_reposts (user_id, created_at DESC);

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
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (local_user_id, remote_actor_id)
);
CREATE INDEX IF NOT EXISTS idx_glipz_protocol_remote_followers_local ON glipz_protocol_remote_followers (local_user_id);

-- Idempotent Glipz-to-remote follow state table (for /api/v1/federation/remote-follow).
CREATE TABLE IF NOT EXISTS federation_remote_follows (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    local_user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    remote_actor_id TEXT NOT NULL,
    remote_inbox TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('pending', 'accepted')),
    follow_activity_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (local_user_id, remote_actor_id)
);
CREATE INDEX IF NOT EXISTS idx_federation_remote_follows_local ON federation_remote_follows (local_user_id);

CREATE TABLE IF NOT EXISTS notes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    title TEXT NOT NULL DEFAULT '',
    body_md TEXT NOT NULL DEFAULT '',
    body_premium_md TEXT NOT NULL DEFAULT '',
    editor_mode TEXT NOT NULL DEFAULT 'markdown' CHECK (editor_mode IN ('markdown', 'richtext')),
    status TEXT NOT NULL DEFAULT 'published' CHECK (status IN ('draft', 'published')),
    visibility TEXT NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'followers', 'private')),
    patreon_campaign_id TEXT,
    patreon_required_reward_tier_id TEXT,
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
