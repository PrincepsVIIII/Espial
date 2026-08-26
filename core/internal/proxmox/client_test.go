package proxmox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
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

func TestClientRejectsUnapprovedTarget(t *testing.T) {
	_, err := NewClient(Config{
		BaseURL: "https://proxmox.example:8006", DeploymentID: "lab",
		TokenID: "user@pve!espial", TokenSecret: "secret",
	}, ClientOptions{ApprovedHosts: []string{"other.example"}, AllowedPorts: []int{8006}})
	if err == nil || err.Error() != "target_not_approved" {
		t.Fatalf("error = %v", err)
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
