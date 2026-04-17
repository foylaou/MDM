package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anthropics/mdm-server/internal/domain"
)

type RentalRepo struct{ pool *pgxpool.Pool }

func NewRentalRepo(pool *pgxpool.Pool) *RentalRepo { return &RentalRepo{pool: pool} }

func (r *RentalRepo) List(ctx context.Context, status, deviceUdid string, showArchived bool) ([]*domain.Rental, error) {
	q := `SELECT r.id, r.device_udid, r.borrower_id, r.borrower_name, r.approver_id, r.approver_name,
	             r.status, r.purpose, r.borrow_date, r.expected_return, r.actual_return, r.notes,
	             r.created_at, r.updated_at,
	             COALESCE(d.device_name,'') as device_name, COALESCE(d.serial_number,'') as device_serial,
	             a.custodian_id, COALESCE(a.custodian_name,'') as custodian_name,
	             r.rental_number, r.is_archived, r.return_checklist, r.return_notes
	      FROM rentals r LEFT JOIN devices d ON r.device_udid = d.udid
	      LEFT JOIN assets a ON a.device_udid = r.device_udid WHERE 1=1`
	args := []interface{}{}
	idx := 1
	if status != "" {
		q += fmt.Sprintf(` AND r.status=$%d`, idx)
		args = append(args, status)
		idx++
	}
	if deviceUdid != "" {
		q += fmt.Sprintf(` AND r.device_udid=$%d`, idx)
		args = append(args, deviceUdid)
		idx++
	}
	if !showArchived {
		q += ` AND r.is_archived = false`
	}
	q += ` ORDER BY r.rental_number DESC`

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rentals []*domain.Rental
	for rows.Next() {
		rental := &domain.Rental{}
		var approverID *string
		var expectedReturn, actualReturn *time.Time
		var checklistJSON []byte
		if err := rows.Scan(
			&rental.ID, &rental.DeviceUdid, &rental.BorrowerID, &rental.BorrowerName,
			&approverID, &rental.ApproverName,
			&rental.Status, &rental.Purpose, &rental.BorrowDate, &expectedReturn, &actualReturn, &rental.Notes,
			&rental.CreatedAt, &rental.UpdatedAt,
			&rental.DeviceName, &rental.DeviceSerial,
			&rental.CustodianID, &rental.CustodianName,
			&rental.RentalNumber, &rental.IsArchived, &checklistJSON, &rental.ReturnNotes,
		); err != nil {
			continue
		}
		rental.ApproverID = approverID
		rental.ExpectedReturn = expectedReturn
		rental.ActualReturn = actualReturn
		if len(checklistJSON) > 0 {
			json.Unmarshal(checklistJSON, &rental.ReturnChecklist)
		}
		rentals = append(rentals, rental)
	}
	return rentals, nil
}

func (r *RentalRepo) Create(ctx context.Context, rental *domain.Rental) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`INSERT INTO rentals (device_udid, borrower_id, borrower_name, purpose, expected_return, notes, rental_number)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		rental.DeviceUdid, rental.BorrowerID, rental.BorrowerName, rental.Purpose,
		rental.ExpectedReturn, rental.Notes, rental.RentalNumber,
	).Scan(&id)
	return id, err
}

func (r *RentalRepo) GetByID(ctx context.Context, id string) (*domain.Rental, error) {
	rental := &domain.Rental{}
	var rentalNumber int
	err := r.pool.QueryRow(ctx,
		`SELECT device_udid, status, rental_number FROM rentals WHERE id=$1`, id,
	).Scan(&rental.DeviceUdid, &rental.Status, &rentalNumber)
	if err != nil {
		return nil, err
	}
	rental.ID = id
	rental.RentalNumber = rentalNumber
	return rental, nil
}

func (r *RentalRepo) NextRentalNumber(ctx context.Context) (int, error) {
	var num int
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(MAX(rental_number), 0) + 1 FROM rentals`).Scan(&num)
	return num, err
}

func (r *RentalRepo) UpdateStatusByNumber(ctx context.Context, rentalNumber int, fromStatus, toStatus string, approverID *string, approverName string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE rentals SET status=$1, approver_id=$2, approver_name=$3, updated_at=now() WHERE rental_number=$4 AND status=$5`,
		toStatus, approverID, approverName, rentalNumber, fromStatus)
	return err
}

func (r *RentalRepo) ActivateByNumber(ctx context.Context, rentalNumber int) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE rentals SET status='active', borrow_date=now(), updated_at=now() WHERE rental_number=$1 AND status='approved'`,
		rentalNumber)
	return err
}

func (r *RentalRepo) ReturnByNumber(ctx context.Context, rentalNumber int, checklist []byte, notes string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE rentals SET status='returned', actual_return=now(), updated_at=now(),
		 return_checklist=$1, return_notes=$2
		 WHERE rental_number=$3 AND status='active'`,
		checklist, notes, rentalNumber)
	return err
}

func (r *RentalRepo) DeleteByNumber(ctx context.Context, rentalNumber int) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM rentals WHERE rental_number=$1`, rentalNumber)
	return err
}

func (r *RentalRepo) Archive(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	q := fmt.Sprintf("UPDATE rentals SET is_archived = true, updated_at = now() WHERE id IN (%s)", strings.Join(placeholders, ","))
	_, err := r.pool.Exec(ctx, q, args...)
	return err
}

func (r *RentalRepo) ListDeviceUdidsByNumber(ctx context.Context, rentalNumber int) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT device_udid FROM rentals WHERE rental_number=$1`, rentalNumber)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var udids []string
	for rows.Next() {
		var udid string
		rows.Scan(&udid)
		udids = append(udids, udid)
	}
	return udids, nil
}

func (r *RentalRepo) GetBorrowerInfo(ctx context.Context, rentalID string) (string, string, error) {
	var borrowerID, borrowerName string
	err := r.pool.QueryRow(ctx,
		`SELECT borrower_id, borrower_name FROM rentals WHERE id=$1`, rentalID,
	).Scan(&borrowerID, &borrowerName)
	return borrowerID, borrowerName, err
}

// ListOverdue returns active rentals whose expected_return is before today (grouped by rental_number, one row per group).
func (r *RentalRepo) ListOverdue(ctx context.Context) ([]*domain.Rental, error) {
	q := `SELECT DISTINCT ON (r.rental_number)
	             r.id, r.device_udid, r.borrower_id, r.borrower_name, r.approver_id, r.approver_name,
	             r.status, r.purpose, r.borrow_date, r.expected_return, r.actual_return, r.notes,
	             r.created_at, r.updated_at,
	             COALESCE(d.device_name,'') as device_name, COALESCE(d.serial_number,'') as device_serial,
	             a.custodian_id, COALESCE(a.custodian_name,'') as custodian_name,
	             r.rental_number, r.is_archived, r.return_checklist, r.return_notes
	      FROM rentals r LEFT JOIN devices d ON r.device_udid = d.udid
	      LEFT JOIN assets a ON a.device_udid = r.device_udid
	      WHERE r.status = 'active' AND r.expected_return IS NOT NULL AND r.expected_return < CURRENT_DATE
	      ORDER BY r.rental_number, r.created_at`

	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rentals []*domain.Rental
	for rows.Next() {
		rental := &domain.Rental{}
		var approverID *string
		var expectedReturn, actualReturn *time.Time
		var checklistJSON []byte
		if err := rows.Scan(
			&rental.ID, &rental.DeviceUdid, &rental.BorrowerID, &rental.BorrowerName,
			&approverID, &rental.ApproverName,
			&rental.Status, &rental.Purpose, &rental.BorrowDate, &expectedReturn, &actualReturn, &rental.Notes,
			&rental.CreatedAt, &rental.UpdatedAt,
			&rental.DeviceName, &rental.DeviceSerial,
			&rental.CustodianID, &rental.CustodianName,
			&rental.RentalNumber, &rental.IsArchived, &checklistJSON, &rental.ReturnNotes,
		); err != nil {
			continue
		}
		rental.ApproverID = approverID
		rental.ExpectedReturn = expectedReturn
		rental.ActualReturn = actualReturn
		if len(checklistJSON) > 0 {
			json.Unmarshal(checklistJSON, &rental.ReturnChecklist)
		}
		rentals = append(rentals, rental)
	}
	return rentals, nil
}

func (r *RentalRepo) CheckDeviceAvailability(ctx context.Context, udid string) (string, bool, bool, error) {
	var assetStatus string
	var isRented, isLostMode bool
	err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(a.asset_status,'available'),
		        EXISTS(SELECT 1 FROM rentals rl WHERE rl.device_udid=$1 AND rl.status IN ('pending','approved','active')),
		        d.is_lost_mode
		 FROM devices d LEFT JOIN assets a ON a.device_udid=d.udid WHERE d.udid=$1`, udid,
	).Scan(&assetStatus, &isRented, &isLostMode)
	return assetStatus, isRented, isLostMode, err
}
