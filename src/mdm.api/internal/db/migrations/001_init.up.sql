CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS users (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username   TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role       TEXT NOT NULL DEFAULT 'viewer',
    display_name TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS devices (
    udid              TEXT PRIMARY KEY,
    serial_number     TEXT NOT NULL DEFAULT '',
    device_name       TEXT NOT NULL DEFAULT '',
    model             TEXT NOT NULL DEFAULT '',
    os_version        TEXT NOT NULL DEFAULT '',
    last_seen         TIMESTAMPTZ NOT NULL DEFAULT now(),
    enrollment_status TEXT NOT NULL DEFAULT 'enrolled'
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id   TEXT NOT NULL DEFAULT '',
    username  TEXT NOT NULL DEFAULT '',
    action    TEXT NOT NULL,
    target    TEXT NOT NULL DEFAULT '',
    detail    TEXT NOT NULL DEFAULT '',
    timestamp TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id ON audit_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_timestamp ON audit_logs(timestamp DESC);

CREATE TABLE IF NOT EXISTS profiles (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    filename   TEXT NOT NULL,
    content    BYTEA NOT NULL,
    size       INTEGER NOT NULL DEFAULT 0,
    uploaded_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- admin user will be bootstrapped at startup if no users exist
