// Package hypervisor exposes typed virtualization read models.
package hypervisor

import "time"

type ProxmoxClusterDefinition struct {
	DisplayName            string
	BaseURL                string
	ApproveTarget          bool
	AllowInsecureHTTP      bool
	DeploymentID           string
	TokenID                string
	TokenSecret            string
	IntervalSeconds        int
	ExpectedRefreshSeconds int
	RequestTimeoutSeconds  int
	StorageWarningPercent  float64
	StorageCriticalPercent float64
	SwapWarningPercent     float64
	SwapCriticalPercent    float64
	IOWaitWarningPercent   float64
	IOWaitCriticalPercent  float64
	ExpectedRunningGuests  []string
	ActorUserID            string
	SourceAddress          string
	CorrelationID          string
	CanReadAudit           bool
}

type ClusterList struct {
	Items []ClusterCard `json:"items"`
}

type ClusterCard struct {
	ID             string            `json:"id"`
	IntegrationID  string            `json:"integration_id"`
	Name           string            `json:"name"`
	State          string            `json:"state"`
	Freshness      string            `json:"freshness"`
	LastObservedAt *time.Time        `json:"last_observed_at,omitempty"`
	LastCollection *time.Time        `json:"last_collection_at,omitempty"`
	Hosts          []HostSummary     `json:"hosts"`
	Storage        []StorageSummary  `json:"storage"`
	Totals         UtilizationTotals `json:"totals"`
	LoadNodes      []LoadNodeSummary `json:"load_nodes"`
}

type HostSummary struct {
	ID            string  `json:"id"`
	CPUPercent    float64 `json:"cpu_percent"`
	RAMPercent    float64 `json:"ram_percent"`
	CPUCores      float64 `json:"cpu_cores"`
	RAMBytes      float64 `json:"ram_bytes"`
	SwapPercent   float64 `json:"swap_percent"`
	SwapIndicator string  `json:"swap_indicator"`
	HealthState   string  `json:"health_state"`
}

type StorageSummary struct {
	ID              string  `json:"id"`
	Scope           string  `json:"scope"`
	UsedBytes       float64 `json:"used_bytes"`
	TotalBytes      float64 `json:"total_bytes"`
	UsedPercent     float64 `json:"used_percent"`
	IOWaitPercent   float64 `json:"io_wait_percent"`
	IOWaitIndicator string  `json:"io_wait_indicator"`
	HealthState     string  `json:"health_state"`
}

type UtilizationTotals struct {
	CPUPercent     float64 `json:"cpu_percent"`
	RAMPercent     float64 `json:"ram_percent"`
	StoragePercent float64 `json:"storage_percent"`
}

type LoadNodeSummary struct {
	ID        string `json:"id"`
	ActiveVMs int    `json:"active_vms"`
}
