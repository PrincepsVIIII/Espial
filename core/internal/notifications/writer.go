package notifications

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/incidents"
	"github.com/jackc/pgx/v5"
)

// IntentWriter persists one intent per enabled destination in the incident
// transaction. A matching silence is terminal evidence, never deferred work.
type IntentWriter struct{}

func NewIntentWriter() *IntentWriter { return &IntentWriter{} }

func (writer *IntentWriter) EnqueueIncidentEvent(ctx context.Context, tx pgx.Tx, event incidents.NotificationEvent) error {
	if event.TimelineEventID == "" || event.IncidentID == "" {
		return errors.New("invalid incident notification event")
	}
	rows, err := tx.Query(ctx, `
		SELECT id::text, display_name
		FROM notification_destinations
		WHERE enabled
		ORDER BY id
	`)
	if err != nil {
		return fmt.Errorf("list notification destinations: %w", err)
	}
	type destination struct{ id, name string }
	destinations := []destination{}
	for rows.Next() {
		var item destination
		if err := rows.Scan(&item.id, &item.name); err != nil {
			rows.Close()
			return fmt.Errorf("scan notification destination: %w", err)
		}
		destinations = append(destinations, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("read notification destinations: %w", err)
	}
	rows.Close()

	silenceID, err := matchingSilence(ctx, tx, event, event.OccurredAt)
	if err != nil {
		return err
	}
	now := event.CreatedAt.UTC().Truncate(time.Microsecond)
	if now.IsZero() {
		now = time.Now().UTC().Truncate(time.Microsecond)
	}
	for _, destination := range destinations {
		state := StateQueued
		var terminalAt any
		if silenceID != "" {
			state, terminalAt = StateSuppressed, now
		}
		var intentID string
		err := tx.QueryRow(ctx, `
			INSERT INTO notification_intents (
				incident_event_id, incident_id, destination_id, event_kind,
				title, summary, severity, incident_status, event_occurred_at,
				state, suppressed_silence_id, terminal_at, created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9,
				$10, NULLIF($11, '')::uuid, $12, $13, $13
			)
			ON CONFLICT (incident_event_id, destination_id)
				WHERE incident_event_id IS NOT NULL DO NOTHING
			RETURNING id::text
		`, event.TimelineEventID, event.IncidentID, destination.id, event.Kind,
			event.Title, event.Summary, event.Severity, event.Status,
			event.OccurredAt.UTC(), state, silenceID, terminalAt, now).Scan(&intentID)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return fmt.Errorf("append notification intent: %w", err)
		}
		summary := "Notification queued for " + destination.name + "."
		if state == StateSuppressed {
			summary = "Notification suppressed for " + destination.name + " by a matching silence."
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO incident_timeline (incident_id, kind, summary, occurred_at)
			VALUES ($1, 'notification', $2, $3)
		`, event.IncidentID, summary, now); err != nil {
			return fmt.Errorf("append notification timeline evidence: %w", err)
		}
	}
	return nil
}

func matchingSilence(ctx context.Context, tx pgx.Tx, event incidents.NotificationEvent, at time.Time) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		SELECT id::text
		FROM silences
		WHERE enabled AND revoked_at IS NULL
		  AND starts_at <= $4 AND expires_at > $4
		  AND (
			incident_id = NULLIF($1, '')::uuid
			OR rule_id = NULLIF($2, '')::uuid
			OR resource_id = NULLIF($3, '')::uuid
		  )
		ORDER BY (incident_id IS NOT NULL) DESC,
			(rule_id IS NOT NULL) DESC, created_at DESC, id
		LIMIT 1
	`, event.IncidentID, event.RuleID, event.ResourceID, at.UTC()).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("match notification silence: %w", err)
	}
	return id, nil
}
