package proxmox

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestClientUsesOnlyApprovedEndpointPathsAndTokenHeader(t *testing.T) {
	requests := []string{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		if r.Header.Get("Authorization") != "PVEAPIToken=user@pve!espial=secret" {
			t.Fatalf("unexpected authorization header %q", r.Header.Get("Authorization"))
		}
		writeData(t, w, map[string]string{"version": "8.2", "release": "8.2", "repoid": "fixture"})
	}))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	config := Config{BaseURL: server.URL, DeploymentID: "lab", TokenID: "user@pve!espial", TokenSecret: "secret"}
	client, err := NewClient(config, ClientOptions{
		HTTPClient: server.Client(), ApprovedHosts: []string{target.Hostname()}, AllowedPorts: []int{mustPort(t, target.Port())},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := client.GetVersion(context.Background()); err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if len(requests) != 1 || requests[0] != "/api2/json/version" {
		t.Fatalf("requests = %#v", requests)
	}
}

func TestClientDecodesClusterStatusWithNumericNodeID(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeData(t, w, []map[string]any{
			{"type": "cluster", "name": "lab", "nodes": 1, "quorate": 1, "version": 1},
			{"type": "node", "name": "pve-a", "id": "node/pve-a", "nodeid": 1, "online": 1, "level": nil},
		})
	}))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	client, err := NewClient(Config{
		BaseURL: server.URL, DeploymentID: "lab", TokenID: "user@pve!espial", TokenSecret: "secret",
	}, ClientOptions{
		HTTPClient: server.Client(), ApprovedHosts: []string{target.Hostname()}, AllowedPorts: []int{mustPort(t, target.Port())},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	status, err := client.GetClusterStatus(context.Background())
	if err != nil {
		t.Fatalf("GetClusterStatus: %v", err)
	}
	if len(status) != 2 || status[0].Quorate == nil || *status[0].Quorate != 1 || status[1].Name != "pve-a" {
		t.Fatalf("status = %#v", status)
	}
}

func TestClientReadsBoundedActiveTasks(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/cluster/tasks" || r.URL.Query().Get("source") != "active" || r.URL.Query().Get("limit") != activeTaskLimit {
			t.Fatalf("request = %s", r.URL.String())
		}
		writeData(t, w, []map[string]any{{"id": "402", "node": "cdr-vhost42", "type": "vncproxy"}})
	}))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	client, err := NewClient(Config{BaseURL: server.URL, DeploymentID: "lab", TokenID: "user@pve!espial", TokenSecret: "secret"}, ClientOptions{HTTPClient: server.Client(), ApprovedHosts: []string{target.Hostname()}, AllowedPorts: []int{mustPort(t, target.Port())}})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	tasks, err := client.GetActiveTasks(context.Background())
	if err != nil || len(tasks) != 1 || tasks[0].ID != "402" || tasks[0].Type != "vncproxy" {
		t.Fatalf("GetActiveTasks = %#v, %v", tasks, err)
	}
}

func TestClientRejectsUnapprovedTarget(t *testing.T) {
	_, err := NewClient(Config{
		BaseURL: "https://proxmox.example:8006", DeploymentID: "lab",
		TokenID: "user@pve!espial", TokenSecret: "secret",
	}, ClientOptions{ApprovedHosts: []string{"other.example"}, AllowedPorts: []int{8006}})
	if err == nil || err.Error() != "target_not_approved" {
		t.Fatalf("error = %v", err)
	}
}

func TestClientRejectsUntrustedSelfSignedCertificateByDefault(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeData(t, w, map[string]string{"version": "8.2"})
	}))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	client, err := NewClient(Config{
		BaseURL: server.URL, DeploymentID: "lab", TokenID: "user@pve!espial", TokenSecret: "secret",
	}, ClientOptions{ApprovedHosts: []string{target.Hostname()}, AllowedPorts: []int{mustPort(t, target.Port())}})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := client.GetVersion(context.Background()); err == nil || err.Error() != "tls_certificate_untrusted" {
		t.Fatalf("error = %v", err)
	}
}

func TestClientAllowsExplicitUnverifiedTLSForAnApprovedTarget(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeData(t, w, map[string]string{"version": "8.2"})
	}))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	client, err := NewClient(Config{
		BaseURL: server.URL, DeploymentID: "lab", TokenID: "user@pve!espial", TokenSecret: "secret",
		AllowUnverifiedTLS: true,
	}, ClientOptions{ApprovedHosts: []string{target.Hostname()}, AllowedPorts: []int{mustPort(t, target.Port())}})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := client.GetVersion(context.Background()); err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
}

func TestClientPinsExplicitlyTrustedSelfSignedCertificate(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeData(t, w, map[string]string{"version": "8.2"})
	}))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	options := ClientOptions{ApprovedHosts: []string{target.Hostname()}, AllowedPorts: []int{mustPort(t, target.Port())}}
	config := Config{
		BaseURL: server.URL, DeploymentID: "lab", TokenID: "user@pve!espial", TokenSecret: "secret",
		TrustSelfSignedCertificate: true,
	}
	client, err := NewClient(config, options)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := client.GetVersion(context.Background()); err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	fingerprint := client.CertificateFingerprint()
	if len(fingerprint) != 64 {
		t.Fatalf("fingerprint = %q", fingerprint)
	}

	config.TrustedCertificateSHA256 = fingerprint
	pinned, err := NewClient(config, options)
	if err != nil {
		t.Fatalf("NewClient pinned: %v", err)
	}
	if _, err := pinned.GetVersion(context.Background()); err != nil {
		t.Fatalf("GetVersion pinned: %v", err)
	}

	config.TrustedCertificateSHA256 = strings.Repeat("0", 64)
	changed, err := NewClient(config, options)
	if err != nil {
		t.Fatalf("NewClient changed: %v", err)
	}
	if _, err := changed.GetVersion(context.Background()); err == nil || err.Error() != "tls_certificate_changed" {
		t.Fatalf("changed certificate error = %v", err)
	}
}

func TestClassifyTransportErrorPreservesSafeConnectionCategories(t *testing.T) {
	if got := classifyTransportError(context.Background(), &net.DNSError{Err: "not found", Name: "pve.example"}); got.Error() != "dns_failed" {
		t.Fatalf("DNS error = %v", got)
	}
	if got := classifyTransportError(context.Background(), &net.OpError{Op: "dial", Err: errors.New("refused")}); got.Error() != "connection_failed" {
		t.Fatalf("connection error = %v", got)
	}
	if got := classifyTransportError(context.Background(), x509.CertificateInvalidError{Reason: x509.Expired}); got.Error() != "tls_certificate_invalid" {
		t.Fatalf("certificate error = %v", got)
	}
}

func writeData(t *testing.T, w http.ResponseWriter, data any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(map[string]any{"data": data}); err != nil {
		t.Fatal(err)
	}
}

func mustPort(t *testing.T, value string) int {
	t.Helper()
	parsed, err := strconv.Atoi(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
