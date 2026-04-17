package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anthropics/mdm-server/internal/domain"
)

type AssetRepo struct{ pool *pgxpool.Pool }

func NewAssetRepo(pool *pgxpool.Pool) *AssetRepo { return &AssetRepo{pool: pool} }

// IsCustodianOfAll returns true if the user is the custodian of ALL given device UDIDs.
func (r *AssetRepo) IsCustodianOfAll(ctx context.Context, userID string, udids []string) (bool, error) {
	if len(udids) == 0 {
		return false, nil
	}
	placeholders := make([]string, len(udids))
	args := []interface{}{userID}
	for i, udid := range udids {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, udid)
	}
	q := fmt.Sprintf(
		`SELECT COUNT(DISTINCT device_udid) FROM assets WHERE custodian_id = $1 AND device_udid IN (%s)`,
		strings.Join(placeholders, ","),
	)
	var count int
	if err := r.pool.QueryRow(ctx, q, args...).Scan(&count); err != nil {
		return false, err
	}
	return count == len(udids), nil
}

const assetSelectColumns = `a.id, a.device_udid, a.asset_number, a.name, a.spec, a.quantity, a.unit,
    a.acquired_date, a.unit_price, a.purpose, a.assigned_date,
    a.custodian_id, a.custodian_name, a.location, a.asset_category, a.notes,
    a.created_at, a.updated_at,
    COALESCE(d.device_name,'') as device_name, COALESCE(d.serial_number,'') as device_serial,
    a.category_id, COALESCE(c.name,'') as category_name,
    COALESCE(a.asset_status,'available') as asset_status,
    a.disposed_at, a.disposed_by, COALESCE(a.dispose_reason,'') as dispose_reason,
    COALESCE(a.transferred_to,'') as transferred_to, a.transferred_at,
    a.current_holder_id, COALESCE(a.current_holder_name,'') as current_holder_name, a.current_holder_since`

func scanAsset(row interface {
	Scan(dest ...interface{}) error
}) (*domain.Asset, error) {
	a := &domain.Asset{}
	var acquiredDate, assignedDate *time.Time
	err := row.Scan(
		&a.ID, &a.DeviceUdid, &a.AssetNumber, &a.Name, &a.Spec, &a.Quantity, &a.Unit,
		&acquiredDate, &a.UnitPrice, &a.Purpose, &assignedDate,
		&a.CustodianID, &a.CustodianName, &a.Location, &a.AssetCategory, &a.Notes,
		&a.CreatedAt, &a.UpdatedAt, &a.DeviceName, &a.DeviceSerial,
		&a.CategoryID, &a.CategoryName, &a.AssetStatus,
		&a.DisposedAt, &a.DisposedBy, &a.DisposeReason,
		&a.TransferredTo, &a.TransferredAt,
		&a.CurrentHolderID, &a.CurrentHolderName, &a.CurrentHolderSince,
	)
	if err != nil {
		return nil, err
	}
	a.AcquiredDate = acquiredDate
	a.AssignedDate = assignedDate
	return a, nil
}

func (r *AssetRepo) List(ctx context.Context, deviceUdid string) ([]*domain.Asset, error) {
	q := `SELECT ` + assetSelectColumns + `
	      FROM assets a LEFT JOIN devices d ON a.device_udid = d.udid
	      LEFT JOIN categories c ON a.category_id = c.id`
	args := []interface{}{}
	if deviceUdid != "" {
		q += ` WHERE a.device_udid = $1`
		args = append(args, deviceUdid)
	}
	q += ` ORDER BY a.created_at DESC`

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assets []*domain.Asset
	for rows.Next() {
		a, err := scanAsset(rows)
		if err != nil {
			continue
		}
		assets = append(assets, a)
	}
	return assets, nil
}

func (r *AssetRepo) GetByID(ctx context.Context, id string) (*domain.Asset, error) {
	q := `SELECT ` + assetSelectColumns + `
	      FROM assets a LEFT JOIN devices d ON a.device_udid = d.udid
	      LEFT JOIN categories c ON a.category_id = c.id
	      WHERE a.id = $1`
	return scanAsset(r.pool.QueryRow(ctx, q, id))
}

func (r *AssetRepo) GetByDeviceUdid(ctx context.Context, udid string) (*domain.Asset, error) {
	q := `SELECT ` + assetSelectColumns + `
	      FROM assets a LEFT JOIN devices d ON a.device_udid = d.udid
	      LEFT JOIN categories c ON a.category_id = c.id
	      WHERE a.device_udid = $1 LIMIT 1`
	return scanAsset(r.pool.QueryRow(ctx, q, udid))
}

func (r *AssetRepo) Create(ctx context.Context, asset *domain.Asset) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`INSERT INTO assets (device_udid, asset_number, name, spec, quantity, unit, acquired_date, unit_price, purpose, assigned_date, custodian_id, custodian_name, location, asset_category, notes, category_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING id`,
		asset.DeviceUdid, asset.AssetNumber, asset.Name, asset.Spec, asset.Quantity, asset.Unit,
		asset.AcquiredDate, asset.UnitPrice, asset.Purpose, asset.AssignedDate,
		asset.CustodianID, asset.CustodianName, asset.Location, asset.AssetCategory, asset.Notes, asset.CategoryID,
	).Scan(&id)
	return id, err
}

// Update allows editing asset metadata, but NOT custodian_id / custodian_name / assigned_date.
// Those must go through SetCustodian (which appends an asset_custody_logs entry).
// current_holder_* is also excluded — it's written by rental activate/return only.
func (r *AssetRepo) Update(ctx context.Context, id string, fields map[string]interface{}) error {
	allowed := []string{"device_udid", "asset_number", "name", "spec", "quantity", "unit",
		"acquired_date", "unit_price", "purpose",
		"location", "asset_category", "notes", "category_id", "asset_status",
		"dispose_reason", "transferred_to"}
	sets := []string{}
	args := []interface{}{}
	idx := 1
	for _, k := range allowed {
		if v, ok := fields[k]; ok {
			sets = append(sets, fmt.Sprintf("%s=$%d", k, idx))
			args = append(args, v)
			idx++
		}
	}
	if len(sets) == 0 {
		return nil
	}
	q := fmt.Sprintf("UPDATE assets SET %s, updated_at=now() WHERE id=$%d", strings.Join(sets, ", "), idx)
	args = append(args, id)
	_, err := r.pool.Exec(ctx, q, args...)
	return err
}

func (r *AssetRepo) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM assets WHERE id=$1`, id)
	return err
}

func (r *AssetRepo) UpdateStatus(ctx context.Context, udid string, status string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE assets SET asset_status=$1, updated_at=now() WHERE device_udid=$2`, status, udid)
	return err
}

func (r *AssetRepo) Dispose(ctx context.Context, id string, disposedBy string, reason string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE assets SET asset_status='retired', disposed_at=now(), disposed_by=$1, dispose_reason=$2, updated_at=now() WHERE id=$3`,
		disposedBy, reason, id)
	return err
}

func (r *AssetRepo) Transfer(ctx context.Context, id string, transferredTo string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE assets SET asset_status='transferred', transferred_to=$1, transferred_at=now(), updated_at=now() WHERE id=$2`,
		transferredTo, id)
	return err
}

// SetCustodian writes the custodian fields directly. Intended to be called only by
// the custody controller, which also appends an asset_custody_logs entry.
// Passing custodianID=nil clears the custodian (revoke).
func (r *AssetRepo) SetCustodian(ctx context.Context, id string, custodianID *string, custodianName string, assignedDate *time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE assets SET custodian_id=$1, custodian_name=$2, assigned_date=$3, updated_at=now() WHERE id=$4`,
		custodianID, custodianName, assignedDate, id)
	return err
}

// SetHolderByUdid records the rental borrower as the current physical holder.
// Does NOT touch custodian — the long-term responsibility chain stays intact.
func (r *AssetRepo) SetHolderByUdid(ctx context.Context, udid string, holderID string, holderName string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE assets SET current_holder_id=$1, current_holder_name=$2, current_holder_since=now(), updated_at=now() WHERE device_udid=$3`,
		holderID, holderName, udid)
	return err
}

// ClearHolderByUdid clears the current holder when a rental is returned.
func (r *AssetRepo) ClearHolderByUdid(ctx context.Context, udid string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE assets SET current_holder_id=NULL, current_holder_name='', current_holder_since=NULL, updated_at=now() WHERE device_udid=$1`,
		udid)
	return err
}

// UpdateCustodian is kept for backward compat but deprecated.
// New code should use SetCustodian with an accompanying custody log entry.
func (r *AssetRepo) UpdateCustodian(ctx context.Context, udid, custodianID, custodianName string, clearBorrowDate bool) error {
	if clearBorrowDate {
		_, err := r.pool.Exec(ctx,
			`UPDATE assets SET custodian_id=$1, custodian_name=$2, assigned_date=NULL WHERE device_udid=$3`,
			custodianID, custodianName, udid)
		return err
	}
	_, err := r.pool.Exec(ctx,
		`UPDATE assets SET custodian_id=$1, custodian_name=$2, assigned_date=now() WHERE device_udid=$3`,
		custodianID, custodianName, udid)
	return err
}
