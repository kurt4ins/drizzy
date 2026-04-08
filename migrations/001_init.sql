-- +goose Up

CREATE TABLE users (
    id                  UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    telegram_id         BIGINT          NOT NULL UNIQUE,
    telegram_username   TEXT,
    registered_at       TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    is_active           BOOLEAN         NOT NULL DEFAULT TRUE,
    referral_code       TEXT            NOT NULL UNIQUE,
    referred_by_user_id UUID            REFERENCES users(id)
);

CREATE TABLE profiles (
    user_id             UUID            PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    name                TEXT            NOT NULL,
    bio                 TEXT,
    age                 SMALLINT        NOT NULL,
    birth_date          DATE,
    gender              TEXT            NOT NULL CHECK (gender IN ('male', 'female')),
    city                TEXT            NOT NULL,
    latitude            DOUBLE PRECISION,
    longitude           DOUBLE PRECISION,
    interests           JSONB           NOT NULL DEFAULT '[]',
    completeness_score  REAL            NOT NULL DEFAULT 0,
    updated_at          TIMESTAMPTZ     NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_profiles_city   ON profiles (city);
CREATE INDEX idx_profiles_gender ON profiles (gender);
CREATE INDEX idx_profiles_age    ON profiles (age);

CREATE TABLE profile_photos (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id  UUID        NOT NULL REFERENCES profiles(user_id) ON DELETE CASCADE,
    s3_key      TEXT        NOT NULL,
    sort_order  INT         NOT NULL DEFAULT 0,
    is_primary  BOOLEAN     NOT NULL DEFAULT FALSE,
    uploaded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_profile_photos_profile ON profile_photos (profile_id, sort_order);

CREATE TABLE user_preferences (
    user_id         UUID            PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    pref_age_min    SMALLINT,
    pref_age_max    SMALLINT,
    pref_gender     TEXT[]          NOT NULL DEFAULT '{}',
    pref_radius_km  INT,
    updated_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW()
);

CREATE TABLE interactions (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_user_id   UUID        NOT NULL REFERENCES users(id),
    target_user_id  UUID        NOT NULL REFERENCES users(id),
    action_type     TEXT        NOT NULL CHECK (action_type IN ('like', 'skip')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (actor_user_id, target_user_id)
);

CREATE INDEX idx_interactions_actor  ON interactions (actor_user_id, created_at DESC);
CREATE INDEX idx_interactions_target ON interactions (target_user_id, action_type);

CREATE TABLE matches (
    id                      UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_a_id               UUID        NOT NULL REFERENCES users(id),
    user_b_id               UUID        NOT NULL REFERENCES users(id),
    matched_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    conversation_started    BOOLEAN     NOT NULL DEFAULT FALSE,
    conversation_started_at TIMESTAMPTZ,
    UNIQUE (user_a_id, user_b_id),
    CHECK (user_a_id < user_b_id)
);

CREATE INDEX idx_matches_user_a ON matches (user_a_id);
CREATE INDEX idx_matches_user_b ON matches (user_b_id);

CREATE TABLE user_behavior_stats (
    user_id                 UUID        PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    likes_received          INT         NOT NULL DEFAULT 0,
    skips_received          INT         NOT NULL DEFAULT 0,
    matches_count           INT         NOT NULL DEFAULT 0,
    conversations_started   INT         NOT NULL DEFAULT 0,
    last_active_at          TIMESTAMPTZ,
    activity_histogram      JSONB       NOT NULL DEFAULT '{}',
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- TODO: ranking algorithm not yet defined; schema is a placeholder.
-- Add score component columns here once the algorithm is decided.
CREATE TABLE user_ratings (
    user_id             UUID                PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    score               DOUBLE PRECISION    NOT NULL DEFAULT 0,
    algorithm_version   TEXT                NOT NULL DEFAULT 'v0',
    recalculated_at     TIMESTAMPTZ         NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_user_ratings_score ON user_ratings (score DESC);

CREATE TABLE referrals (
    id            UUID                PRIMARY KEY DEFAULT gen_random_uuid(),
    referrer_id   UUID                NOT NULL REFERENCES users(id),
    referred_id   UUID                NOT NULL REFERENCES users(id),
    created_at    TIMESTAMPTZ         NOT NULL DEFAULT NOW(),
    bonus_applied DOUBLE PRECISION    NOT NULL DEFAULT 0,
    UNIQUE (referred_id)
);

CREATE INDEX idx_referrals_referrer ON referrals (referrer_id);

-- +goose Down

DROP TABLE IF EXISTS referrals;
DROP TABLE IF EXISTS user_ratings;
DROP TABLE IF EXISTS user_behavior_stats;
DROP TABLE IF EXISTS matches;
DROP TABLE IF EXISTS interactions;
DROP TABLE IF EXISTS user_preferences;
DROP TABLE IF EXISTS profile_photos;
DROP TABLE IF EXISTS profiles;
DROP TABLE IF EXISTS users;
