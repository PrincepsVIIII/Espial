package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/audit"
	"github.com/PrincepsVIIII/Espial/core/internal/health"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	DefaultPageLimit  = 50
	MaximumPageLimit  = 200
	MaximumAuditRange = 31 * 24 * time.Hour
)

var ErrNotFound = errors.New("monitoring record not found")

type ReadService struct{ pool *pgxpool.Pool }

func NewReadService(pool *pgxpool.Pool) *ReadService { return &ReadService{pool: pool} }

func (service *ReadService) Overview(ctx context.Context) (Overview, error) {
	tx, err := service.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Overview{}, fmt.Errorf("begin overview read: %w", err)
	}
	defer tx.Rollback(ctx)
	result := Overview{
		ResourceCounts: []StateCount{}, IntegrationCounts: []IntegrationStateCount{},
		RecentChanges: []RecentStateChange{}, ActiveIncidentCounts: []ActiveIncidentCount{},
		ActiveIncidents: []ActiveIncidentSummary{},
	}
	if err := tx.QueryRow(ctx, "SELECT now()").Scan(&result.GeneratedAt); err != nil {
		return Overview{}, fmt.Errorf("read overview time: %w", err)
	}
	rows, err := tx.Query(ctx, `
		SELECT COALESCE(ch.state, 'unknown'), count(*)
		FROM resources r LEFT JOIN current_health ch ON ch.resource_id = r.id
		GROUP BY COALESCE(ch.state, 'unknown') ORDER BY COALESCE(ch.state, 'unknown')
	`)
	if err != nil {
		return Overview{}, fmt.Errorf("read overview resource counts: %w", err)
	}
	for rows.Next() {
		var item StateCount
		if err := rows.Scan(&item.State, &item.Count); err != nil {
			rows.Close()
			return Overview{}, fmt.Errorf("scan overview resource count: %w", err)
		}
		result.ResourceCounts = append(result.ResourceCounts, item)
		if item.State == health.Stale {
			result.StaleCount = item.Count
		}
		if item.State == health.Unknown {
			result.UnknownCount = item.Count
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Overview{}, fmt.Errorf("read overview resource counts: %w", err)
	}
	rows.Close()

	rows, err = tx.Query(ctx, `
		SELECT CASE
			WHEN NOT i.enabled THEN 'disabled'
			ELSE COALESCE(ai.state, 'not_started')
		END AS runtime_state, count(*)
		FROM integrations i LEFT JOIN adapter_instances ai ON ai.integration_id = i.id
		GROUP BY runtime_state ORDER BY runtime_state
	`)
	if err != nil {
		return Overview{}, fmt.Errorf("read overview integration counts: %w", err)
	}
	for rows.Next() {
		var item IntegrationStateCount
		if err := rows.Scan(&item.State, &item.Count); err != nil {
			rows.Close()
			return Overview{}, fmt.Errorf("scan overview integration count: %w", err)
		}
		result.IntegrationCounts = append(result.IntegrationCounts, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Overview{}, fmt.Errorf("read overview integration counts: %w", err)
	}
	rows.Close()

	rows, err = tx.Query(ctx, `
		SELECT r.id::text, r.integration_id::text, r.display_name, ch.state, ch.reason, ch.updated_at
		FROM current_health ch JOIN resources r ON r.id = ch.resource_id
		ORDER BY ch.updated_at DESC, r.id DESC LIMIT 10
	`)
	if err != nil {
		return Overview{}, fmt.Errorf("read recent state changes: %w", err)
	}
	for rows.Next() {
		var item RecentStateChange
		if err := rows.Scan(&item.ResourceID, &item.IntegrationID, &item.DisplayName, &item.State, &item.Reason, &item.ChangedAt); err != nil {
			rows.Close()
			return Overview{}, fmt.Errorf("scan recent state change: %w", err)
		}
		item.ChangedAt = item.ChangedAt.UTC()
		result.RecentChanges = append(result.RecentChanges, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Overview{}, fmt.Errorf("read recent state changes: %w", err)
	}
	rows.Close()

	rows, err = tx.Query(ctx, `
		SELECT severity, count(*)
		FROM incidents
		WHERE status NOT IN ('recovered', 'resolved')
		GROUP BY severity ORDER BY severity
	`)
	if err != nil {
		return Overview{}, fmt.Errorf("read active incident counts: %w", err)
	}
	for rows.Next() {
		var item ActiveIncidentCount
		if err := rows.Scan(&item.Severity, &item.Count); err != nil {
			rows.Close()
			return Overview{}, fmt.Errorf("scan active incident count: %w", err)
		}
		result.ActiveIncidentCounts = append(result.ActiveIncidentCounts, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Overview{}, fmt.Errorf("read active incident counts: %w", err)
	}
	rows.Close()

	rows, err = tx.Query(ctx, `
		SELECT incident.id::text, incident.title, incident.severity, incident.status,
			integration.display_name, resource.display_name,
			incident.detected_at, incident.updated_at
		FROM incidents incident
		JOIN integrations integration ON integration.id = incident.integration_id
		JOIN resources resource ON resource.id = incident.resource_id
		WHERE incident.status NOT IN ('recovered', 'resolved')
		ORDER BY incident.updated_at DESC, incident.id DESC LIMIT 5
	`)
	if err != nil {
		return Overview{}, fmt.Errorf("read active incidents: %w", err)
	}
	for rows.Next() {
		var item ActiveIncidentSummary
		if err := rows.Scan(
			&item.ID, &item.Title, &item.Severity, &item.Status,
			&item.IntegrationName, &item.ResourceName, &item.DetectedAt, &item.UpdatedAt,
		); err != nil {
			rows.Close()
			return Overview{}, fmt.Errorf("scan active incident: %w", err)
		}
		item.DetectedAt = item.DetectedAt.UTC()
		item.UpdatedAt = item.UpdatedAt.UTC()
		result.ActiveIncidents = append(result.ActiveIncidents, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Overview{}, fmt.Errorf("read active incidents: %w", err)
	}
	rows.Close()
	if err := tx.Commit(ctx); err != nil {
		return Overview{}, fmt.Errorf("commit overview read: %w", err)
	}
	result.GeneratedAt = result.GeneratedAt.UTC()
	return result, nil
}

func (service *ReadService) Resources(ctx context.Context, filter ResourceFilter) (ResourceList, error) {
	filter.Limit = normalizedLimit(filter.Limit)
	fingerprint := resourceFingerprint(filter)
	cursor, err := decodeCursor(filter.Cursor, "resources", fingerprint)
	if err != nil {
		return ResourceList{}, &Error{Code: "invalid_cursor"}
	}
	snapshot := cursor.Snapshot
	if snapshot.IsZero() {
		if snapshot, err = service.snapshot(ctx); err != nil {
			return ResourceList{}, err
		}
	}
	staleMode := -1
	if filter.Stale != nil {
		staleMode = 0
		if *filter.Stale {
			staleMode = 1
		}
	}
	states := make([]string, len(filter.States))
	for index, state := range filter.States {
		states[index] = string(state)
	}
	rows, err := service.pool.Query(ctx, `
		WITH candidates AS (
			SELECT
				r.id::text AS resource_id, r.integration_id::text, i.display_name,
				r.external_id, r.kind, r.display_name, r.attributes::text,
				COALESCE(r.source_url, ''), r.first_seen_at, r.last_seen_at,
				COALESCE(ch.state, 'unknown'), COALESCE(ch.reason, 'no valid observation'),
				COALESCE(ch.observation_id::text, ''), ch.observed_at, ch.last_success_at,
				ch.stale_at, ch.unknown_at, COALESCE(ch.updated_at, r.last_seen_at),
				lo.id::text AS latest_observation_id, lo.check_type, lo.observed_state, lo.summary,
				lo.observed_at, lo.received_at, lo.expected_refresh_seconds,
				lo.measurements::text, lo.metadata::text,
				COALESCE(ch.updated_at, r.last_seen_at) AS ordered_at
			FROM resources r
			JOIN integrations i ON i.id = r.integration_id
			LEFT JOIN current_health ch ON ch.resource_id = r.id
			LEFT JOIN LATERAL (
				SELECT o.* FROM observations o WHERE o.resource_id = r.id
				ORDER BY o.observed_at DESC, o.received_at DESC, o.id DESC LIMIT 1
			) lo ON true
			WHERE COALESCE(ch.updated_at, r.last_seen_at) <= $1
			  AND (COALESCE(cardinality($4::text[]), 0) = 0 OR COALESCE(ch.state, 'unknown') = ANY($4::text[]))
			  AND (COALESCE(cardinality($5::text[]), 0) = 0 OR r.kind = ANY($5::text[]))
			  AND (COALESCE(cardinality($6::uuid[]), 0) = 0 OR r.integration_id = ANY($6::uuid[]))
			  AND ($7 = -1 OR ($7 = 1) = (COALESCE(ch.state, 'unknown') = 'stale'))
		)
		SELECT * FROM candidates
		WHERE ($3 = '' OR (ordered_at, resource_id::uuid) < ($2, NULLIF($3, '')::uuid))
		ORDER BY ordered_at DESC, resource_id::uuid DESC LIMIT $8
	`, snapshot, nullableCursorTime(cursor), cursor.ID, states, filter.Kinds,
		filter.IntegrationIDs, staleMode, filter.Limit+1)
	if err != nil {
		return ResourceList{}, fmt.Errorf("list resources: %w", err)
	}
	defer rows.Close()
	items := make([]ResourceView, 0, filter.Limit+1)
	ordered := make([]time.Time, 0, filter.Limit+1)
	for rows.Next() {
		item, orderedAt, err := scanResource(rows)
		if err != nil {
			return ResourceList{}, err
		}
		items = append(items, item)
		ordered = append(ordered, orderedAt)
	}
	if err := rows.Err(); err != nil {
		return ResourceList{}, fmt.Errorf("read resources: %w", err)
	}
	result := ResourceList{Items: items}
	if len(result.Items) > filter.Limit {
		last := filter.Limit - 1
		result.Items = result.Items[:filter.Limit]
		result.NextCursor, err = encodeCursor(pageCursor{
			Kind: "resources", Fingerprint: fingerprint, Snapshot: snapshot,
			OrderedAt: ordered[last], ID: result.Items[last].ID,
		})
		if err != nil {
			return ResourceList{}, fmt.Errorf("encode resource cursor: %w", err)
		}
	}
	return result, nil
}

func (service *ReadService) Resource(ctx context.Context, id string) (ResourceView, error) {
	row := service.pool.QueryRow(ctx, `
		SELECT
			r.id::text, r.integration_id::text, i.display_name,
			r.external_id, r.kind, r.display_name, r.attributes::text,
			COALESCE(r.source_url, ''), r.first_seen_at, r.last_seen_at,
			COALESCE(ch.state, 'unknown'), COALESCE(ch.reason, 'no valid observation'),
			COALESCE(ch.observation_id::text, ''), ch.observed_at, ch.last_success_at,
			ch.stale_at, ch.unknown_at, COALESCE(ch.updated_at, r.last_seen_at),
			lo.id::text, lo.check_type, lo.observed_state, lo.summary,
			lo.observed_at, lo.received_at, lo.expected_refresh_seconds,
			lo.measurements::text, lo.metadata::text,
			COALESCE(ch.updated_at, r.last_seen_at)
		FROM resources r
		JOIN integrations i ON i.id = r.integration_id
		LEFT JOIN current_health ch ON ch.resource_id = r.id
		LEFT JOIN LATERAL (
			SELECT o.* FROM observations o WHERE o.resource_id = r.id
			ORDER BY o.observed_at DESC, o.received_at DESC, o.id DESC LIMIT 1
		) lo ON true
		WHERE r.id = $1
	`, id)
	result, _, err := scanResource(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return ResourceView{}, ErrNotFound
	}
	return result, err
}

type scanner interface{ Scan(...any) error }

func scanResource(row scanner) (ResourceView, time.Time, error) {
	var item ResourceView
	var attributes string
	var observationID pgtype.Text
	var observationState pgtype.Text
	var observationObserved, observationReceived pgtype.Timestamptz
	var observationCheck, observationSummary, measurements, metadata pgtype.Text
	var expectedRefresh pgtype.Int4
	var orderedAt time.Time
	if err := row.Scan(
		&item.ID, &item.IntegrationID, &item.IntegrationName,
		&item.ExternalID, &item.Kind, &item.DisplayName, &attributes,
		&item.SourceURL, &item.FirstSeenAt, &item.LastSeenAt,
		&item.Health.State, &item.Health.Reason, &item.Health.ObservationID,
		&item.Health.ObservedAt, &item.Health.LastSuccessAt, &item.Health.StaleAt,
		&item.Health.UnknownAt, &item.Health.UpdatedAt,
		&observationID, &observationCheck, &observationState, &observationSummary,
		&observationObserved, &observationReceived, &expectedRefresh,
		&measurements, &metadata, &orderedAt,
	); err != nil {
		return ResourceView{}, time.Time{}, err
	}
	item.Attributes = json.RawMessage(attributes)
	item.FirstSeenAt = item.FirstSeenAt.UTC()
	item.LastSeenAt = item.LastSeenAt.UTC()
	normalizeCurrent(&item.Health)
	if observationID.Valid {
		item.LatestObservation = &ObservationView{
			ID: observationID.String, CheckType: observationCheck.String,
			State: health.State(observationState.String), Summary: observationSummary.String,
			ObservedAt: observationObserved.Time.UTC(), ReceivedAt: observationReceived.Time.UTC(),
			ExpectedRefreshSeconds: int(expectedRefresh.Int32),
			Measurements:           json.RawMessage(measurements.String), Metadata: json.RawMessage(metadata.String),
		}
	}
	return item, orderedAt.UTC(), nil
}

func (service *ReadService) Integrations(ctx context.Context, filter IntegrationFilter) (IntegrationList, error) {
	filter.Limit = normalizedLimit(filter.Limit)
	fingerprint := integrationFingerprint(filter)
	cursor, err := decodeCursor(filter.Cursor, "integrations", fingerprint)
	if err != nil {
		return IntegrationList{}, &Error{Code: "invalid_cursor"}
	}
	snapshot := cursor.Snapshot
	if snapshot.IsZero() {
		if snapshot, err = service.snapshot(ctx); err != nil {
			return IntegrationList{}, err
		}
	}
	enabledMode := -1
	if filter.Enabled != nil {
		enabledMode = 0
		if *filter.Enabled {
			enabledMode = 1
		}
	}
	rows, err := service.pool.Query(ctx, integrationSelect+`
		WHERE i.updated_at <= $1
		  AND ($3 = '' OR (i.updated_at, i.id) < ($2, NULLIF($3, '')::uuid))
		  AND (COALESCE(cardinality($4::text[]), 0) = 0 OR i.adapter_id = ANY($4::text[]))
		  AND (COALESCE(cardinality($5::text[]), 0) = 0 OR CASE WHEN NOT i.enabled THEN 'disabled' ELSE COALESCE(ai.state, 'not_started') END = ANY($5::text[]))
		  AND ($6 = -1 OR i.enabled = ($6 = 1))
		ORDER BY i.updated_at DESC, i.id DESC LIMIT $7
	`, snapshot, nullableCursorTime(cursor), cursor.ID, filter.AdapterIDs,
		filter.RuntimeStates, enabledMode, filter.Limit+1)
	if err != nil {
		return IntegrationList{}, fmt.Errorf("list integrations: %w", err)
	}
	defer rows.Close()
	items := make([]IntegrationView, 0, filter.Limit+1)
	for rows.Next() {
		item, err := scanIntegration(rows)
		if err != nil {
			return IntegrationList{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return IntegrationList{}, fmt.Errorf("read integrations: %w", err)
	}
	result := IntegrationList{Items: items}
	if len(result.Items) > filter.Limit {
		result.Items = result.Items[:filter.Limit]
		last := result.Items[len(result.Items)-1]
		result.NextCursor, err = encodeCursor(pageCursor{
			Kind: "integrations", Fingerprint: fingerprint, Snapshot: snapshot,
			OrderedAt: last.UpdatedAt, ID: last.ID,
		})
		if err != nil {
			return IntegrationList{}, fmt.Errorf("encode integration cursor: %w", err)
		}
	}
	return result, nil
}

func (service *ReadService) Integration(ctx context.Context, id string) (IntegrationView, error) {
	item, err := scanIntegration(service.pool.QueryRow(ctx, integrationSelect+" WHERE i.id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return IntegrationView{}, ErrNotFound
	}
	return item, err
}

const integrationSelect = `
	SELECT
		i.id::text, i.adapter_id, i.display_name, i.enabled, i.interval_seconds,
		ARRAY(SELECT key FROM jsonb_object_keys(i.config_nonsecret) key ORDER BY key),
		ARRAY(SELECT key FROM jsonb_object_keys(i.secret_references) key ORDER BY key),
		CASE WHEN NOT i.enabled THEN 'disabled' ELSE COALESCE(ai.state, 'not_started') END,
		COALESCE(rc.resource_count, 0), COALESCE(rc.stale_count, 0), COALESCE(rc.unknown_count, 0),
		ai.integration_id::text, ai.adapter_version, ai.protocol_version, ai.state,
		ai.last_started_at, ai.last_healthy_at, ai.last_stopped_at, ai.last_error_at,
		ai.last_error_code, ai.consecutive_failures, ai.next_restart_at, ai.updated_at,
		lc.id::text, lc.started_at, lc.completed_at, lc.duration_ms, lc.result,
		lc.error_code, lc.resource_count, lc.observation_count,
		lc.observations_inserted, lc.duplicate_observations,
		i.created_at, i.updated_at
	FROM integrations i
	LEFT JOIN adapter_instances ai ON ai.integration_id = i.id
	LEFT JOIN LATERAL (
		SELECT count(*) AS resource_count,
			count(*) FILTER (WHERE ch.state = 'stale') AS stale_count,
			count(*) FILTER (WHERE ch.state = 'unknown' OR ch.state IS NULL) AS unknown_count
		FROM resources r LEFT JOIN current_health ch ON ch.resource_id = r.id
		WHERE r.integration_id = i.id
	) rc ON true
	LEFT JOIN LATERAL (
		SELECT run.* FROM integration_collection_runs run
		WHERE run.integration_id = i.id
		ORDER BY run.completed_at DESC, run.id DESC LIMIT 1
	) lc ON true
`

func scanIntegration(row scanner) (IntegrationView, error) {
	var item IntegrationView
	var instanceID, instanceAdapter, instanceProtocol, instanceState pgtype.Text
	var lastStarted, lastHealthy, lastStopped, lastError, nextRestart, instanceUpdated pgtype.Timestamptz
	var lastErrorCode pgtype.Text
	var failures pgtype.Int4
	var collectionID, collectionResult, collectionError pgtype.Text
	var collectionStarted, collectionCompleted pgtype.Timestamptz
	var duration pgtype.Int8
	var resourceCount, observationCount, inserted, duplicates pgtype.Int4
	if err := row.Scan(
		&item.ID, &item.AdapterID, &item.DisplayName, &item.Enabled, &item.IntervalSeconds,
		&item.ConfigKeys, &item.SecretReferenceKeys, &item.RuntimeState,
		&item.ResourceCount, &item.StaleCount, &item.UnknownCount,
		&instanceID, &instanceAdapter, &instanceProtocol, &instanceState,
		&lastStarted, &lastHealthy, &lastStopped, &lastError, &lastErrorCode,
		&failures, &nextRestart, &instanceUpdated,
		&collectionID, &collectionStarted, &collectionCompleted, &duration,
		&collectionResult, &collectionError, &resourceCount, &observationCount,
		&inserted, &duplicates, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return IntegrationView{}, err
	}
	if instanceID.Valid {
		item.Instance = &AdapterInstanceView{
			AdapterVersion: instanceAdapter.String, ProtocolVersion: instanceProtocol.String,
			State: instanceState.String, LastStartedAt: nullableTimestamp(lastStarted),
			LastHealthyAt: nullableTimestamp(lastHealthy), LastStoppedAt: nullableTimestamp(lastStopped),
			LastErrorAt: nullableTimestamp(lastError), LastErrorCode: lastErrorCode.String,
			ConsecutiveFailures: int(failures.Int32), NextRestartAt: nullableTimestamp(nextRestart),
			UpdatedAt: instanceUpdated.Time.UTC(),
		}
	}
	if collectionID.Valid {
		item.LastCollection = &CollectionRunView{
			StartedAt: collectionStarted.Time.UTC(), CompletedAt: collectionCompleted.Time.UTC(),
			DurationMS: duration.Int64, Result: collectionResult.String,
			ErrorCode: collectionError.String, ResourceCount: int(resourceCount.Int32),
			ObservationCount: int(observationCount.Int32), ObservationsInserted: int(inserted.Int32),
			DuplicateObservations: int(duplicates.Int32),
		}
	}
	item.CreatedAt = item.CreatedAt.UTC()
	item.UpdatedAt = item.UpdatedAt.UTC()
	if item.ConfigKeys == nil {
		item.ConfigKeys = []string{}
	}
	if item.SecretReferenceKeys == nil {
		item.SecretReferenceKeys = []string{}
	}
	return item, nil
}

func (service *ReadService) Audit(ctx context.Context, filter AuditFilter) (AuditList, error) {
	filter.Limit = normalizedLimit(filter.Limit)
	fingerprint := auditFingerprint(filter)
	cursor, err := decodeCursor(filter.Cursor, "audit", fingerprint)
	if err != nil {
		return AuditList{}, &Error{Code: "invalid_cursor"}
	}
	if !cursor.Snapshot.IsZero() {
		if cursor.RangeFrom.IsZero() || cursor.RangeTo.IsZero() ||
			filter.FromExplicit && !filter.From.Equal(cursor.RangeFrom) ||
			filter.ToExplicit && !filter.To.Equal(cursor.RangeTo) {
			return AuditList{}, &Error{Code: "invalid_cursor"}
		}
		filter.From, filter.To = cursor.RangeFrom, cursor.RangeTo
	}
	snapshot := cursor.Snapshot
	if snapshot.IsZero() {
		snapshot = filter.To.UTC()
	}
	rows, err := service.pool.Query(ctx, `
		SELECT a.id::text, a.actor_user_id::text, u.username, a.action, a.target_type,
			a.target_id, a.result, host(a.source_address), a.correlation_id,
			a.before_summary::text, a.after_summary::text, a.occurred_at
		FROM audit_events a LEFT JOIN users u ON u.id = a.actor_user_id
		WHERE a.occurred_at >= $1 AND a.occurred_at <= $2 AND a.occurred_at <= $3
		  AND ($5 = '' OR (a.occurred_at, a.id) < ($4, NULLIF($5, '')::uuid))
		  AND (COALESCE(cardinality($6::text[]), 0) = 0 OR a.action = ANY($6::text[]))
		  AND (COALESCE(cardinality($7::text[]), 0) = 0 OR a.result = ANY($7::text[]))
			  AND (COALESCE(cardinality($8::text[]), 0) = 0 OR a.target_type = ANY($8::text[]))
			  AND ($9 = '' OR a.actor_user_id = NULLIF($9, '')::uuid)
			  AND ($10 = '' OR a.correlation_id = $10)
			ORDER BY a.occurred_at DESC, a.id DESC LIMIT $11
		`, filter.From.UTC(), filter.To.UTC(), snapshot, nullableCursorTime(cursor), cursor.ID,
		filter.Actions, filter.Results, filter.TargetTypes, filter.ActorUserID, filter.CorrelationID, filter.Limit+1)
	if err != nil {
		return AuditList{}, fmt.Errorf("list audit events: %w", err)
	}
	defer rows.Close()
	items := make([]AuditEventView, 0, filter.Limit+1)
	for rows.Next() {
		var item AuditEventView
		var actorID, actorUsername, targetID, source, before, after pgtype.Text
		if err := rows.Scan(
			&item.ID, &actorID, &actorUsername, &item.Action, &item.TargetType,
			&targetID, &item.Result, &source, &item.CorrelationID,
			&before, &after, &item.OccurredAt,
		); err != nil {
			return AuditList{}, fmt.Errorf("scan audit event: %w", err)
		}
		item.ActorUserID, item.ActorUsername = actorID.String, actorUsername.String
		item.TargetID, item.SourceAddress = targetID.String, source.String
		if before.Valid {
			item.BeforeSummary = json.RawMessage(before.String)
		}
		if after.Valid {
			item.AfterSummary = json.RawMessage(after.String)
		}
		item.OccurredAt = item.OccurredAt.UTC()
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return AuditList{}, fmt.Errorf("read audit events: %w", err)
	}
	result := AuditList{Items: items, From: filter.From.UTC(), To: filter.To.UTC()}
	if len(result.Items) > filter.Limit {
		result.Items = result.Items[:filter.Limit]
		last := result.Items[len(result.Items)-1]
		result.NextCursor, err = encodeCursor(pageCursor{
			Kind: "audit", Fingerprint: fingerprint, Snapshot: snapshot,
			OrderedAt: last.OccurredAt, ID: last.ID, RangeFrom: filter.From, RangeTo: filter.To,
		})
		if err != nil {
			return AuditList{}, fmt.Errorf("encode audit cursor: %w", err)
		}
	}
	return result, nil
}

func (service *ReadService) RecordAuditRead(
	ctx context.Context,
	actorUserID, sourceAddress, correlationID string,
	filter AuditFilter,
) error {
	return audit.Append(ctx, service.pool, audit.Event{
		ActorUserID: actorUserID, Action: "audit.read", TargetType: "audit",
		Result: "succeeded", SourceAddress: sourceAddress, CorrelationID: correlationID,
		AfterSummary: map[string]any{
			"from":           filter.From.UTC().Format(time.RFC3339Nano),
			"to":             filter.To.UTC().Format(time.RFC3339Nano),
			"action_filters": len(filter.Actions), "result_filters": len(filter.Results),
			"target_type_filters": len(filter.TargetTypes), "actor_filter": filter.ActorUserID != "",
			"correlation_filter": filter.CorrelationID != "",
			"limit":              filter.Limit,
		},
		OccurredAt: time.Now().UTC(),
	})
}

func (service *ReadService) snapshot(ctx context.Context) (time.Time, error) {
	var result time.Time
	if err := service.pool.QueryRow(ctx, "SELECT now()").Scan(&result); err != nil {
		return time.Time{}, fmt.Errorf("read pagination snapshot: %w", err)
	}
	return result.UTC(), nil
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

func normalizeCurrent(current *CurrentHealthView) {
	current.UpdatedAt = current.UpdatedAt.UTC()
	for _, value := range []*time.Time{current.ObservedAt, current.LastSuccessAt, current.StaleAt, current.UnknownAt} {
		if value != nil {
			*value = value.UTC()
		}
	}
}

func nullableCursorTime(cursor pageCursor) any {
	if cursor.OrderedAt.IsZero() {
		return nil
	}
	return cursor.OrderedAt.UTC()
}

func resourceFingerprint(filter ResourceFilter) string {
	states := make([]string, len(filter.States))
	for index, state := range filter.States {
		states[index] = string(state)
	}
	stale := ""
	if filter.Stale != nil {
		stale = strconv.FormatBool(*filter.Stale)
	}
	return filterFingerprint(joinSorted(states), joinSorted(filter.Kinds), joinSorted(filter.IntegrationIDs), stale)
}

func integrationFingerprint(filter IntegrationFilter) string {
	enabled := ""
	if filter.Enabled != nil {
		enabled = strconv.FormatBool(*filter.Enabled)
	}
	return filterFingerprint(joinSorted(filter.AdapterIDs), joinSorted(filter.RuntimeStates), enabled)
}

func auditFingerprint(filter AuditFilter) string {
	return filterFingerprint(
		joinSorted(filter.Actions), joinSorted(filter.Results), joinSorted(filter.TargetTypes),
		filter.ActorUserID, filter.CorrelationID,
	)
}

func joinSorted(values []string) string {
	copyValues := append([]string(nil), values...)
	sort.Strings(copyValues)
	return strings.Join(copyValues, ",")
}
