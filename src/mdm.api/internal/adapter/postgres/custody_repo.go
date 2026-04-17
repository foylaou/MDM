package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anthropics/mdm-server/internal/domain"
)

type CustodyRepo struct{ pool *pgxpool.Pool }

func NewCustodyRepo(pool *pgxpool.Pool) *CustodyRepo { return &CustodyRepo{pool: pool} }

func (r *CustodyRepo) Append(ctx context.Context, log *domain.AssetCustodyLog) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO asset_custody_logs
		 (asset_id, action, from_user_id, from_user_name, to_user_id, to_user_name,
		  reason, operated_by, operator_name)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		log.AssetID, log.Action,
		log.FromUserID, log.FromUserName,
		log.ToUserID, log.ToUserName,
		log.Reason, log.OperatedBy, log.OperatorName,
	)
	return err
}

func (r *CustodyRepo) ListByAsset(ctx context.Context, assetID string) ([]*domain.AssetCustodyLog, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, asset_id, action,
		        from_user_id, COALESCE(from_user_name,''),
		        to_user_id, COALESCE(to_user_name,''),
		        COALESCE(reason,''), operated_by, COALESCE(operator_name,''),
		        created_at
		 FROM asset_custody_logs
		 WHERE asset_id = $1
		 ORDER BY created_at DESC`, assetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*domain.AssetCustodyLog
	for rows.Next() {
		l := &domain.AssetCustodyLog{}
		if err := rows.Scan(
			&l.ID, &l.AssetID, &l.Action,
			&l.FromUserID, &l.FromUserName,
			&l.ToUserID, &l.ToUserName,
			&l.Reason, &l.OperatedBy, &l.OperatorName,
			&l.CreatedAt,
		); err != nil {
			continue
		}
		logs = append(logs, l)
	}
	return logs, nil
}
