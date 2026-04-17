package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anthropics/mdm-server/internal/domain"
)

type InventoryRepo struct{ pool *pgxpool.Pool }

func NewInventoryRepo(pool *pgxpool.Pool) *InventoryRepo { return &InventoryRepo{pool: pool} }

// --- Sessions ---

func (r *InventoryRepo) CreateSession(ctx context.Context, session *domain.InventorySession) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`INSERT INTO inventory_sessions (name, description, created_by, creator_name, notes)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		session.Name, session.Description, session.CreatedBy, session.CreatorName, session.Notes,
	).Scan(&id)
	return id, err
}

func (r *InventoryRepo) GetSession(ctx context.Context, id string) (*domain.InventorySession, error) {
	s := &domain.InventorySession{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, description, status, created_by, creator_name, created_at,
		        started_at, completed_at, notes, total_count, checked_count, matched_count, missing_count
		 FROM inventory_sessions WHERE id = $1`, id,
	).Scan(
		&s.ID, &s.Name, &s.Description, &s.Status, &s.CreatedBy, &s.CreatorName, &s.CreatedAt,
		&s.StartedAt, &s.CompletedAt, &s.Notes, &s.TotalCount, &s.CheckedCount, &s.MatchedCount, &s.MissingCount,
	)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *InventoryRepo) ListSessions(ctx context.Context) ([]*domain.InventorySession, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, description, status, created_by, creator_name, created_at,
		        started_at, completed_at, notes, total_count, checked_count, matched_count, missing_count
		 FROM inventory_sessions ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*domain.InventorySession
	for rows.Next() {
		s := &domain.InventorySession{}
		if err := rows.Scan(
			&s.ID, &s.Name, &s.Description, &s.Status, &s.CreatedBy, &s.CreatorName, &s.CreatedAt,
			&s.StartedAt, &s.CompletedAt, &s.Notes, &s.TotalCount, &s.CheckedCount, &s.MatchedCount, &s.MissingCount,
		); err != nil {
			continue
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

func (r *InventoryRepo) UpdateSessionStatus(ctx context.Context, id string, status string) error {
	switch status {
	case "in_progress":
		_, err := r.pool.Exec(ctx,
			`UPDATE inventory_sessions SET status = $1, started_at = now() WHERE id = $2`, status, id)
		return err
	case "completed":
		_, err := r.pool.Exec(ctx,
			`UPDATE inventory_sessions SET status = $1, completed_at = now() WHERE id = $2`, status, id)
		return err
	default:
		_, err := r.pool.Exec(ctx,
			`UPDATE inventory_sessions SET status = $1 WHERE id = $2`, status, id)
		return err
	}
}

func (r *InventoryRepo) UpdateSessionNotes(ctx context.Context, id string, notes string) error {
	_, err := r.pool.Exec(ctx, `UPDATE inventory_sessions SET notes = $1 WHERE id = $2`, notes, id)
	return err
}

func (r *InventoryRepo) DeleteSession(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM inventory_sessions WHERE id = $1`, id)
	return err
}

func (r *InventoryRepo) UpdateSessionCounts(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE inventory_sessions SET
			total_count   = (SELECT COUNT(*) FROM inventory_items WHERE session_id = $1),
			checked_count = (SELECT COUNT(*) FROM inventory_items WHERE session_id = $1 AND found IS NOT NULL),
			matched_count = (SELECT COUNT(*) FROM inventory_items WHERE session_id = $1 AND found = true),
			missing_count = (SELECT COUNT(*) FROM inventory_items WHERE session_id = $1 AND found = false)
		 WHERE id = $1`, id)
	return err
}

// --- Items ---

// GenerateItems populates inventory_items from current assets for a session.
func (r *InventoryRepo) GenerateItems(ctx context.Context, sessionID string) (int, error) {
	tag, err := r.pool.Exec(ctx,
		`INSERT INTO inventory_items (session_id, asset_id, device_udid, asset_number, asset_name)
		 SELECT $1, a.id, a.device_udid, a.asset_number, a.name
		 FROM assets a
		 WHERE a.asset_status NOT IN ('retired', 'transferred')
		 ON CONFLICT (session_id, asset_id) DO NOTHING`, sessionID)
	if err != nil {
		return 0, err
	}
	count := int(tag.RowsAffected())
	// Update session total
	r.UpdateSessionCounts(ctx, sessionID)
	return count, nil
}

func (r *InventoryRepo) ListItems(ctx context.Context, sessionID string) ([]*domain.InventoryItem, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT i.id, i.session_id, i.asset_id, COALESCE(i.device_udid,''), i.asset_number, i.asset_name,
		        i.found, i.condition, i.checked_by, i.checker_name, i.checked_at, i.notes
		 FROM inventory_items i
		 WHERE i.session_id = $1
		 ORDER BY i.asset_number, i.asset_name`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*domain.InventoryItem
	for rows.Next() {
		item := &domain.InventoryItem{}
		if err := rows.Scan(
			&item.ID, &item.SessionID, &item.AssetID, &item.DeviceUdid, &item.AssetNumber, &item.AssetName,
			&item.Found, &item.Condition, &item.CheckedBy, &item.CheckerName, &item.CheckedAt, &item.Notes,
		); err != nil {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func (r *InventoryRepo) CheckItem(ctx context.Context, id string, found bool, condition string, checkedBy string, checkerName string, notes string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE inventory_items SET found = $1, condition = $2, checked_by = $3, checker_name = $4, checked_at = now(), notes = $5
		 WHERE id = $6`,
		found, condition, checkedBy, checkerName, notes, id)
	if err != nil {
		return err
	}
	// Update parent session counts
	var sessionID string
	if err := r.pool.QueryRow(ctx, `SELECT session_id FROM inventory_items WHERE id = $1`, id).Scan(&sessionID); err == nil {
		r.UpdateSessionCounts(ctx, sessionID)
	}
	return nil
}
