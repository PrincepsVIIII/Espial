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
	"github.com/PrincepsVIIII/Espial/core/internal/notifications"
)

type fakeNotificationManager struct {
	definition notifications.DestinationDefinition
	metadata   notifications.MutationMetadata
	receipt    adminops.Receipt
}

func (fake *fakeNotificationManager) Destinations(context.Context, notifications.DestinationFilter) (notifications.DestinationList, error) {
	return notifications.DestinationList{Items: []notifications.Destination{{ID: administrativeTestID,
		DisplayName: "Operations Mattermost", DestinationType: notifications.DestinationMattermost,
		Enabled: true, Version: 2, CreatedAt: time.Unix(1, 0), UpdatedAt: time.Unix(2, 0)}}}, nil
}
func (fake *fakeNotificationManager) Destination(context.Context, string) (notifications.Destination, error) {
	return notifications.Destination{ID: administrativeTestID, DisplayName: "Operations Mattermost",
		DestinationType: notifications.DestinationMattermost, Enabled: true, Version: 2,
		CreatedAt: time.Unix(1, 0), UpdatedAt: time.Unix(2, 0)}, nil
}
func (fake *fakeNotificationManager) CreateDestination(_ context.Context, definition notifications.DestinationDefinition, metadata notifications.MutationMetadata) (adminops.Receipt, error) {
	fake.definition, fake.metadata = definition, metadata
	return fake.receipt, nil
}
func (fake *fakeNotificationManager) ReplaceDestination(_ context.Context, _ string, definition notifications.DestinationDefinition, metadata notifications.MutationMetadata) (adminops.Receipt, error) {
	fake.definition, fake.metadata = definition, metadata
	return fake.receipt, nil
}
func (fake *fakeNotificationManager) TestDestination(_ context.Context, _ string, metadata notifications.MutationMetadata) (adminops.Receipt, error) {
	fake.metadata = metadata
	return fake.receipt, nil
}
func (fake *fakeNotificationManager) Deliveries(context.Context, notifications.DeliveryFilter) (notifications.DeliveryList, error) {
	return notifications.DeliveryList{Items: []notifications.Delivery{{ID: administrativeTestID,
		DestinationID: administrativeTestID, DestinationName: "Operations Mattermost",
		DestinationType: notifications.DestinationMattermost, EventKind: "test", Test: true,
		State: notifications.StateRetryWait, AttemptCount: 2, EventOccurredAt: time.Unix(1, 0),
		AvailableAt: time.Unix(2, 0), LastErrorCode: "provider_unavailable",
		CreatedAt: time.Unix(1, 0), UpdatedAt: time.Unix(2, 0)}}}, nil
}

func notificationHandler(user *fakeAuth, manager NotificationManager) http.Handler {
	publicURL, _ := url.Parse("https://espial.test")
	return New(Dependencies{Logger: discardLogger(), Ready: func(context.Context) error { return nil },
		Auth: user, PublicURL: publicURL, Notifications: manager})
}

func TestNotificationAdministrationPermissionRedactionAndReceipt(t *testing.T) {
	manager := &fakeNotificationManager{receipt: adminops.Receipt{ID: administrativeTestID,
		Version: 3, RequestID: "administrative-request"}}
	user := &fakeAuth{session: auth.Session{User: auth.User{ID: "viewer", Permissions: []string{"incidents:read"}}}}
	handler := notificationHandler(user, manager)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/api/v1/notification-destinations", nil))
	if response.Code != http.StatusForbidden || !user.denied {
		t.Fatalf("viewer destination read = %d audited=%v", response.Code, user.denied)
	}

	user.session.User = auth.User{ID: "administrator", DisplayName: "Administrator",
		Permissions: []string{"notification_destinations:manage", "audit:read"}}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/api/v1/notification-destinations", nil))
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "endpoint") ||
		strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("redacted destination read = %d %s", response.Code, response.Body.String())
	}

	body := `{"display_name":"Operations Mattermost","destination_type":"mattermost","enabled":true,"endpoint_host":"chat.example.test","endpoint_port":443,"path_prefix":"/hooks","secret_reference":"operations-webhook"}`
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, administrativeRequest(http.MethodPost, "/api/v1/notification-destinations", body))
	if response.Code != http.StatusCreated || response.Header().Get("ETag") != `"v3"` ||
		!strings.Contains(response.Body.String(), "audit_url") {
		t.Fatalf("destination create = %d %q %s", response.Code, response.Header().Get("ETag"), response.Body.String())
	}
	if manager.definition.SecretReference != "operations-webhook" || manager.metadata.ActorName != "Administrator" {
		t.Fatalf("destination mutation = %#v %#v", manager.definition, manager.metadata)
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, administrativeRequest(http.MethodPost,
		"/api/v1/notification-destinations/"+administrativeTestID+"/test", `{}`))
	if response.Code != http.StatusAccepted || manager.metadata.ExpectedVersion != 2 ||
		!strings.Contains(response.Body.String(), "audit_url") {
		t.Fatalf("destination test = %d %#v %s", response.Code, manager.metadata, response.Body.String())
	}
}

func TestNotificationDeliveryReadUsesSafeEvidence(t *testing.T) {
	manager := &fakeNotificationManager{}
	user := &fakeAuth{session: auth.Session{User: auth.User{ID: "administrator",
		Permissions: []string{"notification_destinations:manage"}}}}
	response := httptest.NewRecorder()
	notificationHandler(user, manager).ServeHTTP(response,
		authenticatedRequest(http.MethodGet, "/api/v1/notification-deliveries?state=retry_wait&limit=10", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "provider_unavailable") ||
		strings.Contains(response.Body.String(), "webhook") {
		t.Fatalf("delivery read = %d %s", response.Code, response.Body.String())
	}
}
