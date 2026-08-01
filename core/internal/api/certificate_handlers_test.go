package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/auth"
	"github.com/PrincepsVIIII/Espial/core/internal/certificates"
	"github.com/PrincepsVIIII/Espial/core/internal/health"
)

type fakeCertificateReader struct{ filter certificates.Filter }

func (fake *fakeCertificateReader) Certificates(_ context.Context, filter certificates.Filter) (certificates.List, error) {
	fake.filter = filter
	return certificates.List{Items: []certificates.Summary{{ID: "72000000-0000-4000-8000-000000000026", MonitorID: "70000000-0000-4000-8000-000000000001", Endpoint: "status.test:443", State: health.Warning, RawState: health.Warning, CertificateState: health.Warning, Reason: "Certificate approaches expiry.", ReasonCode: "certificate_approaching_expiry", UpdatedAt: time.Unix(1, 0).UTC(), Source: "webcheck", Freshness: "fresh"}}}, nil
}
func (fake *fakeCertificateReader) Certificate(ctx context.Context, _ string) (certificates.Detail, error) {
	list, _ := fake.Certificates(ctx, certificates.Filter{})
	return certificates.Detail{Summary: list.Items[0], FingerprintChanged: false, IssuerChanged: false, FirstSeenAt: time.Unix(1, 0).UTC(), LastSeenAt: time.Unix(2, 0).UTC()}, nil
}

func certificateHandler(reader *fakeCertificateReader, permissions []string) http.Handler {
	publicURL, _ := url.Parse("https://espial.test")
	return New(Dependencies{Logger: discardLogger(), Ready: func(context.Context) error { return nil }, Auth: &fakeAuth{session: auth.Session{User: auth.User{ID: "viewer", Permissions: permissions}}}, PublicURL: publicURL, Certificates: reader})
}

func TestCertificateReadsRequireWebpagesPermissionAndParseFilters(t *testing.T) {
	reader := &fakeCertificateReader{}
	response := httptest.NewRecorder()
	certificateHandler(reader, []string{"webpages:read"}).ServeHTTP(response, authenticatedRequest(http.MethodGet, "/api/v1/certificates?state=warning&hostname_valid=false&expiry_days=30", nil))
	if response.Code != http.StatusOK || len(reader.filter.States) != 1 || reader.filter.HostnameValid == nil || *reader.filter.HostnameValid || reader.filter.ExpiryDays == nil || *reader.filter.ExpiryDays != 30 {
		t.Fatalf("read=%d filter=%#v body=%s", response.Code, reader.filter, response.Body.String())
	}
	denied := httptest.NewRecorder()
	certificateHandler(reader, []string{"overview:read"}).ServeHTTP(denied, authenticatedRequest(http.MethodGet, "/api/v1/certificates", nil))
	if denied.Code != http.StatusForbidden {
		t.Fatalf("denied=%d %s", denied.Code, denied.Body.String())
	}
}

func TestCertificateDetailUsesOpaqueDirectURL(t *testing.T) {
	response := httptest.NewRecorder()
	certificateHandler(&fakeCertificateReader{}, []string{"webpages:read"}).ServeHTTP(response, authenticatedRequest(http.MethodGet, "/api/v1/certificates/72000000-0000-4000-8000-000000000026", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("detail=%d %s", response.Code, response.Body.String())
	}
}
