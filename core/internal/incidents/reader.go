package incidents

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	DefaultPageLimit = 50
	MaximumPageLimit = 200
)

type Reader struct{ pool *pgxpool.Pool }

func NewReader(pool *pgxpool.Pool) *Reader { return &Reader{pool: pool} }

func (reader *Reader) Incidents(ctx context.Context, filter Filter) (List, error) {
	filter.Limit = normalizedLimit(filter.Limit)
	fingerprintValue := filterFingerprint(filter)
	page, err := decodeCursor(filter.Cursor, "incidents", fingerprintValue)
	if err != nil {
		return List{}, err
	}
	snapshot := page.Snapshot
	if snapshot.IsZero() {
		if err := reader.pool.QueryRow(ctx, "SELECT now()").Scan(&snapshot); err != nil {
			return List{}, fmt.Errorf("read incident snapshot: %w", err)
		}
	}
	activeMode := -1
	if filter.Active != nil {
		activeMode = 0
		if *filter.Active {
			activeMode = 1
		}
	}
	severities := severityStrings(filter.Severities)
	statuses := statusStrings(filter.Statuses)
	rows, err := reader.pool.Query(ctx, incidentSelect+`
		WHERE incident.updated_at <= $1
		  AND ($3 = '' OR (incident.updated_at, incident.id) < ($2, NULLIF($3, '')::uuid))
		  AND (COALESCE(cardinality($4::text[]), 0) = 0 OR incident.severity = ANY($4::text[]))
		  AND (COALESCE(cardinality($5::text[]), 0) = 0 OR incident.status = ANY($5::text[]))
		  AND (COALESCE(cardinality($6::uuid[]), 0) = 0 OR incident.integration_id = ANY($6::uuid[]))
		  AND (COALESCE(cardinality($7::uuid[]), 0) = 0 OR incident.resource_id = ANY($7::uuid[]))
		  AND (COALESCE(cardinality($8::uuid[]), 0) = 0 OR incident.owner_user_id = ANY($8::uuid[]))
		  AND ($9 = -1 OR ($9 = 1) = (incident.status NOT IN ('recovered', 'resolved')))
		  AND ($10::timestamptz IS NULL OR incident.detected_at >= $10)
		  AND ($11::timestamptz IS NULL OR incident.detected_at <= $11)
		ORDER BY incident.updated_at DESC, incident.id DESC LIMIT $12
	`, snapshot.UTC(), nullableTime(page.OrderedAt), page.ID, severities, statuses,
		filter.IntegrationIDs, filter.ResourceIDs, filter.OwnerIDs, activeMode,
		filter.From, filter.To, filter.Limit+1)
	if err != nil {
		return List{}, fmt.Errorf("list incidents: %w", err)
	}
	defer rows.Close()
	items := make([]Summary, 0, filter.Limit+1)
	for rows.Next() {
		item, err := scanSummary(rows)
		if err != nil {
			return List{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return List{}, fmt.Errorf("read incidents: %w", err)
	}
	result := List{Items: items}
	if len(result.Items) > filter.Limit {
		result.Items = result.Items[:filter.Limit]
		last := result.Items[len(result.Items)-1]
		result.NextCursor, err = encodeCursor(cursor{
			Kind: "incidents", Fingerprint: fingerprintValue, Snapshot: snapshot.UTC(),
			OrderedAt: last.UpdatedAt, ID: last.ID,
		})
		if err != nil {
			return List{}, fmt.Errorf("encode incident cursor: %w", err)
		}
	}
	return result, nil
}

func (reader *Reader) Incident(ctx context.Context, id string) (Detail, error) {
	item, err := scanSummary(reader.pool.QueryRow(ctx, incidentSelect+" WHERE incident.id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, ErrNotFound
	}
	if err != nil {
		return Detail{}, err
	}
	var fingerprintValue string
	if err := reader.pool.QueryRow(ctx, "SELECT fingerprint FROM incidents WHERE id = $1", id).Scan(&fingerprintValue); err != nil {
		return Detail{}, fmt.Errorf("read incident fingerprint: %w", err)
	}
	return Detail{Summary: item, Fingerprint: fingerprintValue}, nil
}

func (reader *Reader) Timeline(ctx context.Context, incidentID string, filter TimelineFilter) (Timeline, error) {
	filter.Limit = normalizedLimit(filter.Limit)
	var exists bool
	if err := reader.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM incidents WHERE id = $1)", incidentID).Scan(&exists); err != nil {
		return Timeline{}, fmt.Errorf("check incident: %w", err)
	}
	if !exists {
		return Timeline{}, ErrNotFound
	}
	fingerprintValue := fingerprint(incidentID)
	page, err := decodeCursor(filter.Cursor, "incident_timeline", fingerprintValue)
	if err != nil {
		return Timeline{}, err
	}
	snapshot := page.Snapshot
	if snapshot.IsZero() {
		if err := reader.pool.QueryRow(ctx, "SELECT now()").Scan(&snapshot); err != nil {
			return Timeline{}, err
		}
	}
	rows, err := reader.pool.Query(ctx, `
		SELECT timeline.id::text, timeline.incident_id::text,
			COALESCE(timeline.signal_id::text, ''),
			COALESCE(timeline.actor_user_id::text, ''),
			COALESCE(timeline.actor_display_name, actor.display_name, ''),
			COALESCE(timeline.subject_user_id::text, ''),
			COALESCE(timeline.subject_display_name, subject.display_name, ''),
			timeline.kind, COALESCE(timeline.from_status, ''), COALESCE(timeline.to_status, ''),
			COALESCE(timeline.from_severity, ''), COALESCE(timeline.to_severity, ''),
			timeline.summary, timeline.occurred_at
		FROM incident_timeline timeline
		LEFT JOIN users actor ON actor.id = timeline.actor_user_id
		LEFT JOIN users subject ON subject.id = timeline.subject_user_id
		WHERE timeline.incident_id = $1 AND timeline.created_at <= $2
		  AND ($4 = '' OR (timeline.occurred_at, timeline.id) < ($3, NULLIF($4, '')::uuid))
		ORDER BY timeline.occurred_at DESC, timeline.id DESC LIMIT $5
	`, incidentID, snapshot.UTC(), nullableTime(page.OrderedAt), page.ID, filter.Limit+1)
	if err != nil {
		return Timeline{}, fmt.Errorf("list incident timeline: %w", err)
	}
	defer rows.Close()
	items := make([]TimelineEvent, 0, filter.Limit+1)
	for rows.Next() {
		var item TimelineEvent
		if err := rows.Scan(
			&item.ID, &item.IncidentID, &item.SignalID, &item.ActorUserID,
			&item.ActorName, &item.SubjectUserID, &item.SubjectName,
			&item.Kind, &item.FromStatus, &item.ToStatus,
			&item.FromSeverity, &item.ToSeverity, &item.Summary, &item.OccurredAt,
		); err != nil {
			return Timeline{}, fmt.Errorf("scan incident timeline: %w", err)
		}
		item.OccurredAt = item.OccurredAt.UTC()
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return Timeline{}, fmt.Errorf("read incident timeline: %w", err)
	}
	result := Timeline{Items: items}
	if len(result.Items) > filter.Limit {
		result.Items = result.Items[:filter.Limit]
		last := result.Items[len(result.Items)-1]
		result.NextCursor, err = encodeCursor(cursor{
			Kind: "incident_timeline", Fingerprint: fingerprintValue,
			Snapshot: snapshot.UTC(), OrderedAt: last.OccurredAt, ID: last.ID,
		})
		if err != nil {
			return Timeline{}, err
		}
	}
	return result, nil
}

const incidentSelect = `
	SELECT incident.id::text, incident.rule_id::text, rule.name,
		incident.integration_id::text, integration.display_name,
		incident.resource_id::text, resource.display_name, incident.check_type,
		incident.title, incident.summary, incident.severity, incident.status,
		COALESCE(incident.owner_user_id::text, ''), COALESCE(owner.display_name, ''),
		incident.detected_at, incident.latest_signal_at, incident.acknowledged_at,
		incident.recovered_at, incident.resolved_at, incident.version,
		incident.updated_at
	FROM incidents incident
	JOIN incident_rules rule ON rule.id = incident.rule_id
	JOIN integrations integration ON integration.id = incident.integration_id
	JOIN resources resource ON resource.id = incident.resource_id
	LEFT JOIN users owner ON owner.id = incident.owner_user_id
`

type scanner interface{ Scan(...any) error }

func scanSummary(row scanner) (Summary, error) {
	var item Summary
	var acknowledgedAt, recoveredAt, resolvedAt pgtype.Timestamptz
	if err := row.Scan(
		&item.ID, &item.RuleID, &item.RuleName, &item.IntegrationID,
		&item.IntegrationName, &item.ResourceID, &item.ResourceName,
		&item.CheckType, &item.Title, &item.Summary, &item.Severity, &item.Status,
		&item.OwnerUserID, &item.OwnerName, &item.DetectedAt, &item.LatestSignalAt,
		&acknowledgedAt, &recoveredAt, &resolvedAt, &item.Version, &item.UpdatedAt,
	); err != nil {
		return Summary{}, err
	}
	item.DetectedAt = item.DetectedAt.UTC()
	item.LatestSignalAt = item.LatestSignalAt.UTC()
	item.AcknowledgedAt = nullableTimestamp(acknowledgedAt)
	item.RecoveredAt = nullableTimestamp(recoveredAt)
	item.ResolvedAt = nullableTimestamp(resolvedAt)
	item.UpdatedAt = item.UpdatedAt.UTC()
	return item, nil
}

func nullableTimestamp(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}

func normalizedLimit(limit int) int {
	if limit <= 0 {
		return DefaultPageLimit
	}
	if limit > MaximumPageLimit {
		return MaximumPageLimit
	}
	return limit
}

func filterFingerprint(filter Filter) string {
	active := ""
	if filter.Active != nil {
		active = strconv.FormatBool(*filter.Active)
	}
	from, to := "", ""
	if filter.From != nil {
		from = filter.From.UTC().Format(time.RFC3339Nano)
	}
	if filter.To != nil {
		to = filter.To.UTC().Format(time.RFC3339Nano)
	}
	return fingerprint(
		sorted(severityStrings(filter.Severities)), sorted(statusStrings(filter.Statuses)),
		sorted(filter.IntegrationIDs), sorted(filter.ResourceIDs), sorted(filter.OwnerIDs),
		active, from, to,
	)
}

func severityStrings(values []Severity) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = string(value)
	}
	return result
}

func statusStrings(values []Status) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = string(value)
	}
	return result
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}
