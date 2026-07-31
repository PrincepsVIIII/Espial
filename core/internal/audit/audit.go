// Package audit writes append-only, redacted operational audit events.
package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

const MaxSummaryBytes = 64 * 1024

var (
	actionPattern     = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,126}$`)
	targetTypePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,62}$`)
)

type Event struct {
	ActorUserID   string
	Action        string
	TargetType    string
	TargetID      string
	Result        string
	SourceAddress string
	CorrelationID string
	BeforeSummary map[string]any
	AfterSummary  map[string]any
	OccurredAt    time.Time
}

type Execer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

type Writer struct{ database Execer }

func NewWriter(database Execer) *Writer { return &Writer{database: database} }

func (writer *Writer) Append(ctx context.Context, event Event) error {
	return Append(ctx, writer.database, event)
}

func Append(ctx context.Context, database Execer, event Event) error {
	arguments, err := arguments(event)
	if err != nil {
		return err
	}
	if _, err := database.Exec(ctx, `
		INSERT INTO audit_events (
			id, actor_user_id, action, target_type, target_id, result,
			source_address, correlation_id, before_summary, after_summary, occurred_at
		) VALUES (
			gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9::jsonb, $10
		)
	`, arguments...); err != nil {
		return fmt.Errorf("append audit event: %w", err)
	}
	return nil
}

func arguments(event Event) ([]any, error) {
	if !actionPattern.MatchString(event.Action) || !targetTypePattern.MatchString(event.TargetType) ||
		(event.Result != "succeeded" && event.Result != "failed" && event.Result != "denied") ||
		strings.TrimSpace(event.CorrelationID) == "" || len(event.CorrelationID) > 128 || event.OccurredAt.IsZero() {
		return nil, errors.New("invalid audit event")
	}
	if event.SourceAddress != "" && net.ParseIP(event.SourceAddress) == nil {
		return nil, errors.New("invalid audit event")
	}
	before, err := encodeSummary(event.BeforeSummary)
	if err != nil {
		return nil, err
	}
	after, err := encodeSummary(event.AfterSummary)
	if err != nil {
		return nil, err
	}
	return []any{
		nullable(event.ActorUserID), event.Action, event.TargetType, nullable(event.TargetID),
		event.Result, nullable(event.SourceAddress), event.CorrelationID, before, after,
		event.OccurredAt.UTC(),
	}, nil
}

func encodeSummary(summary map[string]any) (any, error) {
	if summary == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(summary)
	if err != nil || len(encoded) > MaxSummaryBytes {
		return nil, errors.New("invalid audit event")
	}
	return string(encoded), nil
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}
