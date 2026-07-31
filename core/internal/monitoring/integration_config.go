package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/PrincepsVIIII/Espial/core/internal/adapters"
	"github.com/PrincepsVIIII/Espial/core/internal/audit"
	"github.com/PrincepsVIIII/Espial/core/internal/events"
	"github.com/PrincepsVIIII/Espial/core/internal/health"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maxIntegrationConfigBytes = 64 * 1024

var integrationUUIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-8][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

type IntegrationConfigUpdate struct {
	IntegrationID     string
	Enabled           bool
	Interval          time.Duration
	ConfigNonsecret   map[string]any
	SecretReferences  map[string]string
	ExpectedUpdatedAt time.Time
	ActorUserID       string
	SourceAddress     string
	CorrelationID     string
}

type IntegrationConfigService struct {
	pool     *pgxpool.Pool
	hub      *events.Hub
	clock    health.Clock
	registry AdapterRegistry
}

type AdapterRegistry interface {
	Lookup(string) (adapters.Descriptor, error)
}

func NewIntegrationConfigService(pool *pgxpool.Pool, hub *events.Hub, clock health.Clock, registries ...AdapterRegistry) *IntegrationConfigService {
	if clock == nil {
		clock = health.SystemClock{}
	}
	service := &IntegrationConfigService{pool: pool, hub: hub, clock: clock}
	if len(registries) > 0 {
		service.registry = registries[0]
	}
	return service
}

func (service *IntegrationConfigService) Create(ctx context.Context, create CreateIntegration) (string, time.Time, error) {
	create.DisplayName = strings.TrimSpace(create.DisplayName)
	if service.registry == nil || create.CorrelationID == "" ||
		create.DisplayName == "" || utf8.RuneCountInString(create.DisplayName) > 128 ||
		create.Interval < time.Second || create.Interval > 24*time.Hour || create.Interval%time.Second != 0 {
		return "", time.Time{}, &Error{Code: "invalid_integration_config"}
	}
	if _, err := service.registry.Lookup(create.AdapterID); err != nil {
		return "", time.Time{}, &Error{Code: "adapter_not_registered"}
	}
	if create.ConfigNonsecret == nil {
		create.ConfigNonsecret = map[string]any{}
	}
	if create.SecretReferences == nil {
		create.SecretReferences = map[string]string{}
	}
	nonsecret, err := encodeConfig(create.ConfigNonsecret)
	if err != nil {
		return "", time.Time{}, err
	}
	references, err := encodeConfig(create.SecretReferences)
	if err != nil {
		return "", time.Time{}, err
	}
	id, err := newCorrelationID()
	if err != nil {
		return "", time.Time{}, &Error{Code: "correlation_id_failed"}
	}
	createdAt := service.clock.Now().UTC().Truncate(time.Microsecond)
	tx, err := service.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("begin integration creation: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		INSERT INTO integrations (
			id, adapter_id, display_name, enabled, config_nonsecret,
			secret_references, interval_seconds, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5::jsonb, $6::jsonb, $7, $8, $8)
	`, id, create.AdapterID, create.DisplayName, create.Enabled, string(nonsecret),
		string(references), int(create.Interval/time.Second), createdAt); err != nil {
		return "", time.Time{}, fmt.Errorf("create integration: %w", err)
	}
	if err := audit.Append(ctx, tx, audit.Event{
		ActorUserID: create.ActorUserID, Action: "integration.created",
		TargetType: "integration", TargetID: id, Result: "succeeded",
		SourceAddress: create.SourceAddress, CorrelationID: create.CorrelationID,
		AfterSummary: map[string]any{
			"adapter_id": create.AdapterID, "display_name": create.DisplayName,
			"enabled": create.Enabled, "interval_seconds": int(create.Interval / time.Second),
			"config_keys":           sortedKeys(create.ConfigNonsecret),
			"secret_reference_keys": sortedKeys(create.SecretReferences),
		},
		OccurredAt: createdAt,
	}); err != nil {
		return "", time.Time{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", time.Time{}, fmt.Errorf("commit integration creation: %w", err)
	}
	if service.hub != nil {
		service.hub.Publish(events.Event{
			Kind: events.IntegrationChanged, IntegrationID: id,
			Result: "created", ChangedAt: createdAt,
		})
	}
	return id, createdAt, nil
}

// Update applies configuration and its redacted audit record atomically. Only key
// names enter audit summaries; non-secret values and secret references never do.
func (service *IntegrationConfigService) Update(ctx context.Context, update IntegrationConfigUpdate) (time.Time, error) {
	if !integrationUUIDPattern.MatchString(update.IntegrationID) || update.Interval < time.Second || update.Interval > 24*time.Hour ||
		update.Interval%time.Second != 0 || update.CorrelationID == "" {
		return time.Time{}, &Error{Code: "invalid_integration_config"}
	}
	if update.ConfigNonsecret == nil {
		update.ConfigNonsecret = map[string]any{}
	}
	if update.SecretReferences == nil {
		update.SecretReferences = map[string]string{}
	}
	nonsecret, err := encodeConfig(update.ConfigNonsecret)
	if err != nil {
		return time.Time{}, err
	}
	secretReferences, err := encodeConfig(update.SecretReferences)
	if err != nil {
		return time.Time{}, err
	}
	tx, err := service.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return time.Time{}, fmt.Errorf("begin integration config update: %w", err)
	}
	defer tx.Rollback(ctx)
	var beforeEnabled bool
	var beforeInterval int
	var beforeNonsecret map[string]any
	var beforeReferences map[string]string
	var previousUpdatedAt time.Time
	if err := tx.QueryRow(ctx, `
		SELECT enabled, interval_seconds, config_nonsecret, secret_references, updated_at
		FROM integrations WHERE id = $1 FOR UPDATE
	`, update.IntegrationID).Scan(
		&beforeEnabled, &beforeInterval, &beforeNonsecret, &beforeReferences, &previousUpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return time.Time{}, &Error{Code: "integration_not_found"}
		}
		return time.Time{}, fmt.Errorf("load integration config: %w", err)
	}
	if !update.ExpectedUpdatedAt.IsZero() && !previousUpdatedAt.Equal(update.ExpectedUpdatedAt) {
		return time.Time{}, &Error{Code: "integration_config_conflict"}
	}
	updatedAt := service.clock.Now().UTC().Truncate(time.Microsecond)
	if !updatedAt.After(previousUpdatedAt) {
		updatedAt = previousUpdatedAt.Add(time.Microsecond)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE integrations SET enabled = $2, interval_seconds = $3,
			config_nonsecret = $4::jsonb, secret_references = $5::jsonb, updated_at = $6
		WHERE id = $1
	`, update.IntegrationID, update.Enabled, int(update.Interval/time.Second),
		string(nonsecret), string(secretReferences), updatedAt); err != nil {
		return time.Time{}, fmt.Errorf("update integration config: %w", err)
	}
	if err := audit.Append(ctx, tx, audit.Event{
		ActorUserID: update.ActorUserID, Action: "integration.configuration.updated",
		TargetType: "integration", TargetID: update.IntegrationID, Result: "succeeded",
		SourceAddress: update.SourceAddress, CorrelationID: update.CorrelationID,
		BeforeSummary: configSummary(beforeEnabled, beforeInterval, beforeNonsecret, beforeReferences),
		AfterSummary:  configSummary(update.Enabled, int(update.Interval/time.Second), update.ConfigNonsecret, update.SecretReferences),
		OccurredAt:    updatedAt,
	}); err != nil {
		return time.Time{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return time.Time{}, fmt.Errorf("commit integration config: %w", err)
	}
	if service.hub != nil {
		service.hub.Publish(events.Event{
			Kind: events.IntegrationChanged, IntegrationID: update.IntegrationID,
			Result: "configuration_updated", ChangedAt: updatedAt,
		})
	}
	return updatedAt, nil
}

func encodeConfig(value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) > maxIntegrationConfigBytes {
		return nil, &Error{Code: "invalid_integration_config"}
	}
	return encoded, nil
}

func configSummary(enabled bool, interval int, nonsecret map[string]any, references map[string]string) map[string]any {
	return map[string]any{
		"enabled": enabled, "interval_seconds": interval,
		"config_keys": sortedKeys(nonsecret), "secret_reference_keys": sortedKeys(references),
	}
}

func sortedKeys[T any](values map[string]T) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
