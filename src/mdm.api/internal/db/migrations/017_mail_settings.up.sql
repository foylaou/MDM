-- Mail server settings (outgoing SMTP + incoming IMAP/POP3).
-- Single-row table keyed by id='default' so GET/PUT always target the same row.
CREATE TABLE IF NOT EXISTS mail_settings (
    id                  TEXT        PRIMARY KEY DEFAULT 'default',

    -- Outgoing (SMTP)
    smtp_enabled        BOOLEAN     NOT NULL DEFAULT FALSE,
    smtp_host           TEXT        NOT NULL DEFAULT '',
    smtp_port           TEXT        NOT NULL DEFAULT '587',
    smtp_username       TEXT        NOT NULL DEFAULT '',
    smtp_password       TEXT        NOT NULL DEFAULT '',
    smtp_from           TEXT        NOT NULL DEFAULT '',
    smtp_from_name      TEXT        NOT NULL DEFAULT '',
    smtp_tls            BOOLEAN     NOT NULL DEFAULT TRUE,

    -- Incoming (IMAP or POP3)
    incoming_enabled    BOOLEAN     NOT NULL DEFAULT FALSE,
    incoming_protocol   TEXT        NOT NULL DEFAULT 'imap',  -- 'imap' | 'pop3'
    incoming_host       TEXT        NOT NULL DEFAULT '',
    incoming_port       TEXT        NOT NULL DEFAULT '993',
    incoming_username   TEXT        NOT NULL DEFAULT '',
    incoming_password   TEXT        NOT NULL DEFAULT '',
    incoming_tls        BOOLEAN     NOT NULL DEFAULT TRUE,
    incoming_mailbox    TEXT        NOT NULL DEFAULT 'INBOX',

    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by          TEXT
);

-- Seed the default row so UPSERTs work without a pre-check.
INSERT INTO mail_settings (id) VALUES ('default') ON CONFLICT DO NOTHING;
