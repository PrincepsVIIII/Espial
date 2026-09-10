package proxmox

import (
	"context"
	"errors"
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
	return []ClusterStatusItem{{Type: "cluster", Name: "lab", Quorate: &quorate}, {Type: "node", Name: "pve-a"}, {Type: "node", Name: "pve-b"}}, nil
}

func (fixtureAPI) GetClusterResources(context.Context) ([]ClusterResource, error) {
	shared := 1
	return []ClusterResource{
		{ID: "node/pve-a", Type: "node", Node: "pve-a", Name: "pve-a", Status: "online", CPU: 0.50, MaxCPU: 16, Mem: 16, MaxMem: 32},
		{ID: "node/pve-b", Type: "node", Node: "pve-b", Name: "pve-b", Status: "online", CPU: 0.25, MaxCPU: 8, Mem: 8, MaxMem: 32},
		{ID: "qemu/100", Type: "qemu", Node: "pve-a", Name: "auth", Status: "running", VMID: 100},
		{ID: "lxc/200", Type: "lxc", Node: "pve-a", Name: "dns", Status: "stopped", VMID: 200},
		{ID: "storage/pve-a/vmstore", Type: "storage", Node: "pve-a", Storage: "vmstore", Status: "available", Disk: 85, MaxDisk: 100, Shared: &shared},
	}, nil
}

func (fixtureAPI) GetActiveTasks(_ context.Context, node string) ([]Task, error) {
	if node != "pve-a" {
		return []Task{}, nil
	}
	endedAt := int64(101)
	return []Task{
		{ID: "100", Node: "pve-a", Type: "vncproxy"},
		{ID: "100", Node: "pve-b", Type: "vncproxy"},
		{ID: "200", Node: "pve-a", Type: "vncproxy", EndTime: &endedAt},
		{ID: "not-a-vmid", Node: "pve-a", Type: "vncproxy"},
	}, nil
}

func (fixtureAPI) GetGuestConfig(_ context.Context, node, guestType string, vmid int) (GuestConfig, error) {
	if node != "pve-a" || guestType != "qemu" || vmid != 100 {
		return nil, errors.New("unexpected guest")
	}
	return GuestConfig{
		"boot":     "order=efidisk0;ide2;scsi0",
		"efidisk0": "local-zfs:vm-100-disk-0,size=4M",
		"ide2":     "local:iso/installer.iso,media=cdrom",
		"scsi0":    "vmstore:vm-100-disk-1,size=64G",
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
	if len(payload.Resources) != 6 || len(payload.Observations) != 7 {
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
	idleByNode, ok := payload.Observations[0].Metadata["idle_vms_by_node"].(map[string]int)
	if !ok {
		t.Fatalf("idle_vms_by_node = %#v", payload.Observations[0].Metadata["idle_vms_by_node"])
	}
	if idleByNode["pve-a"] != 0 || idleByNode["pve-b"] != 0 {
		t.Fatalf("idle_vms_by_node = %#v", idleByNode)
	}
	activeByNode, ok := payload.Observations[0].Metadata["active_vms_by_node"].(map[string]int)
	if !ok || activeByNode["pve-a"] != 1 {
		t.Fatalf("active_vms_by_node = %#v", payload.Observations[0].Metadata["active_vms_by_node"])
	}
	connectionsByNode, ok := payload.Observations[0].Metadata["console_connections_by_node"].(map[string]int)
	if !ok || connectionsByNode["pve-a"] != 2 || connectionsByNode["pve-b"] != 0 {
		t.Fatalf("console_connections_by_node = %#v", payload.Observations[0].Metadata["console_connections_by_node"])
	}
	if available, ok := payload.Observations[0].Metadata["console_activity_available"].(bool); !ok || !available {
		t.Fatalf("console_activity_available = %#v", payload.Observations[0].Metadata["console_activity_available"])
	}
	if available, ok := payload.Observations[0].Metadata["storage_attribution_available"].(bool); !ok || !available {
		t.Fatalf("storage_attribution_available = %#v", payload.Observations[0].Metadata["storage_attribution_available"])
	}
	if idle := payload.Observations[0].Metadata["idle_vms_by_storage"].(map[string]int); len(idle) != 0 {
		t.Fatalf("idle_vms_by_storage = %#v", idle)
	}
	if active := payload.Observations[0].Metadata["active_vms_by_storage"].(map[string]int); active["vmstore"] != 1 || len(active) != 1 {
		t.Fatalf("active_vms_by_storage = %#v", active)
	}
	if connections := payload.Observations[0].Metadata["console_connections_by_storage"].(map[string]int); connections["vmstore"] != 2 || len(connections) != 1 {
		t.Fatalf("console_connections_by_storage = %#v", connections)
	}
}

func TestConsoleConnectionsFollowTheCurrentGuestNodeAfterMigration(t *testing.T) {
	connections := consoleConnectionsByGuest(
		[]Task{{ID: "100", Node: "pve-source", Type: "vncproxy"}},
		map[int]string{100: "pve-destination"},
	)
	if got := connections["pve-destination"][100]; got != 1 {
		t.Fatalf("connection count at current guest node = %d, want 1", got)
	}
	if _, found := connections["pve-source"]; found {
		t.Fatalf("connection was attributed to the prior task node: %#v", connections)
	}
}

type duplicatedSharedStorageAPI struct{ fixtureAPI }

func (duplicatedSharedStorageAPI) GetClusterResources(context.Context) ([]ClusterResource, error) {
	resources, err := fixtureAPI{}.GetClusterResources(context.Background())
	if err != nil {
		return nil, err
	}
	shared := 1
	return append(resources, ClusterResource{
		ID: "storage/pve-b/vmstore", Type: "storage", Node: "pve-b", Storage: "vmstore",
		Status: "available", Disk: 85, MaxDisk: 100, Shared: &shared,
	}), nil
}

func TestCollectCollapsesDuplicateSharedStorageEntries(t *testing.T) {
	payload, err := Collect(context.Background(), Config{
		BaseURL: "https://proxmox.example:8006", DeploymentID: "lab",
		TokenID: "user@pve!espial", TokenSecret: "secret",
	}, duplicatedSharedStorageAPI{}, time.Unix(100, 0).UTC())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	storageResources := 0
	storageObservations := 0
	for _, resource := range payload.Resources {
		if resource.ExternalID == "proxmox:lab:storage:shared:vmstore" {
			storageResources++
		}
	}
	for _, observation := range payload.Observations {
		if observation.ExternalResourceID == "proxmox:lab:storage:shared:vmstore" {
			storageObservations++
		}
	}
	if storageResources != 1 || storageObservations != 2 {
		t.Fatalf("shared storage resources=%d observations=%d", storageResources, storageObservations)
	}
}

type unavailableTasksAPI struct{ fixtureAPI }

func (unavailableTasksAPI) GetActiveTasks(ctx context.Context, node string) ([]Task, error) {
	if node == "pve-b" {
		return nil, errors.New("forbidden")
	}
	return fixtureAPI{}.GetActiveTasks(ctx, node)
}

func TestCollectPreservesCoreMetricsWhenConsoleActivityIsUnavailable(t *testing.T) {
	payload, err := Collect(context.Background(), Config{
		BaseURL: "https://proxmox.example:8006", DeploymentID: "lab",
		TokenID: "user@pve!espial", TokenSecret: "secret",
	}, unavailableTasksAPI{}, time.Unix(100, 0).UTC())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if available, ok := payload.Observations[0].Metadata["console_activity_available"].(bool); !ok || available {
		t.Fatalf("console_activity_available = %#v", payload.Observations[0].Metadata["console_activity_available"])
	}
	if active := payload.Observations[0].Metadata["active_vms_by_node"].(map[string]int); len(active) != 0 {
		t.Fatalf("active_vms_by_node = %#v", active)
	}
	if connections := payload.Observations[0].Metadata["console_connections_by_node"].(map[string]int); len(connections) != 0 {
		t.Fatalf("console_connections_by_node = %#v", connections)
	}
	if idle := payload.Observations[0].Metadata["idle_vms_by_node"].(map[string]int); idle["pve-a"] != 1 {
		t.Fatalf("idle_vms_by_node = %#v", idle)
	}
	if idle := payload.Observations[0].Metadata["idle_vms_by_storage"].(map[string]int); idle["vmstore"] != 1 {
		t.Fatalf("idle_vms_by_storage = %#v", idle)
	}
}

type unavailableGuestConfigAPI struct{ fixtureAPI }

func (unavailableGuestConfigAPI) GetGuestConfig(context.Context, string, string, int) (GuestConfig, error) {
	return nil, errors.New("forbidden")
}

func TestCollectWithholdsPartialStorageAttribution(t *testing.T) {
	payload, err := Collect(context.Background(), Config{
		BaseURL: "https://proxmox.example:8006", DeploymentID: "lab",
		TokenID: "user@pve!espial", TokenSecret: "secret",
	}, unavailableGuestConfigAPI{}, time.Unix(100, 0).UTC())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if available := payload.Observations[0].Metadata["storage_attribution_available"]; available != false {
		t.Fatalf("storage_attribution_available = %#v", available)
	}
	if idle := payload.Observations[0].Metadata["idle_vms_by_storage"].(map[string]int); len(idle) != 0 {
		t.Fatalf("idle_vms_by_storage = %#v", idle)
	}
	if active := payload.Observations[0].Metadata["active_vms_by_storage"].(map[string]int); len(active) != 0 {
		t.Fatalf("active_vms_by_storage = %#v", active)
	}
	if connections := payload.Observations[0].Metadata["console_connections_by_storage"].(map[string]int); len(connections) != 0 {
		t.Fatalf("console_connections_by_storage = %#v", connections)
	}
}

func TestMainDiskStorageExcludesEFIAndISOVolumes(t *testing.T) {
	storage, ok := mainDiskStorage(GuestConfig{
		"boot":     "order=efidisk0;ide2;scsi0",
		"efidisk0": "local-zfs:vm-100-disk-0,size=4M",
		"ide2":     "local:iso/installer.iso,media=cdrom",
		"scsi0":    "iscsi-main:vm-100-disk-1,size=64G",
	})
	if !ok || storage != "iscsi-main" {
		t.Fatalf("mainDiskStorage() = %q, %t", storage, ok)
	}
}

func TestMainDiskStorageUsesContainerRootFS(t *testing.T) {
	storage, ok := mainDiskStorage(GuestConfig{"rootfs": "iscsi-main:subvol-200-disk-0,size=8G"})
	if !ok || storage != "iscsi-main" {
		t.Fatalf("mainDiskStorage() = %q, %t", storage, ok)
	}
}
