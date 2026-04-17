package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anthropics/mdm-server/internal/domain"
)

type NotificationRepo struct{ pool *pgxpool.Pool }

func NewNotificationRepo(pool *pgxpool.Pool) *NotificationRepo {
	return &NotificationRepo{pool: pool}
}

func (r *NotificationRepo) Create(ctx context.Context, notif *domain.Notification) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`INSERT INTO notifications (type, event, recipient, subject, body, status, reference_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		notif.Type, notif.Event, notif.Recipient, notif.Subject, notif.Body, notif.Status, notif.ReferenceID,
	).Scan(&id)
	return id, err
}

func (r *NotificationRepo) UpdateStatus(ctx context.Context, id string, status string, errMsg string) error {
	if status == "sent" {
		_, err := r.pool.Exec(ctx,
			`UPDATE notifications SET status=$1, sent_at=now() WHERE id=$2`, status, id)
		return err
	}
	_, err := r.pool.Exec(ctx,
		`UPDATE notifications SET status=$1, error_message=$2 WHERE id=$3`, status, errMsg, id)
	return err
}

func (r *NotificationRepo) List(ctx context.Context, event string, referenceID string, limit int) ([]*domain.Notification, error) {
	q := `SELECT id, type, event, recipient, subject, body, status, error_message, reference_id, created_at, sent_at
	      FROM notifications WHERE 1=1`
	args := []interface{}{}
	idx := 1
	if event != "" {
		q += fmt.Sprintf(` AND event=$%d`, idx)
		args = append(args, event)
		idx++
	}
	if referenceID != "" {
		q += fmt.Sprintf(` AND reference_id=$%d`, idx)
		args = append(args, referenceID)
		idx++
	}
	q += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d`, idx)
	args = append(args, limit)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notifs []*domain.Notification
	for rows.Next() {
		n := &domain.Notification{}
		if err := rows.Scan(&n.ID, &n.Type, &n.Event, &n.Recipient, &n.Subject, &n.Body,
			&n.Status, &n.ErrorMessage, &n.ReferenceID, &n.CreatedAt, &n.SentAt); err != nil {
			continue
		}
		notifs = append(notifs, n)
	}
	return notifs, nil
}

