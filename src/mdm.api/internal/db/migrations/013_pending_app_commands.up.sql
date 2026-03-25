-- Track pending app install/uninstall commands until device acknowledges
CREATE TABLE IF NOT EXISTS pending_app_commands (
    command_uuid TEXT PRIMARY KEY,
    action       TEXT NOT NULL,  -- 'install' or 'uninstall'
    device_udid  TEXT NOT NULL,
    app_id       UUID NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_pending_app_cmd_uuid ON pending_app_commands(command_uuid);
