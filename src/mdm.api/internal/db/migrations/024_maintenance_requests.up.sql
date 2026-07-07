-- Equipment maintenance dispatch requests (資通設備進出及維護申請單)
CREATE TABLE IF NOT EXISTS maintenance_requests (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_number   INT NOT NULL,
    asset_id         UUID NOT NULL REFERENCES assets(id),
    applicant_id     UUID NOT NULL REFERENCES users(id),
    applicant_name   TEXT NOT NULL DEFAULT '',
    reason           TEXT NOT NULL DEFAULT '',        -- 申請原因
    vendor           TEXT NOT NULL DEFAULT '',        -- 維修廠商
    technician       TEXT NOT NULL DEFAULT '',        -- 維修人員
    checkout_date    DATE,                            -- 攜出日期
    return_date      DATE,                            -- 歸還日期
    process_notes    TEXT NOT NULL DEFAULT '',         -- 作業過程
    status           TEXT NOT NULL DEFAULT 'pending',  -- pending, handler_signed, approved, returned, rejected
    handler_id       UUID REFERENCES users(id),        -- 承辦人員
    handler_name     TEXT NOT NULL DEFAULT '',
    handled_at       TIMESTAMPTZ,
    supervisor_id    UUID REFERENCES users(id),        -- 權責主管
    supervisor_name  TEXT NOT NULL DEFAULT '',
    approved_at      TIMESTAMPTZ,
    reject_reason    TEXT NOT NULL DEFAULT '',
    is_archived      BOOLEAN NOT NULL DEFAULT false,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_maint_asset ON maintenance_requests(asset_id);
CREATE INDEX IF NOT EXISTS idx_maint_status ON maintenance_requests(status);
CREATE INDEX IF NOT EXISTS idx_maint_number ON maintenance_requests(request_number);
