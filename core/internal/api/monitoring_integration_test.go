package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/adapters"
	"github.com/PrincepsVIIII/Espial/core/internal/auth"
	"github.com/PrincepsVIIII/Espial/core/internal/events"
	"github.com/PrincepsVIIII/Espial/core/internal/health"
	"github.com/PrincepsVIIII/Espial/core/internal/monitoring"
	"github.com/PrincepsVIIII/Espial/core/internal/observations"
	"github.com/PrincepsVIIII/Espial/core/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAuthenticatedMonitoringFlowThroughDocumentedAPI(t *testing.T) {
	pool := apiIntegrationTestPool(t)
	authOptions := auth.DefaultOptions()
	authOptions.Hasher = auth.PasswordHasher{
		Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 16,
	}
	authService, err := auth.NewService(pool, authOptions)
	if err != nil {
		t.Fatal(err)
	}
	const password = "API monitoring test password 90210"
	if _, err := authService.BootstrapAdmin(context.Background(), "api-admin", password, "bootstrap", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	registry, err := adapters.NewRegistry(adapters.Descriptor{
		AdapterID: "org.ubnetdef.espial.sample", Executable: "/trusted/sample-adapter",
	})
	if err != nil {
		t.Fatal(err)
	}
	hub := events.NewHub(16)
	publicURL, _ := url.Parse("https://espial.test")
	handler := New(Dependencies{
		Logger: discardLogger(), Ready: func(context.Context) error { return nil },
		Auth: authService, PublicURL: publicURL,
		Monitoring:   monitoring.NewReadService(pool),
		Integrations: monitoring.NewIntegrationConfigService(pool, hub, nil, registry),
		Events:       hub,
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	login := apiRequest(t, server.Client(), http.MethodPost, server.URL+"/api/v1/auth/local/login",
		`{"username":"api-admin","password":"`+password+`"}`, nil, "")
	if login.StatusCode != http.StatusOK {
		t.Fatalf("login status=%d body=%s", login.StatusCode, readResponse(t, login))
	}
	cookies := login.Cookies()
	csrf := cookieValue(cookies, csrfCookie)
	if cookieValue(cookies, sessionCookie) == "" || csrf == "" {
		t.Fatalf("login cookies = %#v", cookies)
	}
	login.Body.Close()

	createBody := `{
		"adapter_id":"org.ubnetdef.espial.sample",
		"display_name":"API sample",
		"enabled":true,
		"interval_seconds":60,
		"config_nonsecret":{"scenario":"healthy"},
		"secret_references":{}
	}`
	created := apiRequest(t, server.Client(), http.MethodPost, server.URL+"/api/v1/integrations", createBody, cookies, csrf)
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.StatusCode, readResponse(t, created))
	}
	var createdBody struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(created.Body).Decode(&createdBody); err != nil {
		t.Fatal(err)
	}
	created.Body.Close()
	if createdBody.ID == "" {
		t.Fatal("create response omitted integration id")
	}

	now := time.Now().UTC().Truncate(time.Second)
	ingestor := observations.NewService(pool, observations.Options{Publisher: hub})
	if _, err := ingestor.Ingest(context.Background(), createdBody.ID, observations.Batch{
		Resources: []observations.ResourceInput{{
			ExternalID: "api-node-1", Kind: "host", DisplayName: "API node", ObservedAt: now,
			Attributes: map[string]any{"site": "test"},
		}},
		Observations: []observations.ObservationInput{{
			ExternalResourceID: "api-node-1", CheckType: "availability", State: health.Healthy,
			Summary: "reachable", ObservedAt: now, ExpectedRefreshSeconds: 60,
			Measurements: map[string]any{}, Metadata: map[string]any{},
		}},
	}); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{
		"/api/v1/overview",
		"/api/v1/resources?state=healthy&kind=host&limit=1",
		"/api/v1/integrations?adapter_id=org.ubnetdef.espial.sample&limit=1",
		"/api/v1/audit?action=integration.created&limit=10",
	} {
		response := apiRequest(t, server.Client(), http.MethodGet, server.URL+path, "", cookies, "")
		if response.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status=%d body=%s", path, response.StatusCode, readResponse(t, response))
		}
		var payload map[string]any
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		response.Body.Close()
		if len(payload) == 0 {
			t.Fatalf("GET %s returned an empty document", path)
		}
	}
}

func apiRequest(t *testing.T, client *http.Client, method, target, body string, cookies []*http.Cookie, csrf string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, target, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", "https://espial.test")
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if csrf != "" {
		request.Header.Set("X-CSRF-Token", csrf)
	}
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func cookieValue(cookies []*http.Cookie, name string) string {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie.Value
		}
	}
	return ""
}

func readResponse(t *testing.T, response *http.Response) string {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func apiIntegrationTestPool(t *testing.T) *pgxpool.Pool {
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
	if err := base.Ping(ctx); err != nil {
		base.Close()
		t.Fatal(err)
	}
	schema := fmt.Sprintf("espial_api_test_%d", time.Now().UnixNano())
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := base.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		base.Close()
		t.Fatal(err)
	}
	configuration, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		base.Close()
		t.Fatal(err)
	}
	configuration.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, configuration)
	if err != nil {
		base.Close()
		t.Fatal(err)
	}
	if err := storage.Migrate(ctx, pool); err != nil {
		pool.Close()
		base.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := base.Exec(cleanup, "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Errorf("drop schema: %v", err)
		}
		base.Close()
	})
	return pool
}
