package incidents

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/adminops"
	"github.com/PrincepsVIIII/Espial/core/internal/audit"
	"github.com/PrincepsVIIII/Espial/core/internal/health"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrRuleNotFound = errors.New("incident rule not found")
	ErrRuleConflict = errors.New("incident rule version conflict")
	ErrRuleScope    = errors.New("incident rule scope is invalid")
)

type RuleView struct {
	ID                     string              `json:"id"`
	Name                   string              `json:"name"`
	Enabled                bool                `json:"enabled"`
	Priority               int                 `json:"priority"`
	IntegrationID          string              `json:"integration_id,omitempty"`
	ResourceID             string              `json:"resource_id,omitempty"`
	ResourceKind           string              `json:"resource_kind,omitempty"`
	CheckType              string              `json:"check_type,omitempty"`
	ReasonCode             string              `json:"reason_code,omitempty"`
	RecoveryState          health.State        `json:"recovery_state"`
	RecoveryMinOccurrences int                 `json:"recovery_min_occurrences"`
	RecoveryForSeconds     int                 `json:"recovery_for_seconds"`
	Conditions             []RuleConditionView `json:"conditions"`
	Version                int64               `json:"version"`
	CreatedAt              time.Time           `json:"created_at"`
	UpdatedAt              time.Time           `json:"updated_at"`
}

type RuleConditionView struct {
	State          health.State `json:"state"`
	Severity       Severity     `json:"severity"`
	MinOccurrences int          `json:"min_occurrences"`
	ForSeconds     int          `json:"for_seconds"`
}

type RuleList struct {
	Items []RuleView `json:"items"`
}

type RuleWrite struct {
	Definition      RuleDefinition
	Enabled         bool
	ExpectedVersion int64
	IdempotencyKey  string
	ActorUserID     string
	ActorName       string
	SourceAddress   string
	CorrelationID   string
}

type RulePreviewInput struct {
	IntegrationID string       `json:"integration_id"`
	ResourceID    string       `json:"resource_id"`
	CheckType     string       `json:"check_type"`
	State         health.State `json:"state"`
	ReasonCode    string       `json:"reason_code,omitempty"`
}

type RulePreviewCandidate struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Priority    int    `json:"priority"`
	Specificity int    `json:"specificity"`
	Winner      bool   `json:"winner"`
	Explanation string `json:"explanation"`
}

type RulePreview struct {
	Winner      *RulePreviewCandidate  `json:"winner,omitempty"`
	Candidates  []RulePreviewCandidate `json:"candidates"`
	Explanation string                 `json:"explanation"`
}

type RuleService struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func NewRuleService(pool *pgxpool.Pool, now func() time.Time) *RuleService {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &RuleService{pool: pool, now: now}
}

func (service *RuleService) List(ctx context.Context) (RuleList, error) {
	rows, err := service.pool.Query(ctx, `SELECT id::text FROM incident_rules ORDER BY priority DESC, name, id LIMIT 200`)
	if err != nil {
		return RuleList{}, fmt.Errorf("list incident rules: %w", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return RuleList{}, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return RuleList{}, err
	}
	result := RuleList{Items: make([]RuleView, 0, len(ids))}
	for _, id := range ids {
		item, err := service.Detail(ctx, id)
		if err != nil {
			return RuleList{}, err
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}

func (service *RuleService) Detail(ctx context.Context, id string) (RuleView, error) {
	return readRule(ctx, service.pool, id)
}

type ruleQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func readRule(ctx context.Context, db ruleQuerier, id string) (RuleView, error) {
	var item RuleView
	var integrationID, resourceID, resourceKind, checkType, reasonCode pgtype.Text
	var recovery string
	err := db.QueryRow(ctx, `
		SELECT id::text, name, enabled, priority, integration_id::text, resource_id::text,
			resource_kind, check_type, reason_code, recovery_state,
			recovery_min_occurrences, recovery_for_seconds, version, created_at, updated_at
		FROM incident_rules WHERE id = $1
	`, id).Scan(&item.ID, &item.Name, &item.Enabled, &item.Priority, &integrationID, &resourceID,
		&resourceKind, &checkType, &reasonCode, &recovery, &item.RecoveryMinOccurrences,
		&item.RecoveryForSeconds, &item.Version, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return RuleView{}, ErrRuleNotFound
	}
	if err != nil {
		return RuleView{}, fmt.Errorf("read incident rule: %w", err)
	}
	item.IntegrationID, item.ResourceID, item.ResourceKind = integrationID.String, resourceID.String, resourceKind.String
	item.CheckType, item.ReasonCode, item.RecoveryState = checkType.String, reasonCode.String, health.State(recovery)
	item.CreatedAt, item.UpdatedAt = item.CreatedAt.UTC(), item.UpdatedAt.UTC()
	rows, err := db.Query(ctx, `SELECT state, severity, min_occurrences, for_seconds FROM incident_rule_conditions WHERE rule_id = $1 ORDER BY state`, id)
	if err != nil {
		return RuleView{}, fmt.Errorf("read incident rule conditions: %w", err)
	}
	defer rows.Close()
	item.Conditions = []RuleConditionView{}
	for rows.Next() {
		var condition RuleConditionView
		if err := rows.Scan(&condition.State, &condition.Severity, &condition.MinOccurrences, &condition.ForSeconds); err != nil {
			return RuleView{}, err
		}
		item.Conditions = append(item.Conditions, condition)
	}
	return item, rows.Err()
}

func (service *RuleService) Create(ctx context.Context, input RuleWrite) (adminops.Receipt, error) {
	return service.mutate(ctx, "create", "", input)
}

func (service *RuleService) Replace(ctx context.Context, id string, input RuleWrite) (adminops.Receipt, error) {
	return service.mutate(ctx, "replace", id, input)
}

func (service *RuleService) mutate(ctx context.Context, operation, id string, input RuleWrite) (adminops.Receipt, error) {
	if err := ValidateRule(input.Definition); err != nil {
		return adminops.Receipt{}, err
	}
	hash, err := adminops.Hash(struct {
		Operation, ID string
		Definition    RuleDefinition
		Enabled       bool
		Version       int64
	}{operation, id, input.Definition, input.Enabled, input.ExpectedVersion})
	if err != nil {
		return adminops.Receipt{}, err
	}
	tx, err := service.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return adminops.Receipt{}, err
	}
	defer tx.Rollback(ctx)
	if replay, found, err := adminops.Replay(ctx, tx, input.ActorUserID, "incident_rule", operation, input.IdempotencyKey, hash); err != nil {
		return adminops.Receipt{}, err
	} else if found {
		return replay, tx.Commit(ctx)
	}
	if err := validateRuleReferences(ctx, tx, input.Definition); err != nil {
		return adminops.Receipt{}, err
	}
	now := service.now().UTC().Truncate(time.Microsecond)
	var before map[string]any
	var version int64
	if operation == "create" {
		if err := tx.QueryRow(ctx, `
			INSERT INTO incident_rules (id, name, enabled, priority, integration_id, resource_id,
				resource_kind, check_type, reason_code, recovery_state, recovery_min_occurrences,
				recovery_for_seconds, created_at, updated_at)
			VALUES (gen_random_uuid(), $1, $2, $3, NULLIF($4, '')::uuid, NULLIF($5, '')::uuid,
				NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''), $9, $10, $11, $12, $12)
			RETURNING id::text, version
		`, input.Definition.Name, input.Enabled, input.Definition.Priority, input.Definition.IntegrationID,
			input.Definition.ResourceID, input.Definition.ResourceKind, input.Definition.CheckType,
			input.Definition.ReasonCode, input.Definition.RecoveryState, input.Definition.RecoveryMinOccurrences,
			int(input.Definition.RecoveryFor/time.Second), now).Scan(&id, &version); err != nil {
			return adminops.Receipt{}, fmt.Errorf("create incident rule: %w", err)
		}
	} else {
		current, err := readRule(ctx, tx, id)
		if err != nil {
			return adminops.Receipt{}, err
		}
		if current.Version != input.ExpectedVersion {
			return adminops.Receipt{}, ErrRuleConflict
		}
		before = ruleAuditSummary(current)
		if err := tx.QueryRow(ctx, `
			UPDATE incident_rules SET name=$2, enabled=$3, priority=$4,
				integration_id=NULLIF($5, '')::uuid, resource_id=NULLIF($6, '')::uuid,
				resource_kind=NULLIF($7, ''), check_type=NULLIF($8, ''), reason_code=NULLIF($9, ''),
				recovery_state=$10, recovery_min_occurrences=$11, recovery_for_seconds=$12,
				version=version+1, updated_at=$13 WHERE id=$1 RETURNING version
		`, id, input.Definition.Name, input.Enabled, input.Definition.Priority, input.Definition.IntegrationID,
			input.Definition.ResourceID, input.Definition.ResourceKind, input.Definition.CheckType,
			input.Definition.ReasonCode, input.Definition.RecoveryState, input.Definition.RecoveryMinOccurrences,
			int(input.Definition.RecoveryFor/time.Second), now).Scan(&version); err != nil {
			return adminops.Receipt{}, fmt.Errorf("replace incident rule: %w", err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM incident_rule_conditions WHERE rule_id=$1`, id); err != nil {
			return adminops.Receipt{}, err
		}
	}
	for _, condition := range input.Definition.Conditions {
		if _, err := tx.Exec(ctx, `INSERT INTO incident_rule_conditions (rule_id,state,severity,min_occurrences,for_seconds) VALUES ($1,$2,$3,$4,$5)`, id, condition.State, condition.Severity, condition.MinOccurrences, int(condition.For/time.Second)); err != nil {
			return adminops.Receipt{}, fmt.Errorf("save rule condition: %w", err)
		}
	}
	current, err := readRule(ctx, tx, id)
	if err != nil {
		return adminops.Receipt{}, err
	}
	receipt := adminops.Receipt{ID: id, Version: version, RequestID: input.CorrelationID}
	if err := audit.Append(ctx, tx, audit.Event{ActorUserID: input.ActorUserID, Action: "incident_rule." + operation, TargetType: "incident_rule", TargetID: id, Result: "succeeded", SourceAddress: input.SourceAddress, CorrelationID: input.CorrelationID, BeforeSummary: before, AfterSummary: ruleAuditSummary(current), OccurredAt: now}); err != nil {
		return adminops.Receipt{}, err
	}
	if err := adminops.Save(ctx, tx, input.ActorUserID, "incident_rule", operation, input.IdempotencyKey, hash, receipt); err != nil {
		return adminops.Receipt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return adminops.Receipt{}, err
	}
	return receipt, nil
}

func validateRuleReferences(ctx context.Context, tx pgx.Tx, definition RuleDefinition) error {
	if definition.ResourceID != "" {
		var integrationID string
		if err := tx.QueryRow(ctx, `SELECT integration_id::text FROM resources WHERE id=$1`, definition.ResourceID).Scan(&integrationID); err != nil {
			return ErrRuleScope
		}
		if definition.IntegrationID != "" && definition.IntegrationID != integrationID {
			return ErrRuleScope
		}
	} else if definition.IntegrationID != "" {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM integrations WHERE id=$1)`, definition.IntegrationID).Scan(&exists); err != nil || !exists {
			return ErrRuleScope
		}
	}
	return nil
}

func ruleAuditSummary(rule RuleView) map[string]any {
	return map[string]any{"name": rule.Name, "enabled": rule.Enabled, "priority": rule.Priority, "integration_id": rule.IntegrationID, "resource_id": rule.ResourceID, "resource_kind": rule.ResourceKind, "check_type": rule.CheckType, "reason_code": rule.ReasonCode, "condition_count": len(rule.Conditions), "version": rule.Version}
}

func (service *RuleService) Preview(ctx context.Context, input RulePreviewInput) (RulePreview, error) {
	if !ruleUUIDPattern.MatchString(input.IntegrationID) || !ruleUUIDPattern.MatchString(input.ResourceID) || !ruleIdentifierPattern.MatchString(input.CheckType) || !validRuleState(input.State) || input.ReasonCode != "" && !ruleIdentifierPattern.MatchString(input.ReasonCode) {
		return RulePreview{}, ErrRuleScope
	}
	rows, err := service.pool.Query(ctx, `
		SELECT ir.id::text, ir.name, ir.priority,
			((ir.integration_id IS NOT NULL)::int + (ir.resource_id IS NOT NULL)::int +
			 (ir.resource_kind IS NOT NULL)::int + (ir.check_type IS NOT NULL)::int +
			 (ir.reason_code IS NOT NULL)::int) specificity
		FROM incident_rules ir JOIN resources r ON r.id=$2
		WHERE r.integration_id=$1 AND ir.enabled
		 AND (ir.integration_id IS NULL OR ir.integration_id=$1)
		 AND (ir.resource_id IS NULL OR ir.resource_id=$2)
		 AND (ir.resource_kind IS NULL OR ir.resource_kind=r.kind)
		 AND (ir.check_type IS NULL OR ir.check_type=$3)
		 AND (ir.reason_code IS NULL OR ir.reason_code=NULLIF($4,''))
		ORDER BY (ir.resource_id IS NOT NULL) DESC, specificity DESC, ir.priority DESC, ir.id
	`, input.IntegrationID, input.ResourceID, input.CheckType, input.ReasonCode)
	if err != nil {
		return RulePreview{}, fmt.Errorf("preview incident rules: %w", err)
	}
	defer rows.Close()
	result := RulePreview{Candidates: []RulePreviewCandidate{}}
	for rows.Next() {
		var item RulePreviewCandidate
		if err := rows.Scan(&item.ID, &item.Name, &item.Priority, &item.Specificity); err != nil {
			return RulePreview{}, err
		}
		item.Winner = len(result.Candidates) == 0
		if item.Winner {
			item.Explanation = "Selected by exact-resource, specificity, priority, then opaque ID precedence."
			copy := item
			result.Winner = &copy
		} else {
			item.Explanation = "Matched, but ranked below the selected rule."
		}
		result.Candidates = append(result.Candidates, item)
	}
	if err := rows.Err(); err != nil {
		return RulePreview{}, err
	}
	if result.Winner == nil {
		result.Explanation = "No enabled incident rule matches this normalized signal."
	} else {
		result.Explanation = fmt.Sprintf("%q wins among %d matching rule(s).", result.Winner.Name, len(result.Candidates))
	}
	return result, nil
}
