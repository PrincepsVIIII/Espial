package notifications

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/PrincepsVIIII/Espial/core/internal/adminops"
	"github.com/PrincepsVIIII/Espial/core/internal/audit"
	"github.com/PrincepsVIIII/Espial/core/internal/events"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	destinationUUIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-8][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
	hostnameLabelPattern   = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	pathPrefixPattern      = regexp.MustCompile(`^/[A-Za-z0-9._~/-]{0,255}$`)
	secretReferencePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
)

type Service struct {
	pool      *pgxpool.Pool
	hub       *events.Hub
	validator DestinationValidator
	secrets   SecretResolver
	now       func() time.Time
}

func NewService(pool *pgxpool.Pool, hub *events.Hub, validator DestinationValidator, secrets SecretResolver, now func() time.Time) *Service {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{pool: pool, hub: hub, validator: validator, secrets: secrets, now: now}
}

func (service *Service) Destinations(ctx context.Context, filter DestinationFilter) (DestinationList, error) {
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.Limit > 200 {
		filter.Limit = 200
	}
	cursor, err := decodePageCursor(filter.Cursor, "notification_destinations", "all")
	if err != nil {
		return DestinationList{}, err
	}
	rows, err := service.pool.Query(ctx, destinationSelect+`
		WHERE ($1::timestamptz IS NULL OR (updated_at,id) < ($1,$2::uuid))
		ORDER BY updated_at DESC, id DESC LIMIT $3`, nullableCursorTime(cursor), nullableCursorID(cursor), filter.Limit+1)
	if err != nil {
		return DestinationList{}, fmt.Errorf("list notification destinations: %w", err)
	}
	defer rows.Close()
	result := DestinationList{Items: []Destination{}}
	for rows.Next() {
		item, err := scanDestination(rows)
		if err != nil {
			return DestinationList{}, err
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return DestinationList{}, err
	}
	if len(result.Items) > filter.Limit {
		result.Items = result.Items[:filter.Limit]
		last := result.Items[len(result.Items)-1]
		result.NextCursor, err = encodePageCursor(pageCursor{Kind: "notification_destinations", Fingerprint: "all", Timestamp: last.UpdatedAt, ID: last.ID})
		if err != nil {
			return DestinationList{}, err
		}
	}
	return result, nil
}

func (service *Service) Destination(ctx context.Context, id string) (Destination, error) {
	if !destinationUUIDPattern.MatchString(id) {
		return Destination{}, ErrNotFound
	}
	item, err := scanDestination(service.pool.QueryRow(ctx, destinationSelect+` WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Destination{}, ErrNotFound
	}
	return item, err
}

const destinationSelect = `
	SELECT id::text, display_name, destination_type, enabled, version, created_at, updated_at
	FROM notification_destinations
`

type scanner interface{ Scan(...any) error }

func scanDestination(row scanner) (Destination, error) {
	var item Destination
	if err := row.Scan(&item.ID, &item.DisplayName, &item.DestinationType, &item.Enabled,
		&item.Version, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return Destination{}, err
	}
	item.CreatedAt, item.UpdatedAt = item.CreatedAt.UTC(), item.UpdatedAt.UTC()
	return item, nil
}

func (service *Service) CreateDestination(ctx context.Context, definition DestinationDefinition, metadata MutationMetadata) (adminops.Receipt, error) {
	return service.mutateDestination(ctx, "create", "", definition, metadata)
}

func (service *Service) ReplaceDestination(ctx context.Context, id string, definition DestinationDefinition, metadata MutationMetadata) (adminops.Receipt, error) {
	return service.mutateDestination(ctx, "replace", id, definition, metadata)
}

func (service *Service) mutateDestination(ctx context.Context, operation, id string, definition DestinationDefinition, metadata MutationMetadata) (adminops.Receipt, error) {
	definition = normalizeDefinition(definition)
	if operation != "create" && (!destinationUUIDPattern.MatchString(id) || metadata.ExpectedVersion < 1) {
		return adminops.Receipt{}, ErrInvalid
	}
	if err := service.validateDefinition(ctx, definition); err != nil {
		return adminops.Receipt{}, err
	}
	hash, err := adminops.Hash(struct {
		Operation string
		ID        string
		Version   int64
		Value     DestinationDefinition
	}{operation, id, metadata.ExpectedVersion, definition})
	if err != nil {
		return adminops.Receipt{}, err
	}
	tx, err := service.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return adminops.Receipt{}, err
	}
	defer tx.Rollback(ctx)
	if replay, found, err := adminops.Replay(ctx, tx, metadata.ActorUserID, "notification_destination", operation, metadata.IdempotencyKey, hash); err != nil {
		if errors.Is(err, adminops.ErrIdempotencyConflict) {
			return adminops.Receipt{}, ErrIdempotencyConflict
		}
		return adminops.Receipt{}, err
	} else if found {
		return replay, tx.Commit(ctx)
	}
	now := service.now().UTC().Truncate(time.Microsecond)
	var before map[string]any
	var version int64
	if operation == "create" {
		err = tx.QueryRow(ctx, `
			INSERT INTO notification_destinations (
				display_name, destination_type, enabled, endpoint_host,
				endpoint_port, endpoint_path_prefix, secret_reference,
				created_at, updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8)
			RETURNING id::text, version
		`, definition.DisplayName, definition.DestinationType, definition.Enabled,
			definition.EndpointHost, definition.EndpointPort, definition.PathPrefix,
			definition.SecretReference, now).Scan(&id, &version)
	} else {
		current, readErr := readInternalDestination(ctx, tx, id, true)
		if errors.Is(readErr, pgx.ErrNoRows) {
			return adminops.Receipt{}, ErrNotFound
		}
		if readErr != nil {
			return adminops.Receipt{}, readErr
		}
		if current.Version != metadata.ExpectedVersion {
			return adminops.Receipt{}, ErrConflict
		}
		before = destinationAuditSummary(current.Destination)
		err = tx.QueryRow(ctx, `
			UPDATE notification_destinations SET
				display_name=$2, destination_type=$3, enabled=$4,
				endpoint_host=$5, endpoint_port=$6, endpoint_path_prefix=$7,
				secret_reference=$8, version=version+1, updated_at=$9
			WHERE id=$1 RETURNING version
		`, id, definition.DisplayName, definition.DestinationType, definition.Enabled,
			definition.EndpointHost, definition.EndpointPort, definition.PathPrefix,
			definition.SecretReference, now).Scan(&version)
	}
	if err != nil {
		return adminops.Receipt{}, fmt.Errorf("save notification destination: %w", err)
	}
	receipt := adminops.Receipt{ID: id, Version: version, RequestID: metadata.CorrelationID}
	if err := audit.Append(ctx, tx, audit.Event{
		ActorUserID: metadata.ActorUserID, Action: "notification_destination." + operation,
		TargetType: "notification_destination", TargetID: id, Result: "succeeded",
		SourceAddress: metadata.SourceAddress, CorrelationID: metadata.CorrelationID,
		BeforeSummary: before, AfterSummary: destinationAuditSummary(Destination{
			ID: id, DisplayName: definition.DisplayName, DestinationType: definition.DestinationType,
			Enabled: definition.Enabled, Version: version, UpdatedAt: now,
		}), OccurredAt: now,
	}); err != nil {
		return adminops.Receipt{}, err
	}
	if err := adminops.Save(ctx, tx, metadata.ActorUserID, "notification_destination", operation,
		metadata.IdempotencyKey, hash, receipt); err != nil {
		return adminops.Receipt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return adminops.Receipt{}, err
	}
	service.publish(events.NotificationDestinationChanged, id, "configured", now)
	return receipt, nil
}

func (service *Service) TestDestination(ctx context.Context, id string, metadata MutationMetadata) (adminops.Receipt, error) {
	if !destinationUUIDPattern.MatchString(id) || metadata.ExpectedVersion < 1 {
		return adminops.Receipt{}, ErrInvalid
	}
	hash, err := adminops.Hash(struct {
		ID      string
		Version int64
	}{id, metadata.ExpectedVersion})
	if err != nil {
		return adminops.Receipt{}, err
	}
	tx, err := service.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return adminops.Receipt{}, err
	}
	defer tx.Rollback(ctx)
	if replay, found, err := adminops.Replay(ctx, tx, metadata.ActorUserID, "notification_destination", "test", metadata.IdempotencyKey, hash); err != nil {
		if errors.Is(err, adminops.ErrIdempotencyConflict) {
			return adminops.Receipt{}, ErrIdempotencyConflict
		}
		return adminops.Receipt{}, err
	} else if found {
		return replay, tx.Commit(ctx)
	}
	current, err := readInternalDestination(ctx, tx, id, true)
	if errors.Is(err, pgx.ErrNoRows) {
		return adminops.Receipt{}, ErrNotFound
	}
	if err != nil {
		return adminops.Receipt{}, err
	}
	if current.Version != metadata.ExpectedVersion {
		return adminops.Receipt{}, ErrConflict
	}
	if err := service.validateDefinition(ctx, current.definition()); err != nil {
		return adminops.Receipt{}, err
	}
	now := service.now().UTC().Truncate(time.Microsecond)
	var intentID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO notification_intents (
			destination_id, event_kind, is_test, title, summary,
			event_occurred_at, state, available_at, created_at, updated_at
		) VALUES ($1, 'test', true, 'Espial notification test',
			'An administrator requested a destination test.', $2, 'queued', $2, $2, $2)
		RETURNING id::text
	`, id, now).Scan(&intentID); err != nil {
		return adminops.Receipt{}, fmt.Errorf("create test notification: %w", err)
	}
	receipt := adminops.Receipt{ID: id, Version: current.Version, RequestID: metadata.CorrelationID}
	if err := audit.Append(ctx, tx, audit.Event{
		ActorUserID: metadata.ActorUserID, Action: "notification_destination.test",
		TargetType: "notification_destination", TargetID: id, Result: "succeeded",
		SourceAddress: metadata.SourceAddress, CorrelationID: metadata.CorrelationID,
		AfterSummary: map[string]any{"delivery_id": intentID, "test": true}, OccurredAt: now,
	}); err != nil {
		return adminops.Receipt{}, err
	}
	if err := adminops.Save(ctx, tx, metadata.ActorUserID, "notification_destination", "test",
		metadata.IdempotencyKey, hash, receipt); err != nil {
		return adminops.Receipt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return adminops.Receipt{}, err
	}
	service.publish(events.NotificationDeliveryChanged, intentID, "queued", now)
	return receipt, nil
}

func (service *Service) Deliveries(ctx context.Context, filter DeliveryFilter) (DeliveryList, error) {
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.Limit > 200 {
		filter.Limit = 200
	}
	states := make([]string, len(filter.States))
	for index, state := range filter.States {
		states[index] = string(state)
	}
	fingerprint := deliveryFingerprint(filter)
	cursor, err := decodePageCursor(filter.Cursor, "notification_deliveries", fingerprint)
	if err != nil {
		return DeliveryList{}, err
	}
	rows, err := service.pool.Query(ctx, deliverySelect+`
		WHERE ($1='' OR intent.incident_id=NULLIF($1,'')::uuid)
		  AND ($2='' OR intent.destination_id=NULLIF($2,'')::uuid)
		  AND (COALESCE(cardinality($3::text[]),0)=0 OR intent.state=ANY($3::text[]))
		  AND ($4::timestamptz IS NULL OR (intent.created_at,intent.id) < ($4,$5::uuid))
		ORDER BY intent.created_at DESC, intent.id DESC LIMIT $6
	`, filter.IncidentID, filter.DestinationID, states, nullableCursorTime(cursor), nullableCursorID(cursor), filter.Limit+1)
	if err != nil {
		return DeliveryList{}, fmt.Errorf("list notification deliveries: %w", err)
	}
	defer rows.Close()
	result := DeliveryList{Items: []Delivery{}}
	for rows.Next() {
		item, err := scanDelivery(rows)
		if err != nil {
			return DeliveryList{}, err
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return DeliveryList{}, err
	}
	if len(result.Items) > filter.Limit {
		result.Items = result.Items[:filter.Limit]
		last := result.Items[len(result.Items)-1]
		result.NextCursor, err = encodePageCursor(pageCursor{Kind: "notification_deliveries", Fingerprint: fingerprint, Timestamp: last.CreatedAt, ID: last.ID})
		if err != nil {
			return DeliveryList{}, err
		}
	}
	return result, nil
}

func nullableCursorTime(cursor pageCursor) any {
	if cursor.Timestamp.IsZero() {
		return nil
	}
	return cursor.Timestamp
}

func nullableCursorID(cursor pageCursor) any {
	if cursor.ID == "" {
		return nil
	}
	return cursor.ID
}

const deliverySelect = `
	SELECT intent.id::text, COALESCE(intent.incident_id::text,''),
		COALESCE(intent.incident_event_id::text,''), intent.destination_id::text,
		destination.display_name, destination.destination_type, intent.event_kind,
		intent.is_test, intent.state, intent.attempt_count, intent.event_occurred_at,
		last_attempt.completed_at, intent.available_at, intent.delivered_at,
		intent.terminal_at, COALESCE(intent.suppressed_silence_id::text,''),
		COALESCE(intent.last_error_code,''), intent.created_at, intent.updated_at
	FROM notification_intents intent
	JOIN notification_destinations destination ON destination.id=intent.destination_id
	LEFT JOIN LATERAL (
		SELECT completed_at FROM notification_attempts
		WHERE intent_id=intent.id ORDER BY attempt_number DESC LIMIT 1
	) last_attempt ON true
`

func scanDelivery(row scanner) (Delivery, error) {
	var item Delivery
	var lastAttempt, delivered, terminal pgtype.Timestamptz
	if err := row.Scan(&item.ID, &item.IncidentID, &item.IncidentEventID,
		&item.DestinationID, &item.DestinationName, &item.DestinationType,
		&item.EventKind, &item.Test, &item.State, &item.AttemptCount,
		&item.EventOccurredAt, &lastAttempt, &item.AvailableAt, &delivered,
		&terminal, &item.SuppressedSilenceID, &item.LastErrorCode,
		&item.CreatedAt, &item.UpdatedAt); err != nil {
		return Delivery{}, fmt.Errorf("scan notification delivery: %w", err)
	}
	item.EventOccurredAt, item.AvailableAt = item.EventOccurredAt.UTC(), item.AvailableAt.UTC()
	item.CreatedAt, item.UpdatedAt = item.CreatedAt.UTC(), item.UpdatedAt.UTC()
	item.LastAttemptAt, item.DeliveredAt, item.TerminalAt = optionalTime(lastAttempt), optionalTime(delivered), optionalTime(terminal)
	return item, nil
}

func (service *Service) Metrics(ctx context.Context, now time.Time) (Metrics, error) {
	var result Metrics
	var oldest pgtype.Timestamptz
	err := service.pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE state='queued'),
			count(*) FILTER (WHERE state='attempting'),
			count(*) FILTER (WHERE state='retry_wait'),
			count(*) FILTER (WHERE state='delivered'),
			count(*) FILTER (WHERE state='failed'),
			count(*) FILTER (WHERE state='dead_letter'),
			count(*) FILTER (WHERE state='suppressed'),
			min(available_at) FILTER (WHERE state IN ('queued','retry_wait')),
			(SELECT count(*) FROM notification_attempts)
		FROM notification_intents
	`).Scan(&result.Queued, &result.Attempting, &result.RetryWaiting,
		&result.Delivered, &result.Failed, &result.DeadLetters, &result.Suppressed,
		&oldest, &result.AttemptTotal)
	if err != nil {
		return Metrics{}, fmt.Errorf("read notification metrics: %w", err)
	}
	if oldest.Valid && now.After(oldest.Time) {
		result.OldestDueAge = now.UTC().Sub(oldest.Time.UTC())
	}
	return result, nil
}

type internalDestination struct {
	Destination
	EndpointHost    string
	EndpointPort    int
	PathPrefix      string
	SecretReference string
}

func (item internalDestination) definition() DestinationDefinition {
	return DestinationDefinition{DisplayName: item.DisplayName, DestinationType: item.DestinationType,
		Enabled: item.Enabled, EndpointHost: item.EndpointHost, EndpointPort: item.EndpointPort,
		PathPrefix: item.PathPrefix, SecretReference: item.SecretReference}
}

func readInternalDestination(ctx context.Context, tx pgx.Tx, id string, lock bool) (internalDestination, error) {
	query := `SELECT id::text,display_name,destination_type,enabled,version,created_at,updated_at,
		endpoint_host,endpoint_port,endpoint_path_prefix,secret_reference
		FROM notification_destinations WHERE id=$1`
	if lock {
		query += ` FOR UPDATE`
	}
	var item internalDestination
	err := tx.QueryRow(ctx, query, id).Scan(&item.ID, &item.DisplayName, &item.DestinationType,
		&item.Enabled, &item.Version, &item.CreatedAt, &item.UpdatedAt,
		&item.EndpointHost, &item.EndpointPort, &item.PathPrefix, &item.SecretReference)
	item.CreatedAt, item.UpdatedAt = item.CreatedAt.UTC(), item.UpdatedAt.UTC()
	return item, err
}

func normalizeDefinition(value DestinationDefinition) DestinationDefinition {
	value.DisplayName = strings.TrimSpace(value.DisplayName)
	value.DestinationType = strings.ToLower(strings.TrimSpace(value.DestinationType))
	value.EndpointHost = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value.EndpointHost), "."))
	value.PathPrefix = strings.TrimSpace(value.PathPrefix)
	value.SecretReference = strings.TrimSpace(value.SecretReference)
	return value
}

func (service *Service) validateDefinition(ctx context.Context, value DestinationDefinition) error {
	if value.DestinationType != DestinationMattermost || value.DisplayName == "" ||
		utf8.RuneCountInString(value.DisplayName) > 128 || value.EndpointPort < 1 || value.EndpointPort > 65535 ||
		!pathPrefixPattern.MatchString(value.PathPrefix) || strings.Contains(value.PathPrefix, "..") ||
		!secretReferencePattern.MatchString(value.SecretReference) {
		return ErrInvalid
	}
	if !validEndpointHost(value.EndpointHost) {
		return ErrInvalid
	}
	if service.validator == nil {
		return ErrNetworkPolicy
	}
	if err := service.validator.Validate(ctx, Target{Host: value.EndpointHost, Port: value.EndpointPort, PathPrefix: value.PathPrefix}); err != nil {
		return ErrNetworkPolicy
	}
	if service.secrets == nil {
		return ErrSecretUnavailable
	}
	secret, err := service.secrets.Resolve(ctx, value.SecretReference)
	if err != nil || strings.TrimSpace(secret) == "" || len(secret) > 4096 || strings.ContainsAny(secret, "\r\n\x00") {
		return ErrSecretUnavailable
	}
	return nil
}

func validEndpointHost(host string) bool {
	if address := net.ParseIP(host); address != nil {
		return true
	}
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if !hostnameLabelPattern.MatchString(label) {
			return false
		}
	}
	return true
}

func destinationAuditSummary(item Destination) map[string]any {
	return map[string]any{"display_name": item.DisplayName, "destination_type": item.DestinationType,
		"enabled": item.Enabled, "version": item.Version, "network_target_configured": true,
		"secret_reference_configured": true}
}

func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}

func (service *Service) publish(kind, id, result string, at time.Time) {
	if service.hub != nil {
		service.hub.Publish(events.Event{Kind: kind, DeliveryID: id, Result: result, ChangedAt: at})
	}
}
