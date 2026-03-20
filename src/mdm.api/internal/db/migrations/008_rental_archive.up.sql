-- Add auto-increment rental number for sorting/display
ALTER TABLE rentals ADD COLUMN IF NOT EXISTS rental_number SERIAL;

-- Add archived flag for "存查" feature (archived records hidden by default)
ALTER TABLE rentals ADD COLUMN IF NOT EXISTS is_archived BOOLEAN NOT NULL DEFAULT false;

CREATE INDEX IF NOT EXISTS idx_rentals_archived ON rentals(is_archived);
CREATE INDEX IF NOT EXISTS idx_rentals_number ON rentals(rental_number);
