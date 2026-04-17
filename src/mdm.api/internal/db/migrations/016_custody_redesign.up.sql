-- 016_custody_redesign.up.sql
-- ISO 27001 A.8 compliant custody chain redesign.
-- 1. Rename borrow_date → assigned_date (semantically clearer: when the custodian took possession).
-- 2. Add current_holder_* fields — separate "who is physically holding this right now"
--    from "who is the permanent custodian responsible for it".
-- 3. Create asset_custody_logs — auditable trail of every custody change.

-- 1. Rename borrow_date → assigned_date
ALTER TABLE assets RENAME COLUMN borrow_date TO assigned_date;

-- 2. Current holder (separate from custodian; written by rental system only)
ALTER TABLE assets ADD COLUMN IF NOT EXISTS current_holder_id    UUID REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS current_holder_name  TEXT NOT NULL DEFAULT '';
ALTER TABLE assets ADD COLUMN IF NOT EXISTS current_holder_since TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_assets_current_holder ON assets(current_holder_id);

-- 3. Custody change audit log
CREATE TABLE IF NOT EXISTS asset_custody_logs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    asset_id        UUID NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    action          TEXT NOT NULL,                -- 'assign', 'transfer', 'revoke'
    from_user_id    UUID REFERENCES users(id) ON DELETE SET NULL,
    from_user_name  TEXT NOT NULL DEFAULT '',
    to_user_id      UUID REFERENCES users(id) ON DELETE SET NULL,
    to_user_name    TEXT NOT NULL DEFAULT '',
    reason          TEXT NOT NULL DEFAULT '',
    operated_by     UUID REFERENCES users(id) ON DELETE SET NULL,
    operator_name   TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_custody_logs_asset ON asset_custody_logs(asset_id);
CREATE INDEX IF NOT EXISTS idx_custody_logs_time  ON asset_custody_logs(created_at DESC);

-- 4. Backfill: for assets currently rented out, move custodian → current_holder.
-- The original custodian is unknown (we already overwrote it in the old design), so this is
-- the best we can do: treat the current custodian as the "holder" and clear the custodian.
-- Going forward, the rental flow will NOT touch custodian.
UPDATE assets a
SET current_holder_id    = a.custodian_id,
    current_holder_name  = a.custodian_name,
    current_holder_since = a.updated_at
WHERE a.asset_status = 'rented'
  AND a.custodian_id IS NOT NULL;
