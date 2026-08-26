package proxmox

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/adapters"
	"github.com/PrincepsVIIII/Espial/core/internal/health"
)

type API interface {
	GetVersion(context.Context) (VersionResponse, error)
	GetClusterStatus(context.Context) ([]ClusterStatusItem, error)
	GetClusterResources(context.Context) ([]ClusterResource, error)
	GetNodeStatus(context.Context, string) (NodeStatus, error)
}

func Collect(ctx context.Context, config Config, api API, observedAt time.Time) (adapters.CollectionPayload, error) {
	config = WithDefaults(config)
	if err := ValidateConfig(config); err != nil {
		return adapters.CollectionPayload{}, err
	}
	version, err := api.GetVersion(ctx)
	if err != nil {
		return adapters.CollectionPayload{}, err
	}
	clusterStatus, err := api.GetClusterStatus(ctx)
	if err != nil {
		return adapters.CollectionPayload{}, err
	}
	resources, err := api.GetClusterResources(ctx)
	if err != nil {
		return adapters.CollectionPayload{}, err
	}
	nodes := nodeIDs(clusterStatus, resources)
	nodeStatus := make(map[string]NodeStatus, len(nodes))
	for _, node := range nodes {
		status, err := api.GetNodeStatus(ctx, node)
		if err != nil {
			return adapters.CollectionPayload{}, err
		}
		nodeStatus[node] = status
	}
	return mapPayload(config, version, clusterStatus, resources, nodeStatus, observedAt.UTC()), nil
}

func mapPayload(config Config, version VersionResponse, status []ClusterStatusItem, source []ClusterResource, nodeStatus map[string]NodeStatus, observedAt time.Time) adapters.CollectionPayload {
	payload := adapters.CollectionPayload{}
	clusterExternalID := clusterID(config.DeploymentID)
	clusterState, clusterSummary := clusterHealth(status)
	payload.Resources = append(payload.Resources, adapters.CollectionResource{
		ExternalID: clusterExternalID, Kind: ClusterKind, DisplayName: config.DeploymentID,
		ObservedAt: observedAt, SourceURL: config.BaseURL,
		Attributes: map[string]any{
			"deployment_id": config.DeploymentID, "version": version.Version,
			"release": version.Release, "repoid": version.RepoID,
		},
	})
	payload.Observations = append(payload.Observations, adapters.CollectionObservation{
		ExternalResourceID: clusterExternalID, CheckType: ClusterQuorumCheck,
		State: clusterState, Summary: clusterSummary, ObservedAt: observedAt,
		ExpectedRefreshSeconds: config.ExpectedRefreshSeconds,
		Metadata:               map[string]any{"reason_code": reasonCode(clusterState, "cluster_quorum")},
	})

	expected := expectedGuestSet(config.ExpectedRunningGuests)
	activeByNode := map[string]int{}
	for _, item := range source {
		switch item.Type {
		case "node":
			node := itemNode(item)
			status := nodeStatus[node]
			cpuUsed, cpuMax := item.CPU, item.MaxCPU
			if cpuMax == 0 {
				cpuMax = statusCPUCapacity(status)
			}
			if cpuUsed > 1 && cpuMax > 0 {
				cpuUsed = cpuUsed / cpuMax
			}
			memoryUsed, memoryMax := item.Mem, item.MaxMem
			if memoryMax == 0 {
				memoryUsed, memoryMax = status.Memory.Used, status.Memory.Total
			}
			swapPercent := percent(status.Swap.Used, status.Swap.Total)
			ioWaitPercent := clamp(status.Wait * 100)
			payload.Resources = append(payload.Resources, adapters.CollectionResource{
				ExternalID: nodeID(config.DeploymentID, node), Kind: NodeKind, DisplayName: node,
				ObservedAt: observedAt, SourceURL: sourceURL(config.BaseURL, "/pve2/nodes/"+url.PathEscape(node)),
				Attributes: map[string]any{"node": node, "cpu_cores": cpuMax, "memory_total_bytes": memoryMax},
			})
			state := health.Healthy
			summary := "Node is online."
			if item.Status != "" && item.Status != "online" {
				state, summary = health.Critical, "Node is not online."
			}
			payload.Observations = append(payload.Observations, adapters.CollectionObservation{
				ExternalResourceID: nodeID(config.DeploymentID, node), CheckType: NodeAvailabilityCheck,
				State: state, Summary: summary, ObservedAt: observedAt,
				ExpectedRefreshSeconds: config.ExpectedRefreshSeconds,
				Measurements: map[string]any{
					"cpu_percent": clamp(cpuUsed * 100), "cpu_cores": cpuMax,
					"memory_used_bytes": memoryUsed, "memory_total_bytes": memoryMax,
					"swap_used_bytes": status.Swap.Used, "swap_total_bytes": status.Swap.Total,
					"swap_percent": swapPercent, "io_wait_percent": ioWaitPercent,
				},
				Metadata: map[string]any{
					"reason_code":       reasonCode(state, "node_availability"),
					"swap_indicator":    indicator(swapPercent, config.SwapWarningPercent, config.SwapCriticalPercent),
					"io_wait_indicator": indicator(ioWaitPercent, config.IOWaitWarningPercent, config.IOWaitCriticalPercent),
				},
			})
		case "qemu", "lxc":
			kind := QEMUKind
			if item.Type == "lxc" {
				kind = LXCKind
			}
			if item.Status == "running" {
				activeByNode[item.Node]++
			}
			guestKey := item.Type + ":" + strconv.Itoa(item.VMID)
			state := health.Healthy
			summary := "Guest is running."
			if item.Status != "running" {
				state, summary = health.Disabled, "Guest is not running."
				if expected[guestKey] {
					state, summary = health.Critical, "Expected-running guest is stopped."
				}
			}
			payload.Resources = append(payload.Resources, adapters.CollectionResource{
				ExternalID: guestID(config.DeploymentID, item.Type, item.VMID), Kind: kind,
				DisplayName: displayName(item.Name, guestKey), ObservedAt: observedAt,
				SourceURL:  sourceURL(config.BaseURL, fmt.Sprintf("/pve2/nodes/%s/%s/%d/summary", url.PathEscape(item.Node), item.Type, item.VMID)),
				Attributes: map[string]any{"node": item.Node, "vmid": item.VMID, "guest_type": item.Type, "status": item.Status},
			})
			payload.Observations = append(payload.Observations, adapters.CollectionObservation{
				ExternalResourceID: guestID(config.DeploymentID, item.Type, item.VMID), CheckType: GuestRuntimeCheck,
				State: state, Summary: summary, ObservedAt: observedAt,
				ExpectedRefreshSeconds: config.ExpectedRefreshSeconds,
				Measurements:           map[string]any{"cpu_percent": clamp(item.CPU * 100), "memory_used_bytes": item.Mem, "memory_total_bytes": item.MaxMem},
				Metadata:               map[string]any{"reason_code": reasonCode(state, "guest_runtime")},
			})
		case "storage":
			scope := storageScope(item)
			used, total := item.Disk, item.MaxDisk
			usedPercent := percent(used, total)
			state := health.Healthy
			summary := "Storage is available."
			if item.Status != "" && item.Status != "available" {
				state, summary = health.Critical, "Storage is not available."
			}
			capacityState := capacityState(usedPercent, config.StorageWarningPercent, config.StorageCriticalPercent)
			if state == health.Critical {
				capacityState = health.Critical
			}
			ioWait := nodeStatus[item.Node].Wait * 100
			externalID := storageID(config.DeploymentID, scope, displayName(item.Storage, item.Name))
			payload.Resources = append(payload.Resources, adapters.CollectionResource{
				ExternalID: externalID, Kind: StorageKind, DisplayName: displayName(item.Storage, item.Name),
				ObservedAt: observedAt, SourceURL: sourceURL(config.BaseURL, "/pve2/nodes/"+url.PathEscape(item.Node)+"/storage"),
				Attributes: map[string]any{"node": item.Node, "scope": scope, "storage": displayName(item.Storage, item.Name)},
			})
			payload.Observations = append(payload.Observations,
				adapters.CollectionObservation{ExternalResourceID: externalID, CheckType: StorageAvailabilityCheck,
					State: state, Summary: summary, ObservedAt: observedAt, ExpectedRefreshSeconds: config.ExpectedRefreshSeconds,
					Metadata: map[string]any{"reason_code": reasonCode(state, "storage_availability")}},
				adapters.CollectionObservation{ExternalResourceID: externalID, CheckType: StorageCapacityCheck,
					State: capacityState, Summary: fmt.Sprintf("Storage is %.1f%% full.", usedPercent),
					ObservedAt: observedAt, ExpectedRefreshSeconds: config.ExpectedRefreshSeconds,
					Measurements: map[string]any{"used_bytes": used, "total_bytes": total, "used_percent": usedPercent, "io_wait_percent": clamp(ioWait)},
					Metadata: map[string]any{
						"reason_code":       reasonCode(capacityState, "storage_capacity"),
						"io_wait_indicator": indicator(clamp(ioWait), config.IOWaitWarningPercent, config.IOWaitCriticalPercent),
					}},
			)
		}
	}
	sort.Slice(payload.Resources, func(i, j int) bool { return payload.Resources[i].ExternalID < payload.Resources[j].ExternalID })
	for index := range payload.Observations {
		if payload.Observations[index].Metadata == nil {
			payload.Observations[index].Metadata = map[string]any{}
		}
		payload.Observations[index].Metadata["active_vms_by_node"] = activeByNode
	}
	return payload
}

func clusterHealth(items []ClusterStatusItem) (health.State, string) {
	for _, item := range items {
		if item.Type == "cluster" && item.Quorate != nil && *item.Quorate == 0 {
			return health.Critical, "Cluster quorum is lost."
		}
	}
	return health.Healthy, "Cluster quorum is healthy."
}

func nodeIDs(status []ClusterStatusItem, resources []ClusterResource) []string {
	nodes := map[string]struct{}{}
	for _, item := range status {
		if item.Type == "node" && item.Name != "" {
			nodes[item.Name] = struct{}{}
		}
	}
	for _, item := range resources {
		if item.Type == "node" {
			nodes[itemNode(item)] = struct{}{}
		}
	}
	result := make([]string, 0, len(nodes))
	for node := range nodes {
		result = append(result, node)
	}
	sort.Strings(result)
	return result
}

func expectedGuestSet(values []string) map[string]bool {
	result := map[string]bool{}
	for _, value := range values {
		result[value] = true
	}
	return result
}

func capacityState(value, warning, critical float64) health.State {
	if value >= critical {
		return health.Critical
	}
	if value >= warning {
		return health.Warning
	}
	return health.Healthy
}

func indicator(value, warning, critical float64) string {
	if value >= critical {
		return "critical"
	}
	if value >= warning {
		return "warning"
	}
	return "healthy"
}

func reasonCode(state health.State, base string) string {
	if state == health.Healthy {
		return base + "_healthy"
	}
	return base + "_" + string(state)
}

func percent(used, total float64) float64 {
	if total <= 0 {
		return 0
	}
	return clamp(used / total * 100)
}

func clamp(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func statusCPUCapacity(status NodeStatus) float64 {
	if len(status.LoadAvg) == 0 {
		return 0
	}
	return 0
}

func itemNode(item ClusterResource) string {
	if item.Node != "" {
		return item.Node
	}
	return strings.TrimPrefix(item.ID, "node/")
}

func storageScope(item ClusterResource) string {
	if item.Shared != nil && *item.Shared == 1 {
		return "shared"
	}
	if item.Node != "" {
		return item.Node
	}
	return "unknown"
}

func displayName(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return strings.TrimSpace(primary)
	}
	return fallback
}

func sourceURL(base, path string) string {
	target, err := url.Parse(base)
	if err != nil {
		return base
	}
	target.Path = path
	target.RawQuery = ""
	target.Fragment = ""
	return target.String()
}

func clusterID(deployment string) string { return "proxmox:" + deployment + ":cluster" }
func nodeID(deployment, node string) string {
	return "proxmox:" + deployment + ":node:" + node
}
func guestID(deployment, guestType string, vmid int) string {
	return "proxmox:" + deployment + ":" + guestType + ":" + strconv.Itoa(vmid)
}
func storageID(deployment, scope, storage string) string {
	return "proxmox:" + deployment + ":storage:" + scope + ":" + storage
}
