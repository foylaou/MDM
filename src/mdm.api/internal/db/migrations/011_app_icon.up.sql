-- Store app icon URL from iTunes Lookup API
ALTER TABLE managed_apps ADD COLUMN IF NOT EXISTS icon_url TEXT NOT NULL DEFAULT '';
