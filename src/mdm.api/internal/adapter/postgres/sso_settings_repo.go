package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anthropics/mdm-server/internal/domain"
)

type SSOSettingsRepo struct{ pool *pgxpool.Pool }

func NewSSOSettingsRepo(pool *pgxpool.Pool) *SSOSettingsRepo {
	return &SSOSettingsRepo{pool: pool}
}

func (r *SSOSettingsRepo) Get(ctx context.Context) (*domain.SSOSettings, error) {
	s := &domain.SSOSettings{}
	var updatedBy *string
	err := r.pool.QueryRow(ctx, `
		SELECT enabled, issuer_url, client_id, client_secret, redirect_url, updated_at, updated_by
		FROM sso_settings WHERE id='default'`).
		Scan(&s.Enabled, &s.IssuerURL, &s.ClientID, &s.ClientSecret, &s.RedirectURL,
			&s.UpdatedAt, &updatedBy)
	if err != nil {
		return nil, err
	}
	if updatedBy != nil {
		s.UpdatedBy = *updatedBy
	}
	return s, nil
}

func (r *SSOSettingsRepo) Upsert(ctx context.Context, s *domain.SSOSettings, updatedBy string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE sso_settings SET
		  enabled=$1, issuer_url=$2, client_id=$3, client_secret=$4, redirect_url=$5,
		  updated_at=now(), updated_by=$6
		WHERE id='default'`,
		s.Enabled, s.IssuerURL, s.ClientID, s.ClientSecret, s.RedirectURL, updatedBy,
	)
	return err
}
