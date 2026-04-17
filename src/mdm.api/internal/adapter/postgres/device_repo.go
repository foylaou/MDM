package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anthropics/mdm-server/internal/domain"
)

type DeviceRepo struct{ pool *pgxpool.Pool }

func NewDeviceRepo(pool *pgxpool.Pool) *DeviceRepo { return &DeviceRepo{pool: pool} }

func (r *DeviceRepo) Upsert(ctx context.Context, d *domain.Device) error {
	detailsJSON, _ := json.Marshal(d.Details)
	if d.Details == nil {
		detailsJSON = nil // don't overwrite existing details with empty
	}

	if detailsJSON != nil && string(detailsJSON) != "{}" {
		// Full upsert including details
		_, err := r.pool.Exec(ctx,
			`INSERT INTO devices (udid, serial_number, device_name, model, os_version, last_seen, enrollment_status, is_supervised, is_lost_mode, battery_level, details)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			 ON CONFLICT (udid) DO UPDATE SET
			   serial_number=COALESCE(NULLIF(EXCLUDED.serial_number,''), devices.serial_number),
			   device_name=COALESCE(NULLIF(EXCLUDED.device_name,''), devices.device_name),
			   model=COALESCE(NULLIF(EXCLUDED.model,''), devices.model),
			   os_version=COALESCE(NULLIF(EXCLUDED.os_version,''), devices.os_version),
			   last_seen=EXCLUDED.last_seen,
			   enrollment_status=EXCLUDED.enrollment_status,
			   is_supervised=EXCLUDED.is_supervised,
			   is_lost_mode=EXCLUDED.is_lost_mode,
			   battery_level=CASE WHEN EXCLUDED.battery_level >= 0 THEN EXCLUDED.battery_level ELSE devices.battery_level END,
			   details=devices.details || EXCLUDED.details`,
			d.UDID, d.SerialNumber, d.DeviceName, d.Model, d.OSVersion, d.LastSeen, d.EnrollmentStatus,
			d.IsSupervised, d.IsLostMode, d.BatteryLevel, detailsJSON)
		return err
	}

	// Basic upsert without details (sync from MicroMDM device list)
	_, err := r.pool.Exec(ctx,
		`INSERT INTO devices (udid, serial_number, device_name, model, os_version, last_seen, enrollment_status)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 ON CONFLICT (udid) DO UPDATE SET
		   serial_number=COALESCE(NULLIF(EXCLUDED.serial_number,''), devices.serial_number),
		   device_name=COALESCE(NULLIF(EXCLUDED.device_name,''), devices.device_name),
		   model=COALESCE(NULLIF(EXCLUDED.model,''), devices.model),
		   os_version=COALESCE(NULLIF(EXCLUDED.os_version,''), devices.os_version),
		   last_seen=EXCLUDED.last_seen,
		   enrollment_status=EXCLUDED.enrollment_status`,
		d.UDID, d.SerialNumber, d.DeviceName, d.Model, d.OSVersion, d.LastSeen, d.EnrollmentStatus)
	return err
}

func (r *DeviceRepo) GetByUDID(ctx context.Context, udid string) (*domain.Device, error) {
	d := &domain.Device{}
	var detailsJSON []byte
	err := r.pool.QueryRow(ctx,
		`SELECT udid, serial_number, device_name, model, os_version, last_seen, enrollment_status,
		        is_supervised, is_lost_mode, battery_level, details
		 FROM devices WHERE udid=$1`, udid).
		Scan(&d.UDID, &d.SerialNumber, &d.DeviceName, &d.Model, &d.OSVersion, &d.LastSeen, &d.EnrollmentStatus,
			&d.IsSupervised, &d.IsLostMode, &d.BatteryLevel, &detailsJSON)
	if err != nil {
		return nil, err
	}
	if len(detailsJSON) > 0 {
		json.Unmarshal(detailsJSON, &d.Details)
	}
	return d, nil
}

func (r *DeviceRepo) SetLostMode(ctx context.Context, udid string, enabled bool) error {
	_, err := r.pool.Exec(ctx, `UPDATE devices SET is_lost_mode=$1 WHERE udid=$2`, enabled, udid)
	return err
}

func (r *DeviceRepo) List(ctx context.Context, filter string, limit int, offset int) ([]*domain.Device, int, error) {
	var total int
	q := `SELECT count(*) FROM devices`
	args := []interface{}{}
	if filter != "" {
		q += ` WHERE serial_number ILIKE $1 OR device_name ILIKE $1 OR udid ILIKE $1`
		args = append(args, "%"+filter+"%")
	}
	if err := r.pool.QueryRow(ctx, q, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	q2 := `SELECT udid, serial_number, device_name, model, os_version, last_seen, enrollment_status,
	              is_supervised, is_lost_mode, battery_level
	       FROM devices`
	args2 := []interface{}{}
	argIdx := 1
	if filter != "" {
		q2 += fmt.Sprintf(` WHERE serial_number ILIKE $%d OR device_name ILIKE $%d OR udid ILIKE $%d`, argIdx, argIdx, argIdx)
		args2 = append(args2, "%"+filter+"%")
		argIdx++
	}
	q2 += fmt.Sprintf(` ORDER BY last_seen DESC LIMIT $%d OFFSET $%d`, argIdx, argIdx+1)
	args2 = append(args2, limit, offset)

	rows, err := r.pool.Query(ctx, q2, args2...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var devices []*domain.Device
	for rows.Next() {
		d := &domain.Device{}
		if err := rows.Scan(&d.UDID, &d.SerialNumber, &d.DeviceName, &d.Model, &d.OSVersion, &d.LastSeen, &d.EnrollmentStatus,
			&d.IsSupervised, &d.IsLostMode, &d.BatteryLevel); err != nil {
			return nil, 0, err
		}
		devices = append(devices, d)
	}
	return devices, total, nil
}

// ListWithAssets returns devices joined with asset/category info for the frontend device list.
func (r *DeviceRepo) ListWithAssets(ctx context.Context, filter, categoryID, custodianID, rentalStatus, viewerUserID string) ([]*domain.DeviceListItem, error) {
	q := `SELECT d.udid, d.serial_number, d.device_name, d.model, d.os_version,
	             d.last_seen, d.enrollment_status, d.is_supervised, d.is_lost_mode, d.battery_level,
	             COALESCE(a.custodian_name,'') as custodian_name,
	             COALESCE(c.name,'') as category_name,
	             a.category_id, a.custodian_id,
	             COALESCE(a.asset_status,'available') as asset_status,
	             EXISTS(SELECT 1 FROM rentals rl WHERE rl.device_udid = d.udid AND rl.status = 'active') as is_rented
	      FROM devices d
	      LEFT JOIN assets a ON a.device_udid = d.udid
	      LEFT JOIN categories c ON a.category_id = c.id
	      WHERE 1=1`
	args := []interface{}{}
	idx := 1
	if filter != "" {
		q += fmt.Sprintf(` AND (d.serial_number ILIKE $%d OR d.device_name ILIKE $%d OR d.udid ILIKE $%d)`, idx, idx, idx)
		args = append(args, "%"+filter+"%")
		idx++
	}
	if categoryID != "" {
		q += fmt.Sprintf(` AND a.category_id = $%d`, idx)
		args = append(args, categoryID)
		idx++
	}
	if custodianID != "" {
		q += fmt.Sprintf(` AND a.custodian_id = $%d`, idx)
		args = append(args, custodianID)
		idx++
	}
	if rentalStatus != "" {
		q += fmt.Sprintf(` AND EXISTS (SELECT 1 FROM rentals rl WHERE rl.device_udid = d.udid AND rl.status = $%d)`, idx)
		args = append(args, rentalStatus)
		idx++
	}
	if viewerUserID != "" {
		q += fmt.Sprintf(` AND EXISTS (SELECT 1 FROM rentals rl WHERE rl.device_udid = d.udid AND rl.borrower_id = $%d AND rl.status = 'active')`, idx)
		args = append(args, viewerUserID)
		idx++
	}
	q += ` ORDER BY d.last_seen DESC`

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []*domain.DeviceListItem
	for rows.Next() {
		d := &domain.DeviceListItem{}
		var assetStatus string
		var isRented bool
		if err := rows.Scan(&d.UDID, &d.SerialNumber, &d.DeviceName, &d.Model, &d.OSVersion,
			&d.LastSeen, &d.EnrollmentStatus, &d.IsSupervised, &d.IsLostMode, &d.BatteryLevel,
			&d.CustodianName, &d.CategoryName, &d.CategoryID, &d.CustodianID,
			&assetStatus, &isRented); err != nil {
			continue
		}
		if isRented {
			d.AssetStatus = "rented"
		} else if d.IsLostMode {
			d.AssetStatus = "lost"
		} else {
			d.AssetStatus = assetStatus
		}
		devices = append(devices, d)
	}
	return devices, nil
}

// ListAvailable returns all devices with asset status for the rental picker.
func (r *DeviceRepo) ListAvailable(ctx context.Context) ([]*domain.DeviceListItem, error) {
	q := `SELECT d.udid, d.serial_number, d.device_name, d.model, d.os_version,
	             d.enrollment_status,
	             COALESCE(a.asset_status,'available') as asset_status,
	             a.category_id, COALESCE(c.name,'') as category_name,
	             EXISTS(SELECT 1 FROM rentals rl WHERE rl.device_udid = d.udid AND rl.status = 'active') as is_rented
	      FROM devices d
	      LEFT JOIN assets a ON a.device_udid = d.udid
	      LEFT JOIN categories c ON a.category_id = c.id
	      ORDER BY d.device_name`
	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []*domain.DeviceListItem
	for rows.Next() {
		d := &domain.DeviceListItem{}
		var assetStatus string
		var isRented bool
		if err := rows.Scan(&d.UDID, &d.SerialNumber, &d.DeviceName, &d.Model, &d.OSVersion,
			&d.EnrollmentStatus, &assetStatus, &d.CategoryID, &d.CategoryName, &isRented); err != nil {
			continue
		}
		if isRented {
			d.AssetStatus = "rented"
		} else {
			d.AssetStatus = assetStatus
		}
		devices = append(devices, d)
	}
	return devices, nil
}
