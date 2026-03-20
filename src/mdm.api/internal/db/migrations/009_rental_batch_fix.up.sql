-- Fix: rentals created in the same batch should share the same rental_number.
-- Identify batches by (borrower_id, purpose, DATE(borrow_date)) and set them to the MIN rental_number.
UPDATE rentals r
SET rental_number = sub.min_number
FROM (
    SELECT borrower_id, purpose, borrow_date::date as bd, MIN(rental_number) as min_number
    FROM rentals
    GROUP BY borrower_id, purpose, borrow_date::date
    HAVING COUNT(*) > 1
) sub
WHERE r.borrower_id = sub.borrower_id
  AND r.purpose = sub.purpose
  AND r.borrow_date::date = sub.bd;

-- Remove auto-increment default so we control rental_number manually in application code
ALTER TABLE rentals ALTER COLUMN rental_number DROP DEFAULT;
