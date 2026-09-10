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
	GetActiveTasks(context.Context, string) ([]Task, error)
	GetGuestConfig(context.Context, string, string, int) (GuestConfig, error)
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
	activeTasks := []Task{}
	consoleActivityAvailable := true
	nodeStatus := make(map[string]NodeStatus, len(nodes))
	for _, node := range nodes {
		if consoleActivityAvailable {
			tasks, err := api.GetActiveTasks(ctx, node)
			if err != nil {
				// A partial node read cannot support an authoritative cluster
				// total. Discard partial counts instead of presenting them as
				// complete, while preserving the rest of the collection.
				consoleActivityAvailable = false
				activeTasks = nil
			} else {
				activeTasks = append(activeTasks, tasks...)
			}
		}
		status, err := api.GetNodeStatus(ctx, node)
		if err != nil {
			return adapters.CollectionPayload{}, err
		}
		nodeStatus[node] = status
	}
	guestConfigs, storageAttributionAvailable := runningGuestConfigs(ctx, api, resources)
	return mapPayload(config, version, clusterStatus, resources, nodeStatus, activeTasks, consoleActivityAvailable, guestConfigs, storageAttributionAvailable, observedAt.UTC()), nil
}

func mapPayload(config Config, version VersionResponse, status []ClusterStatusItem, source []ClusterResource, nodeStatus map[string]NodeStatus, activeTasks []Task, consoleActivityAvailable bool, guestConfigs map[guestReference]GuestConfig, storageAttributionAvailable bool, observedAt time.Time) adapters.CollectionPayload {
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
	runningGuests := map[string]map[int]struct{}{}
	for _, item := range source {
		switch item.Type {
		case "node":
			node := itemNode(item)
			if _, exists := runningGuests[node]; !exists {
				runningGuests[node] = map[int]struct{}{}
			}
		case "qemu", "lxc":
			if item.Status != "running" {
				continue
			}
			if _, exists := runningGuests[item.Node]; !exists {
				runningGuests[item.Node] = map[int]struct{}{}
			}
			runningGuests[item.Node][item.VMID] = struct{}{}
		}
	}
	activeByNode := map[string]int{}
	consoleConnections := map[string]int{}
	activeConnections := map[string]map[int]int{}
	if consoleActivityAvailable {
		activeConnections = consoleConnectionsByGuest(activeTasks, runningGuestNodes(runningGuests))
		activeByNode = activeConsoleVMs(activeConnections, runningGuests)
		consoleConnections = consoleConnectionsByNode(activeConnections, runningGuests)
	}
	// Idle means a running guest without an open Proxmox console proxy. The
	// running-guest inventory remains available even when the optional task read
	// is forbidden; in that case console activity is marked unavailable instead
	// of silently treating it as zero.
	idleByNode := make(map[string]int, len(runningGuests))
	for node, guests := range runningGuests {
		idleByNode[node] = len(guests) - activeByNode[node]
		if idleByNode[node] < 0 {
			idleByNode[node] = 0
		}
	}
	storageIdleVMs := map[string]int{}
	storageActiveVMs := map[string]int{}
	storageConsoleConnections := map[string]int{}
	if storageAttributionAvailable {
		activeGuests := activeConsoleGuests(activeConnections)
		storageIdleVMs, storageActiveVMs, storageConsoleConnections = storageVMCounts(guestConfigs, activeGuests, activeConnections)
	}
	// /cluster/resources can report the same shared storage once per node. The
	// shared scope deliberately gives those rows one Espial identity, so emit it
	// once rather than producing an invalid batch with duplicate resource IDs.
	seenStorage := map[string]struct{}{}
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
			externalID := storageID(config.DeploymentID, scope, displayName(item.Storage, item.Name))
			if _, exists := seenStorage[externalID]; exists {
				continue
			}
			seenStorage[externalID] = struct{}{}
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
		payload.Observations[index].Metadata["idle_vms_by_node"] = idleByNode
		payload.Observations[index].Metadata["active_vms_by_node"] = activeByNode
		payload.Observations[index].Metadata["console_connections_by_node"] = consoleConnections
		payload.Observations[index].Metadata["console_activity_available"] = consoleActivityAvailable
		payload.Observations[index].Metadata["idle_vms_by_storage"] = storageIdleVMs
		payload.Observations[index].Metadata["active_vms_by_storage"] = storageActiveVMs
		payload.Observations[index].Metadata["console_connections_by_storage"] = storageConsoleConnections
		payload.Observations[index].Metadata["storage_attribution_available"] = storageAttributionAvailable
	}
	return payload
}

func activeConsoleVMs(connections map[string]map[int]int, runningGuests map[string]map[int]struct{}) map[string]int {
	result := make(map[string]int, len(runningGuests))
	for node := range runningGuests {
		result[node] = len(connections[node])
	}
	return result
}

// consoleConnectionsByGuest accepts an active vncproxy task when its VMID belongs
// to any currently running guest. Proxmox keeps a console task on the node where
// it was opened while a guest can migrate, so task.Node is not a reliable current
// guest-location filter. Aggregate to the current guest node instead, keeping the
// compute and storage comparisons aligned with the resource inventory.
func consoleConnectionsByGuest(tasks []Task, runningNodes map[int]string) map[string]map[int]int {
	connections := map[string]map[int]int{}
	for _, task := range tasks {
		if task.Type != "vncproxy" || task.EndTime != nil {
			continue
		}
		vmid, err := strconv.Atoi(strings.TrimSpace(task.ID))
		if err != nil || vmid <= 0 {
			continue
		}
		node, running := runningNodes[vmid]
		if !running {
			continue
		}
		if _, exists := connections[node]; !exists {
			connections[node] = map[int]int{}
		}
		connections[node][vmid]++
	}
	return connections
}

func runningGuestNodes(guests map[string]map[int]struct{}) map[int]string {
	nodes := map[int]string{}
	for node, vmids := range guests {
		for vmid := range vmids {
			nodes[vmid] = node
		}
	}
	return nodes
}

func activeConsoleGuests(connections map[string]map[int]int) map[string]map[int]struct{} {
	guests := make(map[string]map[int]struct{}, len(connections))
	for node, byGuest := range connections {
		guests[node] = make(map[int]struct{}, len(byGuest))
		for vmid := range byGuest {
			guests[node][vmid] = struct{}{}
		}
	}
	return guests
}

func consoleConnectionsByNode(connections map[string]map[int]int, runningGuests map[string]map[int]struct{}) map[string]int {
	result := make(map[string]int, len(runningGuests))
	for node := range runningGuests {
		for _, count := range connections[node] {
			result[node] += count
		}
	}
	return result
}

type guestReference struct {
	Node string
	Type string
	VMID int
}

func runningGuestConfigs(ctx context.Context, api API, source []ClusterResource) (map[guestReference]GuestConfig, bool) {
	refs := map[guestReference]struct{}{}
	for _, item := range source {
		if (item.Type != "qemu" && item.Type != "lxc") || item.Status != "running" || item.Node == "" || item.VMID <= 0 {
			continue
		}
		refs[guestReference{Node: item.Node, Type: item.Type, VMID: item.VMID}] = struct{}{}
	}
	ordered := make([]guestReference, 0, len(refs))
	for ref := range refs {
		ordered = append(ordered, ref)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Node != ordered[j].Node {
			return ordered[i].Node < ordered[j].Node
		}
		if ordered[i].Type != ordered[j].Type {
			return ordered[i].Type < ordered[j].Type
		}
		return ordered[i].VMID < ordered[j].VMID
	})
	configs := make(map[guestReference]GuestConfig, len(ordered))
	for _, ref := range ordered {
		config, err := api.GetGuestConfig(ctx, ref.Node, ref.Type, ref.VMID)
		if err != nil {
			// A partial disk attribution would incorrectly move capacity between
			// storage targets. Keep Compute available, but withhold Storage.
			return nil, false
		}
		configs[ref] = config
	}
	return configs, true
}

func storageVMCounts(configs map[guestReference]GuestConfig, activeGuests map[string]map[int]struct{}, connections map[string]map[int]int) (map[string]int, map[string]int, map[string]int) {
	idle := map[string]int{}
	active := map[string]int{}
	consoleConnections := map[string]int{}
	for ref, config := range configs {
		storage, ok := mainDiskStorage(config)
		if !ok {
			continue
		}
		if _, open := activeGuests[ref.Node][ref.VMID]; open {
			active[storage]++
			consoleConnections[storage] += connections[ref.Node][ref.VMID]
		} else {
			idle[storage]++
		}
	}
	return idle, active, consoleConnections
}

func mainDiskStorage(config GuestConfig) (string, bool) {
	for _, device := range bootDevices(config["boot"]) {
		if storage, ok := diskStorage(config[device]); ok {
			return storage, true
		}
	}
	devices := make([]string, 0, len(config))
	for device := range config {
		if isDiskDevice(device) {
			devices = append(devices, device)
		}
	}
	sort.Slice(devices, func(i, j int) bool { return diskDeviceLess(devices[i], devices[j]) })
	for _, device := range devices {
		if storage, ok := diskStorage(config[device]); ok {
			return storage, true
		}
	}
	return "", false
}

func bootDevices(value string) []string {
	order := strings.TrimSpace(value)
	if strings.HasPrefix(order, "order=") {
		order = strings.TrimPrefix(order, "order=")
	}
	if order == "" {
		return nil
	}
	devices := []string{}
	for _, device := range strings.Split(order, ";") {
		device = strings.TrimSpace(device)
		if isDiskDevice(device) {
			devices = append(devices, device)
		}
	}
	return devices
}

func isDiskDevice(device string) bool {
	if device == "rootfs" {
		return true
	}
	for _, prefix := range []string{"scsi", "virtio", "sata", "ide", "nvme"} {
		suffix := strings.TrimPrefix(device, prefix)
		if suffix != device && suffix != "" {
			if _, err := strconv.Atoi(suffix); err == nil {
				return true
			}
		}
	}
	return false
}

func diskDeviceLess(left, right string) bool {
	priority := func(value string) int {
		for index, prefix := range []string{"scsi", "virtio", "sata", "ide", "nvme", "rootfs"} {
			if value == prefix || strings.HasPrefix(value, prefix) {
				return index
			}
		}
		return len([]string{"scsi", "virtio", "sata", "ide", "nvme", "rootfs"})
	}
	if priority(left) != priority(right) {
		return priority(left) < priority(right)
	}
	return left < right
}

func diskStorage(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "media=cdrom") || strings.Contains(value, "cdrom") || strings.Contains(value, "cloudinit") {
		return "", false
	}
	storage, _, found := strings.Cut(value, ":")
	storage = strings.TrimSpace(storage)
	if !found || storage == "" || strings.ContainsAny(storage, "/,=") {
		return "", false
	}
	return storage, true
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
