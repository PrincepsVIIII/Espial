package certificateprojection

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/health"
	"github.com/PrincepsVIIII/Espial/core/internal/observations"
	"github.com/PrincepsVIIII/Espial/core/internal/storage"
	"github.com/PrincepsVIIII/Espial/core/internal/webcheck"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const projectionIntegrationID = "60000000-0000-4000-8000-000000000026"

func TestProjectionRecordsReplacementAndIssuerChangeWithoutSecrets(t *testing.T) {
	pool := projectionTestPool(t)
	_, err := pool.Exec(context.Background(), `INSERT INTO integrations(id,adapter_id,display_name,enabled) VALUES($1,$2,'Certificate test',true)`, projectionIntegrationID, webcheck.AdapterID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	service := observations.NewService(pool, observations.Options{Clock: health.FixedClock{Time: now}})
	for index, evidence := range []map[string]any{
		{"reason_code": "certificate_valid", "endpoint": "status.test:443", "subject": "CN=status.test", "san_summary": "status.test", "issuer": "CN=Issuer A", "serial_number": "1", "fingerprint_sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "not_before": now.Add(-time.Hour).Format(time.RFC3339Nano), "not_after": now.Add(60 * 24 * time.Hour).Format(time.RFC3339Nano), "hostname_valid": true, "chain_valid": true},
		{"reason_code": "certificate_valid", "endpoint": "status.test:443", "subject": "CN=status.test", "san_summary": "status.test", "issuer": "CN=Issuer B", "serial_number": "2", "fingerprint_sha256": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "not_before": now.Add(-time.Hour).Format(time.RFC3339Nano), "not_after": now.Add(90 * 24 * time.Hour).Format(time.RFC3339Nano), "hostname_valid": true, "chain_valid": true},
	} {
		at := now.Add(time.Duration(index) * time.Minute)
		batch := observations.Batch{Resources: []observations.ResourceInput{{ExternalID: "certificate:status.test:443", Kind: webcheck.CertificateResourceKind, DisplayName: "status.test:443", ObservedAt: at, Attributes: map[string]any{"source": "webcheck"}, SourceURL: "https://status.test/"}}, Observations: []observations.ObservationInput{{ExternalResourceID: "certificate:status.test:443", CheckType: webcheck.CertificateCheckType, State: health.Healthy, Summary: "Certificate valid.", ObservedAt: at, ExpectedRefreshSeconds: 60, Measurements: map[string]any{"days_remaining": 60}, Metadata: evidence}}}
		_, err = service.IngestWithCommit(context.Background(), projectionIntegrationID, batch, func(ctx context.Context, tx pgx.Tx, _ observations.Result) error {
			return ProjectBatch(ctx, tx, projectionIntegrationID, batch)
		})
		if err != nil {
			t.Fatalf("ingest %d: %v", index, err)
		}
	}
	var count int
	var fingerprintChanged, issuerChanged bool
	var metadata string
	err = pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM certificate_observations),fingerprint_changed,issuer_changed,o.metadata::text FROM certificate_observations c JOIN observations o ON o.id=c.observation_id ORDER BY c.observed_at DESC LIMIT 1`).Scan(&count, &fingerprintChanged, &issuerChanged, &metadata)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 || !fingerprintChanged || !issuerChanged {
		t.Fatalf("count=%d fingerprint=%v issuer=%v", count, fingerprintChanged, issuerChanged)
	}
	if containsAny(metadata, []string{"private_key", "secret_header", "BEGIN CERTIFICATE"}) {
		t.Fatalf("unbounded or secret evidence persisted: %s", metadata)
	}
}

func containsAny(value string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func projectionTestPool(t *testing.T) *pgxpool.Pool {
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
	schema := fmt.Sprintf("espial_certificate_projection_test_%d", time.Now().UnixNano())
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err = base.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
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
	if err = storage.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_, _ = base.Exec(cleanup, "DROP SCHEMA "+identifier+" CASCADE")
		base.Close()
	})
	return pool
}
