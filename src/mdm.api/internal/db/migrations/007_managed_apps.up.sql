-- Managed apps registry: apps available for installation
CREATE TABLE IF NOT EXISTS managed_apps (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL,
    bundle_id       TEXT NOT NULL UNIQUE,
    app_type        TEXT NOT NULL DEFAULT 'vpp',  -- 'vpp' or 'enterprise'
    itunes_store_id TEXT NOT NULL DEFAULT '',
    manifest_url    TEXT NOT NULL DEFAULT '',
    purchased_qty   INTEGER NOT NULL DEFAULT 0,
    notes           TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Device-app install bindings: tracks which apps are installed on which devices
CREATE TABLE IF NOT EXISTS device_apps (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_udid     TEXT NOT NULL REFERENCES devices(udid) ON DELETE CASCADE,
    app_id          UUID NOT NULL REFERENCES managed_apps(id) ON DELETE CASCADE,
    installed_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(device_udid, app_id)
);

CREATE INDEX IF NOT EXISTS idx_device_apps_device ON device_apps(device_udid);
CREATE INDEX IF NOT EXISTS idx_device_apps_app ON device_apps(app_id);

-- Backfill: Register Teams and Google Drive as managed apps
INSERT INTO managed_apps (name, bundle_id, app_type, purchased_qty)
VALUES ('Teams', 'com.microsoft.skype.teams', 'vpp', 35)
ON CONFLICT (bundle_id) DO NOTHING;

INSERT INTO managed_apps (name, bundle_id, app_type, purchased_qty)
VALUES ('雲端硬碟', 'com.google.Drive', 'vpp', 35)
ON CONFLICT (bundle_id) DO NOTHING;

-- Backfill: Bind Teams and Google Drive to all existing devices
INSERT INTO device_apps (device_udid, app_id)
SELECT d.udid, ma.id
FROM devices d
CROSS JOIN managed_apps ma
WHERE ma.bundle_id IN ('com.microsoft.skype.teams', 'com.google.Drive')
ON CONFLICT (device_udid, app_id) DO NOTHING;
