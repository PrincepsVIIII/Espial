package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/PrincepsVIIII/Espial/core/internal/adminops"
	"github.com/PrincepsVIIII/Espial/core/internal/auth"
	"github.com/PrincepsVIIII/Espial/core/internal/incidents"
	"github.com/PrincepsVIIII/Espial/core/internal/suppressions"
)

const administrativeTestID = "90000000-0000-4000-8000-000000000031"

type fakeRuleManager struct {
	write   incidents.RuleWrite
	receipt adminops.Receipt
}

func (fake *fakeRuleManager) List(context.Context) (incidents.RuleList, error) {
	return incidents.RuleList{Items: []incidents.RuleView{}}, nil
}
func (fake *fakeRuleManager) Detail(context.Context, string) (incidents.RuleView, error) {
	return incidents.RuleView{ID: administrativeTestID, Version: 2}, nil
}
func (fake *fakeRuleManager) Create(_ context.Context, input incidents.RuleWrite) (adminops.Receipt, error) {
	fake.write = input
	return fake.receipt, nil
}
func (fake *fakeRuleManager) Replace(_ context.Context, _ string, input incidents.RuleWrite) (adminops.Receipt, error) {
	fake.write = input
	return fake.receipt, nil
}
func (fake *fakeRuleManager) Preview(context.Context, incidents.RulePreviewInput) (incidents.RulePreview, error) {
	return incidents.RulePreview{Candidates: []incidents.RulePreviewCandidate{}, Explanation: "No enabled incident rule matches this normalized signal."}, nil
}

type fakeSuppressionManager struct {
	metadata suppressions.MutationMetadata
	receipt  adminops.Receipt
}

func (fake *fakeSuppressionManager) MaintenanceWindows(context.Context) (suppressions.MaintenanceList, error) {
	return suppressions.MaintenanceList{Items: []suppressions.MaintenanceWindow{}}, nil
}
func (fake *fakeSuppressionManager) MaintenanceWindow(context.Context, string) (suppressions.MaintenanceWindow, error) {
	return suppressions.MaintenanceWindow{ID: administrativeTestID, Version: 2}, nil
}
func (fake *fakeSuppressionManager) CreateMaintenance(_ context.Context, _ suppressions.MaintenanceDefinition, metadata suppressions.MutationMetadata) (adminops.Receipt, error) {
	fake.metadata = metadata
	return fake.receipt, nil
}
func (fake *fakeSuppressionManager) ReplaceMaintenance(_ context.Context, _ string, _ suppressions.MaintenanceDefinition, metadata suppressions.MutationMetadata) (adminops.Receipt, error) {
	fake.metadata = metadata
	return fake.receipt, nil
}
func (fake *fakeSuppressionManager) RevokeMaintenance(_ context.Context, _ string, metadata suppressions.MutationMetadata) (adminops.Receipt, error) {
	fake.metadata = metadata
	return fake.receipt, nil
}
func (fake *fakeSuppressionManager) Silences(context.Context) (suppressions.SilenceList, error) {
	return suppressions.SilenceList{Items: []suppressions.Silence{}}, nil
}
func (fake *fakeSuppressionManager) Silence(context.Context, string) (suppressions.Silence, error) {
	return suppressions.Silence{ID: administrativeTestID, Version: 2}, nil
}
func (fake *fakeSuppressionManager) CreateSilence(_ context.Context, _ suppressions.SilenceDefinition, metadata suppressions.MutationMetadata) (adminops.Receipt, error) {
	fake.metadata = metadata
	return fake.receipt, nil
}
func (fake *fakeSuppressionManager) ReplaceSilence(_ context.Context, _ string, _ suppressions.SilenceDefinition, metadata suppressions.MutationMetadata) (adminops.Receipt, error) {
	fake.metadata = metadata
	return fake.receipt, nil
}
func (fake *fakeSuppressionManager) RevokeSilence(_ context.Context, _ string, metadata suppressions.MutationMetadata) (adminops.Receipt, error) {
	fake.metadata = metadata
	return fake.receipt, nil
}

func administrationHandler(authService AuthService, rules IncidentRuleManager, controls SuppressionManager) http.Handler {
	publicURL, _ := url.Parse("https://espial.test")
	return New(Dependencies{Logger: discardLogger(), Ready: func(context.Context) error { return nil }, Auth: authService, PublicURL: publicURL, IncidentRules: rules, Suppressions: controls})
}

func administrativeRequest(method, path, body string) *http.Request {
	request := authenticatedRequest(method, path, strings.NewReader(body))
	request.Header.Set("Origin", "https://espial.test")
	request.Header.Set("X-CSRF-Token", "csrf-secret")
	request.AddCookie(&http.Cookie{Name: csrfCookie, Value: "csrf-secret"})
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "administrative-test-key")
	request.Header.Set("If-Match", `"v2"`)
	request.Header.Set("X-Request-ID", "administrative-request")
	return request
}

func TestAdministrativeRuleMutationIsPermissionGatedAndReturnsAuditReceipt(t *testing.T) {
	rules := &fakeRuleManager{receipt: adminops.Receipt{ID: administrativeTestID, Version: 3, RequestID: "administrative-request"}}
	user := &fakeAuth{session: auth.Session{User: auth.User{ID: "operator", DisplayName: "Operator", Permissions: []string{"incidents:operate"}}}}
	handler := administrationHandler(user, rules, nil)
	body := `{"name":"Exact rule","enabled":true,"priority":100,"conditions":[{"state":"critical","severity":"critical","min_occurrences":1,"for_seconds":0}],"recovery_state":"healthy","recovery_min_occurrences":1,"recovery_for_seconds":0}`
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, administrativeRequest(http.MethodPost, "/api/v1/incident-rules", body))
	if response.Code != http.StatusForbidden || !user.denied {
		t.Fatalf("operator rule mutation = %d audited=%v", response.Code, user.denied)
	}
	user.session.User.ID = "administrator"
	user.session.User.DisplayName = "Administrator"
	user.session.User.Permissions = []string{"incident_rules:manage", "audit:read"}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, administrativeRequest(http.MethodPost, "/api/v1/incident-rules", body))
	if response.Code != http.StatusCreated || response.Header().Get("ETag") != `"v3"` || !strings.Contains(response.Body.String(), `"audit_url":"/audit?correlation_id=administrative-request"`) {
		t.Fatalf("administrator rule receipt = %d %q %s", response.Code, response.Header().Get("ETag"), response.Body.String())
	}
	if rules.write.ActorName != "Administrator" || rules.write.IdempotencyKey != "administrative-test-key" {
		t.Fatalf("rule mutation metadata = %#v", rules.write)
	}
}

func TestSuppressionMutationRequiresConcurrencyAndIdempotencyHeaders(t *testing.T) {
	controls := &fakeSuppressionManager{receipt: adminops.Receipt{ID: administrativeTestID, Version: 3, RequestID: "administrative-request"}}
	user := &fakeAuth{session: auth.Session{User: auth.User{ID: "administrator", DisplayName: "Administrator", Permissions: []string{"suppressions:manage", "audit:read"}}}}
	handler := administrationHandler(user, nil, controls)
	body := `{"reason":"Planned work","resource_id":"90000000-0000-4000-8000-000000000032","starts_at":"2026-07-31T12:00:00Z","ends_at":"2026-07-31T13:00:00Z","enabled":true}`
	request := administrativeRequest(http.MethodPut, "/api/v1/maintenance-windows/"+administrativeTestID, body)
	request.Header.Del("If-Match")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusPreconditionRequired {
		t.Fatalf("missing precondition = %d %s", response.Code, response.Body.String())
	}
	request = administrativeRequest(http.MethodPut, "/api/v1/maintenance-windows/"+administrativeTestID, body)
	request.Header.Del("Idempotency-Key")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusPreconditionRequired {
		t.Fatalf("missing idempotency = %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, administrativeRequest(http.MethodPut, "/api/v1/maintenance-windows/"+administrativeTestID, body))
	if response.Code != http.StatusOK || controls.metadata.ExpectedVersion != 2 || !strings.Contains(response.Body.String(), "audit_url") {
		t.Fatalf("maintenance receipt = %d metadata=%#v %s", response.Code, controls.metadata, response.Body.String())
	}
}
