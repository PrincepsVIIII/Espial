package webcheck

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/health"
)

type resolverFunc func(context.Context, string, string) ([]netip.Addr, error)

type fixedWebcheckClock struct{ now time.Time }

func (clock fixedWebcheckClock) Now() time.Time { return clock.now }

func (resolve resolverFunc) LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error) {
	return resolve(ctx, network, host)
}

func TestCheckerDeterministicHTTPOutcomes(t *testing.T) {
	tests := []struct {
		name      string
		handler   http.HandlerFunc
		configure func(*Config)
		bodyLimit int64
		wantState health.State
		wantCode  string
	}{
		{"healthy", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200); _, _ = w.Write([]byte("ready")) }, func(c *Config) { c.ContentMatch = "ready" }, 64, health.Healthy, "available"},
		{"unexpected status", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) }, nil, 64, health.Critical, "status_unexpected"},
		{"content mismatch", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("wrong")) }, func(c *Config) { c.ContentMatch = "ready" }, 64, health.Critical, "content_mismatch"},
		{"oversize body", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(strings.Repeat("x", 65))) }, nil, 64, health.Critical, "body_too_large"},
		{"oversize headers", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-Oversized", strings.Repeat("x", 40*1024))
			_, _ = w.Write([]byte("ok"))
		}, nil, 64, health.Critical, "headers_too_large"},
		{"slow response", func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(200 * time.Millisecond)
			_, _ = w.Write([]byte("ok"))
		}, func(c *Config) { c.TimeoutMS = 100 }, 64, health.Critical, "response_timeout"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(test.handler)
			defer server.Close()
			checker, config := localChecker(t, server.URL, test.bodyLimit)
			if test.configure != nil {
				test.configure(&config)
			}
			result := checker.Check(context.Background(), config)
			if result.State != test.wantState || result.ReasonCode != test.wantCode {
				t.Fatalf("result=%#v", result)
			}
			encoded := result.Summary + result.ReasonCode
			if strings.Contains(encoded, "ready") && test.wantCode != "available" {
				t.Fatalf("response content leaked: %q", encoded)
			}
		})
	}
}

func TestCheckerConnectFailureRecoveryAndAmbientProxyIsolation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	policy, err := NewPolicy(PolicyOptions{
		ApprovedHosts: []string{"monitor.test"}, ApprovedCIDRs: []string{"127.0.0.0/8"}, AllowedPorts: []int{port},
		Resolver: resolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	config := validConfig("http://monitor.test:" + strconv.Itoa(port) + "/")
	if result := NewChecker(policy, CheckerOptions{}).Check(context.Background(), config); result.ReasonCode != "connect_failed" {
		t.Fatalf("refusal result=%#v", result)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ready"))
	}))
	defer server.Close()
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	checker, recovered := localChecker(t, server.URL, 64)
	if result := checker.Check(context.Background(), recovered); result.ReasonCode != "available" {
		t.Fatalf("recovery result=%#v", result)
	}
}

func TestPolicyRejectsMixedAddressAnswers(t *testing.T) {
	policy, err := NewPolicy(PolicyOptions{
		ApprovedHosts: []string{"monitor.test"}, ApprovedCIDRs: []string{"192.0.2.0/24"}, AllowedPorts: []int{443},
		Resolver: resolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("192.0.2.10"), netip.MustParseAddr("2001:db8::10")}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	result := NewChecker(policy, CheckerOptions{}).Check(context.Background(), validConfig("https://monitor.test/"))
	if result.ReasonCode != "address_not_approved" {
		t.Fatalf("result=%#v", result)
	}
}

func TestCheckerRedirectRevalidatesEveryHop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://unapproved.test/secret", http.StatusFound)
	}))
	defer server.Close()
	checker, config := localChecker(t, server.URL, 64)
	config.FollowRedirects = true
	config.MaxRedirects = 1
	result := checker.Check(context.Background(), config)
	if result.ReasonCode != "host_not_approved" {
		t.Fatalf("result=%#v", result)
	}
}

func TestCheckerDoesNotForwardProtectedHeadersAcrossOrigins(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://other.test/", http.StatusFound)
	}))
	defer server.Close()
	checker, config := localChecker(t, server.URL, 64)
	config.FollowRedirects, config.MaxRedirects = true, 1
	config.HeaderNames, config.HeaderValue1 = []string{"Authorization"}, "canary-secret"
	result := checker.Check(context.Background(), config)
	if result.ReasonCode != "redirect_credentials_rejected" || strings.Contains(result.Summary, "canary-secret") {
		t.Fatalf("result=%#v", result)
	}
}

func TestCheckerRejectsDNSAndTLSFailuresSafely(t *testing.T) {
	policy, err := NewPolicy(PolicyOptions{ApprovedHosts: []string{"missing.test"}, ApprovedCIDRs: []string{"127.0.0.0/8"}, AllowedPorts: []int{443}, Resolver: resolverFunc(func(context.Context, string, string) ([]netip.Addr, error) { return nil, errors.New("lookup detail") })})
	if err != nil {
		t.Fatal(err)
	}
	result := NewChecker(policy, CheckerOptions{}).Check(context.Background(), validConfig("https://missing.test/"))
	if result.ReasonCode != "dns_failed" || strings.Contains(result.Summary, "lookup detail") {
		t.Fatalf("result=%#v", result)
	}
	tlsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer tlsServer.Close()
	checker, config := localChecker(t, tlsServer.URL, 64)
	result = checker.Check(context.Background(), config)
	if result.ReasonCode != "tls_certificate_untrusted_chain" || result.Certificate == nil || result.Certificate.ReasonCode != "certificate_untrusted_chain" {
		t.Fatalf("result=%#v", result)
	}
}

func TestConfigRejectsSecretBearingAndAmbiguousTargets(t *testing.T) {
	for _, target := range []string{"https://user:pass@example.invalid/", "https://example.invalid/?api_key=value", "ftp://example.invalid/"} {
		config := validConfig(target)
		if ValidateConfig(config) == nil {
			t.Fatalf("accepted %q", target)
		}
	}
	config := validConfig("https://example.invalid/")
	config.HeaderNames = []string{"Authorization"}
	config.HeaderValue1 = "secret\nvalue"
	if ValidateConfig(config) == nil {
		t.Fatal("accepted header injection")
	}
	manifest := Manifest()
	if manifest.AdapterID != AdapterID || len(manifest.CheckTypes) != 2 || manifest.CheckTypes[0] != CheckType || manifest.CheckTypes[1] != CertificateCheckType || !manifest.ReadOnly {
		t.Fatalf("manifest=%#v", manifest)
	}
}

func TestCertificateThresholdsAndValidationUseInjectedClock(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	root, rootKey := testCertificateAuthority(t, now)
	roots := x509.NewCertPool()
	roots.AddCert(root)
	tests := []struct {
		name       string
		notBefore  time.Time
		notAfter   time.Time
		host       string
		roots      *x509.CertPool
		wantState  health.State
		wantReason string
	}{
		{"valid", now.Add(-time.Hour), now.Add(31 * 24 * time.Hour), "status.test", roots, health.Healthy, "certificate_valid"},
		{"30 day boundary", now.Add(-time.Hour), now.Add(30 * 24 * time.Hour), "status.test", roots, health.Warning, "certificate_approaching_expiry"},
		{"14 day boundary", now.Add(-time.Hour), now.Add(14 * 24 * time.Hour), "status.test", roots, health.Critical, "certificate_expiring"},
		{"7 day boundary", now.Add(-time.Hour), now.Add(7 * 24 * time.Hour), "status.test", roots, health.Critical, "certificate_expiry_escalated"},
		{"expired", now.Add(-48 * time.Hour), now.Add(-time.Second), "status.test", roots, health.Critical, "certificate_expired"},
		{"not yet valid", now.Add(time.Second), now.Add(60 * 24 * time.Hour), "status.test", roots, health.Critical, "certificate_not_yet_valid"},
		{"wrong hostname", now.Add(-time.Hour), now.Add(60 * 24 * time.Hour), "wrong.test", roots, health.Critical, "certificate_hostname_mismatch"},
		{"untrusted", now.Add(-time.Hour), now.Add(60 * 24 * time.Hour), "status.test", x509.NewCertPool(), health.Critical, "certificate_untrusted_chain"},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			leaf := testLeafCertificate(t, root, rootKey, now, test.notBefore, test.notAfter, int64(index+10))
			checker := &Checker{clock: fixedWebcheckClock{now: now}, rootCAs: test.roots}
			target, _ := url.Parse("https://" + test.host + "/")
			result := checker.inspectCertificate(target, tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}}, WithCertificateDefaults(Config{}))
			if result.State != test.wantState || result.ReasonCode != test.wantReason {
				t.Fatalf("result=%#v", result)
			}
			if result.FingerprintSHA256 == "" || result.NotAfter == nil || result.HostnameValid == nil || result.ChainValid == nil {
				t.Fatalf("bounded identity evidence missing: %#v", result)
			}
		})
	}
}

func testCertificateAuthority(t *testing.T, now time.Time) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Test CA"}, NotBefore: now.Add(-365 * 24 * time.Hour), NotAfter: now.Add(365 * 24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return certificate, key
}

func testLeafCertificate(t *testing.T, root *x509.Certificate, rootKey *rsa.PrivateKey, now, notBefore, notAfter time.Time, serial int64) *x509.Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: "status.test"}, DNSNames: []string{"status.test"}, NotBefore: notBefore, NotAfter: notAfter, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, root, &key.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	_ = now
	return certificate
}

func localChecker(t *testing.T, rawURL string, bodyLimit int64) (*Checker, Config) {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatal(err)
	}
	policy, err := NewPolicy(PolicyOptions{ApprovedHosts: []string{"monitor.test"}, ApprovedCIDRs: []string{"127.0.0.0/8"}, AllowedPorts: []int{port}, BodyLimit: bodyLimit, Resolver: resolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	}), Dialer: (&net.Dialer{}).DialContext})
	if err != nil {
		t.Fatal(err)
	}
	parsed.Host = net.JoinHostPort("monitor.test", parsed.Port())
	return NewChecker(policy, CheckerOptions{}), validConfig(parsed.String())
}
func validConfig(target string) Config {
	return Config{URL: target, AllowedStatuses: []int{200}, TimeoutMS: 1000, FollowRedirects: false, MaxRedirects: 0, ExpectedRefreshSeconds: 60}
}
