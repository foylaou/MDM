-- Asset disposal applications (資訊資產報廢申請)
CREATE TABLE IF NOT EXISTS disposal_requests (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_number INT NOT NULL,
    applicant_id   UUID NOT NULL REFERENCES users(id),
    applicant_name TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL DEFAULT 'pending',  -- pending, approved, rejected
    approver_id    UUID REFERENCES users(id),
    approver_name  TEXT NOT NULL DEFAULT '',
    approved_at    TIMESTAMPTZ,
    reject_reason  TEXT NOT NULL DEFAULT '',
    is_archived    BOOLEAN NOT NULL DEFAULT false,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS disposal_request_items (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    disposal_id       UUID NOT NULL REFERENCES disposal_requests(id) ON DELETE CASCADE,
    line_no           INT NOT NULL DEFAULT 1,
    asset_id          UUID NOT NULL REFERENCES assets(id),
    asset_name        TEXT NOT NULL DEFAULT '',  -- 申請當下快照
    asset_number      TEXT NOT NULL DEFAULT '',  -- 申請當下快照
    dispose_date      DATE,                      -- 報廢日期
    dispose_reason    TEXT NOT NULL DEFAULT '',   -- 報廢原因
    data_wipe_checked BOOLEAN NOT NULL DEFAULT false  -- 資料清除檢核
);

CREATE INDEX IF NOT EXISTS idx_disp_items_disposal ON disposal_request_items(disposal_id);
CREATE INDEX IF NOT EXISTS idx_disp_status ON disposal_requests(status);
CREATE INDEX IF NOT EXISTS idx_disp_number ON disposal_requests(request_number);
