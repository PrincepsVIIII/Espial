package proxmox

import (
	"context"
	"testing"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/health"
)

type fixtureAPI struct{}

func (fixtureAPI) GetVersion(context.Context) (VersionResponse, error) {
	return VersionResponse{Version: "8.2", Release: "8.2", RepoID: "fixture"}, nil
}

func (fixtureAPI) GetClusterStatus(context.Context) ([]ClusterStatusItem, error) {
	quorate := 1
	return []ClusterStatusItem{{Type: "cluster", Name: "lab", Quorate: &quorate}, {Type: "node", Name: "pve-a"}}, nil
}

func (fixtureAPI) GetClusterResources(context.Context) ([]ClusterResource, error) {
	shared := 1
	return []ClusterResource{
		{ID: "node/pve-a", Type: "node", Node: "pve-a", Name: "pve-a", Status: "online", CPU: 0.50, MaxCPU: 16, Mem: 16, MaxMem: 32},
		{ID: "qemu/100", Type: "qemu", Node: "pve-a", Name: "auth", Status: "running", VMID: 100},
		{ID: "lxc/200", Type: "lxc", Node: "pve-a", Name: "dns", Status: "stopped", VMID: 200},
		{ID: "storage/pve-a/vmstore", Type: "storage", Node: "pve-a", Storage: "vmstore", Status: "available", Disk: 85, MaxDisk: 100, Shared: &shared},
	}, nil
}

func (fixtureAPI) GetNodeStatus(context.Context, string) (NodeStatus, error) {
	return NodeStatus{CPU: 0.50, Wait: 0.07, Memory: Usage{Used: 16, Total: 32}, Swap: Usage{Used: 2, Total: 10}}, nil
}

func TestCollectMapsProxmoxResourcesAndIndicators(t *testing.T) {
	payload, err := Collect(context.Background(), Config{
		BaseURL: "https://proxmox.example:8006", DeploymentID: "lab",
		TokenID: "user@pve!espial", TokenSecret: "secret",
		ExpectedRunningGuests: []string{"lxc:200"},
	}, fixtureAPI{}, time.Unix(100, 0).UTC())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(payload.Resources) != 5 || len(payload.Observations) != 6 {
		t.Fatalf("resources=%d observations=%d", len(payload.Resources), len(payload.Observations))
	}
	foundExpectedStopped := false
	foundStorageWarning := false
	for _, observation := range payload.Observations {
		if observation.ExternalResourceID == "proxmox:lab:lxc:200" && observation.State == health.Critical {
			foundExpectedStopped = true
		}
		if observation.CheckType == StorageCapacityCheck && observation.State == health.Warning {
			foundStorageWarning = true
			if observation.Metadata["io_wait_indicator"] != "warning" {
				t.Fatalf("io wait indicator = %#v", observation.Metadata["io_wait_indicator"])
			}
		}
	}
	if !foundExpectedStopped || !foundStorageWarning {
		t.Fatalf("missing expected stopped guest or storage warning")
	}
}
