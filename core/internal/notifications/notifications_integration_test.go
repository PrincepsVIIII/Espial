package notifications

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/incidents"
	"github.com/PrincepsVIIII/Espial/core/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	testActorID       = "60000000-0000-4000-8000-000000000024"
	testIntegrationID = "50000000-0000-4000-8000-000000000024"
	testResourceID    = "40000000-0000-4000-8000-000000000024"
	testIncidentID    = "30000000-0000-4000-8000-000000000024"
)

type allowValidator struct{}

func (allowValidator) Validate(context.Context, Target) error { return nil }

type mutableClock struct {
	mu  sync.Mutex
	now time.Time
}

func (clock *mutableClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

func (clock *mutableClock) Add(duration time.Duration) {
	clock.mu.Lock()
	clock.now = clock.now.Add(duration)
	clock.mu.Unlock()
}

type sequenceDriver struct {
	mu      sync.Mutex
	results []DeliveryResult
	seen    []DeliveryRequest
}

type blockingDriver struct {
	started chan struct{}
	release chan struct{}
}

func (driver *blockingDriver) Deliver(context.Context, DeliveryRequest) DeliveryResult {
	driver.started <- struct{}{}
	<-driver.release
	return DeliveryResult{Delivered: true}
}

func (driver *sequenceDriver) Deliver(_ context.Context, request DeliveryRequest) DeliveryResult {
	driver.mu.Lock()
	defer driver.mu.Unlock()
	driver.seen = append(driver.seen, request)
	if len(driver.results) == 0 {
		return DeliveryResult{Delivered: true}
	}
	result := driver.results[0]
	driver.results = driver.results[1:]
	return result
}

func TestNotificationIntentAdministrationDeliveryAndEvidence(t *testing.T) {
	pool := notificationTestPool(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	seedNotificationDomain(t, pool, now)
	service := NewService(pool, nil, allowValidator{}, SecretResolverFunc(func(context.Context, string) (string, error) {
		return "super-secret-token", nil
	}), func() time.Time { return now })
	definition := DestinationDefinition{DisplayName: "Operations Mattermost", DestinationType: DestinationMattermost,
		Enabled: true, EndpointHost: "chat.example.test", EndpointPort: 443,
		PathPrefix: "/hooks", SecretReference: "operations-webhook"}
	metadata := MutationMetadata{IdempotencyKey: "destination-create", ActorUserID: testActorID,
		ActorName: "Notification Admin", SourceAddress: "192.0.2.10", CorrelationID: "destination-create-request"}
	receipt, err := service.CreateDestination(ctx, definition, metadata)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := service.CreateDestination(ctx, definition, metadata)
	if err != nil || !replay.Replayed || replay.ID != receipt.ID {
		t.Fatalf("destination replay = %#v, %v", replay, err)
	}
	detail, err := service.Destination(ctx, receipt.ID)
	if err != nil || detail.DisplayName != definition.DisplayName || detail.Version != 1 {
		t.Fatalf("redacted destination = %#v, %v", detail, err)
	}
	auditText := ""
	if err := pool.QueryRow(ctx, `SELECT COALESCE(before_summary::text,'') || COALESCE(after_summary::text,'') FROM audit_events WHERE correlation_id=$1`, metadata.CorrelationID).Scan(&auditText); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{definition.EndpointHost, definition.PathPrefix, definition.SecretReference, "super-secret-token"} {
		if secret != "" && contains(auditText, secret) {
			t.Fatalf("audit summary leaked %q: %s", secret, auditText)
		}
	}

	eventID := insertTimeline(t, pool, "detected", now)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	event := incidents.NotificationEvent{TimelineEventID: eventID, IncidentID: testIncidentID,
		RuleID: "20000000-0000-4000-8000-000000000001", ResourceID: testResourceID,
		Kind: "detected", Title: "Node failed", Summary: "Host unreachable", Severity: "critical",
		Status: "open", OccurredAt: now, CreatedAt: now}
	writer := NewIntentWriter()
	if err := writer.EnqueueIncidentEvent(ctx, tx, event); err != nil {
		t.Fatal(err)
	}
	if err := writer.EnqueueIncidentEvent(ctx, tx, event); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if count := countRows(t, pool, `SELECT count(*) FROM notification_intents WHERE incident_event_id=$1`, eventID); count != 1 {
		t.Fatalf("duplicate intent count = %d", count)
	}

	if _, err := pool.Exec(ctx, `INSERT INTO silences(reason,incident_id,starts_at,expires_at,created_by_name)
		VALUES('quiet investigation',$1,$2,$3,'Notification Admin')`, testIncidentID, now.Add(-time.Minute), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	recoveryID := insertTimeline(t, pool, "recovered", now.Add(time.Minute))
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	event.TimelineEventID, event.Kind, event.Status = recoveryID, "recovered", "recovered"
	event.OccurredAt, event.CreatedAt = now.Add(time.Minute), now.Add(time.Minute)
	if err := writer.EnqueueIncidentEvent(ctx, tx, event); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var suppressedState State
	if err := pool.QueryRow(ctx, `SELECT state FROM notification_intents WHERE incident_event_id=$1`, recoveryID).Scan(&suppressedState); err != nil || suppressedState != StateSuppressed {
		t.Fatalf("silenced recovery state = %s, %v", suppressedState, err)
	}
	firstPage, err := service.Deliveries(ctx, DeliveryFilter{IncidentID: testIncidentID, Limit: 1})
	if err != nil || len(firstPage.Items) != 1 || firstPage.NextCursor == "" {
		t.Fatalf("first delivery page = %#v, %v", firstPage, err)
	}
	secondPage, err := service.Deliveries(ctx, DeliveryFilter{IncidentID: testIncidentID, Limit: 1, Cursor: firstPage.NextCursor})
	if err != nil || len(secondPage.Items) != 1 || secondPage.Items[0].ID == firstPage.Items[0].ID {
		t.Fatalf("second delivery page = %#v, %v", secondPage, err)
	}
	if _, err := service.Deliveries(ctx, DeliveryFilter{DestinationID: receipt.ID, Cursor: firstPage.NextCursor}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("filter-mismatched cursor error = %v", err)
	}

	clock := &mutableClock{now: now.Add(2 * time.Minute)}
	driver := &sequenceDriver{results: []DeliveryResult{{Delivered: true, HTTPStatus: 204, ProviderRequestID: "provider-1"}}}
	publicURL, _ := url.Parse("https://espial.example/base")
	worker := NewWorker(pool, nil, WorkerOptions{Clock: clock, MaxAttempts: 6, PublicURL: publicURL,
		Secrets: SecretResolverFunc(func(context.Context, string) (string, error) { return "super-secret-token", nil }),
		Drivers: map[string]Driver{DestinationMattermost: driver}})
	if processed, err := worker.ProcessOnce(ctx); err != nil || !processed {
		t.Fatalf("delivery process = %v, %v", processed, err)
	}
	deliveries, err := service.Deliveries(ctx, DeliveryFilter{IncidentID: testIncidentID})
	if err != nil || len(deliveries.Items) != 2 {
		t.Fatalf("delivery evidence = %#v, %v", deliveries, err)
	}
	if deliveries.Items[1].State != StateDelivered || deliveries.Items[1].AttemptCount != 1 {
		t.Fatalf("delivered evidence = %#v", deliveries.Items[1])
	}
	if len(driver.seen) != 1 || driver.seen[0].Message.EventID != deliveries.Items[1].ID ||
		driver.seen[0].Message.IncidentURL != "https://espial.example/alerts/"+testIncidentID {
		t.Fatalf("driver evidence = %#v", driver.seen)
	}
	if _, err := pool.Exec(ctx, `UPDATE notification_attempts SET outcome='failed'`); err == nil {
		t.Fatal("notification attempt accepted an update")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM notification_attempts`); err == nil {
		t.Fatal("notification attempt accepted a delete")
	}
	metrics, err := service.Metrics(ctx, clock.Now())
	if err != nil || metrics.Delivered != 1 || metrics.Suppressed != 1 || metrics.AttemptTotal != 1 {
		t.Fatalf("notification metrics = %#v, %v", metrics, err)
	}
}

func TestNotificationRetriesDeadLetterAndExpiredClaimRestart(t *testing.T) {
	pool := notificationTestPool(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	seedNotificationDomain(t, pool, now)
	destinationID := insertDestination(t, pool)
	first := insertTestIntent(t, pool, destinationID, now)
	clock := &mutableClock{now: now}
	driver := &sequenceDriver{results: []DeliveryResult{
		{Retryable: true, HTTPStatus: 503, ErrorCode: "provider_unavailable"},
		{Retryable: true, HTTPStatus: 503, ErrorCode: "provider_unavailable"},
	}}
	worker := NewWorker(pool, nil, WorkerOptions{Clock: clock, MaxAttempts: 2, MaxRetryDelay: time.Second,
		Secrets: SecretResolverFunc(func(context.Context, string) (string, error) { return "token", nil }),
		Drivers: map[string]Driver{DestinationMattermost: driver}})
	if processed, err := worker.ProcessOnce(ctx); err != nil || !processed {
		t.Fatalf("first retry = %v, %v", processed, err)
	}
	clock.Add(2 * time.Second)
	if processed, err := worker.ProcessOnce(ctx); err != nil || !processed {
		t.Fatalf("terminal retry = %v, %v", processed, err)
	}
	var state State
	var attempts int
	if err := pool.QueryRow(ctx, `SELECT state,attempt_count FROM notification_intents WHERE id=$1`, first).Scan(&state, &attempts); err != nil || state != StateDeadLetter || attempts != 2 {
		t.Fatalf("dead letter = %s/%d, %v", state, attempts, err)
	}

	second := insertTestIntent(t, pool, destinationID, clock.Now())
	if _, err := pool.Exec(ctx, `UPDATE notification_intents SET state='attempting',attempt_count=2,
		claim_token=gen_random_uuid(),claimed_until=$2 WHERE id=$1`, second, clock.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if processed, err := worker.ProcessOnce(ctx); err != nil || !processed {
		t.Fatalf("restart expiry = %v, %v", processed, err)
	}
	if err := pool.QueryRow(ctx, `SELECT state FROM notification_intents WHERE id=$1`, second).Scan(&state); err != nil || state != StateDeadLetter {
		t.Fatalf("expired claim state = %s, %v", state, err)
	}
	third := insertTestIntent(t, pool, destinationID, clock.Now())
	if _, err := pool.Exec(ctx, `UPDATE notification_intents SET state='attempting',attempt_count=1,
		claim_token=gen_random_uuid(),claimed_until=$2 WHERE id=$1`, third, clock.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	restarted := NewWorker(pool, nil, WorkerOptions{Clock: clock, MaxAttempts: 3,
		Secrets: SecretResolverFunc(func(context.Context, string) (string, error) { return "token", nil }),
		Drivers: map[string]Driver{DestinationMattermost: driver}})
	if processed, err := restarted.ProcessOnce(ctx); err != nil || !processed {
		t.Fatalf("restart recovery = %v, %v", processed, err)
	}
	if err := pool.QueryRow(ctx, `SELECT state,attempt_count FROM notification_intents WHERE id=$1`, third).Scan(&state, &attempts); err != nil || state != StateDelivered || attempts != 2 {
		t.Fatalf("recovered expired claim = %s/%d, %v", state, attempts, err)
	}
}

func TestSlowDestinationDoesNotBlockUnrelatedClaims(t *testing.T) {
	pool := notificationTestPool(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	seedNotificationDomain(t, pool, now)
	destinationID := insertDestination(t, pool)
	insertTestIntent(t, pool, destinationID, now)
	insertTestIntent(t, pool, destinationID, now)
	driver := &blockingDriver{started: make(chan struct{}, 2), release: make(chan struct{})}
	worker := NewWorker(pool, nil, WorkerOptions{Concurrency: 2, PollInterval: time.Millisecond,
		Secrets: SecretResolverFunc(func(context.Context, string) (string, error) { return "token", nil }),
		Drivers: map[string]Driver{DestinationMattermost: driver}})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()
	for range 2 {
		select {
		case <-driver.started:
		case <-time.After(2 * time.Second):
			close(driver.release)
			cancel()
			t.Fatal("slow delivery blocked another worker claim")
		}
	}
	close(driver.release)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("worker shutdown = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("notification worker did not shut down")
	}
}

func seedNotificationDomain(t *testing.T, pool *pgxpool.Pool, now time.Time) {
	t.Helper()
	ctx := context.Background()
	statements := []string{
		`INSERT INTO users(id,username,display_name,identity_provider,enabled,created_at,updated_at) VALUES ('` + testActorID + `','notification-admin','Notification Admin','local',true,$1,$1)`,
		`INSERT INTO integrations(id,adapter_id,display_name,enabled,created_at,updated_at) VALUES ('` + testIntegrationID + `','sample','Test integration',true,$1,$1)`,
		`INSERT INTO resources(id,integration_id,external_id,kind,display_name,first_seen_at,last_seen_at) VALUES ('` + testResourceID + `','` + testIntegrationID + `','notification-node','host','Notification node',$1,$1)`,
		`INSERT INTO incidents(id,rule_id,integration_id,resource_id,check_type,fingerprint,title,summary,severity,status,detected_at,latest_signal_at,created_at,updated_at) VALUES ('` + testIncidentID + `','20000000-0000-4000-8000-000000000001','` + testIntegrationID + `','` + testResourceID + `','availability','notification-test','Node failed','Host unreachable','critical','open',$1,$1,$1,$1)`,
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement, now); err != nil {
			t.Fatal(err)
		}
	}
}

func insertTimeline(t *testing.T, pool *pgxpool.Pool, kind string, occurredAt time.Time) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `INSERT INTO incident_timeline(incident_id,kind,summary,occurred_at)
		VALUES($1,$2,$3,$4) RETURNING id::text`, testIncidentID, kind, "Incident "+kind+".", occurredAt).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func insertDestination(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `INSERT INTO notification_destinations(display_name,endpoint_host,endpoint_path_prefix,secret_reference)
		VALUES('Test destination','chat.example.test','/hooks','test-secret') RETURNING id::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func insertTestIntent(t *testing.T, pool *pgxpool.Pool, destinationID string, now time.Time) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `INSERT INTO notification_intents(destination_id,event_kind,is_test,title,summary,event_occurred_at,state,available_at,created_at,updated_at)
		VALUES($1,'test',true,'Test','Test delivery',$2,'queued',$2,$2,$2) RETURNING id::text`, destinationID, now).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func countRows(t *testing.T, pool *pgxpool.Pool, query string, arguments ...any) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), query, arguments...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func contains(value, substring string) bool {
	for index := 0; index+len(substring) <= len(value); index++ {
		if value[index:index+len(substring)] == substring {
			return true
		}
	}
	return false
}

func notificationTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("ESPIAL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ESPIAL_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	base, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("espial_notifications_%d", time.Now().UnixNano())
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := base.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		base.Close()
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := base.Exec(cleanup, "DROP SCHEMA "+identifier+" CASCADE"); err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("drop schema: %v", err)
		}
		base.Close()
	})
	return pool
}
