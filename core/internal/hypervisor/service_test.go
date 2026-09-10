package hypervisor

import (
	"errors"
	"testing"
)

func TestProxmoxCreateErrorCodePreservesActionableCategories(t *testing.T) {
	for _, code := range []string{
		"unauthorized",
		"forbidden",
		"throttled",
		"timeout",
		"tls_certificate_untrusted",
		"tls_hostname_mismatch",
		"tls_certificate_invalid",
		"tls_certificate_changed",
		"tls_handshake_failed",
		"dns_failed",
		"connection_failed",
		"proxmox_api_not_found",
	} {
		if got := proxmoxCreateErrorCode(errors.New(code)); got != code {
			t.Errorf("proxmoxCreateErrorCode(%q) = %q", code, got)
		}
	}
	if got := proxmoxCreateErrorCode(errors.New("internal detail")); got != "proxmox_validation_failed" {
		t.Fatalf("unexpected safe fallback %q", got)
	}
}

func TestCompleteLoadNodesIncludesZeroGuestHostsAndKeepsCounts(t *testing.T) {
	hosts := []HostSummary{{ID: "pve-b"}, {ID: "pve-a"}}
	loads := []LoadNodeSummary{{ID: "pve-a", ActiveVMs: 3, ConsoleConnections: 5}, {ID: "pve-c", ActiveVMs: -2, ConsoleConnections: -2}}

	got := completeLoadNodes(hosts, loads)
	want := []LoadNodeSummary{
		{ID: "pve-a", ActiveVMs: 3, ConsoleConnections: 5},
		{ID: "pve-b", ActiveVMs: 0},
		{ID: "pve-c", ActiveVMs: 0},
	}
	if len(got) != len(want) {
		t.Fatalf("completeLoadNodes() = %#v", got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("completeLoadNodes()[%d] = %#v, want %#v", index, got[index], want[index])
		}
	}
}

func TestCompleteStorageLoadNodesIncludesZeroGuestStorageAndKeepsCounts(t *testing.T) {
	storage := []StorageSummary{{ID: "iscsi-main"}, {ID: "local-zfs"}}
	loads := []LoadNodeSummary{{ID: "iscsi-main", IdleVMs: 4, ActiveVMs: 1, ConsoleConnections: 2}, {ID: "missing-storage", ActiveVMs: -2, ConsoleConnections: -2}}

	got := completeStorageLoadNodes(storage, loads)
	want := []LoadNodeSummary{
		{ID: "iscsi-main", IdleVMs: 4, ActiveVMs: 1, ConsoleConnections: 2},
		{ID: "local-zfs"},
		{ID: "missing-storage"},
	}
	if len(got) != len(want) {
		t.Fatalf("completeStorageLoadNodes() = %#v", got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("completeStorageLoadNodes()[%d] = %#v, want %#v", index, got[index], want[index])
		}
	}
}

func TestLoadNodesIncludesConsoleConnections(t *testing.T) {
	got := loadNodes(
		map[string]any{"pve-a": 3},
		map[string]any{"pve-a": 1},
		map[string]any{"pve-a": 2},
	)
	want := []LoadNodeSummary{{ID: "pve-a", IdleVMs: 3, ActiveVMs: 1, ConsoleConnections: 2}}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("loadNodes() = %#v, want %#v", got, want)
	}
}

func TestConsoleConnectionsAvailableRequiresTheConnectionAggregate(t *testing.T) {
	if consoleConnectionsAvailable(map[string]any{"console_activity_available": true}) {
		t.Fatal("consoleConnectionsAvailable() accepted an observation without connection counts")
	}
	if !consoleConnectionsAvailable(map[string]any{
		"console_activity_available":  true,
		"console_connections_by_node": map[string]any{},
	}) {
		t.Fatal("consoleConnectionsAvailable() rejected an available empty connection aggregate")
	}
}
