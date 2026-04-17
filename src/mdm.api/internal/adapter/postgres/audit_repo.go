package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anthropics/mdm-server/internal/domain"
)

type AuditRepo struct{ pool *pgxpool.Pool }

func NewAuditRepo(pool *pgxpool.Pool) *AuditRepo { return &AuditRepo{pool: pool} }

func (r *AuditRepo) Create(ctx context.Context, log *domain.AuditLog) error {
	module := log.Module
	if module == "" {
		module = "system"
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO audit_logs (user_id, username, action, target, detail, module, ip_address, user_agent)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		log.UserID, log.Username, log.Action, log.Target, log.Detail, module, log.IPAddress, log.UserAgent)
	return err
}

func (r *AuditRepo) List(ctx context.Context, userID string, action string, module string, limit int, offset int) ([]*domain.AuditLog, error) {
	q := `SELECT id, user_id, username, action, target, detail, module, ip_address, user_agent, timestamp FROM audit_logs WHERE 1=1`
	args := []interface{}{}
	argIdx := 1
	if userID != "" {
		q += fmt.Sprintf(` AND user_id=$%d`, argIdx)
		args = append(args, userID)
		argIdx++
	}
	if action != "" {
		q += fmt.Sprintf(` AND action=$%d`, argIdx)
		args = append(args, action)
		argIdx++
	}
	if module != "" {
		q += fmt.Sprintf(` AND module=$%d`, argIdx)
		args = append(args, module)
		argIdx++
	}
	q += fmt.Sprintf(` ORDER BY timestamp DESC LIMIT $%d OFFSET $%d`, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []*domain.AuditLog
	for rows.Next() {
		l := &domain.AuditLog{}
		if err := rows.Scan(&l.ID, &l.UserID, &l.Username, &l.Action, &l.Target, &l.Detail, &l.Module, &l.IPAddress, &l.UserAgent, &l.Timestamp); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, nil
}
