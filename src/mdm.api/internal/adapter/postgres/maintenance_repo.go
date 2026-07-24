package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anthropics/mdm-server/internal/domain"
)

type MaintenanceRepo struct{ pool *pgxpool.Pool }

func NewMaintenanceRepo(pool *pgxpool.Pool) *MaintenanceRepo { return &MaintenanceRepo{pool: pool} }

const maintenanceSelectColumns = `m.id, m.request_number, m.asset_id, m.applicant_id, m.applicant_name,
	m.reason, m.vendor, m.technician, m.checkout_date, m.return_date, m.process_notes,
	m.status, m.handler_id, m.handler_name, m.handled_at,
	m.supervisor_id, m.supervisor_name, m.approved_at, m.reject_reason,
	m.is_archived, m.created_at, m.updated_at,
	COALESCE(a.asset_number,'') as asset_number, COALESCE(a.name,'') as asset_name`

const maintenanceFromJoin = `FROM maintenance_requests m LEFT JOIN assets a ON a.id = m.asset_id`

func scanMaintenance(row interface {
	Scan(dest ...interface{}) error
}) (*domain.MaintenanceRequest, error) {
	m := &domain.MaintenanceRequest{}
	var handlerID, supervisorID *string
	err := row.Scan(
		&m.ID, &m.RequestNumber, &m.AssetID, &m.ApplicantID, &m.ApplicantName,
		&m.Reason, &m.Vendor, &m.Technician, &m.CheckoutDate, &m.ReturnDate, &m.ProcessNotes,
		&m.Status, &handlerID, &m.HandlerName, &m.HandledAt,
		&supervisorID, &m.SupervisorName, &m.ApprovedAt, &m.RejectReason,
		&m.IsArchived, &m.CreatedAt, &m.UpdatedAt,
		&m.AssetNumber, &m.AssetName,
	)
	if err != nil {
		return nil, err
	}
	m.HandlerID = handlerID
	m.SupervisorID = supervisorID
	return m, nil
}

func (r *MaintenanceRepo) List(ctx context.Context, status string, showArchived bool) ([]*domain.MaintenanceRequest, error) {
	q := `SELECT ` + maintenanceSelectColumns + ` ` + maintenanceFromJoin + ` WHERE 1=1`
	args := []interface{}{}
	idx := 1
	if status != "" {
		q += fmt.Sprintf(` AND m.status=$%d`, idx)
		args = append(args, status)
		idx++
	}
	if !showArchived {
		q += ` AND m.is_archived = false`
	}
	q += ` ORDER BY m.request_number DESC`

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*domain.MaintenanceRequest
	for rows.Next() {
		m, err := scanMaintenance(rows)
		if err != nil {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

func (r *MaintenanceRepo) Create(ctx context.Context, req *domain.MaintenanceRequest) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`INSERT INTO maintenance_requests
			(request_number, asset_id, applicant_id, applicant_name, reason, vendor, technician, checkout_date, return_date)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
		req.RequestNumber, req.AssetID, req.ApplicantID, req.ApplicantName,
		req.Reason, req.Vendor, req.Technician, req.CheckoutDate, req.ReturnDate,
	).Scan(&id)
	return id, err
}

func (r *MaintenanceRepo) GetByID(ctx context.Context, id string) (*domain.MaintenanceRequest, error) {
	q := `SELECT ` + maintenanceSelectColumns + ` ` + maintenanceFromJoin + ` WHERE m.id=$1`
	return scanMaintenance(r.pool.QueryRow(ctx, q, id))
}

func (r *MaintenanceRepo) NextRequestNumber(ctx context.Context) (int, error) {
	var num int
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(MAX(request_number), 0) + 1 FROM maintenance_requests`).Scan(&num)
	return num, err
}

func (r *MaintenanceRepo) SignByHandler(ctx context.Context, requestNumber int, handlerID string, handlerName string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE maintenance_requests SET status='handler_signed', handler_id=$1, handler_name=$2, handled_at=now(), updated_at=now()
		 WHERE request_number=$3 AND status='pending'`,
		handlerID, handlerName, requestNumber)
	return err
}

func (r *MaintenanceRepo) ApproveBySupervisor(ctx context.Context, requestNumber int, supervisorID string, supervisorName string, checkoutDate *time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE maintenance_requests SET status='approved', supervisor_id=$1, supervisor_name=$2, approved_at=now(),
		 checkout_date=COALESCE($3, checkout_date), updated_at=now()
		 WHERE request_number=$4 AND status='handler_signed'`,
		supervisorID, supervisorName, checkoutDate, requestNumber)
	return err
}

func (r *MaintenanceRepo) Return(ctx context.Context, requestNumber int, returnDate *time.Time, processNotes string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE maintenance_requests SET status='returned', return_date=COALESCE($1, return_date),
		 process_notes=$2, updated_at=now()
		 WHERE request_number=$3 AND status='approved'`,
		returnDate, processNotes, requestNumber)
	return err
}

func (r *MaintenanceRepo) Reject(ctx context.Context, requestNumber int, reason string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE maintenance_requests SET status='rejected', reject_reason=$1, updated_at=now()
		 WHERE request_number=$2 AND status IN ('pending','handler_signed')`,
		reason, requestNumber)
	return err
}

func (r *MaintenanceRepo) DeleteByNumber(ctx context.Context, requestNumber int) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM maintenance_requests WHERE request_number=$1`, requestNumber)
	return err
}

func (r *MaintenanceRepo) Archive(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	q := fmt.Sprintf("UPDATE maintenance_requests SET is_archived = true, updated_at = now() WHERE id IN (%s)", strings.Join(placeholders, ","))
	_, err := r.pool.Exec(ctx, q, args...)
	return err
}

func (r *MaintenanceRepo) ListAssetIDsByNumber(ctx context.Context, requestNumber int) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT asset_id FROM maintenance_requests WHERE request_number=$1`, requestNumber)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids, nil
}
