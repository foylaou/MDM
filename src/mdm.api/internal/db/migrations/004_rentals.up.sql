CREATE TABLE IF NOT EXISTS rentals (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_udid     TEXT NOT NULL REFERENCES devices(udid),
    borrower_id     UUID NOT NULL REFERENCES users(id),
    borrower_name   TEXT NOT NULL DEFAULT '',
    approver_id     UUID REFERENCES users(id),
    approver_name   TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'pending',  -- pending, approved, active, returned, rejected
    purpose         TEXT NOT NULL DEFAULT '',
    borrow_date     TIMESTAMPTZ NOT NULL DEFAULT now(),
    expected_return DATE,
    actual_return   TIMESTAMPTZ,
    notes           TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_rentals_device ON rentals(device_udid);
CREATE INDEX IF NOT EXISTS idx_rentals_borrower ON rentals(borrower_id);
CREATE INDEX IF NOT EXISTS idx_rentals_status ON rentals(status);
