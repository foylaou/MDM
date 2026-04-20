-- 018_rental_asset_id.up.sql
-- Rentals become asset-centric instead of device-centric so standalone assets
-- (assets with no MDM device) can be borrowed too.
-- 1. Add rentals.asset_id referencing assets(id).
-- 2. Backfill asset_id from assets.device_udid for existing rentals.
-- 3. Make rentals.device_udid nullable (kept for MDM-linked rentals for convenience).

ALTER TABLE rentals
    ADD COLUMN IF NOT EXISTS asset_id UUID REFERENCES assets(id) ON DELETE SET NULL;

-- Backfill: match existing rentals to the asset with the same device_udid.
-- If multiple assets share the same UDID (shouldn't happen, but be defensive) take the first.
UPDATE rentals r
SET asset_id = a.id
FROM assets a
WHERE r.asset_id IS NULL
  AND r.device_udid IS NOT NULL
  AND a.device_udid = r.device_udid;

-- device_udid was NOT NULL; loosen it so rentals for standalone assets can exist.
ALTER TABLE rentals ALTER COLUMN device_udid DROP NOT NULL;

CREATE INDEX IF NOT EXISTS idx_rentals_asset ON rentals(asset_id);
