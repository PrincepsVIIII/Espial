package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/auth"
	"github.com/PrincepsVIIII/Espial/core/internal/events"
	"github.com/PrincepsVIIII/Espial/core/internal/incidents"
)

const incidentTestID = "80000000-0000-4000-8000-000000000021"

type fakeIncidents struct {
	list           incidents.List
	detail         incidents.Detail
	timeline       incidents.Timeline
	listFilter     incidents.Filter
	timelineFilter incidents.TimelineFilter
	err            error
}

func TestIncidentChangedSSECarriesIncidentReceipt(t *testing.T) {
	output := httptest.NewRecorder()
	err := writeSSEEvent(output, events.Event{
		ID: 7, SchemaVersion: 1, Kind: events.IncidentChanged,
		IncidentID: incidentTestID, Result: "open", ChangedAt: time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC),
	})
	if err != nil || !strings.Contains(output.Body.String(), "event: incident_changed") ||
		!strings.Contains(output.Body.String(), `"incident_id":"`+incidentTestID+`"`) {
		t.Fatalf("incident SSE = %q, %v", output.Body.String(), err)
	}
}

func (fake *fakeIncidents) Incidents(_ context.Context, filter incidents.Filter) (incidents.List, error) {
	fake.listFilter = filter
	return fake.list, fake.err
}

func (fake *fakeIncidents) Incident(context.Context, string) (incidents.Detail, error) {
	return fake.detail, fake.err
}

func (fake *fakeIncidents) Timeline(_ context.Context, _ string, filter incidents.TimelineFilter) (incidents.Timeline, error) {
	fake.timelineFilter = filter
	return fake.timeline, fake.err
}

func incidentHandler(authService AuthService, reader IncidentReader) http.Handler {
	publicURL, _ := url.Parse("https://espial.test")
	return New(Dependencies{
		Logger: discardLogger(), Ready: func(context.Context) error { return nil },
		Auth: authService, PublicURL: publicURL, SecureCookies: true, Incidents: reader,
	})
}

func TestIncidentReadsEnforceViewerPermissionAndValidateFilters(t *testing.T) {
	reader := &fakeIncidents{list: incidents.List{Items: []incidents.Summary{{ID: incidentTestID}}}}
	viewer := &fakeAuth{session: auth.Session{User: auth.User{ID: "viewer", Permissions: []string{"incidents:read"}}}}
	handler := incidentHandler(viewer, reader)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/incidents", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", response.Code)
	}

	request := authenticatedRequest(http.MethodGet, "/api/v1/incidents?active=true&severity=critical&status=open&limit=25", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), incidentTestID) {
		t.Fatalf("incident list = %d %s", response.Code, response.Body.String())
	}
	if reader.listFilter.Active == nil || !*reader.listFilter.Active || reader.listFilter.Limit != 25 ||
		len(reader.listFilter.Severities) != 1 || len(reader.listFilter.Statuses) != 1 {
		t.Fatalf("parsed incident filter = %#v", reader.listFilter)
	}

	request = authenticatedRequest(http.MethodGet, "/api/v1/incidents?severity=urgent&unexpected=true", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"fields"`) {
		t.Fatalf("invalid filter = %d %s", response.Code, response.Body.String())
	}

	viewer.session.User.Permissions = nil
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/api/v1/incidents", nil))
	if response.Code != http.StatusForbidden || !viewer.denied {
		t.Fatalf("permission denial = %d audited=%v", response.Code, viewer.denied)
	}
}

func TestIncidentDetailTimelineETagAndSafeErrors(t *testing.T) {
	updated := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	reader := &fakeIncidents{
		detail:   incidents.Detail{Summary: incidents.Summary{ID: incidentTestID, Version: 42, UpdatedAt: updated}, Fingerprint: "rule:resource:check"},
		timeline: incidents.Timeline{Items: []incidents.TimelineEvent{{ID: incidentTestID, IncidentID: incidentTestID, Kind: "detected", Summary: "Detected", OccurredAt: updated}}},
	}
	viewer := &fakeAuth{session: auth.Session{User: auth.User{Permissions: []string{"incidents:read"}}}}
	handler := incidentHandler(viewer, reader)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/api/v1/incidents/"+incidentTestID, nil))
	if response.Code != http.StatusOK || response.Header().Get("ETag") != `"v2a"` || !strings.Contains(response.Body.String(), `"fingerprint"`) {
		t.Fatalf("detail = %d etag=%q %s", response.Code, response.Header().Get("ETag"), response.Body.String())
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/api/v1/incidents/"+incidentTestID+"/timeline?limit=10", nil))
	if response.Code != http.StatusOK || reader.timelineFilter.Limit != 10 || !strings.Contains(response.Body.String(), `"detected"`) {
		t.Fatalf("timeline = %d filter=%#v %s", response.Code, reader.timelineFilter, response.Body.String())
	}

	reader.err = incidents.ErrInvalidCursor
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/api/v1/incidents?cursor=bad", nil))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"cursor"`) {
		t.Fatalf("cursor error = %d %s", response.Code, response.Body.String())
	}

	reader.err = incidents.ErrNotFound
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/api/v1/incidents/"+incidentTestID, nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("not found = %d %s", response.Code, response.Body.String())
	}

	reader.err = errors.New("database secret-value")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/api/v1/incidents/"+incidentTestID, nil))
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "secret-value") {
		t.Fatalf("unsafe internal error = %d %s", response.Code, response.Body.String())
	}
}
