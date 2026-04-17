package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anthropics/mdm-server/internal/domain"
)

type ProfileRepo struct{ pool *pgxpool.Pool }

func NewProfileRepo(pool *pgxpool.Pool) *ProfileRepo { return &ProfileRepo{pool: pool} }

func (r *ProfileRepo) List(ctx context.Context) ([]*domain.Profile, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, filename, size, uploaded_by, created_at FROM profiles ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var profiles []*domain.Profile
	for rows.Next() {
		p := &domain.Profile{}
		if err := rows.Scan(&p.ID, &p.Name, &p.Filename, &p.Size, &p.UploadedBy, &p.CreatedAt); err != nil {
			continue
		}
		profiles = append(profiles, p)
	}
	return profiles, nil
}

func (r *ProfileRepo) Create(ctx context.Context, profile *domain.Profile) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`INSERT INTO profiles (name, filename, content, size, uploaded_by) VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		profile.Name, profile.Filename, profile.Content, profile.Size, profile.UploadedBy,
	).Scan(&id)
	return id, err
}

func (r *ProfileRepo) GetContent(ctx context.Context, id string) ([]byte, string, error) {
	var content []byte
	var filename string
	err := r.pool.QueryRow(ctx, `SELECT content, filename FROM profiles WHERE id=$1`, id).Scan(&content, &filename)
	return content, filename, err
}

func (r *ProfileRepo) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM profiles WHERE id=$1`, id)
	return err
}
