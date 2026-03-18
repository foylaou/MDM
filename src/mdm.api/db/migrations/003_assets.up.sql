CREATE TABLE IF NOT EXISTS assets (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_udid     TEXT REFERENCES devices(udid) ON DELETE SET NULL,
    asset_number    TEXT NOT NULL DEFAULT '',
    name            TEXT NOT NULL DEFAULT '',
    spec            TEXT NOT NULL DEFAULT '',
    quantity        INTEGER NOT NULL DEFAULT 1,
    unit            TEXT NOT NULL DEFAULT '台',
    acquired_date   DATE,
    unit_price      NUMERIC(12,2) NOT NULL DEFAULT 0,
    purpose         TEXT NOT NULL DEFAULT '',
    borrow_date     DATE,
    custodian_id    UUID REFERENCES users(id) ON DELETE SET NULL,
    custodian_name  TEXT NOT NULL DEFAULT '',
    location        TEXT NOT NULL DEFAULT '',
    asset_category  TEXT NOT NULL DEFAULT '',
    notes           TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_assets_device ON assets(device_udid);
CREATE INDEX IF NOT EXISTS idx_assets_custodian ON assets(custodian_id);
CREATE INDEX IF NOT EXISTS idx_assets_number ON assets(asset_number);
