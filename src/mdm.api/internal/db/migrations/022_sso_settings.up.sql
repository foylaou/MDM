-- SSO / OIDC settings (single-row, like mail_settings).
CREATE TABLE IF NOT EXISTS sso_settings (
    id             TEXT PRIMARY KEY DEFAULT 'default',
    enabled        BOOLEAN NOT NULL DEFAULT FALSE,
    issuer_url     TEXT    NOT NULL DEFAULT '',
    client_id      TEXT    NOT NULL DEFAULT '',
    client_secret  TEXT    NOT NULL DEFAULT '',
    redirect_url   TEXT    NOT NULL DEFAULT '',
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by     TEXT
);

INSERT INTO sso_settings (id) VALUES ('default') ON CONFLICT DO NOTHING;
