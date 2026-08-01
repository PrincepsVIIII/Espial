package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/adminops"
	"github.com/PrincepsVIIII/Espial/core/internal/auth"
	"github.com/PrincepsVIIII/Espial/core/internal/health"
	"github.com/PrincepsVIIII/Espial/core/internal/webpages"
)

type fakeWebsiteManager struct {
	definition webpages.MonitorDefinition
	metadata   webpages.MutationMetadata
	receipt    adminops.Receipt
}

func (fake *fakeWebsiteManager) Monitors(context.Context, webpages.ListFilter) (webpages.MonitorList, error) {
	return webpages.MonitorList{Items: []webpages.Monitor{{
		ID: administrativeTestID, DisplayName: "Status", Enabled: true,
		URL: "https://status.example.invalid/", IntervalSeconds: 60, TimeoutMS: 5000,
		AllowedStatuses: []int{200}, SecretHeaderNames: []string{"Authorization"},
		RuntimeState: "healthy", Version: 2, CreatedAt: time.Unix(1, 0), UpdatedAt: time.Unix(2, 0),
	}}}, nil
}
func (fake *fakeWebsiteManager) Monitor(context.Context, string) (webpages.Monitor, error) {
	items, _ := fake.Monitors(context.Background(), webpages.ListFilter{})
	return items.Items[0], nil
}
func (fake *fakeWebsiteManager) Create(_ context.Context, definition webpages.MonitorDefinition, metadata webpages.MutationMetadata) (adminops.Receipt, error) {
	fake.definition, fake.metadata = definition, metadata
	return fake.receipt, nil
}
func (fake *fakeWebsiteManager) Replace(_ context.Context, _ string, definition webpages.MonitorDefinition, metadata webpages.MutationMetadata) (adminops.Receipt, error) {
	fake.definition, fake.metadata = definition, metadata
	return fake.receipt, nil
}
func (fake *fakeWebsiteManager) Check(_ context.Context, _ string, metadata webpages.MutationMetadata) (adminops.Receipt, error) {
	fake.metadata = metadata
	return fake.receipt, nil
}
func (fake *fakeWebsiteManager) Webpages(context.Context, webpages.ListFilter) (webpages.List, error) {
	return webpages.List{Items: []webpages.Summary{{
		ID: administrativeTestID, MonitorID: administrativeTestID,
		DisplayName: "status.example.invalid", URL: "https://status.example.invalid/",
		State: health.Healthy, RawState: health.Healthy, Reason: "Available.",
		UpdatedAt: time.Unix(2, 0), Stages: webpages.Stages{Completed: []string{"dns", "tcp", "tls", "http", "body"}},
	}}}, nil
}
func (fake *fakeWebsiteManager) Webpage(context.Context, string) (webpages.Detail, error) {
	items, _ := fake.Webpages(context.Background(), webpages.ListFilter{})
	return webpages.Detail{Summary: items.Items[0], FirstSeenAt: time.Unix(1, 0), LastSeenAt: time.Unix(2, 0)}, nil
}

func websiteHandler(user *fakeAuth, manager WebsiteManager) http.Handler {
	publicURL, _ := url.Parse("https://espial.test")
	return New(Dependencies{Logger: discardLogger(), Ready: func(context.Context) error { return nil },
		Auth: user, PublicURL: publicURL, Websites: manager})
}

func TestWebsiteReadAndAdministrationPermissions(t *testing.T) {
	manager := &fakeWebsiteManager{receipt: adminops.Receipt{ID: administrativeTestID, Version: 3, RequestID: "administrative-request"}}
	user := &fakeAuth{session: auth.Session{User: auth.User{ID: "viewer", Permissions: []string{"webpages:read"}}}}
	handler := websiteHandler(user, manager)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/api/v1/webpages", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "status.example.invalid") {
		t.Fatalf("viewer webpage read=%d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/api/v1/website-monitors", nil))
	if response.Code != http.StatusForbidden || !user.denied {
		t.Fatalf("viewer monitor read=%d audited=%v", response.Code, user.denied)
	}
	user.session.User = auth.User{ID: "administrator", DisplayName: "Administrator", Permissions: []string{"website_monitors:manage", "audit:read"}}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/api/v1/website-monitors", nil))
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "secret_reference") {
		t.Fatalf("monitor redaction=%d %s", response.Code, response.Body.String())
	}
	body := `{"display_name":"Status","enabled":true,"url":"https://status.example.invalid/","interval_seconds":60,"timeout_ms":5000,"warning_latency_ms":1000,"allowed_statuses":[200],"content_match":"ready","follow_redirects":false,"max_redirects":0,"secret_headers":[{"name":"Authorization","secret_reference":"status-auth"}]}`
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, administrativeRequest(http.MethodPost, "/api/v1/website-monitors", body))
	if response.Code != http.StatusCreated || response.Header().Get("ETag") != `"v3"` || !strings.Contains(response.Body.String(), "audit_url") {
		t.Fatalf("monitor create=%d %s", response.Code, response.Body.String())
	}
	if manager.definition.SecretHeaders[0].SecretReference != "status-auth" || manager.metadata.ActorName != "Administrator" {
		t.Fatalf("mutation=%#v %#v", manager.definition, manager.metadata)
	}
}
