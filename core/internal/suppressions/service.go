// Package suppressions owns maintenance windows and notification silences.
package suppressions

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/PrincepsVIIII/Espial/core/internal/adminops"
	"github.com/PrincepsVIIII/Espial/core/internal/audit"
	"github.com/PrincepsVIIII/Espial/core/internal/events"
	"github.com/PrincepsVIIII/Espial/core/internal/health"
	"github.com/PrincepsVIIII/Espial/core/internal/signals"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound = errors.New("suppression not found")
	ErrConflict = errors.New("suppression version conflict")
	ErrInvalid  = errors.New("invalid suppression")
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-8][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
var identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,126}$`)

type MaintenanceWindow struct {
	ID            string     `json:"id"`
	Reason        string     `json:"reason"`
	IntegrationID string     `json:"integration_id,omitempty"`
	ResourceID    string     `json:"resource_id,omitempty"`
	CheckType     string     `json:"check_type,omitempty"`
	StartsAt      time.Time  `json:"starts_at"`
	EndsAt        time.Time  `json:"ends_at"`
	Enabled       bool       `json:"enabled"`
	RevokedAt     *time.Time `json:"revoked_at,omitempty"`
	ExpiredAt     *time.Time `json:"expired_at,omitempty"`
	CreatedByName string     `json:"created_by_name"`
	Version       int64      `json:"version"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type MaintenanceList struct {
	Items []MaintenanceWindow `json:"items"`
}

type MaintenanceDefinition struct {
	Reason        string    `json:"reason"`
	IntegrationID string    `json:"integration_id,omitempty"`
	ResourceID    string    `json:"resource_id,omitempty"`
	CheckType     string    `json:"check_type,omitempty"`
	StartsAt      time.Time `json:"starts_at"`
	EndsAt        time.Time `json:"ends_at"`
	Enabled       bool      `json:"enabled"`
}

type Silence struct {
	ID            string     `json:"id"`
	Reason        string     `json:"reason"`
	IncidentID    string     `json:"incident_id,omitempty"`
	RuleID        string     `json:"rule_id,omitempty"`
	ResourceID    string     `json:"resource_id,omitempty"`
	StartsAt      time.Time  `json:"starts_at"`
	ExpiresAt     time.Time  `json:"expires_at"`
	Enabled       bool       `json:"enabled"`
	RevokedAt     *time.Time `json:"revoked_at,omitempty"`
	ExpiredAt     *time.Time `json:"expired_at,omitempty"`
	CreatedByName string     `json:"created_by_name"`
	Version       int64      `json:"version"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type SilenceList struct {
	Items []Silence `json:"items"`
}

type SilenceDefinition struct {
	Reason     string    `json:"reason"`
	IncidentID string    `json:"incident_id,omitempty"`
	RuleID     string    `json:"rule_id,omitempty"`
	ResourceID string    `json:"resource_id,omitempty"`
	StartsAt   time.Time `json:"starts_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Enabled    bool      `json:"enabled"`
}

type MutationMetadata struct {
	ExpectedVersion int64
	IdempotencyKey  string
	ActorUserID     string
	ActorName       string
	SourceAddress   string
	CorrelationID   string
}

type IncidentContext struct{ IncidentID, RuleID, ResourceID string }
type SilenceMatch struct {
	ID, Reason string
	ExpiresAt  time.Time
}

type Service struct {
	pool *pgxpool.Pool
	hub  *events.Hub
	now  func() time.Time
}

func NewService(pool *pgxpool.Pool, hub *events.Hub, now func() time.Time) *Service {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{pool: pool, hub: hub, now: now}
}

func validateReason(reason string) bool {
	return reason != "" && strings.TrimSpace(reason) == reason && utf8.RuneCountInString(reason) <= 512
}

func ValidateMaintenance(value MaintenanceDefinition) error {
	if !validateReason(value.Reason) || value.StartsAt.IsZero() || value.EndsAt.IsZero() || !value.EndsAt.After(value.StartsAt) || value.EndsAt.Sub(value.StartsAt) > 366*24*time.Hour {
		return ErrInvalid
	}
	if value.IntegrationID == "" && value.ResourceID == "" && value.CheckType == "" {
		return ErrInvalid
	}
	if value.IntegrationID != "" && !uuidPattern.MatchString(value.IntegrationID) || value.ResourceID != "" && !uuidPattern.MatchString(value.ResourceID) || value.CheckType != "" && !identifierPattern.MatchString(value.CheckType) {
		return ErrInvalid
	}
	return nil
}

func ValidateSilence(value SilenceDefinition) error {
	if !validateReason(value.Reason) || value.StartsAt.IsZero() || value.ExpiresAt.IsZero() || !value.ExpiresAt.After(value.StartsAt) || value.ExpiresAt.Sub(value.StartsAt) > 366*24*time.Hour {
		return ErrInvalid
	}
	count := 0
	for _, id := range []string{value.IncidentID, value.RuleID, value.ResourceID} {
		if id != "" {
			count++
			if !uuidPattern.MatchString(id) {
				return ErrInvalid
			}
		}
	}
	if count != 1 {
		return ErrInvalid
	}
	return nil
}

func (service *Service) MaintenanceWindows(ctx context.Context) (MaintenanceList, error) {
	rows, err := service.pool.Query(ctx, maintenanceSelect+` ORDER BY starts_at DESC,id DESC LIMIT 200`)
	if err != nil {
		return MaintenanceList{}, err
	}
	defer rows.Close()
	result := MaintenanceList{Items: []MaintenanceWindow{}}
	for rows.Next() {
		item, err := scanMaintenance(rows)
		if err != nil {
			return MaintenanceList{}, err
		}
		result.Items = append(result.Items, item)
	}
	return result, rows.Err()
}

func (service *Service) MaintenanceWindow(ctx context.Context, id string) (MaintenanceWindow, error) {
	item, err := scanMaintenance(service.pool.QueryRow(ctx, maintenanceSelect+` WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return MaintenanceWindow{}, ErrNotFound
	}
	return item, err
}

const maintenanceSelect = `SELECT id::text,reason,integration_id::text,resource_id::text,check_type,starts_at,ends_at,enabled,revoked_at,expired_at,created_by_name,version,created_at,updated_at FROM maintenance_windows`

type scanner interface{ Scan(...any) error }

func scanMaintenance(row scanner) (MaintenanceWindow, error) {
	var item MaintenanceWindow
	var integration, resource, check pgtype.Text
	var revoked, expired pgtype.Timestamptz
	if err := row.Scan(&item.ID, &item.Reason, &integration, &resource, &check, &item.StartsAt, &item.EndsAt, &item.Enabled, &revoked, &expired, &item.CreatedByName, &item.Version, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return item, err
	}
	item.IntegrationID, item.ResourceID, item.CheckType = integration.String, resource.String, check.String
	item.RevokedAt = timestamp(revoked)
	item.ExpiredAt = timestamp(expired)
	item.StartsAt = item.StartsAt.UTC()
	item.EndsAt = item.EndsAt.UTC()
	item.CreatedAt = item.CreatedAt.UTC()
	item.UpdatedAt = item.UpdatedAt.UTC()
	return item, nil
}

func (service *Service) Silences(ctx context.Context) (SilenceList, error) {
	rows, err := service.pool.Query(ctx, silenceSelect+` ORDER BY starts_at DESC,id DESC LIMIT 200`)
	if err != nil {
		return SilenceList{}, err
	}
	defer rows.Close()
	result := SilenceList{Items: []Silence{}}
	for rows.Next() {
		item, err := scanSilence(rows)
		if err != nil {
			return SilenceList{}, err
		}
		result.Items = append(result.Items, item)
	}
	return result, rows.Err()
}
func (service *Service) Silence(ctx context.Context, id string) (Silence, error) {
	item, err := scanSilence(service.pool.QueryRow(ctx, silenceSelect+` WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Silence{}, ErrNotFound
	}
	return item, err
}

const silenceSelect = `SELECT id::text,reason,incident_id::text,rule_id::text,resource_id::text,starts_at,expires_at,enabled,revoked_at,expired_at,created_by_name,version,created_at,updated_at FROM silences`

func scanSilence(row scanner) (Silence, error) {
	var item Silence
	var incident, rule, resource pgtype.Text
	var revoked, expired pgtype.Timestamptz
	if err := row.Scan(&item.ID, &item.Reason, &incident, &rule, &resource, &item.StartsAt, &item.ExpiresAt, &item.Enabled, &revoked, &expired, &item.CreatedByName, &item.Version, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return item, err
	}
	item.IncidentID, item.RuleID, item.ResourceID = incident.String, rule.String, resource.String
	item.RevokedAt = timestamp(revoked)
	item.ExpiredAt = timestamp(expired)
	item.StartsAt = item.StartsAt.UTC()
	item.ExpiresAt = item.ExpiresAt.UTC()
	item.CreatedAt = item.CreatedAt.UTC()
	item.UpdatedAt = item.UpdatedAt.UTC()
	return item, nil
}

func (service *Service) CreateMaintenance(ctx context.Context, value MaintenanceDefinition, metadata MutationMetadata) (adminops.Receipt, error) {
	return service.mutateMaintenance(ctx, "create", "", value, metadata)
}
func (service *Service) ReplaceMaintenance(ctx context.Context, id string, value MaintenanceDefinition, metadata MutationMetadata) (adminops.Receipt, error) {
	return service.mutateMaintenance(ctx, "replace", id, value, metadata)
}

func (service *Service) mutateMaintenance(ctx context.Context, operation, id string, value MaintenanceDefinition, metadata MutationMetadata) (adminops.Receipt, error) {
	if err := ValidateMaintenance(value); err != nil {
		return adminops.Receipt{}, err
	}
	hash, err := adminops.Hash(struct {
		Operation, ID string
		Value         MaintenanceDefinition
		Version       int64
	}{operation, id, value, metadata.ExpectedVersion})
	if err != nil {
		return adminops.Receipt{}, err
	}
	tx, err := service.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return adminops.Receipt{}, err
	}
	defer tx.Rollback(ctx)
	if replay, found, err := adminops.Replay(ctx, tx, metadata.ActorUserID, "maintenance_window", operation, metadata.IdempotencyKey, hash); err != nil {
		return adminops.Receipt{}, err
	} else if found {
		return replay, tx.Commit(ctx)
	}
	if err := validateMaintenanceReferences(ctx, tx, value); err != nil {
		return adminops.Receipt{}, err
	}
	now := service.now().UTC().Truncate(time.Microsecond)
	var before map[string]any
	var version int64
	var previous MaintenanceWindow
	if operation == "create" {
		err = tx.QueryRow(ctx, `INSERT INTO maintenance_windows(reason,integration_id,resource_id,check_type,starts_at,ends_at,enabled,created_by_user_id,created_by_name,created_at,updated_at) VALUES($1,NULLIF($2,'')::uuid,NULLIF($3,'')::uuid,NULLIF($4,''),$5,$6,$7,$8,$9,$10,$10) RETURNING id::text,version`, value.Reason, value.IntegrationID, value.ResourceID, value.CheckType, value.StartsAt.UTC(), value.EndsAt.UTC(), value.Enabled, metadata.ActorUserID, metadata.ActorName, now).Scan(&id, &version)
	} else {
		current, readErr := readMaintenance(ctx, tx, id, true)
		if readErr != nil {
			return adminops.Receipt{}, readErr
		}
		if current.Version != metadata.ExpectedVersion {
			return adminops.Receipt{}, ErrConflict
		}
		before = maintenanceSummary(current)
		previous = current
		err = tx.QueryRow(ctx, `UPDATE maintenance_windows SET reason=$2,integration_id=NULLIF($3,'')::uuid,resource_id=NULLIF($4,'')::uuid,check_type=NULLIF($5,''),starts_at=$6,ends_at=$7,enabled=$8,expired_at=NULL,version=version+1,updated_at=$9 WHERE id=$1 AND revoked_at IS NULL RETURNING version`, id, value.Reason, value.IntegrationID, value.ResourceID, value.CheckType, value.StartsAt.UTC(), value.EndsAt.UTC(), value.Enabled, now).Scan(&version)
		if errors.Is(err, pgx.ErrNoRows) {
			return adminops.Receipt{}, ErrConflict
		}
	}
	if err != nil {
		return adminops.Receipt{}, fmt.Errorf("save maintenance window: %w", err)
	}
	if operation == "replace" && maintenanceActive(previous, now) &&
		(!value.Enabled || now.Before(value.StartsAt) || !now.Before(value.EndsAt) ||
			previous.IntegrationID != value.IntegrationID || previous.ResourceID != value.ResourceID || previous.CheckType != value.CheckType) {
		if err := appendExitSignals(ctx, tx, previous, now); err != nil {
			return adminops.Receipt{}, err
		}
	}
	current, err := readMaintenance(ctx, tx, id, false)
	if err != nil {
		return adminops.Receipt{}, err
	}
	receipt := adminops.Receipt{ID: id, Version: version, RequestID: metadata.CorrelationID}
	if err := audit.Append(ctx, tx, audit.Event{ActorUserID: metadata.ActorUserID, Action: "maintenance_window." + operation, TargetType: "maintenance_window", TargetID: id, Result: "succeeded", SourceAddress: metadata.SourceAddress, CorrelationID: metadata.CorrelationID, BeforeSummary: before, AfterSummary: maintenanceSummary(current), OccurredAt: now}); err != nil {
		return adminops.Receipt{}, err
	}
	if err := adminops.Save(ctx, tx, metadata.ActorUserID, "maintenance_window", operation, metadata.IdempotencyKey, hash, receipt); err != nil {
		return adminops.Receipt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return adminops.Receipt{}, err
	}
	service.publish(id, now)
	return receipt, nil
}

func maintenanceActive(value MaintenanceWindow, at time.Time) bool {
	return value.Enabled && value.RevokedAt == nil && !at.Before(value.StartsAt) && at.Before(value.EndsAt)
}

func readMaintenance(ctx context.Context, tx pgx.Tx, id string, lock bool) (MaintenanceWindow, error) {
	query := maintenanceSelect + ` WHERE id=$1`
	if lock {
		query += ` FOR UPDATE`
	}
	item, err := scanMaintenance(tx.QueryRow(ctx, query, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return item, ErrNotFound
	}
	return item, err
}

func (service *Service) CreateSilence(ctx context.Context, value SilenceDefinition, metadata MutationMetadata) (adminops.Receipt, error) {
	return service.mutateSilence(ctx, "create", "", value, metadata)
}
func (service *Service) ReplaceSilence(ctx context.Context, id string, value SilenceDefinition, metadata MutationMetadata) (adminops.Receipt, error) {
	return service.mutateSilence(ctx, "replace", id, value, metadata)
}
func (service *Service) mutateSilence(ctx context.Context, operation, id string, value SilenceDefinition, metadata MutationMetadata) (adminops.Receipt, error) {
	if err := ValidateSilence(value); err != nil {
		return adminops.Receipt{}, err
	}
	hash, err := adminops.Hash(struct {
		Operation, ID string
		Value         SilenceDefinition
		Version       int64
	}{operation, id, value, metadata.ExpectedVersion})
	if err != nil {
		return adminops.Receipt{}, err
	}
	tx, err := service.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return adminops.Receipt{}, err
	}
	defer tx.Rollback(ctx)
	if replay, found, err := adminops.Replay(ctx, tx, metadata.ActorUserID, "silence", operation, metadata.IdempotencyKey, hash); err != nil {
		return adminops.Receipt{}, err
	} else if found {
		return replay, tx.Commit(ctx)
	}
	if err := validateSilenceReferences(ctx, tx, value); err != nil {
		return adminops.Receipt{}, err
	}
	now := service.now().UTC().Truncate(time.Microsecond)
	var before map[string]any
	var version int64
	if operation == "create" {
		err = tx.QueryRow(ctx, `INSERT INTO silences(reason,incident_id,rule_id,resource_id,starts_at,expires_at,enabled,created_by_user_id,created_by_name,created_at,updated_at)VALUES($1,NULLIF($2,'')::uuid,NULLIF($3,'')::uuid,NULLIF($4,'')::uuid,$5,$6,$7,$8,$9,$10,$10)RETURNING id::text,version`, value.Reason, value.IncidentID, value.RuleID, value.ResourceID, value.StartsAt.UTC(), value.ExpiresAt.UTC(), value.Enabled, metadata.ActorUserID, metadata.ActorName, now).Scan(&id, &version)
	} else {
		current, readErr := readSilence(ctx, tx, id, true)
		if readErr != nil {
			return adminops.Receipt{}, readErr
		}
		if current.Version != metadata.ExpectedVersion {
			return adminops.Receipt{}, ErrConflict
		}
		before = silenceSummary(current)
		err = tx.QueryRow(ctx, `UPDATE silences SET reason=$2,incident_id=NULLIF($3,'')::uuid,rule_id=NULLIF($4,'')::uuid,resource_id=NULLIF($5,'')::uuid,starts_at=$6,expires_at=$7,enabled=$8,expired_at=NULL,version=version+1,updated_at=$9 WHERE id=$1 AND revoked_at IS NULL RETURNING version`, id, value.Reason, value.IncidentID, value.RuleID, value.ResourceID, value.StartsAt.UTC(), value.ExpiresAt.UTC(), value.Enabled, now).Scan(&version)
		if errors.Is(err, pgx.ErrNoRows) {
			return adminops.Receipt{}, ErrConflict
		}
	}
	if err != nil {
		return adminops.Receipt{}, fmt.Errorf("save silence: %w", err)
	}
	current, err := readSilence(ctx, tx, id, false)
	if err != nil {
		return adminops.Receipt{}, err
	}
	receipt := adminops.Receipt{ID: id, Version: version, RequestID: metadata.CorrelationID}
	if err := audit.Append(ctx, tx, audit.Event{ActorUserID: metadata.ActorUserID, Action: "silence." + operation, TargetType: "silence", TargetID: id, Result: "succeeded", SourceAddress: metadata.SourceAddress, CorrelationID: metadata.CorrelationID, BeforeSummary: before, AfterSummary: silenceSummary(current), OccurredAt: now}); err != nil {
		return adminops.Receipt{}, err
	}
	if err := adminops.Save(ctx, tx, metadata.ActorUserID, "silence", operation, metadata.IdempotencyKey, hash, receipt); err != nil {
		return adminops.Receipt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return adminops.Receipt{}, err
	}
	service.publish(id, now)
	return receipt, nil
}
func readSilence(ctx context.Context, tx pgx.Tx, id string, lock bool) (Silence, error) {
	query := silenceSelect + ` WHERE id=$1`
	if lock {
		query += ` FOR UPDATE`
	}
	item, err := scanSilence(tx.QueryRow(ctx, query, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return item, ErrNotFound
	}
	return item, err
}

func (service *Service) RevokeMaintenance(ctx context.Context, id string, metadata MutationMetadata) (adminops.Receipt, error) {
	return service.revoke(ctx, "maintenance_window", id, metadata)
}
func (service *Service) RevokeSilence(ctx context.Context, id string, metadata MutationMetadata) (adminops.Receipt, error) {
	return service.revoke(ctx, "silence", id, metadata)
}
func (service *Service) revoke(ctx context.Context, target, id string, metadata MutationMetadata) (adminops.Receipt, error) {
	hash, err := adminops.Hash(struct {
		Target, ID string
		Version    int64
	}{target, id, metadata.ExpectedVersion})
	if err != nil {
		return adminops.Receipt{}, err
	}
	tx, err := service.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return adminops.Receipt{}, err
	}
	defer tx.Rollback(ctx)
	if replay, found, err := adminops.Replay(ctx, tx, metadata.ActorUserID, target, "revoke", metadata.IdempotencyKey, hash); err != nil {
		return adminops.Receipt{}, err
	} else if found {
		return replay, tx.Commit(ctx)
	}
	now := service.now().UTC().Truncate(time.Microsecond)
	var before map[string]any
	var version int64
	if target == "maintenance_window" {
		item, err := readMaintenance(ctx, tx, id, true)
		if err != nil {
			return adminops.Receipt{}, err
		}
		if item.Version != metadata.ExpectedVersion {
			return adminops.Receipt{}, ErrConflict
		}
		before = maintenanceSummary(item)
		if item.RevokedAt != nil {
			return adminops.Receipt{}, ErrConflict
		}
		if err := tx.QueryRow(ctx, `UPDATE maintenance_windows SET revoked_at=$2,version=version+1,updated_at=$2 WHERE id=$1 RETURNING version`, id, now).Scan(&version); err != nil {
			return adminops.Receipt{}, err
		}
		if item.Enabled && !now.Before(item.StartsAt) && now.Before(item.EndsAt) {
			if err := appendExitSignals(ctx, tx, item, now); err != nil {
				return adminops.Receipt{}, err
			}
		}
	} else {
		item, err := readSilence(ctx, tx, id, true)
		if err != nil {
			return adminops.Receipt{}, err
		}
		if item.Version != metadata.ExpectedVersion {
			return adminops.Receipt{}, ErrConflict
		}
		before = silenceSummary(item)
		if item.RevokedAt != nil {
			return adminops.Receipt{}, ErrConflict
		}
		if err := tx.QueryRow(ctx, `UPDATE silences SET revoked_at=$2,version=version+1,updated_at=$2 WHERE id=$1 RETURNING version`, id, now).Scan(&version); err != nil {
			return adminops.Receipt{}, err
		}
	}
	receipt := adminops.Receipt{ID: id, Version: version, RequestID: metadata.CorrelationID}
	if err := audit.Append(ctx, tx, audit.Event{ActorUserID: metadata.ActorUserID, Action: target + ".revoke", TargetType: target, TargetID: id, Result: "succeeded", SourceAddress: metadata.SourceAddress, CorrelationID: metadata.CorrelationID, BeforeSummary: before, AfterSummary: map[string]any{"revoked_at": now, "version": version}, OccurredAt: now}); err != nil {
		return adminops.Receipt{}, err
	}
	if err := adminops.Save(ctx, tx, metadata.ActorUserID, target, "revoke", metadata.IdempotencyKey, hash, receipt); err != nil {
		return adminops.Receipt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return adminops.Receipt{}, err
	}
	service.publish(id, now)
	return receipt, nil
}

func validateMaintenanceReferences(ctx context.Context, tx pgx.Tx, value MaintenanceDefinition) error {
	if value.ResourceID != "" {
		var integration string
		if err := tx.QueryRow(ctx, `SELECT integration_id::text FROM resources WHERE id=$1`, value.ResourceID).Scan(&integration); err != nil {
			return ErrInvalid
		}
		if value.IntegrationID != "" && integration != value.IntegrationID {
			return ErrInvalid
		}
	} else if value.IntegrationID != "" {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM integrations WHERE id=$1)`, value.IntegrationID).Scan(&exists); err != nil || !exists {
			return ErrInvalid
		}
	}
	return nil
}
func validateSilenceReferences(ctx context.Context, tx pgx.Tx, value SilenceDefinition) error {
	var exists bool
	var query string
	var id string
	switch {
	case value.IncidentID != "":
		query = `SELECT EXISTS(SELECT 1 FROM incidents WHERE id=$1)`
		id = value.IncidentID
	case value.RuleID != "":
		query = `SELECT EXISTS(SELECT 1 FROM incident_rules WHERE id=$1)`
		id = value.RuleID
	default:
		query = `SELECT EXISTS(SELECT 1 FROM resources WHERE id=$1)`
		id = value.ResourceID
	}
	if err := tx.QueryRow(ctx, query, id).Scan(&exists); err != nil || !exists {
		return ErrInvalid
	}
	return nil
}
func maintenanceSummary(v MaintenanceWindow) map[string]any {
	return map[string]any{"reason": v.Reason, "integration_id": v.IntegrationID, "resource_id": v.ResourceID, "check_type": v.CheckType, "starts_at": v.StartsAt, "ends_at": v.EndsAt, "enabled": v.Enabled, "version": v.Version}
}
func silenceSummary(v Silence) map[string]any {
	return map[string]any{"reason": v.Reason, "incident_id": v.IncidentID, "rule_id": v.RuleID, "resource_id": v.ResourceID, "starts_at": v.StartsAt, "expires_at": v.ExpiresAt, "enabled": v.Enabled, "version": v.Version}
}
func timestamp(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}
func (service *Service) publish(id string, now time.Time) {
	if service.hub != nil {
		service.hub.Publish(events.Event{Kind: events.SuppressionChanged, Result: id, ChangedAt: now})
	}
}

// MatchSilence is the notification-intent suppression decision boundary. It is
// intentionally independent of incident status and never mutates an incident.
func (service *Service) MatchSilence(ctx context.Context, incident IncidentContext, at time.Time) (*SilenceMatch, error) {
	var result SilenceMatch
	err := service.pool.QueryRow(ctx, `SELECT id::text,reason,expires_at FROM silences WHERE enabled AND revoked_at IS NULL AND starts_at<=$4 AND expires_at>$4 AND (incident_id=NULLIF($1,'')::uuid OR rule_id=NULLIF($2,'')::uuid OR resource_id=NULLIF($3,'')::uuid) ORDER BY (incident_id IS NOT NULL) DESC,(rule_id IS NOT NULL) DESC,created_at DESC,id LIMIT 1`, incident.IncidentID, incident.RuleID, incident.ResourceID, at.UTC()).Scan(&result.ID, &result.Reason, &result.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result.ExpiresAt = result.ExpiresAt.UTC()
	return &result, nil
}

func appendExitSignals(ctx context.Context, tx pgx.Tx, window MaintenanceWindow, at time.Time) error {
	rows, err := tx.Query(ctx, `SELECT r.integration_id::text,r.id::text,o.id::text,o.check_type,ch.state,ch.reason FROM current_health ch JOIN resources r ON r.id=ch.resource_id JOIN observations o ON o.id=ch.observation_id WHERE ($1='' OR r.integration_id=NULLIF($1,'')::uuid) AND ($2='' OR r.id=NULLIF($2,'')::uuid) AND ($3='' OR o.check_type=$3)`, window.IntegrationID, window.ResourceID, window.CheckType)
	if err != nil {
		return err
	}
	type exitSignal struct {
		integrationID, resourceID, observationID, checkType, reason string
		state                                                       health.State
	}
	items := []exitSignal{}
	for rows.Next() {
		var item exitSignal
		if err := rows.Scan(&item.integrationID, &item.resourceID, &item.observationID, &item.checkType, &item.state, &item.reason); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	writer := signals.NewWriter()
	for _, item := range items {
		if err := writer.Append(ctx, tx, signals.Input{SourceKey: fmt.Sprintf("maintenance-expiry:%s:%s:%s:%d", window.ID, item.resourceID, item.checkType, at.UTC().UnixMicro()), Kind: signals.KindMaintenanceExpiry, IntegrationID: item.integrationID, ResourceID: item.resourceID, ObservationID: item.observationID, CheckType: item.checkType, State: item.state, Reason: item.reason, OccurredAt: at, AvailableAt: at}); err != nil {
			return err
		}
	}
	return nil
}
