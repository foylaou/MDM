-- Module-level permissions for ISO 27001 RBAC
CREATE TABLE IF NOT EXISTS user_module_permissions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    module      TEXT NOT NULL,   -- 'asset', 'mdm', 'rental'
    permission  TEXT NOT NULL,   -- 'viewer', 'operator', 'manager', 'requester', 'approver'
    granted_by  UUID REFERENCES users(id),
    granted_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, module)
);

CREATE INDEX IF NOT EXISTS idx_ump_user ON user_module_permissions(user_id);
CREATE INDEX IF NOT EXISTS idx_ump_module ON user_module_permissions(module);

-- Users table: add system_role and email
ALTER TABLE users ADD COLUMN IF NOT EXISTS system_role TEXT NOT NULL DEFAULT 'user';
ALTER TABLE users ADD COLUMN IF NOT EXISTS email TEXT NOT NULL DEFAULT '';

-- Migrate existing roles to system_role
UPDATE users SET system_role = 'sys_admin' WHERE role = 'admin' AND system_role = 'user';

-- Migrate existing role-based access to module permissions
-- admin → all modules as manager
INSERT INTO user_module_permissions (user_id, module, permission)
SELECT id, m.module, 'manager'
FROM users, (VALUES ('asset'), ('mdm'), ('rental')) AS m(module)
WHERE role = 'admin'
ON CONFLICT DO NOTHING;

-- operator → mdm:operator, rental:manager, asset:operator
INSERT INTO user_module_permissions (user_id, module, permission)
SELECT id, 'mdm', 'operator' FROM users WHERE role = 'operator'
ON CONFLICT DO NOTHING;
INSERT INTO user_module_permissions (user_id, module, permission)
SELECT id, 'rental', 'manager' FROM users WHERE role = 'operator'
ON CONFLICT DO NOTHING;
INSERT INTO user_module_permissions (user_id, module, permission)
SELECT id, 'asset', 'operator' FROM users WHERE role = 'operator'
ON CONFLICT DO NOTHING;

-- viewer → mdm:viewer, rental:requester, asset:viewer
INSERT INTO user_module_permissions (user_id, module, permission)
SELECT id, 'mdm', 'viewer' FROM users WHERE role = 'viewer'
ON CONFLICT DO NOTHING;
INSERT INTO user_module_permissions (user_id, module, permission)
SELECT id, 'rental', 'requester' FROM users WHERE role = 'viewer'
ON CONFLICT DO NOTHING;
INSERT INTO user_module_permissions (user_id, module, permission)
SELECT id, 'asset', 'viewer' FROM users WHERE role = 'viewer'
ON CONFLICT DO NOTHING;

-- Notification records (for audit trail)
CREATE TABLE IF NOT EXISTS notifications (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type          TEXT NOT NULL DEFAULT 'email',
    event         TEXT NOT NULL,          -- 'rental_request', 'rental_approved', etc.
    recipient     TEXT NOT NULL,          -- email address
    subject       TEXT NOT NULL,
    body          TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'pending',  -- 'pending', 'sent', 'failed'
    error_message TEXT,
    reference_id  TEXT,                   -- related rental ID, etc.
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at       TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_notifications_event ON notifications(event);
CREATE INDEX IF NOT EXISTS idx_notifications_status ON notifications(status);
CREATE INDEX IF NOT EXISTS idx_notifications_reference ON notifications(reference_id);

-- Audit logs enhancement for ISO 27001 A.5.33
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS module TEXT NOT NULL DEFAULT 'system';
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS ip_address TEXT NOT NULL DEFAULT '';
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS user_agent TEXT NOT NULL DEFAULT '';
