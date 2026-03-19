package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type AssetRepo struct{ pool *pgxpool.Pool }

func NewAssetRepo(pool *pgxpool.Pool) *AssetRepo { return &AssetRepo{pool: pool} }

// IsCustodianOfAll returns true if the user is the custodian of ALL given device UDIDs.
func (r *AssetRepo) IsCustodianOfAll(ctx context.Context, userID string, udids []string) (bool, error) {
	if len(udids) == 0 {
		return false, nil
	}

	// Build placeholders: $2, $3, $4, ...
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
