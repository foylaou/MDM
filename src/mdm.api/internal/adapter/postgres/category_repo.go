package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anthropics/mdm-server/internal/domain"
)

type CategoryRepo struct{ pool *pgxpool.Pool }

func NewCategoryRepo(pool *pgxpool.Pool) *CategoryRepo { return &CategoryRepo{pool: pool} }

func (r *CategoryRepo) List(ctx context.Context) ([]*domain.Category, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, parent_id, name, level, sort_order FROM categories ORDER BY sort_order, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cats []*domain.Category
	for rows.Next() {
		c := &domain.Category{}
		if err := rows.Scan(&c.ID, &c.ParentID, &c.Name, &c.Level, &c.SortOrder); err != nil {
			continue
		}
		cats = append(cats, c)
	}
	return cats, nil
}

func (r *CategoryRepo) Create(ctx context.Context, cat *domain.Category) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`INSERT INTO categories (parent_id, name, level) VALUES ($1, $2, $3) RETURNING id`,
		cat.ParentID, cat.Name, cat.Level,
	).Scan(&id)
	return id, err
}

func (r *CategoryRepo) GetLevel(ctx context.Context, id string) (int, error) {
	var level int
	err := r.pool.QueryRow(ctx, `SELECT level FROM categories WHERE id=$1`, id).Scan(&level)
	return level, err
}

func (r *CategoryRepo) Update(ctx context.Context, id string, name string) error {
	_, err := r.pool.Exec(ctx, `UPDATE categories SET name=$1 WHERE id=$2`, name, id)
	return err
}

func (r *CategoryRepo) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM categories WHERE id=$1`, id)
	return err
}
