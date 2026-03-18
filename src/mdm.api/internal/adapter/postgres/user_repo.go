package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anthropics/mdm-server/internal/domain"
)

type UserRepo struct{ pool *pgxpool.Pool }

func NewUserRepo(pool *pgxpool.Pool) *UserRepo { return &UserRepo{pool: pool} }

func (r *UserRepo) Create(ctx context.Context, u *domain.User) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO users (username, password_hash, role, display_name) VALUES ($1,$2,$3,$4) RETURNING id`,
		u.Username, u.PasswordHash, u.Role, u.DisplayName).Scan(&u.ID)
}

func (r *UserRepo) GetByID(ctx context.Context, id string) (*domain.User, error) {
	u := &domain.User{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, username, password_hash, role, display_name, is_active, created_at, updated_at FROM users WHERE id=$1`, id).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.DisplayName, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}
	return u, nil
}

func (r *UserRepo) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	u := &domain.User{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, username, password_hash, role, display_name, is_active, created_at, updated_at FROM users WHERE username=$1`, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.DisplayName, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}
	return u, nil
}

func (r *UserRepo) List(ctx context.Context) ([]*domain.User, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, username, role, display_name, is_active FROM users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []*domain.User
	for rows.Next() {
		u := &domain.User{}
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.DisplayName, &u.IsActive); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (r *UserRepo) Update(ctx context.Context, u *domain.User) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET role=$1, display_name=$2, password_hash=$3, updated_at=now() WHERE id=$4`,
		u.Role, u.DisplayName, u.PasswordHash, u.ID)
	return err
}

func (r *UserRepo) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, id)
	return err
}
