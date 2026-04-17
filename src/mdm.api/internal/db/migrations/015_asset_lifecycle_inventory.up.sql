-- Asset lifecycle enhancements
ALTER TABLE assets ADD COLUMN IF NOT EXISTS disposed_at TIMESTAMPTZ;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS disposed_by UUID REFERENCES users(id);
ALTER TABLE assets ADD COLUMN IF NOT EXISTS dispose_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE assets ADD COLUMN IF NOT EXISTS transferred_to TEXT NOT NULL DEFAULT '';
ALTER TABLE assets ADD COLUMN IF NOT EXISTS transferred_at TIMESTAMPTZ;

-- Inventory sessions
CREATE TABLE IF NOT EXISTS inventory_sessions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          TEXT NOT NULL,
    description   TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'draft',  -- 'draft', 'in_progress', 'completed'
    created_by    UUID NOT NULL REFERENCES users(id),
    creator_name  TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at    TIMESTAMPTZ,
    completed_at  TIMESTAMPTZ,
    notes         TEXT NOT NULL DEFAULT '',
    total_count   INT NOT NULL DEFAULT 0,
    checked_count INT NOT NULL DEFAULT 0,
    matched_count INT NOT NULL DEFAULT 0,
    missing_count INT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_inv_sessions_status ON inventory_sessions(status);

-- Inventory items (one per asset per session)
CREATE TABLE IF NOT EXISTS inventory_items (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id   UUID NOT NULL REFERENCES inventory_sessions(id) ON DELETE CASCADE,
    asset_id     UUID NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    device_udid  TEXT,
    asset_number TEXT NOT NULL DEFAULT '',
    asset_name   TEXT NOT NULL DEFAULT '',
    found        BOOLEAN,                        -- NULL = not yet checked, true = found, false = missing
    condition    TEXT NOT NULL DEFAULT '',        -- 'good', 'damaged', 'other'
    checked_by   UUID REFERENCES users(id),
    checker_name TEXT NOT NULL DEFAULT '',
    checked_at   TIMESTAMPTZ,
    notes        TEXT NOT NULL DEFAULT '',
    UNIQUE(session_id, asset_id)
);

CREATE INDEX IF NOT EXISTS idx_inv_items_session ON inventory_items(session_id);
CREATE INDEX IF NOT EXISTS idx_inv_items_asset ON inventory_items(asset_id);
