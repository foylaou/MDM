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

func (r *AssetRepo) List(ctx context.Context, deviceUdid string) ([]*domain.Asset, error) {
	q := `SELECT a.id, a.device_udid, a.asset_number, a.name, a.spec, a.quantity, a.unit,
	             a.acquired_date, a.unit_price, a.purpose, a.borrow_date,
	             a.custodian_id, a.custodian_name, a.location, a.asset_category, a.notes,
	             a.created_at, a.updated_at,
	             COALESCE(d.device_name,'') as device_name, COALESCE(d.serial_number,'') as device_serial,
	             a.category_id, COALESCE(c.name,'') as category_name,
	             COALESCE(a.asset_status,'available') as asset_status
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
		a := &domain.Asset{}
		var acquiredDate, borrowDate *time.Time
		if err := rows.Scan(
			&a.ID, &a.DeviceUdid, &a.AssetNumber, &a.Name, &a.Spec, &a.Quantity, &a.Unit,
			&acquiredDate, &a.UnitPrice, &a.Purpose, &borrowDate,
			&a.CustodianID, &a.CustodianName, &a.Location, &a.AssetCategory, &a.Notes,
			&a.CreatedAt, &a.UpdatedAt, &a.DeviceName, &a.DeviceSerial,
			&a.CategoryID, &a.CategoryName, &a.AssetStatus,
		); err != nil {
			continue
		}
		a.AcquiredDate = acquiredDate
		a.BorrowDate = borrowDate
		assets = append(assets, a)
	}
	return assets, nil
}

func (r *AssetRepo) Create(ctx context.Context, asset *domain.Asset) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`INSERT INTO assets (device_udid, asset_number, name, spec, quantity, unit, acquired_date, unit_price, purpose, borrow_date, custodian_id, custodian_name, location, asset_category, notes, category_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING id`,
		asset.DeviceUdid, asset.AssetNumber, asset.Name, asset.Spec, asset.Quantity, asset.Unit,
		asset.AcquiredDate, asset.UnitPrice, asset.Purpose, asset.BorrowDate,
		asset.CustodianID, asset.CustodianName, asset.Location, asset.AssetCategory, asset.Notes, asset.CategoryID,
	).Scan(&id)
	return id, err
}

func (r *AssetRepo) Update(ctx context.Context, id string, fields map[string]interface{}) error {
	allowed := []string{"device_udid", "asset_number", "name", "spec", "quantity", "unit",
		"acquired_date", "unit_price", "purpose", "borrow_date",
		"custodian_id", "custodian_name", "location", "asset_category", "notes", "category_id", "asset_status"}
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

func (r *AssetRepo) UpdateCustodian(ctx context.Context, udid, custodianID, custodianName string, clearBorrowDate bool) error {
	if clearBorrowDate {
		_, err := r.pool.Exec(ctx,
			`UPDATE assets SET custodian_id=$1, custodian_name=$2, borrow_date=NULL WHERE device_udid=$3`,
			custodianID, custodianName, udid)
		return err
	}
	_, err := r.pool.Exec(ctx,
		`UPDATE assets SET custodian_id=$1, custodian_name=$2, borrow_date=now() WHERE device_udid=$3`,
		custodianID, custodianName, udid)
	return err
}
