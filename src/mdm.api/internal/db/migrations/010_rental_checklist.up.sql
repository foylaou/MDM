-- Store return checklist results and return notes
ALTER TABLE rentals ADD COLUMN IF NOT EXISTS return_checklist JSONB;
ALTER TABLE rentals ADD COLUMN IF NOT EXISTS return_notes TEXT NOT NULL DEFAULT '';
