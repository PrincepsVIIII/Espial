package mattermost

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/notifications"
)

type staticResolver struct {
	addresses []netip.Addr
	err       error
}

func (resolver staticResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return resolver.addresses, resolver.err
}

func TestMattermostDeliveryFormattingAndClassification(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	var payload string
	var redirectHits atomic.Int64
	redirectTarget := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirectHits.Add(1)
	}))
	defer redirectTarget.Close()
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		payload = string(body)
		switch request.URL.Path {
		case "/hooks/good":
			response.Header().Set("X-Request-ID", "provider-17")
			response.WriteHeader(http.StatusNoContent)
		case "/hooks/rate":
			response.Header().Set("Retry-After", "900")
			response.WriteHeader(http.StatusTooManyRequests)
		case "/hooks/server-error":
			response.WriteHeader(http.StatusServiceUnavailable)
		case "/hooks/bad":
			response.WriteHeader(http.StatusBadRequest)
		case "/hooks/redirect":
			http.Redirect(response, request, redirectTarget.URL, http.StatusTemporaryRedirect)
		default:
			response.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	driver, target := testDriver(t, server)
	base := notifications.DeliveryRequest{Target: target, Message: notifications.Message{
		EventID: "event-17", Kind: "detected", Title: "Disk *@channel*", Summary: "[look](bad)\nnext",
		Severity: "critical", Status: "open", IncidentURL: "https://espial.example/alerts/incident-1",
	}}
	tests := []struct {
		token      string
		delivered  bool
		retryable  bool
		errorCode  string
		retryAfter time.Duration
	}{
		{"good", true, false, "", 0},
		{"rate", false, true, "rate_limited", 5 * time.Minute},
		{"server-error", false, true, "provider_unavailable", 0},
		{"bad", false, false, "provider_rejected", 0},
		{"redirect", false, false, "redirect_rejected", 0},
	}
	for _, test := range tests {
		request := base
		request.WebhookToken = test.token
		result := driver.Deliver(context.Background(), request)
		if result.Delivered != test.delivered || result.Retryable != test.retryable ||
			result.ErrorCode != test.errorCode || result.RetryAfter != test.retryAfter {
			t.Errorf("%s result = %#v", test.token, result)
		}
	}
	if redirectHits.Load() != 0 {
		t.Fatal("Mattermost driver followed a redirect")
	}
	if strings.Contains(payload, "@channel") {
		t.Fatal("payload retained an active mention")
	}
	if !strings.Contains(payload, "event-17") || !strings.Contains(payload, `\*`) {
		t.Fatalf("payload did not preserve stable evidence and escaped formatting: %s", payload)
	}
	if strings.Contains(payload, "good") {
		t.Fatal("webhook secret appeared in the request body")
	}
	var decoded map[string]string
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil || decoded["text"] == "" {
		t.Fatalf("Mattermost JSON payload = %#v, %v", decoded, err)
	}
}

func TestMattermostConnectionResetIsRetryable(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		connection, _, err := response.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = connection.Close()
	}))
	defer server.Close()
	driver, target := testDriver(t, server)
	result := driver.Deliver(context.Background(), notifications.DeliveryRequest{Target: target, WebhookToken: "reset"})
	if !result.Retryable || (result.ErrorCode != "connection_failed" && result.ErrorCode != "response_read_failed") {
		t.Fatalf("connection reset result = %#v", result)
	}
}

func TestMattermostTestMessageIsExplicit(t *testing.T) {
	message := formatMessage(notifications.Message{Test: true, EventID: "test-event"})
	if !strings.Contains(message, "test notification") || !strings.Contains(message, "explicitly labeled test") {
		t.Fatalf("test message = %q", message)
	}
}

func TestMattermostNetworkPolicyMatrix(t *testing.T) {
	approved := netip.MustParseAddr("203.0.113.10")
	unapproved := netip.MustParseAddr("127.0.0.1")
	tests := []struct {
		name      string
		hosts     []string
		cidrs     []string
		ports     []int
		addresses []netip.Addr
		target    notifications.Target
		allowed   bool
	}{
		{"approved", []string{"chat.example"}, []string{"203.0.113.0/24"}, []int{443}, []netip.Addr{approved}, notifications.Target{Host: "chat.example", Port: 443, PathPrefix: "/hooks"}, true},
		{"approved internal", []string{"10.0.0.5"}, []string{"10.0.0.0/24"}, []int{443}, nil, notifications.Target{Host: "10.0.0.5", Port: 443, PathPrefix: "/hooks"}, true},
		{"host denied", []string{"other.example"}, []string{"203.0.113.0/24"}, []int{443}, []netip.Addr{approved}, notifications.Target{Host: "chat.example", Port: 443, PathPrefix: "/hooks"}, false},
		{"port denied", []string{"chat.example"}, []string{"203.0.113.0/24"}, []int{443}, []netip.Addr{approved}, notifications.Target{Host: "chat.example", Port: 8443, PathPrefix: "/hooks"}, false},
		{"private address denied", []string{"chat.example"}, []string{"203.0.113.0/24"}, []int{443}, []netip.Addr{unapproved}, notifications.Target{Host: "chat.example", Port: 443, PathPrefix: "/hooks"}, false},
		{"mixed DNS denied", []string{"chat.example"}, []string{"203.0.113.0/24"}, []int{443}, []netip.Addr{approved, unapproved}, notifications.Target{Host: "chat.example", Port: 443, PathPrefix: "/hooks"}, false},
		{"query denied", []string{"chat.example"}, []string{"203.0.113.0/24"}, []int{443}, []netip.Addr{approved}, notifications.Target{Host: "chat.example", Port: 443, PathPrefix: "/hooks?next=bad"}, false},
		{"traversal denied", []string{"chat.example"}, []string{"203.0.113.0/24"}, []int{443}, []netip.Addr{approved}, notifications.Target{Host: "chat.example", Port: 443, PathPrefix: "/../admin"}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			driver, err := New(Options{ApprovedHosts: test.hosts, ApprovedCIDRs: test.cidrs,
				AllowedPorts: test.ports, Resolver: staticResolver{addresses: test.addresses}})
			if err != nil {
				t.Fatal(err)
			}
			err = driver.Validate(context.Background(), test.target)
			if (err == nil) != test.allowed {
				t.Fatalf("Validate() error = %v, allowed = %v", err, test.allowed)
			}
		})
	}
	if _, err := New(Options{ApprovedHosts: []string{"user@chat.example"}, ApprovedCIDRs: []string{"203.0.113.0/24"}, AllowedPorts: []int{443}}); err == nil {
		t.Fatal("approved-host userinfo was accepted")
	}
}

func TestMattermostRequestTimeoutAndResponseCap(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if strings.HasSuffix(request.URL.Path, "/slow") {
			time.Sleep(100 * time.Millisecond)
			response.WriteHeader(http.StatusNoContent)
			return
		}
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write([]byte(strings.Repeat("x", 65)))
	}))
	defer server.Close()
	driver, target := testDriverWithOptions(t, server, 20*time.Millisecond, 64)
	result := driver.Deliver(context.Background(), notifications.DeliveryRequest{Target: target, WebhookToken: "slow"})
	if !result.Retryable || result.ErrorCode != "timeout" {
		t.Fatalf("timeout result = %#v", result)
	}
	driver, target = testDriverWithOptions(t, server, time.Second, 64)
	result = driver.Deliver(context.Background(), notifications.DeliveryRequest{Target: target, WebhookToken: "large"})
	if result.Retryable || result.ErrorCode != "response_too_large" {
		t.Fatalf("large response result = %#v", result)
	}
}

func testDriver(t *testing.T, server *httptest.Server) (*Driver, notifications.Target) {
	return testDriverWithOptions(t, server, time.Second, 4096)
}

func testDriverWithOptions(t *testing.T, server *httptest.Server, timeout time.Duration, limit int64) (*Driver, notifications.Target) {
	t.Helper()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := netip.ParseAddrPort(strings.TrimPrefix(parsed.Host, "["))
	if err != nil {
		// httptest uses IPv4 in the supported test environments.
		hostPort := strings.Split(parsed.Host, ":")
		port = netip.MustParseAddrPort(hostPort[0] + ":" + hostPort[1])
	}
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	driver, err := New(Options{ApprovedHosts: []string{port.Addr().String()},
		ApprovedCIDRs: []string{port.Addr().String() + "/32"}, AllowedPorts: []int{int(port.Port())},
		RequestTimeout: timeout, ResponseBodyLimit: limit, RootCAs: pool})
	if err != nil {
		t.Fatal(err)
	}
	return driver, notifications.Target{Host: port.Addr().String(), Port: int(port.Port()), PathPrefix: "/hooks"}
}
