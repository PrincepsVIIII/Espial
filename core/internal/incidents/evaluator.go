package incidents

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/PrincepsVIIII/Espial/core/internal/events"
	"github.com/PrincepsVIIII/Espial/core/internal/health"
	"github.com/PrincepsVIIII/Espial/core/internal/signals"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Clock interface{ Now() time.Time }

type Options struct {
	Clock        Clock
	BatchSize    int
	PollInterval time.Duration
	ClaimLease   time.Duration
	MaxAttempts  int
	Intents      IntentWriter
	OnError      func(error)
}

type Evaluator struct {
	pool    *pgxpool.Pool
	store   *signals.Store
	hub     *events.Hub
	options Options
	running atomic.Bool
}

func NewEvaluator(pool *pgxpool.Pool, hub *events.Hub, options Options) *Evaluator {
	if options.Clock == nil {
		options.Clock = health.SystemClock{}
	}
	if options.BatchSize <= 0 {
		options.BatchSize = 50
	}
	if options.PollInterval <= 0 {
		options.PollInterval = time.Second
	}
	if options.ClaimLease <= 0 {
		options.ClaimLease = 30 * time.Second
	}
	if options.MaxAttempts <= 0 {
		options.MaxAttempts = 8
	}
	return &Evaluator{pool: pool, store: signals.NewStore(pool), hub: hub, options: options}
}

func (evaluator *Evaluator) Run(ctx context.Context) error {
	if !evaluator.running.CompareAndSwap(false, true) {
		return errors.New("incident evaluator is already running")
	}
	defer evaluator.running.Store(false)
	for {
		processed, err := evaluator.ProcessOnce(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		if processed >= evaluator.options.BatchSize {
			continue
		}
		timer := time.NewTimer(evaluator.options.PollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (evaluator *Evaluator) ProcessOnce(ctx context.Context) (int, error) {
	now := evaluator.options.Clock.Now().UTC().Truncate(time.Microsecond)
	claimed, err := evaluator.store.Claim(ctx, now, evaluator.options.BatchSize, evaluator.options.ClaimLease, evaluator.options.MaxAttempts)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, signal := range claimed {
		change, processErr := evaluator.processSignal(ctx, signal, now)
		if processErr != nil {
			retry := time.Duration(1<<min(signal.Attempts-1, 6)) * time.Second
			if failErr := evaluator.store.Fail(ctx, signal.ID, now, retry, evaluator.options.MaxAttempts, "incident_evaluation_failed"); failErr != nil {
				return processed, failErr
			}
			if evaluator.options.OnError != nil {
				evaluator.options.OnError(processErr)
			}
			continue
		}
		processed++
		evaluator.publish(change)
	}
	dueChanges, err := evaluator.processDue(ctx, now, evaluator.options.BatchSize)
	if err != nil {
		return processed, err
	}
	for _, change := range dueChanges {
		evaluator.publish(change)
	}
	return processed, nil
}

type rule struct {
	ID                     string
	Name                   string
	RecoveryState          health.State
	RecoveryMinOccurrences int
	RecoveryFor            time.Duration
	Condition              *condition
}

type condition struct {
	Severity       Severity
	MinOccurrences int
	For            time.Duration
}

type ruleState struct {
	ActiveIncidentID    string
	LastSignalID        string
	LastSignalAt        *time.Time
	LastState           health.State
	LastReason          string
	MatchingSince       *time.Time
	MatchingOccurrences int
	RecoverySince       *time.Time
	RecoveryOccurrences int
	DeadlineAt          *time.Time
}

type incidentChange struct {
	IncidentID    string
	IntegrationID string
	ResourceID    string
	Status        Status
	ChangedAt     time.Time
	Changed       bool
}

func (evaluator *Evaluator) processSignal(ctx context.Context, signal signals.Signal, now time.Time) (incidentChange, error) {
	tx, err := evaluator.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return incidentChange{}, fmt.Errorf("begin incident evaluation: %w", err)
	}
	defer tx.Rollback(ctx)
	var processedAt, deadLetteredAt pgtype.Timestamptz
	if err := tx.QueryRow(ctx, `
		SELECT processed_at, dead_lettered_at
		FROM monitoring_signals WHERE id = $1 FOR UPDATE
	`, signal.ID).Scan(&processedAt, &deadLetteredAt); err != nil {
		return incidentChange{}, fmt.Errorf("lock monitoring signal: %w", err)
	}
	if processedAt.Valid || deadLetteredAt.Valid {
		return incidentChange{}, tx.Commit(ctx)
	}

	rule, resourceName, err := matchingRule(ctx, tx, signal)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := appendEvaluationEvidence(ctx, tx, signal.ID, "", "", "no_rule", "No enabled incident rule matched the normalized signal.", now); err != nil {
			return incidentChange{}, err
		}
		if err := signals.MarkProcessed(ctx, tx, signal.ID, now); err != nil {
			return incidentChange{}, err
		}
		return incidentChange{}, tx.Commit(ctx)
	}
	if err != nil {
		return incidentChange{}, err
	}
	state, err := lockRuleState(ctx, tx, rule.ID, signal.ResourceID, signal.CheckType)
	if err != nil {
		return incidentChange{}, err
	}
	if state.LastSignalAt != nil && state.LastSignalAt.After(signal.OccurredAt) {
		if err := appendEvaluationEvidence(ctx, tx, signal.ID, rule.ID, "", "debounced", "An older out-of-order signal was recorded without changing rule state.", now); err != nil {
			return incidentChange{}, err
		}
		if err := signals.MarkProcessed(ctx, tx, signal.ID, now); err != nil {
			return incidentChange{}, err
		}
		return incidentChange{}, tx.Commit(ctx)
	}
	maintenance, err := activeMaintenance(ctx, tx, signal, signal.OccurredAt)
	if err != nil {
		return incidentChange{}, err
	}
	if maintenance.ID != "" {
		state.MatchingSince, state.RecoverySince, state.DeadlineAt = nil, nil, nil
		state.MatchingOccurrences, state.RecoveryOccurrences = 0, 0
		state.LastSignalID = signal.ID
		occurredAt := signal.OccurredAt.UTC()
		state.LastSignalAt = &occurredAt
		state.LastState = health.Maintenance
		state.LastReason = "Maintenance: " + maintenance.Reason
		if err := saveRuleState(ctx, tx, rule.ID, signal.ResourceID, signal.CheckType, state, now); err != nil {
			return incidentChange{}, err
		}
		if err := appendEvaluationEvidence(ctx, tx, signal.ID, rule.ID, maintenance.ID, "maintenance", "Raw failure preserved; incident evaluation suppressed by maintenance window.", now); err != nil {
			return incidentChange{}, err
		}
		if err := signals.MarkProcessed(ctx, tx, signal.ID, now); err != nil {
			return incidentChange{}, err
		}
		return incidentChange{}, tx.Commit(ctx)
	}

	change, err := evaluate(ctx, tx, signal, rule, &state, resourceName, now, false, evaluator.options.Intents)
	if err != nil {
		return incidentChange{}, err
	}
	state.LastSignalID = signal.ID
	occurredAt := signal.OccurredAt.UTC()
	state.LastSignalAt = &occurredAt
	state.LastState = signal.State
	state.LastReason = signal.Reason
	if err := saveRuleState(ctx, tx, rule.ID, signal.ResourceID, signal.CheckType, state, now); err != nil {
		return incidentChange{}, err
	}
	outcome, explanation := "debounced", "The matching rule did not yet satisfy its occurrence or duration threshold."
	if rule.Condition == nil {
		outcome, explanation = "no_condition", "The winning rule has no condition for this health state."
	}
	if change.Changed {
		outcome, explanation = "incident_changed", "The winning rule changed the authoritative incident lifecycle."
	}
	if err := appendEvaluationEvidence(ctx, tx, signal.ID, rule.ID, "", outcome, explanation, now); err != nil {
		return incidentChange{}, err
	}
	if err := signals.MarkProcessed(ctx, tx, signal.ID, now); err != nil {
		return incidentChange{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return incidentChange{}, fmt.Errorf("commit incident evaluation: %w", err)
	}
	return change, nil
}

type maintenanceMatch struct{ ID, Reason string }

func activeMaintenance(ctx context.Context, tx pgx.Tx, signal signals.Signal, at time.Time) (maintenanceMatch, error) {
	var result maintenanceMatch
	err := tx.QueryRow(ctx, `
		SELECT id::text, reason FROM maintenance_windows
		WHERE enabled AND revoked_at IS NULL AND starts_at <= $4 AND ends_at > $4
		  AND (integration_id IS NULL OR integration_id = $1)
		  AND (resource_id IS NULL OR resource_id = $2)
		  AND (check_type IS NULL OR check_type = $3)
		ORDER BY (resource_id IS NOT NULL) DESC,
		 ((integration_id IS NOT NULL)::int + (check_type IS NOT NULL)::int) DESC,
		 starts_at DESC, id LIMIT 1
	`, signal.IntegrationID, signal.ResourceID, signal.CheckType, at.UTC()).Scan(&result.ID, &result.Reason)
	if errors.Is(err, pgx.ErrNoRows) {
		return maintenanceMatch{}, nil
	}
	if err != nil {
		return maintenanceMatch{}, fmt.Errorf("match maintenance window: %w", err)
	}
	return result, nil
}

func appendEvaluationEvidence(ctx context.Context, tx pgx.Tx, signalID, ruleID, windowID, outcome, explanation string, at time.Time) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO incident_evaluation_evidence (
			signal_id, rule_id, maintenance_window_id, outcome, explanation, evaluated_at
		) VALUES ($1, NULLIF($2, '')::uuid, NULLIF($3, '')::uuid, $4, $5, $6)
		ON CONFLICT (signal_id) DO NOTHING
	`, signalID, ruleID, windowID, outcome, boundedText(explanation, 1024), at.UTC())
	if err != nil {
		return fmt.Errorf("append incident evaluation evidence: %w", err)
	}
	return nil
}

func matchingRule(ctx context.Context, tx pgx.Tx, signal signals.Signal) (rule, string, error) {
	var result rule
	var resourceName string
	var severity pgtype.Text
	var minimum, conditionSeconds pgtype.Int4
	var recoverySeconds int
	var recoveryState string
	err := tx.QueryRow(ctx, `
		SELECT ir.id::text, ir.name, ir.recovery_state,
			ir.recovery_min_occurrences, ir.recovery_for_seconds,
			irc.severity, irc.min_occurrences, irc.for_seconds,
			r.display_name
		FROM incident_rules ir
		JOIN resources r ON r.id = $2
		LEFT JOIN incident_rule_conditions irc
			ON irc.rule_id = ir.id AND irc.state = $5
		WHERE ir.enabled
		  AND (ir.integration_id IS NULL OR ir.integration_id = $1)
		  AND (ir.resource_id IS NULL OR ir.resource_id = $2)
		  AND (ir.resource_kind IS NULL OR ir.resource_kind = r.kind)
		  AND (ir.check_type IS NULL OR ir.check_type = $3)
		  AND (ir.reason_code IS NULL OR ir.reason_code = NULLIF($4, ''))
		ORDER BY (ir.resource_id IS NOT NULL) DESC,
			((ir.integration_id IS NOT NULL)::int + (ir.check_type IS NOT NULL)::int +
			 (ir.resource_kind IS NOT NULL)::int + (ir.reason_code IS NOT NULL)::int) DESC,
			ir.priority DESC, ir.id
		LIMIT 1
	`, signal.IntegrationID, signal.ResourceID, signal.CheckType,
		signal.ReasonCode, signal.State).Scan(
		&result.ID, &result.Name, &recoveryState,
		&result.RecoveryMinOccurrences, &recoverySeconds,
		&severity, &minimum, &conditionSeconds, &resourceName,
	)
	if err != nil {
		return rule{}, "", err
	}
	result.RecoveryState = health.State(recoveryState)
	result.RecoveryFor = time.Duration(recoverySeconds) * time.Second
	if severity.Valid {
		result.Condition = &condition{Severity: Severity(severity.String), MinOccurrences: int(minimum.Int32), For: time.Duration(conditionSeconds.Int32) * time.Second}
	}
	return result, resourceName, nil
}

func matchingRuleByID(ctx context.Context, tx pgx.Tx, id string, signal signals.Signal) (rule, string, error) {
	var result rule
	var resourceName string
	var severity pgtype.Text
	var minimum, conditionSeconds pgtype.Int4
	var recoverySeconds int
	var recoveryState string
	err := tx.QueryRow(ctx, `
		SELECT ir.id::text, ir.name, ir.recovery_state,
			ir.recovery_min_occurrences, ir.recovery_for_seconds,
			irc.severity, irc.min_occurrences, irc.for_seconds, r.display_name
		FROM incident_rules ir JOIN resources r ON r.id=$3
		LEFT JOIN incident_rule_conditions irc ON irc.rule_id=ir.id AND irc.state=$6
		WHERE ir.id=$1 AND ir.enabled
		 AND (ir.integration_id IS NULL OR ir.integration_id=$2)
		 AND (ir.resource_id IS NULL OR ir.resource_id=$3)
		 AND (ir.resource_kind IS NULL OR ir.resource_kind=r.kind)
		 AND (ir.check_type IS NULL OR ir.check_type=$4)
		 AND (ir.reason_code IS NULL OR ir.reason_code=NULLIF($5,''))
	`, id, signal.IntegrationID, signal.ResourceID, signal.CheckType, signal.ReasonCode, signal.State).Scan(
		&result.ID, &result.Name, &recoveryState, &result.RecoveryMinOccurrences, &recoverySeconds,
		&severity, &minimum, &conditionSeconds, &resourceName,
	)
	if err != nil {
		return rule{}, "", err
	}
	result.RecoveryState = health.State(recoveryState)
	result.RecoveryFor = time.Duration(recoverySeconds) * time.Second
	if severity.Valid {
		result.Condition = &condition{Severity: Severity(severity.String), MinOccurrences: int(minimum.Int32), For: time.Duration(conditionSeconds.Int32) * time.Second}
	}
	return result, resourceName, nil
}

func lockRuleState(ctx context.Context, tx pgx.Tx, ruleID, resourceID, checkType string) (ruleState, error) {
	if _, err := tx.Exec(ctx, `
		INSERT INTO incident_rule_states (rule_id, resource_id, check_type)
		VALUES ($1, $2, $3) ON CONFLICT DO NOTHING
	`, ruleID, resourceID, checkType); err != nil {
		return ruleState{}, fmt.Errorf("initialize incident rule state: %w", err)
	}
	var result ruleState
	var activeID, lastSignalID, lastState, lastReason pgtype.Text
	var lastSignalAt, matchingSince, recoverySince, deadlineAt pgtype.Timestamptz
	if err := tx.QueryRow(ctx, `
		SELECT active_incident_id::text, last_signal_id::text, last_signal_at,
			last_state, last_reason, matching_since, matching_occurrences,
			recovery_since, recovery_occurrences, deadline_at
		FROM incident_rule_states
		WHERE rule_id = $1 AND resource_id = $2 AND check_type = $3
		FOR UPDATE
	`, ruleID, resourceID, checkType).Scan(
		&activeID, &lastSignalID, &lastSignalAt, &lastState, &lastReason,
		&matchingSince, &result.MatchingOccurrences, &recoverySince,
		&result.RecoveryOccurrences, &deadlineAt,
	); err != nil {
		return ruleState{}, fmt.Errorf("lock incident rule state: %w", err)
	}
	result.ActiveIncidentID = activeID.String
	result.LastSignalID = lastSignalID.String
	result.LastSignalAt = timestamp(lastSignalAt)
	result.LastState = health.State(lastState.String)
	result.LastReason = lastReason.String
	result.MatchingSince = timestamp(matchingSince)
	result.RecoverySince = timestamp(recoverySince)
	result.DeadlineAt = timestamp(deadlineAt)
	return result, nil
}

func evaluate(ctx context.Context, tx pgx.Tx, signal signals.Signal, rule rule, state *ruleState, resourceName string, now time.Time, deadline bool, intents IntentWriter) (incidentChange, error) {
	change := incidentChange{IntegrationID: signal.IntegrationID, ResourceID: signal.ResourceID, ChangedAt: now}
	if signal.State == rule.RecoveryState {
		state.MatchingSince, state.DeadlineAt = nil, nil
		state.MatchingOccurrences = 0
		if !deadline {
			if state.LastState == signal.State && state.RecoverySince != nil {
				state.RecoveryOccurrences++
			} else {
				started := signal.OccurredAt.UTC()
				state.RecoverySince = &started
				state.RecoveryOccurrences = 1
			}
		}
		if state.RecoverySince != nil && state.RecoveryOccurrences >= rule.RecoveryMinOccurrences {
			due := state.RecoverySince.Add(rule.RecoveryFor)
			if !now.Before(due) {
				return recoverIncident(ctx, tx, signal, rule, state, resourceName, now, intents)
			}
			state.DeadlineAt = &due
		}
		return change, nil
	}

	state.RecoverySince = nil
	state.RecoveryOccurrences = 0
	if rule.Condition == nil {
		state.MatchingSince, state.DeadlineAt = nil, nil
		state.MatchingOccurrences = 0
		return change, nil
	}
	if !deadline {
		if state.LastState == signal.State && state.MatchingSince != nil {
			state.MatchingOccurrences++
		} else {
			started := signal.OccurredAt.UTC()
			state.MatchingSince = &started
			state.MatchingOccurrences = 1
		}
	}
	if state.MatchingSince == nil || state.MatchingOccurrences < rule.Condition.MinOccurrences {
		state.DeadlineAt = nil
		return change, nil
	}
	due := state.MatchingSince.Add(rule.Condition.For)
	if now.Before(due) {
		state.DeadlineAt = &due
		return change, nil
	}
	state.DeadlineAt = nil
	return openOrUpdateIncident(ctx, tx, signal, rule, state, resourceName, now, intents)
}

func openOrUpdateIncident(ctx context.Context, tx pgx.Tx, signal signals.Signal, rule rule, state *ruleState, resourceName string, now time.Time, intents IntentWriter) (incidentChange, error) {
	change := incidentChange{IntegrationID: signal.IntegrationID, ResourceID: signal.ResourceID, ChangedAt: now}
	fingerprint := strings.Join([]string{rule.ID, signal.ResourceID, signal.CheckType}, ":")
	if state.ActiveIncidentID == "" {
		var id string
		title := boundedText(resourceName+": "+signal.CheckType, 256)
		err := tx.QueryRow(ctx, `
			INSERT INTO incidents (
				rule_id, integration_id, resource_id, check_type, fingerprint,
				title, summary, severity, status, detected_at, latest_signal_at,
				created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'open', $9, $9, $10, $10)
			ON CONFLICT (fingerprint) WHERE status <> 'resolved' DO NOTHING
			RETURNING id::text
		`, rule.ID, signal.IntegrationID, signal.ResourceID, signal.CheckType,
			fingerprint, title, signal.Reason, rule.Condition.Severity,
			signal.OccurredAt.UTC(), now).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx, `SELECT id::text FROM incidents WHERE fingerprint = $1 AND status <> 'resolved'`, fingerprint).Scan(&id)
		}
		if err != nil {
			return change, fmt.Errorf("create incident: %w", err)
		}
		state.ActiveIncidentID = id
		var createdAt time.Time
		if err := tx.QueryRow(ctx, "SELECT created_at FROM incidents WHERE id = $1", id).Scan(&createdAt); err != nil {
			return change, err
		}
		if createdAt.Equal(now) {
			timelineID, err := appendTimeline(ctx, tx, id, signal.ID, "detected", "", StatusOpen, "", rule.Condition.Severity, "Incident detected: "+signal.Reason, signal.OccurredAt)
			if err != nil {
				return change, err
			}
			if err := enqueueNotification(ctx, tx, intents, NotificationEvent{TimelineEventID: timelineID, IncidentID: id, RuleID: rule.ID, ResourceID: signal.ResourceID, Kind: "detected", Title: title, Summary: signal.Reason, Severity: rule.Condition.Severity, Status: StatusOpen, OccurredAt: signal.OccurredAt, CreatedAt: now}); err != nil {
				return change, err
			}
			change.IncidentID, change.Status, change.Changed = id, StatusOpen, true
		}
		return change, nil
	}

	var status string
	var severity string
	if err := tx.QueryRow(ctx, `SELECT status, severity FROM incidents WHERE id = $1 FOR UPDATE`, state.ActiveIncidentID).Scan(&status, &severity); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			state.ActiveIncidentID = ""
			return openOrUpdateIncident(ctx, tx, signal, rule, state, resourceName, now, intents)
		}
		return change, err
	}
	if Status(status) == StatusRecovered {
		if _, err := tx.Exec(ctx, `
			UPDATE incidents SET status = 'open', severity = $2, summary = $3,
				latest_signal_at = GREATEST(latest_signal_at, $4), recovered_at = NULL,
				version = version + 1, updated_at = $5 WHERE id = $1
		`, state.ActiveIncidentID, rule.Condition.Severity, signal.Reason,
			signal.OccurredAt.UTC(), now); err != nil {
			return change, err
		}
		timelineID, err := appendTimeline(ctx, tx, state.ActiveIncidentID, signal.ID, "recurrence", StatusRecovered, StatusOpen, Severity(severity), rule.Condition.Severity, "Condition recurred: "+signal.Reason, signal.OccurredAt)
		if err != nil {
			return change, err
		}
		if err := enqueueNotification(ctx, tx, intents, NotificationEvent{TimelineEventID: timelineID, IncidentID: state.ActiveIncidentID, RuleID: rule.ID, ResourceID: signal.ResourceID, Kind: "recurrence", Title: boundedText(resourceName+": "+signal.CheckType, 256), Summary: signal.Reason, Severity: rule.Condition.Severity, Status: StatusOpen, OccurredAt: signal.OccurredAt, CreatedAt: now}); err != nil {
			return change, err
		}
		return incidentChange{IncidentID: state.ActiveIncidentID, IntegrationID: signal.IntegrationID, ResourceID: signal.ResourceID, Status: StatusOpen, ChangedAt: now, Changed: true}, nil
	}
	if Severity(severity) != rule.Condition.Severity {
		if _, err := tx.Exec(ctx, `
			UPDATE incidents SET severity = $2, summary = $3,
				latest_signal_at = GREATEST(latest_signal_at, $4),
				version = version + 1, updated_at = $5 WHERE id = $1
		`, state.ActiveIncidentID, rule.Condition.Severity, signal.Reason,
			signal.OccurredAt.UTC(), now); err != nil {
			return change, err
		}
		timelineID, err := appendTimeline(ctx, tx, state.ActiveIncidentID, signal.ID, "severity_changed", Status(status), Status(status), Severity(severity), rule.Condition.Severity, "Severity changed: "+signal.Reason, signal.OccurredAt)
		if err != nil {
			return change, err
		}
		if err := enqueueNotification(ctx, tx, intents, NotificationEvent{TimelineEventID: timelineID, IncidentID: state.ActiveIncidentID, RuleID: rule.ID, ResourceID: signal.ResourceID, Kind: "severity_changed", Title: boundedText(resourceName+": "+signal.CheckType, 256), Summary: signal.Reason, Severity: rule.Condition.Severity, Status: Status(status), OccurredAt: signal.OccurredAt, CreatedAt: now}); err != nil {
			return change, err
		}
		return incidentChange{IncidentID: state.ActiveIncidentID, IntegrationID: signal.IntegrationID, ResourceID: signal.ResourceID, Status: Status(status), ChangedAt: now, Changed: true}, nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE incidents SET summary = $2,
			latest_signal_at = GREATEST(latest_signal_at, $3),
			version = version + 1, updated_at = $4 WHERE id = $1
	`, state.ActiveIncidentID, signal.Reason, signal.OccurredAt.UTC(), now); err != nil {
		return change, err
	}
	return incidentChange{IncidentID: state.ActiveIncidentID, IntegrationID: signal.IntegrationID, ResourceID: signal.ResourceID, Status: Status(status), ChangedAt: now, Changed: true}, nil
}

func recoverIncident(ctx context.Context, tx pgx.Tx, signal signals.Signal, rule rule, state *ruleState, resourceName string, now time.Time, intents IntentWriter) (incidentChange, error) {
	change := incidentChange{IntegrationID: signal.IntegrationID, ResourceID: signal.ResourceID, ChangedAt: now}
	if state.ActiveIncidentID == "" {
		return change, nil
	}
	var status, severity string
	if err := tx.QueryRow(ctx, `SELECT status, severity FROM incidents WHERE id = $1 FOR UPDATE`, state.ActiveIncidentID).Scan(&status, &severity); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			state.ActiveIncidentID = ""
			return change, nil
		}
		return change, err
	}
	if Status(status) == StatusRecovered || Status(status) == StatusResolved {
		return change, nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE incidents SET status = 'recovered', summary = $2,
			latest_signal_at = GREATEST(latest_signal_at, $3), recovered_at = $3,
			version = version + 1, updated_at = $4 WHERE id = $1
	`, state.ActiveIncidentID, signal.Reason, signal.OccurredAt.UTC(), now); err != nil {
		return change, err
	}
	timelineID, err := appendTimeline(ctx, tx, state.ActiveIncidentID, signal.ID, "recovered", Status(status), StatusRecovered, Severity(severity), Severity(severity), "Condition recovered: "+signal.Reason, signal.OccurredAt)
	if err != nil {
		return change, err
	}
	if err := enqueueNotification(ctx, tx, intents, NotificationEvent{TimelineEventID: timelineID, IncidentID: state.ActiveIncidentID, RuleID: rule.ID, ResourceID: signal.ResourceID, Kind: "recovered", Title: boundedText(resourceName+": "+signal.CheckType, 256), Summary: signal.Reason, Severity: Severity(severity), Status: StatusRecovered, OccurredAt: signal.OccurredAt, CreatedAt: now}); err != nil {
		return change, err
	}
	return incidentChange{IncidentID: state.ActiveIncidentID, IntegrationID: signal.IntegrationID, ResourceID: signal.ResourceID, Status: StatusRecovered, ChangedAt: now, Changed: true}, nil
}

func appendTimeline(ctx context.Context, tx pgx.Tx, incidentID, signalID, kind string, fromStatus, toStatus Status, fromSeverity, toSeverity Severity, summary string, occurredAt time.Time) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO incident_timeline (
			incident_id, signal_id, kind, from_status, to_status,
			from_severity, to_severity, summary, occurred_at
		) VALUES ($1, NULLIF($2, '')::uuid, $3, NULLIF($4, ''), NULLIF($5, ''),
			NULLIF($6, ''), NULLIF($7, ''), $8, $9)
		RETURNING id::text
	`, incidentID, signalID, kind, fromStatus, toStatus, fromSeverity, toSeverity,
		boundedText(summary, 2048), occurredAt.UTC()).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("append incident timeline: %w", err)
	}
	return id, nil
}

func enqueueNotification(ctx context.Context, tx pgx.Tx, writer IntentWriter, event NotificationEvent) error {
	if writer == nil {
		return nil
	}
	return writer.EnqueueIncidentEvent(ctx, tx, event)
}

func saveRuleState(ctx context.Context, tx pgx.Tx, ruleID, resourceID, checkType string, state ruleState, now time.Time) error {
	_, err := tx.Exec(ctx, `
		UPDATE incident_rule_states SET
			active_incident_id = NULLIF($4, '')::uuid,
			last_signal_id = NULLIF($5, '')::uuid, last_signal_at = $6,
			last_state = NULLIF($7, ''), last_reason = NULLIF($8, ''),
			matching_since = $9, matching_occurrences = $10,
			recovery_since = $11, recovery_occurrences = $12,
			deadline_at = $13, updated_at = $14
		WHERE rule_id = $1 AND resource_id = $2 AND check_type = $3
	`, ruleID, resourceID, checkType, state.ActiveIncidentID, state.LastSignalID,
		state.LastSignalAt, state.LastState, state.LastReason, state.MatchingSince,
		state.MatchingOccurrences, state.RecoverySince, state.RecoveryOccurrences,
		state.DeadlineAt, now)
	if err != nil {
		return fmt.Errorf("save incident rule state: %w", err)
	}
	return nil
}

func (evaluator *Evaluator) processDue(ctx context.Context, now time.Time, limit int) ([]incidentChange, error) {
	changes := make([]incidentChange, 0)
	for range limit {
		change, found, err := evaluator.processOneDue(ctx, now)
		if err != nil {
			return changes, err
		}
		if !found {
			break
		}
		if change.Changed {
			changes = append(changes, change)
		}
	}
	return changes, nil
}

func (evaluator *Evaluator) processOneDue(ctx context.Context, now time.Time) (incidentChange, bool, error) {
	tx, err := evaluator.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return incidentChange{}, false, err
	}
	defer tx.Rollback(ctx)
	var ruleID, resourceID, checkType string
	err = tx.QueryRow(ctx, `
		SELECT rule_id::text, resource_id::text, check_type
		FROM incident_rule_states
		WHERE deadline_at <= $1
		ORDER BY deadline_at, rule_id, resource_id, check_type
		FOR UPDATE SKIP LOCKED LIMIT 1
	`, now).Scan(&ruleID, &resourceID, &checkType)
	if errors.Is(err, pgx.ErrNoRows) {
		return incidentChange{}, false, nil
	}
	if err != nil {
		return incidentChange{}, false, err
	}
	state, err := lockRuleState(ctx, tx, ruleID, resourceID, checkType)
	if err != nil {
		return incidentChange{}, false, err
	}
	var signal signals.Signal
	if err := tx.QueryRow(ctx, `
		SELECT id::text, kind, integration_id::text, resource_id::text,
			COALESCE(observation_id::text, ''), check_type, state, reason,
			COALESCE(reason_code, ''), occurred_at, created_at, attempts
		FROM monitoring_signals WHERE id = $1
	`, state.LastSignalID).Scan(
		&signal.ID, &signal.Kind, &signal.IntegrationID, &signal.ResourceID,
		&signal.ObservationID, &signal.CheckType, &signal.State, &signal.Reason,
		&signal.ReasonCode, &signal.OccurredAt, &signal.CreatedAt, &signal.Attempts,
	); err != nil {
		return incidentChange{}, false, err
	}
	rule, resourceName, err := matchingRuleByID(ctx, tx, ruleID, signal)
	if errors.Is(err, pgx.ErrNoRows) {
		state.DeadlineAt = nil
		if err := saveRuleState(ctx, tx, ruleID, resourceID, checkType, state, now); err != nil {
			return incidentChange{}, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return incidentChange{}, false, err
		}
		return incidentChange{}, true, nil
	}
	if err != nil {
		return incidentChange{}, false, err
	}
	maintenance, err := activeMaintenance(ctx, tx, signal, now)
	if err != nil {
		return incidentChange{}, false, err
	}
	if maintenance.ID != "" {
		state.MatchingSince, state.RecoverySince, state.DeadlineAt = nil, nil, nil
		state.MatchingOccurrences, state.RecoveryOccurrences = 0, 0
		state.LastState, state.LastReason = health.Maintenance, "Maintenance: "+maintenance.Reason
		if err := saveRuleState(ctx, tx, ruleID, resourceID, checkType, state, now); err != nil {
			return incidentChange{}, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return incidentChange{}, false, err
		}
		return incidentChange{}, true, nil
	}
	change, err := evaluate(ctx, tx, signal, rule, &state, resourceName, now, true, evaluator.options.Intents)
	if err != nil {
		return incidentChange{}, false, err
	}
	state.DeadlineAt = nil
	if err := saveRuleState(ctx, tx, ruleID, resourceID, checkType, state, now); err != nil {
		return incidentChange{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return incidentChange{}, false, err
	}
	return change, true, nil
}

func (evaluator *Evaluator) publish(change incidentChange) {
	if evaluator.hub == nil || !change.Changed {
		return
	}
	evaluator.hub.Publish(events.Event{
		Kind: events.IncidentChanged, IncidentID: change.IncidentID,
		IntegrationID: change.IntegrationID, ResourceID: change.ResourceID,
		Result: string(change.Status), ChangedAt: change.ChangedAt,
	})
}

func timestamp(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}

func boundedText(value string, maximum int) string {
	if utf8.RuneCountInString(value) <= maximum {
		return value
	}
	runes := []rune(value)
	return string(runes[:maximum])
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
