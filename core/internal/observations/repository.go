package observations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/health"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type repository struct{ tx pgx.Tx }

func (repo repository) integrationExists(ctx context.Context, integrationID string) (bool, error) {
	var exists bool
	if err := repo.tx.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM integrations WHERE id = $1)", integrationID,
	).Scan(&exists); err != nil {
		return false, fmt.Errorf("check integration: %w", err)
	}
	return exists, nil
}

func (repo repository) upsertResource(ctx context.Context, integrationID string, input ResourceInput) (string, error) {
	attributes, err := json.Marshal(input.Attributes)
	if err != nil {
		return "", fmt.Errorf("encode resource attributes: %w", err)
	}
	var sourceURL any
	if input.SourceURL != "" {
		sourceURL = input.SourceURL
	}
	var id string
	err = repo.tx.QueryRow(ctx, `
		INSERT INTO resources (
			id, integration_id, external_id, kind, display_name, attributes,
			source_url, first_seen_at, last_seen_at
		) VALUES (
			COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5,
			$6::jsonb, $7, $8, $8
		)
		ON CONFLICT (integration_id, external_id) DO UPDATE SET
			first_seen_at = LEAST(resources.first_seen_at, EXCLUDED.first_seen_at),
			last_seen_at = GREATEST(resources.last_seen_at, EXCLUDED.last_seen_at),
			kind = CASE WHEN EXCLUDED.last_seen_at >= resources.last_seen_at
				THEN EXCLUDED.kind ELSE resources.kind END,
			display_name = CASE WHEN EXCLUDED.last_seen_at >= resources.last_seen_at
				THEN EXCLUDED.display_name ELSE resources.display_name END,
			attributes = CASE WHEN EXCLUDED.last_seen_at >= resources.last_seen_at
				THEN EXCLUDED.attributes ELSE resources.attributes END,
			source_url = CASE WHEN EXCLUDED.last_seen_at >= resources.last_seen_at
				THEN EXCLUDED.source_url ELSE resources.source_url END
		RETURNING id::text
	`, input.ID, integrationID, input.ExternalID, input.Kind, input.DisplayName,
		string(attributes), sourceURL, postgresTime(input.ObservedAt)).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("upsert resource: %w", err)
	}
	return id, nil
}

func (repo repository) resolveResource(ctx context.Context, integrationID, externalID string) (string, error) {
	var id string
	if err := repo.tx.QueryRow(ctx, `
		SELECT id::text FROM resources
		WHERE integration_id = $1 AND external_id = $2
	`, integrationID, externalID).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", &ConflictError{Record: "observation", Index: -1, Code: "resource_not_found"}
		}
		return "", fmt.Errorf("resolve observation resource: %w", err)
	}
	return id, nil
}

func (repo repository) lockResource(ctx context.Context, integrationID, resourceID string) error {
	var locked string
	if err := repo.tx.QueryRow(ctx, `
		SELECT id::text FROM resources
		WHERE integration_id = $1 AND id = $2
		FOR UPDATE
	`, integrationID, resourceID).Scan(&locked); err != nil {
		return fmt.Errorf("lock observation resource: %w", err)
	}
	return nil
}

func (repo repository) insertObservation(
	ctx context.Context,
	integrationID, resourceID string,
	input ObservationInput,
	receivedAt time.Time,
) (health.Observation, bool, error) {
	measurements, err := json.Marshal(input.Measurements)
	if err != nil {
		return health.Observation{}, false, fmt.Errorf("encode observation measurements: %w", err)
	}
	metadata, err := json.Marshal(input.Metadata)
	if err != nil {
		return health.Observation{}, false, fmt.Errorf("encode observation metadata: %w", err)
	}
	observedAt := postgresTime(input.ObservedAt)
	receivedAt = postgresTime(receivedAt)
	refresh := time.Duration(input.ExpectedRefreshSeconds) * time.Second
	staleAt, _ := health.TransitionTimes(health.Observation{
		ObservedAt: observedAt, ExpectedRefresh: refresh,
	})

	var id string
	err = repo.tx.QueryRow(ctx, `
		INSERT INTO observations (
			id, resource_id, integration_id, check_type, observed_state, summary,
			observed_at, received_at, expires_at, expected_refresh_seconds,
			measurements, metadata
		) VALUES (
			COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11::jsonb, $12::jsonb
		)
		ON CONFLICT DO NOTHING
		RETURNING id::text
	`, input.ID, resourceID, integrationID, input.CheckType, input.State,
		input.Summary, observedAt, receivedAt, staleAt,
		input.ExpectedRefreshSeconds, string(measurements), string(metadata)).Scan(&id)
	if err == nil {
		return health.Observation{
			ID: id, ResourceID: resourceID, State: input.State, Summary: input.Summary,
			ObservedAt: observedAt, ReceivedAt: receivedAt, ExpectedRefresh: refresh,
		}, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return health.Observation{}, false, fmt.Errorf("insert observation: %w", err)
	}

	existing, existingMeasurements, existingMetadata, err := repo.findConflictingObservation(
		ctx, integrationID, resourceID, input,
	)
	if err != nil {
		return health.Observation{}, false, err
	}
	if !sameObservationContent(existing, input, existingMeasurements, existingMetadata, measurements, metadata) {
		return health.Observation{}, false, &ConflictError{Record: "observation", Index: -1, Code: "idempotency_conflict"}
	}
	return existing, false, nil
}

func (repo repository) findConflictingObservation(
	ctx context.Context,
	integrationID, resourceID string,
	input ObservationInput,
) (health.Observation, []byte, []byte, error) {
	var result health.Observation
	var state string
	var refreshSeconds int
	var measurements, metadata []byte
	err := repo.tx.QueryRow(ctx, `
		SELECT id::text, resource_id::text, observed_state, summary, observed_at,
			received_at, expected_refresh_seconds, measurements, metadata
		FROM observations
		WHERE integration_id = $1 AND (
			(id = NULLIF($2, '')::uuid) OR
			(resource_id = $3 AND check_type = $4 AND observed_at = $5)
		)
		ORDER BY CASE WHEN id = NULLIF($2, '')::uuid THEN 0 ELSE 1 END
		LIMIT 1
	`, integrationID, input.ID, resourceID, input.CheckType,
		postgresTime(input.ObservedAt)).Scan(
		&result.ID, &result.ResourceID, &state, &result.Summary, &result.ObservedAt,
		&result.ReceivedAt, &refreshSeconds, &measurements, &metadata,
	)
	if err != nil {
		return health.Observation{}, nil, nil, fmt.Errorf("load conflicting observation: %w", err)
	}
	result.State = health.State(state)
	result.ExpectedRefresh = time.Duration(refreshSeconds) * time.Second
	return result, measurements, metadata, nil
}

func (repo repository) latestObservation(ctx context.Context, resourceID string) (health.Observation, bool, error) {
	var result health.Observation
	var state string
	var refreshSeconds int
	err := repo.tx.QueryRow(ctx, `
		SELECT id::text, resource_id::text, observed_state, summary, observed_at,
			received_at, expected_refresh_seconds
		FROM observations
		WHERE resource_id = $1
		ORDER BY observed_at DESC, received_at DESC, id DESC
		LIMIT 1
	`, resourceID).Scan(
		&result.ID, &result.ResourceID, &state, &result.Summary, &result.ObservedAt,
		&result.ReceivedAt, &refreshSeconds,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return health.Observation{}, false, nil
	}
	if err != nil {
		return health.Observation{}, false, fmt.Errorf("load latest observation: %w", err)
	}
	result.State = health.State(state)
	result.ExpectedRefresh = time.Duration(refreshSeconds) * time.Second
	return result, true, nil
}

func (repo repository) loadCurrent(ctx context.Context, resourceID string) (health.Current, bool, error) {
	var result health.Current
	var state, observationID string
	var observedAt, lastSuccessAt, staleAt, unknownAt pgtype.Timestamptz
	err := repo.tx.QueryRow(ctx, `
		SELECT resource_id::text, state, reason, COALESCE(observation_id::text, ''),
			observed_at, last_success_at, stale_at, unknown_at, updated_at
		FROM current_health WHERE resource_id = $1
	`, resourceID).Scan(
		&result.ResourceID, &state, &result.Reason, &observationID,
		&observedAt, &lastSuccessAt, &staleAt, &unknownAt, &result.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return health.Current{}, false, nil
	}
	if err != nil {
		return health.Current{}, false, fmt.Errorf("load current health: %w", err)
	}
	result.State = health.State(state)
	if observationID != "" {
		result.ObservationID = stringPointer(observationID)
	}
	result.ObservedAt = timestampPointer(observedAt)
	result.LastSuccessAt = timestampPointer(lastSuccessAt)
	result.StaleAt = timestampPointer(staleAt)
	result.UnknownAt = timestampPointer(unknownAt)
	return result, true, nil
}

func (repo repository) saveCurrent(ctx context.Context, current health.Current) error {
	observationID := ""
	if current.ObservationID != nil {
		observationID = *current.ObservationID
	}
	_, err := repo.tx.Exec(ctx, `
		INSERT INTO current_health (
			resource_id, state, reason, observation_id, observed_at, last_success_at,
			stale_at, unknown_at, updated_at
		) VALUES ($1, $2, $3, NULLIF($4, '')::uuid, $5, $6, $7, $8, $9)
		ON CONFLICT (resource_id) DO UPDATE SET
			state = EXCLUDED.state,
			reason = EXCLUDED.reason,
			observation_id = EXCLUDED.observation_id,
			observed_at = EXCLUDED.observed_at,
			last_success_at = EXCLUDED.last_success_at,
			stale_at = EXCLUDED.stale_at,
			unknown_at = EXCLUDED.unknown_at,
			updated_at = EXCLUDED.updated_at
	`, current.ResourceID, current.State, current.Reason, observationID,
		current.ObservedAt, current.LastSuccessAt, current.StaleAt, current.UnknownAt,
		postgresTime(current.UpdatedAt))
	if err != nil {
		return fmt.Errorf("save current health: %w", err)
	}
	return nil
}

func sameObservationContent(
	existing health.Observation,
	input ObservationInput,
	existingMeasurements, existingMetadata, inputMeasurements, inputMetadata []byte,
) bool {
	if input.ID != "" && existing.ID != input.ID {
		return false
	}
	if existing.ResourceID == "" || existing.State != input.State ||
		existing.Summary != input.Summary ||
		!existing.ObservedAt.Equal(postgresTime(input.ObservedAt)) ||
		existing.ExpectedRefresh != time.Duration(input.ExpectedRefreshSeconds)*time.Second {
		return false
	}
	return equivalentJSON(existingMeasurements, inputMeasurements) &&
		equivalentJSON(existingMetadata, inputMetadata)
}

func equivalentJSON(left, right []byte) bool {
	var leftValue, rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	return reflect.DeepEqual(leftValue, rightValue)
}

func currentEquivalent(left, right health.Current) bool {
	return left.ResourceID == right.ResourceID && left.State == right.State &&
		left.Reason == right.Reason && equalStringPointers(left.ObservationID, right.ObservationID) &&
		equalTimePointers(left.ObservedAt, right.ObservedAt) &&
		equalTimePointers(left.LastSuccessAt, right.LastSuccessAt) &&
		equalTimePointers(left.StaleAt, right.StaleAt) &&
		equalTimePointers(left.UnknownAt, right.UnknownAt)
}

func equalStringPointers(left, right *string) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func equalTimePointers(left, right *time.Time) bool {
	return left == nil && right == nil || left != nil && right != nil && left.Equal(*right)
}

func postgresTime(value time.Time) time.Time { return value.UTC().Truncate(time.Microsecond) }

func stringPointer(value string) *string { return &value }

func timestampPointer(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}
