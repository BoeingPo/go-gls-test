-- Users table
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    age INT NOT NULL CHECK (age > 0),
    country VARCHAR(2) NOT NULL,
    subscription_type VARCHAR(20) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_users_country ON users(country);
CREATE INDEX idx_users_subscription ON users(subscription_type);

-- Content table
CREATE TABLE content (
    id BIGSERIAL PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    genre VARCHAR(50) NOT NULL,
    popularity_score DOUBLE PRECISION NOT NULL CHECK (popularity_score >= 0),
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_content_genre ON content(genre);
CREATE INDEX idx_content_popularity ON content(popularity_score DESC);

-- User watch history table
CREATE TABLE user_watch_history (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content_id BIGINT NOT NULL REFERENCES content(id) ON DELETE CASCADE,
    watched_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_watch_history_user ON user_watch_history(user_id);
CREATE INDEX idx_watch_history_content ON user_watch_history(content_id);
CREATE INDEX idx_watch_history_composite ON user_watch_history(user_id, watched_at DESC);

-- Country genre restrictions table
CREATE TABLE restrict_country_genre (
    id SERIAL PRIMARY KEY,
    country VARCHAR(2) NOT NULL,
    genre VARCHAR(50) NOT NULL
);

-- Subscription genre permissions table
CREATE TABLE subscription_genre (
    id SERIAL PRIMARY KEY,
    subscription VARCHAR(20) NOT NULL,
    genre VARCHAR(50) NOT NULL
);
