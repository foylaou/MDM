-- SSO (OIDC) support: store the provider's subject identifier per user.
-- email already exists from migration 014.
ALTER TABLE users ADD COLUMN IF NOT EXISTS sso_sub TEXT DEFAULT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS users_sso_sub_idx ON users(sso_sub) WHERE sso_sub IS NOT NULL;
