package audit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// Event represents a security-relevant action.
type Event struct {
	Timestamp time.Time
	UserID    string
	OrgID     string
	Action    string // "login", "logout", "token.create", "token.revoke", "model.deploy", etc.
	Resource  string // resource ID
	IP        string
	UserAgent string
	Status    string // "success", "failed", "denied"
	Details   map[string]string
}

// Logger writes audit events to structured log + optional DB.
type Logger struct {
	zap *zap.Logger
	db  *pgxpool.Pool // optional, can be nil
}

// New creates a new audit logger.
func New(logger *zap.Logger, db *pgxpool.Pool) *Logger {
	return &Logger{
		zap: logger,
		db:  db,
	}
}

// Log writes an audit event to the structured logger and optionally to the database.
func (l *Logger) Log(ctx context.Context, event Event) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	// Always write to zap with audit-specific fields
	fields := []zap.Field{
		zap.String("audit.action", event.Action),
		zap.String("audit.status", event.Status),
		zap.String("audit.user_id", event.UserID),
		zap.String("audit.org_id", event.OrgID),
		zap.String("audit.resource", event.Resource),
		zap.String("audit.ip", event.IP),
		zap.String("audit.user_agent", event.UserAgent),
		zap.Time("audit.timestamp", event.Timestamp),
	}
	if event.Details != nil {
		fields = append(fields, zap.Any("audit.details", event.Details))
	}
	l.zap.Info("audit", fields...)

	// If db is available, persist to audit_log table
	if l.db != nil {
		l.insertDB(ctx, event)
	}
}

func (l *Logger) insertDB(ctx context.Context, event Event) {
	var detailsJSON []byte
	if event.Details != nil {
		var err error
		detailsJSON, err = json.Marshal(event.Details)
		if err != nil {
			l.zap.Error("failed to marshal audit details", zap.Error(err))
			return
		}
	}

	_, err := l.db.Exec(ctx,
		`INSERT INTO audit_log (timestamp, user_id, org_id, action, resource, ip, user_agent, status, details)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		event.Timestamp,
		event.UserID,
		event.OrgID,
		event.Action,
		event.Resource,
		event.IP,
		event.UserAgent,
		event.Status,
		detailsJSON,
	)
	if err != nil {
		l.zap.Error("failed to insert audit log", zap.Error(err))
	}
}
