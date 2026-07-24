-- ISO 27001 compliance fields for equipment maintenance requests:
-- data-leakage risk when the original device leaves the premises (A.7.9 /
-- A.8.10), and third-party loaner-device risk when the vendor provides a
-- replacement (A.8.1 / A.5.20).
ALTER TABLE maintenance_requests ADD COLUMN IF NOT EXISTS contains_sensitive_data BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE maintenance_requests ADD COLUMN IF NOT EXISTS vendor_nda_ref TEXT NOT NULL DEFAULT '';
ALTER TABLE maintenance_requests ADD COLUMN IF NOT EXISTS data_wiped_before_checkout BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE maintenance_requests ADD COLUMN IF NOT EXISTS loaner_info TEXT NOT NULL DEFAULT '';
ALTER TABLE maintenance_requests ADD COLUMN IF NOT EXISTS loaner_provided_date DATE;
ALTER TABLE maintenance_requests ADD COLUMN IF NOT EXISTS loaner_security_checked BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE maintenance_requests ADD COLUMN IF NOT EXISTS loaner_returned_date DATE;
