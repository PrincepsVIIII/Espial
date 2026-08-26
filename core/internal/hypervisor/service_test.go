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
	loads := []LoadNodeSummary{{ID: "pve-a", ActiveVMs: 3}, {ID: "pve-c", ActiveVMs: -2}}

	got := completeLoadNodes(hosts, loads)
	want := []LoadNodeSummary{
		{ID: "pve-a", ActiveVMs: 3},
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
