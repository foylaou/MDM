package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anthropics/mdm-server/internal/domain"
)

// ErrDataWipeNotChecked is returned by Approve when at least one item has not
// had its 資料清除檢核 (data wipe checklist) confirmed yet.
var ErrDataWipeNotChecked = errors.New("data wipe checklist not confirmed for all items")

type DisposalRepo struct{ pool *pgxpool.Pool }

func NewDisposalRepo(pool *pgxpool.Pool) *DisposalRepo { return &DisposalRepo{pool: pool} }

const disposalSelectColumns = `d.id, d.request_number, d.applicant_id, d.applicant_name,
	d.status, d.approver_id, d.approver_name, d.approved_at, d.reject_reason,
	d.is_archived, d.created_at, d.updated_at`

func scanDisposal(row interface {
	Scan(dest ...interface{}) error
}) (*domain.DisposalRequest, error) {
	d := &domain.DisposalRequest{}
	var approverID *string
	err := row.Scan(
		&d.ID, &d.RequestNumber, &d.ApplicantID, &d.ApplicantName,
		&d.Status, &approverID, &d.ApproverName, &d.ApprovedAt, &d.RejectReason,
		&d.IsArchived, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	d.ApproverID = approverID
	return d, nil
}

func (r *DisposalRepo) loadItems(ctx context.Context, disposalID string) ([]domain.DisposalRequestItem, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, disposal_id, line_no, asset_id, asset_name, asset_number, dispose_date, dispose_reason, data_wipe_checked
		 FROM disposal_request_items WHERE disposal_id=$1 ORDER BY line_no`, disposalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.DisposalRequestItem
	for rows.Next() {
		var it domain.DisposalRequestItem
		if err := rows.Scan(&it.ID, &it.DisposalID, &it.LineNo, &it.AssetID, &it.AssetName, &it.AssetNumber,
			&it.DisposeDate, &it.DisposeReason, &it.DataWipeChecked); err == nil {
			items = append(items, it)
		}
	}
	return items, nil
}

func (r *DisposalRepo) List(ctx context.Context, status string, showArchived bool) ([]*domain.DisposalRequest, error) {
	q := `SELECT ` + disposalSelectColumns + ` FROM disposal_requests d WHERE 1=1`
	args := []interface{}{}
	idx := 1
	if status != "" {
		q += fmt.Sprintf(` AND d.status=$%d`, idx)
		args = append(args, status)
		idx++
	}
	if !showArchived {
		q += ` AND d.is_archived = false`
	}
	q += ` ORDER BY d.request_number DESC`

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*domain.DisposalRequest
	for rows.Next() {
		d, err := scanDisposal(rows)
		if err != nil {
			continue
		}
		out = append(out, d)
	}
	rows.Close()

	for _, d := range out {
		items, err := r.loadItems(ctx, d.ID)
		if err != nil {
			return nil, err
		}
		d.Items = items
	}
	return out, nil
}

func (r *DisposalRepo) GetByID(ctx context.Context, id string) (*domain.DisposalRequest, error) {
	q := `SELECT ` + disposalSelectColumns + ` FROM disposal_requests d WHERE d.id=$1`
	d, err := scanDisposal(r.pool.QueryRow(ctx, q, id))
	if err != nil {
		return nil, err
	}
	items, err := r.loadItems(ctx, d.ID)
	if err != nil {
		return nil, err
	}
	d.Items = items
	return d, nil
}

func (r *DisposalRepo) NextRequestNumber(ctx context.Context) (int, error) {
	var num int
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(MAX(request_number), 0) + 1 FROM disposal_requests`).Scan(&num)
	return num, err
}

func (r *DisposalRepo) Create(ctx context.Context, req *domain.DisposalRequest) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var id string
	if err := tx.QueryRow(ctx,
		`INSERT INTO disposal_requests (request_number, applicant_id, applicant_name)
		 VALUES ($1, $2, $3) RETURNING id`,
		req.RequestNumber, req.ApplicantID, req.ApplicantName,
	).Scan(&id); err != nil {
		return "", err
	}

	for i, item := range req.Items {
		if _, err := tx.Exec(ctx,
			`INSERT INTO disposal_request_items
				(disposal_id, line_no, asset_id, asset_name, asset_number, dispose_date, dispose_reason, data_wipe_checked)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			id, i+1, item.AssetID, item.AssetName, item.AssetNumber, item.DisposeDate, item.DisposeReason, item.DataWipeChecked,
		); err != nil {
			return "", err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

// Approve marks the application approved, but only if every item has
// data_wipe_checked = true. Callers are responsible for then executing the
// actual per-asset disposal (AssetRepository.Dispose).
func (r *DisposalRepo) Approve(ctx context.Context, id string, approverID string, approverName string) error {
	var uncheckedCount int
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM disposal_request_items WHERE disposal_id=$1 AND data_wipe_checked = false`, id,
	).Scan(&uncheckedCount); err != nil {
		return err
	}
	if uncheckedCount > 0 {
		return ErrDataWipeNotChecked
	}

	_, err := r.pool.Exec(ctx,
		`UPDATE disposal_requests SET status='approved', approver_id=$1, approver_name=$2, approved_at=now(), updated_at=now()
		 WHERE id=$3 AND status='pending'`,
		approverID, approverName, id)
	return err
}

func (r *DisposalRepo) Reject(ctx context.Context, id string, reason string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE disposal_requests SET status='rejected', reject_reason=$1, updated_at=now()
		 WHERE id=$2 AND status='pending'`,
		reason, id)
	return err
}

func (r *DisposalRepo) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM disposal_requests WHERE id=$1`, id)
	return err
}

func (r *DisposalRepo) Archive(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	q := fmt.Sprintf("UPDATE disposal_requests SET is_archived = true, updated_at = now() WHERE id IN (%s)", strings.Join(placeholders, ","))
	_, err := r.pool.Exec(ctx, q, args...)
	return err
}
