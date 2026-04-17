package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anthropics/mdm-server/internal/domain"
)

type PermissionRepo struct{ pool *pgxpool.Pool }

func NewPermissionRepo(pool *pgxpool.Pool) *PermissionRepo { return &PermissionRepo{pool: pool} }

func (r *PermissionRepo) GetByUserID(ctx context.Context, userID string) ([]*domain.ModulePermission, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, module, permission, granted_by, granted_at
		 FROM user_module_permissions WHERE user_id=$1 ORDER BY module`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var perms []*domain.ModulePermission
	for rows.Next() {
		p := &domain.ModulePermission{}
		if err := rows.Scan(&p.ID, &p.UserID, &p.Module, &p.Permission, &p.GrantedBy, &p.GrantedAt); err != nil {
			continue
		}
		perms = append(perms, p)
	}
	return perms, nil
}

func (r *PermissionRepo) GetByUserAndModule(ctx context.Context, userID string, module string) (*domain.ModulePermission, error) {
	p := &domain.ModulePermission{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, module, permission, granted_by, granted_at
		 FROM user_module_permissions WHERE user_id=$1 AND module=$2`, userID, module).
		Scan(&p.ID, &p.UserID, &p.Module, &p.Permission, &p.GrantedBy, &p.GrantedAt)
	if err != nil {
		return nil, fmt.Errorf("permission not found: %w", err)
	}
	return p, nil
}

func (r *PermissionRepo) Set(ctx context.Context, perm *domain.ModulePermission) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO user_module_permissions (user_id, module, permission, granted_by)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (user_id, module) DO UPDATE SET permission=EXCLUDED.permission, granted_by=EXCLUDED.granted_by, granted_at=now()`,
		perm.UserID, perm.Module, perm.Permission, perm.GrantedBy)
	return err
}

func (r *PermissionRepo) Delete(ctx context.Context, userID string, module string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM user_module_permissions WHERE user_id=$1 AND module=$2`, userID, module)
	return err
}
