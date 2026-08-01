package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/auth"
	"github.com/PrincepsVIIII/Espial/core/internal/operations"
	"github.com/PrincepsVIIII/Espial/core/internal/signals"
)

type fakeMetrics struct {
	snapshot operations.Snapshot
	err      error
}

func (fake fakeMetrics) Snapshot(context.Context) (operations.Snapshot, error) {
	return fake.snapshot, fake.err
}

type fakeAuth struct {
	session   auth.Session
	loginErr  error
	denied    bool
	loggedOut bool
}

func (fake *fakeAuth) Login(context.Context, string, string, string, string) (auth.Session, error) {
	return fake.session, fake.loginErr
}
func (fake *fakeAuth) Authenticate(context.Context, string) (auth.Session, error) {
	return fake.session, fake.loginErr
}
func (fake *fakeAuth) VerifyCSRF(_ auth.Session, token string) bool { return token == "csrf-secret" }
func (fake *fakeAuth) Logout(context.Context, auth.Session, string, string) error {
	fake.loggedOut = true
	return nil
}
func (fake *fakeAuth) RecordDenied(context.Context, auth.User, string, string, string) error {
	fake.denied = true
	return nil
}

func newHandler(fake *fakeAuth, secure bool) http.Handler {
	publicURL, _ := url.Parse("https://espial.test")
	return New(Dependencies{Logger: discardLogger(), Ready: func(context.Context) error { return nil }, Auth: fake, PublicURL: publicURL, SecureCookies: secure})
}

func TestLiveness(t *testing.T) {
	handler := newHandler(&fakeAuth{}, true)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if response.Header().Get("X-Request-ID") == "" {
		t.Fatal("response did not include X-Request-ID")
	}
	if response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("response did not include security headers")
	}
}

func TestReadinessFailure(t *testing.T) {
	publicURL, _ := url.Parse("https://espial.test")
	handler := New(Dependencies{Logger: discardLogger(), Ready: func(context.Context) error { return errors.New("database unavailable") }, Auth: &fakeAuth{}, PublicURL: publicURL})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestOperationalMetricsRemainPrivateAndBounded(t *testing.T) {
	publicURL, _ := url.Parse("https://espial.test")
	handler := New(Dependencies{Logger: discardLogger(), Ready: func(context.Context) error { return nil }, Auth: &fakeAuth{}, PublicURL: publicURL,
		Metrics: fakeMetrics{snapshot: operations.Snapshot{Signals: signals.Metrics{QueueDepth: 7},
			IncidentsBySeverity: map[string]int64{"critical": 2}, IncidentsByStatus: map[string]int64{},
			WebpagesByState: map[string]int64{}, CertificatesByState: map[string]int64{}}}})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "espial_monitoring_signals_queued 7") {
		t.Fatalf("metrics = %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "resource_id") || strings.Contains(response.Body.String(), "hostname") {
		t.Fatalf("metrics exposed prohibited labels: %s", response.Body.String())
	}
	apiResponse := httptest.NewRecorder()
	handler.ServeHTTP(apiResponse, httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil))
	if apiResponse.Code != http.StatusNotFound {
		t.Fatalf("public API metrics status = %d", apiResponse.Code)
	}
}

func TestLoginSetsSecureCookiesWithoutLeakingTokens(t *testing.T) {
	fake := &fakeAuth{session: auth.Session{Token: "session-secret", CSRFToken: "csrf-secret", ExpiresAt: time.Now().Add(time.Hour), AbsoluteExpiry: time.Now().Add(12 * time.Hour), User: auth.User{Username: "admin"}}}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/local/login", bytes.NewBufferString(`{"username":"admin","password":"correct horse battery"}`))
	request.Header.Set("Origin", "https://espial.test")
	response := httptest.NewRecorder()
	newHandler(fake, true).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("response leaked a token: %s", response.Body.String())
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 2 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("unexpected cookies: %#v", cookies)
	}
	if cookies[1].HttpOnly {
		t.Fatal("CSRF cookie must be readable by the browser client")
	}
}

func TestLoginRejectsUntrustedOriginAndUsesGenericFailure(t *testing.T) {
	fake := &fakeAuth{loginErr: auth.ErrInvalidCredentials}
	for _, origin := range []string{"https://evil.test", "https://espial.test"} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/local/login", bytes.NewBufferString(`{"username":"admin","password":"wrong"}`))
		request.Header.Set("Origin", origin)
		response := httptest.NewRecorder()
		newHandler(fake, true).ServeHTTP(response, request)
		if origin == "https://evil.test" && response.Code != http.StatusForbidden {
			t.Fatalf("untrusted origin status = %d", response.Code)
		}
		if origin == "https://espial.test" && (response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "invalid_credentials")) {
			t.Fatalf("credential failure = %d %s", response.Code, response.Body.String())
		}
	}
}

func TestLogoutRequiresMatchingCSRF(t *testing.T) {
	fake := &fakeAuth{session: auth.Session{CSRFDigest: []byte("digest")}}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	request.Header.Set("Origin", "https://espial.test")
	request.Header.Set("X-CSRF-Token", "csrf-secret")
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: "session-secret"})
	request.AddCookie(&http.Cookie{Name: csrfCookie, Value: "csrf-secret"})
	response := httptest.NewRecorder()
	newHandler(fake, true).ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || !fake.loggedOut {
		t.Fatalf("logout = %d, called=%v", response.Code, fake.loggedOut)
	}
}

func TestViewerCannotCrossAdministratorBoundary(t *testing.T) {
	fake := &fakeAuth{session: auth.Session{User: auth.User{ID: "viewer", Permissions: []string{"overview:read"}}}}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ping", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: "session-secret"})
	response := httptest.NewRecorder()
	newHandler(fake, true).ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || !fake.denied {
		t.Fatalf("boundary = %d, audited=%v", response.Code, fake.denied)
	}
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
