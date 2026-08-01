package notifications

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/events"
	"github.com/PrincepsVIIII/Espial/core/internal/health"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type WorkerOptions struct {
	Clock         health.Clock
	Concurrency   int
	PollInterval  time.Duration
	ClaimLease    time.Duration
	MaxAttempts   int
	MaxRetryDelay time.Duration
	PublicURL     *url.URL
	Secrets       SecretResolver
	Drivers       map[string]Driver
	OnError       func(error)
}

type Worker struct {
	pool     *pgxpool.Pool
	hub      *events.Hub
	options  WorkerOptions
	running  atomic.Bool
	random   *rand.Rand
	randomMu sync.Mutex
}

func NewWorker(pool *pgxpool.Pool, hub *events.Hub, options WorkerOptions) *Worker {
	if options.Clock == nil {
		options.Clock = health.SystemClock{}
	}
	if options.Concurrency <= 0 {
		options.Concurrency = 2
	}
	if options.PollInterval <= 0 {
		options.PollInterval = time.Second
	}
	if options.ClaimLease <= 0 {
		options.ClaimLease = 30 * time.Second
	}
	if options.MaxAttempts <= 0 || options.MaxAttempts > 6 {
		options.MaxAttempts = 6
	}
	if options.MaxRetryDelay <= 0 {
		options.MaxRetryDelay = 5 * time.Minute
	}
	return &Worker{pool: pool, hub: hub, options: options, random: rand.New(rand.NewSource(time.Now().UnixNano()))}
}

func (worker *Worker) Run(ctx context.Context) error {
	if !worker.running.CompareAndSwap(false, true) {
		return errors.New("notification worker is already running")
	}
	defer worker.running.Store(false)
	runContext, cancel := context.WithCancel(ctx)
	defer cancel()
	errorsChannel := make(chan error, worker.options.Concurrency)
	for range worker.options.Concurrency {
		go func() { errorsChannel <- worker.runOne(runContext) }()
	}
	first := <-errorsChannel
	cancel()
	for index := 1; index < worker.options.Concurrency; index++ {
		candidate := <-errorsChannel
		if first == nil || errors.Is(first, context.Canceled) {
			first = candidate
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return first
}

func (worker *Worker) runOne(ctx context.Context) error {
	for {
		processed, err := worker.ProcessOnce(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if worker.options.OnError != nil {
				worker.options.OnError(err)
			}
			return err
		}
		if processed {
			continue
		}
		timer := time.NewTimer(worker.options.PollInterval)
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

func (worker *Worker) ProcessOnce(ctx context.Context) (bool, error) {
	now := worker.options.Clock.Now().UTC().Truncate(time.Microsecond)
	if finalized, err := worker.deadLetterExpiredClaim(ctx, now); err != nil || finalized {
		return finalized, err
	}
	claim, found, err := worker.claim(ctx, now)
	if err != nil || !found {
		return false, err
	}
	result := worker.deliver(ctx, claim)
	completedAt := worker.options.Clock.Now().UTC().Truncate(time.Microsecond)
	if completedAt.Before(now) {
		completedAt = now
	}
	return true, worker.complete(ctx, claim, result, now, completedAt)
}

type claimedIntent struct {
	ID              string
	ClaimToken      string
	IncidentID      string
	DestinationID   string
	DestinationName string
	DestinationType string
	EndpointHost    string
	EndpointPort    int
	PathPrefix      string
	SecretReference string
	EventKind       string
	Test            bool
	Title           string
	Summary         string
	Severity        string
	Status          string
	OccurredAt      time.Time
	Attempt         int
}

func (worker *Worker) claim(ctx context.Context, now time.Time) (claimedIntent, bool, error) {
	var item claimedIntent
	err := worker.pool.QueryRow(ctx, `
		WITH candidate AS (
			SELECT id FROM notification_intents
			WHERE attempt_count < $3 AND (
				(state IN ('queued','retry_wait') AND available_at <= $1)
				OR (state='attempting' AND claimed_until <= $1)
			)
			ORDER BY available_at, created_at, id
			FOR UPDATE SKIP LOCKED LIMIT 1
		), claimed AS (
			UPDATE notification_intents intent SET
				state='attempting', attempt_count=attempt_count+1,
				claim_token=gen_random_uuid(), claimed_until=$1+$2::interval,
				updated_at=$1
			FROM candidate WHERE intent.id=candidate.id
			RETURNING intent.*
		)
		SELECT claimed.id::text, claimed.claim_token::text,
			COALESCE(claimed.incident_id::text,''), claimed.destination_id::text,
			destination.display_name, destination.destination_type,
			destination.endpoint_host, destination.endpoint_port,
			destination.endpoint_path_prefix, destination.secret_reference,
			claimed.event_kind, claimed.is_test, claimed.title, claimed.summary,
			COALESCE(claimed.severity,''), COALESCE(claimed.incident_status,''),
			claimed.event_occurred_at, claimed.attempt_count
		FROM claimed JOIN notification_destinations destination
			ON destination.id=claimed.destination_id
	`, now, worker.options.ClaimLease.String(), worker.options.MaxAttempts).Scan(
		&item.ID, &item.ClaimToken, &item.IncidentID, &item.DestinationID,
		&item.DestinationName, &item.DestinationType, &item.EndpointHost,
		&item.EndpointPort, &item.PathPrefix, &item.SecretReference,
		&item.EventKind, &item.Test, &item.Title, &item.Summary, &item.Severity,
		&item.Status, &item.OccurredAt, &item.Attempt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return claimedIntent{}, false, nil
	}
	if err != nil {
		return claimedIntent{}, false, fmt.Errorf("claim notification intent: %w", err)
	}
	item.OccurredAt = item.OccurredAt.UTC()
	return item, true, nil
}

func (worker *Worker) deliver(ctx context.Context, claim claimedIntent) DeliveryResult {
	if worker.options.Secrets == nil {
		return DeliveryResult{Retryable: true, ErrorCode: "secret_resolver_unavailable"}
	}
	secret, err := worker.options.Secrets.Resolve(ctx, claim.SecretReference)
	if err != nil || strings.TrimSpace(secret) == "" || len(secret) > 4096 || strings.ContainsAny(secret, "\r\n\x00") {
		return DeliveryResult{Retryable: true, ErrorCode: "secret_resolution_failed"}
	}
	driver := worker.options.Drivers[claim.DestinationType]
	if driver == nil {
		return DeliveryResult{ErrorCode: "destination_driver_unavailable"}
	}
	incidentURL := ""
	if !claim.Test && worker.options.PublicURL != nil {
		reference, _ := url.Parse("/alerts/" + claim.IncidentID)
		incidentURL = worker.options.PublicURL.ResolveReference(reference).String()
	}
	return driver.Deliver(ctx, DeliveryRequest{
		Target:       Target{Host: claim.EndpointHost, Port: claim.EndpointPort, PathPrefix: claim.PathPrefix},
		WebhookToken: secret,
		Message: Message{EventID: claim.ID, IncidentID: claim.IncidentID, Kind: claim.EventKind,
			Title: claim.Title, Summary: claim.Summary, Severity: claim.Severity,
			Status: claim.Status, OccurredAt: claim.OccurredAt, IncidentURL: incidentURL,
			Test: claim.Test},
	})
}

func (worker *Worker) complete(ctx context.Context, claim claimedIntent, result DeliveryResult, startedAt, completedAt time.Time) error {
	state, outcome, errorCode := StateDelivered, "delivered", ""
	var deliveredAt, terminalAt any = completedAt, completedAt
	availableAt := completedAt
	if !result.Delivered {
		deliveredAt = nil
		errorCode = normalizedErrorCode(result.ErrorCode)
		if result.Retryable && claim.Attempt < worker.options.MaxAttempts {
			state, outcome, terminalAt = StateRetryWait, "retry", nil
			availableAt = completedAt.Add(worker.retryDelay(claim.Attempt, result.RetryAfter))
		} else if result.Retryable {
			state, outcome = StateDeadLetter, "dead_letter"
		} else {
			state, outcome = StateFailed, "failed"
		}
	}
	tx, err := worker.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `
		UPDATE notification_intents SET state=$4, available_at=$5,
			claimed_until=NULL, claim_token=NULL, delivered_at=$6, terminal_at=$7,
			last_error_code=NULLIF($8,''), updated_at=$9
		WHERE id=$1 AND state='attempting' AND claim_token=$2 AND attempt_count=$3
	`, claim.ID, claim.ClaimToken, claim.Attempt, state, availableAt,
		deliveredAt, terminalAt, errorCode, completedAt)
	if err != nil {
		return fmt.Errorf("complete notification intent: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return tx.Commit(ctx)
	}
	duration := completedAt.Sub(startedAt)
	if duration < 0 {
		duration = 0
	}
	if duration > time.Hour {
		duration = time.Hour
	}
	providerID := boundedProviderID(result.ProviderRequestID)
	var status any
	if result.HTTPStatus > 0 {
		status = result.HTTPStatus
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO notification_attempts (
			intent_id, attempt_number, outcome, http_status, safe_error_code,
			provider_request_id, started_at, completed_at, duration_ms, created_at
		) VALUES ($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),$7,$8,$9,$8)
	`, claim.ID, claim.Attempt, outcome, status, errorCode, providerID,
		startedAt, completedAt, duration.Milliseconds()); err != nil {
		return fmt.Errorf("append notification attempt: %w", err)
	}
	if claim.IncidentID != "" && state != StateRetryWait {
		summary := terminalTimelineSummary(state, claim.DestinationName)
		if _, err := tx.Exec(ctx, `
			INSERT INTO incident_timeline (incident_id, kind, summary, occurred_at)
			VALUES ($1,'notification',$2,$3)
		`, claim.IncidentID, summary, completedAt); err != nil {
			return fmt.Errorf("append delivery timeline evidence: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if worker.hub != nil {
		worker.hub.Publish(events.Event{Kind: events.NotificationDeliveryChanged,
			DeliveryID: claim.ID, IncidentID: claim.IncidentID, Result: string(state),
			ChangedAt: completedAt})
	}
	return nil
}

func (worker *Worker) deadLetterExpiredClaim(ctx context.Context, now time.Time) (bool, error) {
	tx, err := worker.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var id, incidentID, destinationName string
	var attempt int
	err = tx.QueryRow(ctx, `
		SELECT intent.id::text, COALESCE(intent.incident_id::text,''),
			destination.display_name, intent.attempt_count
		FROM notification_intents intent
		JOIN notification_destinations destination ON destination.id=intent.destination_id
		WHERE intent.state='attempting' AND intent.claimed_until <= $1
		  AND intent.attempt_count >= $2
		ORDER BY intent.claimed_until, intent.id
		FOR UPDATE OF intent SKIP LOCKED LIMIT 1
	`, now, worker.options.MaxAttempts).Scan(&id, &incidentID, &destinationName, &attempt)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim exhausted notification: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE notification_intents SET state='dead_letter', claimed_until=NULL,
			claim_token=NULL, terminal_at=$2, last_error_code='claim_lease_expired',
			updated_at=$2 WHERE id=$1
	`, id, now); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO notification_attempts (
			intent_id,attempt_number,outcome,safe_error_code,
			started_at,completed_at,duration_ms,created_at
		) VALUES ($1,$2,'dead_letter','claim_lease_expired',$3,$3,0,$3)
	`, id, attempt, now); err != nil {
		return false, err
	}
	if incidentID != "" {
		if _, err := tx.Exec(ctx, `INSERT INTO incident_timeline(incident_id,kind,summary,occurred_at)
			VALUES($1,'notification',$2,$3)`, incidentID,
			terminalTimelineSummary(StateDeadLetter, destinationName), now); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	if worker.hub != nil {
		worker.hub.Publish(events.Event{Kind: events.NotificationDeliveryChanged,
			DeliveryID: id, IncidentID: incidentID, Result: string(StateDeadLetter), ChangedAt: now})
	}
	return true, nil
}

func (worker *Worker) retryDelay(attempt int, retryAfter time.Duration) time.Duration {
	delay := time.Second << min(attempt-1, 8)
	if retryAfter > delay {
		delay = retryAfter
	}
	if delay > worker.options.MaxRetryDelay {
		delay = worker.options.MaxRetryDelay
	}
	worker.randomMu.Lock()
	jitter := time.Duration(worker.random.Int63n(max(1, int64(delay/4))))
	worker.randomMu.Unlock()
	if delay+jitter > worker.options.MaxRetryDelay {
		return worker.options.MaxRetryDelay
	}
	return delay + jitter
}

func terminalTimelineSummary(state State, destination string) string {
	switch state {
	case StateDelivered:
		return "Notification delivered to " + destination + "."
	case StateDeadLetter:
		return "Notification moved to dead letter after bounded retries for " + destination + "."
	default:
		return "Notification delivery failed for " + destination + "."
	}
}

func normalizedErrorCode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "delivery_failed"
	}
	for _, current := range value {
		if !(current >= 'a' && current <= 'z' || current >= '0' && current <= '9' || strings.ContainsRune("_.-", current)) {
			return "delivery_failed"
		}
	}
	if len(value) > 127 {
		return "delivery_failed"
	}
	return value
}

func boundedProviderID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 || strings.ContainsFunc(value, func(current rune) bool { return current < 0x20 || current == 0x7f }) {
		return ""
	}
	return value
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func max(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}
