package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"time"

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
	pool  *pgxpool.Pool
	hub   *events.Hub
	clock health.Clock
}

func NewIntegrationConfigService(pool *pgxpool.Pool, hub *events.Hub, clock health.Clock) *IntegrationConfigService {
	if clock == nil {
		clock = health.SystemClock{}
	}
	return &IntegrationConfigService{pool: pool, hub: hub, clock: clock}
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
