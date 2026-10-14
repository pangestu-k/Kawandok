package audit

import (
	"context"
	"database/sql"
)

type Logger struct {
	db *sql.DB
}

func NewLogger(db *sql.DB) *Logger {
	return &Logger{db: db}
}

func (l *Logger) Record(ctx context.Context, actorID, action, targetType, targetID, result, requestID string) error {
	var actor any
	if actorID != "" {
		actor = sql.NullString{String: actorID, Valid: true}
	}
	_, err := l.db.ExecContext(ctx, `
		INSERT INTO audit_events (actor_id, action, target_type, target_id, result, request_id)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, actor, action, targetType, targetID, result, requestID)
	return err
}
