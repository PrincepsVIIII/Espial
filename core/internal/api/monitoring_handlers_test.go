package api

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/auth"
	"github.com/PrincepsVIIII/Espial/core/internal/events"
	"github.com/PrincepsVIIII/Espial/core/internal/monitoring"
)

type fakeMonitoring struct {
	overview        monitoring.Overview
	resources       monitoring.ResourceList
	resource        monitoring.ResourceView
	integrations    monitoring.IntegrationList
	integration     monitoring.IntegrationView
	audit           monitoring.AuditList
	err             error
	auditReadCalled bool
}

func (fake *fakeMonitoring) Overview(context.Context) (monitoring.Overview, error) {
	return fake.overview, fake.err
}
func (fake *fakeMonitoring) Resources(context.Context, monitoring.ResourceFilter) (monitoring.ResourceList, error) {
	return fake.resources, fake.err
}
func (fake *fakeMonitoring) Resource(context.Context, string) (monitoring.ResourceView, error) {
	return fake.resource, fake.err
}
func (fake *fakeMonitoring) Integrations(context.Context, monitoring.IntegrationFilter) (monitoring.IntegrationList, error) {
	return fake.integrations, fake.err
}
func (fake *fakeMonitoring) Integration(context.Context, string) (monitoring.IntegrationView, error) {
	return fake.integration, fake.err
}
func (fake *fakeMonitoring) Audit(context.Context, monitoring.AuditFilter) (monitoring.AuditList, error) {
	return fake.audit, fake.err
}
func (fake *fakeMonitoring) RecordAuditRead(context.Context, string, string, string, monitoring.AuditFilter) error {
	fake.auditReadCalled = true
	return fake.err
}

type fakeIntegrationManager struct {
	createID string
	updated  time.Time
	err      error
}

func (fake *fakeIntegrationManager) Create(context.Context, monitoring.CreateIntegration) (string, time.Time, error) {
	return fake.createID, fake.updated, fake.err
}
func (fake *fakeIntegrationManager) Update(context.Context, monitoring.IntegrationConfigUpdate) (time.Time, error) {
	return fake.updated, fake.err
}

func monitoringHandler(authService AuthService, reader MonitoringReader, manager IntegrationManager, hub EventSource, heartbeat time.Duration) http.Handler {
	publicURL, _ := url.Parse("https://espial.test")
	return New(Dependencies{
		Logger: discardLogger(), Ready: func(context.Context) error { return nil },
		Auth: authService, PublicURL: publicURL, SecureCookies: true,
		Monitoring: reader, Integrations: manager, Events: hub,
		SSEHeartbeat: heartbeat, SSEMaxClients: 2,
		Now: func() time.Time { return time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC) },
	})
}

func authenticatedRequest(method, target string, body io.Reader) *http.Request {
	request := httptest.NewRequest(method, target, body)
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: "session-secret"})
	return request
}

func TestMonitoringReadsEnforceAuthenticationPermissionAndValidation(t *testing.T) {
	reader := &fakeMonitoring{overview: monitoring.Overview{StaleCount: 2}}
	viewer := &fakeAuth{session: auth.Session{User: auth.User{
		ID: "viewer", Permissions: []string{"overview:read", "resources:read", "integrations:read"},
	}}}
	handler := monitoringHandler(viewer, reader, nil, nil, time.Hour)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/overview", nil))
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "request_id") {
		t.Fatalf("unauthenticated response = %d %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/api/v1/overview", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"stale_count":2`) {
		t.Fatalf("overview response = %d %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/api/v1/resources?limit=201&unknown=yes", nil))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"fields"`) ||
		!strings.Contains(response.Body.String(), `"request_id"`) {
		t.Fatalf("validation response = %d %s", response.Code, response.Body.String())
	}

	viewer.session.User.Permissions = []string{"overview:read"}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/api/v1/resources", nil))
	if response.Code != http.StatusForbidden || !viewer.denied {
		t.Fatalf("permission response = %d audited=%v", response.Code, viewer.denied)
	}
}

func TestMonitoringDetailsReturnSafe404And500(t *testing.T) {
	reader := &fakeMonitoring{err: monitoring.ErrNotFound}
	viewer := &fakeAuth{session: auth.Session{User: auth.User{Permissions: []string{"resources:read"}}}}
	handler := monitoringHandler(viewer, reader, nil, nil, time.Hour)
	id := "60000000-0000-4000-8000-000000000001"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/api/v1/resources/"+id, nil))
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"not_found"`) {
		t.Fatalf("not found = %d %s", response.Code, response.Body.String())
	}

	reader.err = errors.New("postgres secret-value query failed")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/api/v1/resources/"+id, nil))
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "postgres") || strings.Contains(response.Body.String(), "secret-value") {
		t.Fatalf("unsafe internal error = %d %s", response.Code, response.Body.String())
	}
}

func TestAuditReadIsAdministratorOnlyAndRecorded(t *testing.T) {
	reader := &fakeMonitoring{}
	administrator := &fakeAuth{session: auth.Session{User: auth.User{
		ID: "70000000-0000-4000-8000-000000000001", Permissions: []string{"audit:read"},
	}}}
	handler := monitoringHandler(administrator, reader, nil, nil, time.Hour)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/api/v1/audit?limit=10", nil))
	if response.Code != http.StatusOK || !reader.auditReadCalled {
		t.Fatalf("audit read = %d recorded=%v body=%s", response.Code, reader.auditReadCalled, response.Body.String())
	}
}

func TestIntegrationMutationsCoverConflictPreconditionAndBodyLimits(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	administrator := &fakeAuth{session: auth.Session{CSRFDigest: []byte("digest"), User: auth.User{
		ID: "70000000-0000-4000-8000-000000000001", Permissions: []string{"integrations:manage"},
	}}}
	manager := &fakeIntegrationManager{err: &monitoring.Error{Code: "adapter_not_registered"}}
	handler := monitoringHandler(administrator, &fakeMonitoring{}, manager, nil, time.Hour)
	missing := authenticatedRequest(http.MethodPost, "/api/v1/integrations", bytes.NewBufferString(`{
		"adapter_id":"unknown.adapter","display_name":"Unknown","interval_seconds":60
	}`))
	addMutationHeaders(missing)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, missing)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"enabled"`) {
		t.Fatalf("create required fields = %d %s", response.Code, response.Body.String())
	}

	create := authenticatedRequest(http.MethodPost, "/api/v1/integrations", bytes.NewBufferString(`{
		"adapter_id":"unknown.adapter","display_name":"Unknown","enabled":true,
		"interval_seconds":60,"config_nonsecret":{},"secret_references":{}
	}`))
	addMutationHeaders(create)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, create)
	if response.Code != http.StatusConflict {
		t.Fatalf("create conflict = %d %s", response.Code, response.Body.String())
	}

	id := "60000000-0000-4000-8000-000000000001"
	manager.err = &monitoring.Error{Code: "integration_config_conflict"}
	update := authenticatedRequest(http.MethodPut, "/api/v1/integrations/"+id+"/configuration", bytes.NewBufferString(`{
		"enabled":true,"interval_seconds":60,"config_nonsecret":{},"secret_references":{}
	}`))
	addMutationHeaders(update)
	update.Header.Set("If-Match", integrationETag(now))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, update)
	if response.Code != http.StatusPreconditionFailed {
		t.Fatalf("update precondition = %d %s", response.Code, response.Body.String())
	}

	manager.err = nil
	oversized := authenticatedRequest(http.MethodPost, "/api/v1/integrations", strings.NewReader(`{"adapter_id":"`+strings.Repeat("x", 140*1024)+`"}`))
	addMutationHeaders(oversized)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, oversized)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body = %d %s", response.Code, response.Body.String())
	}
}

func addMutationHeaders(request *http.Request) {
	request.Header.Set("Origin", "https://espial.test")
	request.Header.Set("X-CSRF-Token", "csrf-secret")
	request.AddCookie(&http.Cookie{Name: csrfCookie, Value: "csrf-secret"})
}

type streamAuth struct {
	mu                 sync.Mutex
	session            auth.Session
	authenticateErrors []error
	calls              int
}

func (fake *streamAuth) Login(context.Context, string, string, string, string) (auth.Session, error) {
	return fake.session, nil
}
func (fake *streamAuth) Authenticate(context.Context, string) (auth.Session, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	index := fake.calls
	fake.calls++
	if index < len(fake.authenticateErrors) && fake.authenticateErrors[index] != nil {
		return auth.Session{}, fake.authenticateErrors[index]
	}
	return fake.session, nil
}
func (*streamAuth) VerifyCSRF(auth.Session, string) bool                                  { return true }
func (*streamAuth) Logout(context.Context, auth.Session, string, string) error            { return nil }
func (*streamAuth) RecordDenied(context.Context, auth.User, string, string, string) error { return nil }

func TestSSEReconnectReplayAndOldCursorResync(t *testing.T) {
	for _, test := range []struct {
		name        string
		replayLimit int
		lastID      string
		want        string
	}{
		{"replay", 4, "1", "event: integration_changed"},
		{"resync", 1, "0", "event: resync_required"},
	} {
		t.Run(test.name, func(t *testing.T) {
			hub := events.NewHub(test.replayLimit)
			now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
			hub.Publish(events.Event{Kind: events.CollectionChanged, IntegrationID: "one", ChangedAt: now})
			hub.Publish(events.Event{Kind: events.IntegrationChanged, IntegrationID: "two", ChangedAt: now})
			authService := &streamAuth{session: auth.Session{User: auth.User{Permissions: []string{"overview:read"}}}}
			handler := monitoringHandler(authService, &fakeMonitoring{}, nil, hub, time.Hour)
			server := httptest.NewServer(handler)
			defer server.Close()
			ctx, cancel := context.WithCancel(context.Background())
			request, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/events/stream", nil)
			request.AddCookie(&http.Cookie{Name: sessionCookie, Value: "session-secret"})
			request.Header.Set("Last-Event-ID", test.lastID)
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			reader := bufio.NewReader(response.Body)
			var content strings.Builder
			deadline := time.Now().Add(time.Second)
			for (!strings.Contains(content.String(), test.want) || !strings.Contains(content.String(), `"schema_version":1`)) && time.Now().Before(deadline) {
				line, readErr := reader.ReadString('\n')
				content.WriteString(line)
				if readErr != nil {
					break
				}
			}
			cancel()
			response.Body.Close()
			if response.StatusCode != http.StatusOK || !strings.Contains(content.String(), test.want) || !strings.Contains(content.String(), `"schema_version":1`) {
				t.Fatalf("SSE response = %d %q", response.StatusCode, content.String())
			}
		})
	}
}

func TestSSERevokedSessionClosesWithinHeartbeat(t *testing.T) {
	hub := events.NewHub(2)
	authService := &streamAuth{
		session:            auth.Session{User: auth.User{Permissions: []string{"overview:read"}}},
		authenticateErrors: []error{nil, auth.ErrSessionNotFound},
	}
	handler := monitoringHandler(authService, &fakeMonitoring{}, nil, hub, 10*time.Millisecond)
	server := httptest.NewServer(handler)
	defer server.Close()
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/events/stream", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: "session-secret"})
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	completed := make(chan error, 1)
	go func() {
		_, readErr := io.ReadAll(response.Body)
		completed <- readErr
	}()
	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("revoked SSE session remained open beyond heartbeat")
	}
	response.Body.Close()
}

func TestSSEConcurrentClientLimitIsBounded(t *testing.T) {
	hub := events.NewHub(2)
	authService := &streamAuth{session: auth.Session{User: auth.User{Permissions: []string{"overview:read"}}}}
	publicURL, _ := url.Parse("https://espial.test")
	handler := New(Dependencies{
		Logger: discardLogger(), Ready: func(context.Context) error { return nil },
		Auth: authService, PublicURL: publicURL, Monitoring: &fakeMonitoring{}, Events: hub,
		SSEHeartbeat: time.Hour, SSEMaxClients: 1,
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	firstRequest, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/events/stream", nil)
	firstRequest.AddCookie(&http.Cookie{Name: sessionCookie, Value: "session-secret"})
	first, err := server.Client().Do(firstRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Body.Close()

	secondRequest, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/events/stream", nil)
	secondRequest.AddCookie(&http.Cookie{Name: sessionCookie, Value: "session-secret"})
	second, err := server.Client().Do(secondRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Body.Close()
	if second.StatusCode != http.StatusServiceUnavailable || second.Header.Get("Retry-After") != "15" {
		t.Fatalf("second stream = %d retry-after=%q", second.StatusCode, second.Header.Get("Retry-After"))
	}
}

func FuzzResourceQueryParser(f *testing.F) {
	f.Add("limit=50&state=healthy")
	f.Add("cursor=not-valid&kind=host")
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > maxQueryBytes {
			return
		}
		values, err := url.ParseQuery(raw)
		if err != nil {
			return
		}
		_, _ = parseResourceFilter(values)
	})
}

func FuzzSSELastEventID(f *testing.F) {
	f.Add("1")
	f.Add("not-a-number")
	f.Fuzz(func(t *testing.T, value string) {
		if len(value) > 128 {
			return
		}
		hub := events.NewHub(2)
		authService := &streamAuth{session: auth.Session{User: auth.User{Permissions: []string{"overview:read"}}}}
		handler := monitoringHandler(authService, &fakeMonitoring{}, nil, hub, time.Millisecond)
		request := authenticatedRequest(http.MethodGet, "/api/v1/events/stream", nil)
		request.Header.Set("Last-Event-ID", value)
		ctx, cancel := context.WithCancel(request.Context())
		cancel()
		request = request.WithContext(ctx)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
	})
}
