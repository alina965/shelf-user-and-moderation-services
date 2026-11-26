BEGIN;

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY,
    username TEXT NOT NULL,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    role TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'platform_moderator')),
    description VARCHAR(100) NULL,
    avatar_key TEXT NULL,
    banner_key TEXT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sessions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    UNIQUE (id, user_id)
);

CREATE TABLE IF NOT EXISTS idempotency_keys (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    operation TEXT NOT NULL,
    key TEXT NOT NULL CHECK (length(key) BETWEEN 1 AND 128),
    recipient_id UUID NOT NULL,
    response JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    expires_at TIMESTAMPTZ NOT NULL DEFAULT (clock_timestamp() + interval '24 hours'),
    PRIMARY KEY (user_id, operation, key),
    CHECK (expires_at > created_at)
);

CREATE INDEX IF NOT EXISTS idempotency_keys_expires_at_idx ON idempotency_keys(expires_at);

CREATE TABLE IF NOT EXISTS refresh_tokens (
    id UUID PRIMARY KEY,
    token TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    is_revoked BOOLEAN NOT NULL DEFAULT FALSE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL,
    session_id UUID NOT NULL,
    CONSTRAINT refresh_tokens_session_fk
        FOREIGN KEY (session_id, user_id) REFERENCES sessions(id, user_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS profile_privacy (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    profile_visibility TEXT NOT NULL DEFAULT 'public' CHECK (profile_visibility IN ('public', 'friends', 'private')),
    description_visibility TEXT NOT NULL DEFAULT 'public' CHECK (description_visibility IN ('public', 'friends', 'private')),
    friends_visibility TEXT NOT NULL DEFAULT 'public' CHECK (friends_visibility IN ('public', 'friends', 'private')),
    communities_visibility TEXT NOT NULL DEFAULT 'public' CHECK (communities_visibility IN ('public', 'friends', 'private')),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS login_attempts (
    identifier TEXT PRIMARY KEY,
    failed_count SMALLINT NOT NULL DEFAULT 0 CHECK (failed_count BETWEEN 0 AND 5),
    last_failed_at TIMESTAMPTZ,
    locked_until TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS friend_requests (
    id UUID PRIMARY KEY,
    sender_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recipient_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'rejected', 'cancelled')),
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    responded_at TIMESTAMPTZ,
    CHECK (sender_id <> recipient_id)
);

CREATE TABLE IF NOT EXISTS friendships (
    first_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    second_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (first_user_id, second_user_id),
    CHECK (first_user_id < second_user_id)
);

CREATE TABLE IF NOT EXISTS user_blocks (
    blocker_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    blocked_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (blocker_id, blocked_id),
    CHECK (blocker_id <> blocked_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS users_email_ci_unique ON users (lower(email));
CREATE UNIQUE INDEX IF NOT EXISTS users_username_ci_unique ON users (lower(username));
CREATE INDEX IF NOT EXISTS sessions_user_id_idx ON sessions(user_id);
CREATE INDEX IF NOT EXISTS refresh_tokens_session_id_idx ON refresh_tokens(session_id);

CREATE UNIQUE INDEX IF NOT EXISTS friend_requests_pending_pair_unique
    ON friend_requests (
        LEAST(sender_id, recipient_id),
        GREATEST(sender_id, recipient_id)
    )
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS friend_requests_incoming_idx
    ON friend_requests (recipient_id, created_at DESC, id DESC)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS friend_requests_outgoing_idx
    ON friend_requests (sender_id, created_at DESC, id DESC)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS friendships_second_user_idx
    ON friendships (second_user_id, first_user_id);

CREATE INDEX IF NOT EXISTS user_blocks_blocked_user_idx
    ON user_blocks (blocked_id, blocker_id);

COMMIT;
