// Package hypervisor exposes typed virtualization read models.
package hypervisor

import "time"

type ProxmoxClusterDefinition struct {
	DisplayName                string
	BaseURL                    string
	ApproveTarget              bool
	AllowInsecureHTTP          bool
	TrustSelfSignedCertificate bool
	AllowUnverifiedTLS         bool
	DeploymentID               string
	TokenID                    string
	TokenSecret                string
	IntervalSeconds            int
	ExpectedRefreshSeconds     int
	RequestTimeoutSeconds      int
	StorageWarningPercent      float64
	StorageCriticalPercent     float64
	SwapWarningPercent         float64
	SwapCriticalPercent        float64
	IOWaitWarningPercent       float64
	IOWaitCriticalPercent      float64
	ExpectedRunningGuests      []string
	ActorUserID                string
	SourceAddress              string
	CorrelationID              string
	CanReadAudit               bool
}

// ProxmoxClusterConfiguration is the administrator-safe view of a typed
// Proxmox integration. Token material and secret-store references are never
// included; TokenSecretConfigured only confirms that a secret is available.
type ProxmoxClusterConfiguration struct {
	ID                         string    `json:"id"`
	DisplayName                string    `json:"display_name"`
	BaseURL                    string    `json:"base_url"`
	ApproveTarget              bool      `json:"approve_target"`
	AllowInsecureHTTP          bool      `json:"allow_insecure_http"`
	TrustSelfSignedCertificate bool      `json:"trust_self_signed_certificate"`
	AllowUnverifiedTLS         bool      `json:"allow_unverified_tls"`
	DeploymentID               string    `json:"deployment_id"`
	TokenID                    string    `json:"token_id"`
	TokenSecretConfigured      bool      `json:"token_secret_configured"`
	IntervalSeconds            int       `json:"interval_seconds"`
	ExpectedRefreshSeconds     int       `json:"expected_refresh_seconds"`
	RequestTimeoutSeconds      int       `json:"request_timeout_seconds"`
	StorageWarningPercent      float64   `json:"storage_warning_percent"`
	StorageCriticalPercent     float64   `json:"storage_critical_percent"`
	SwapWarningPercent         float64   `json:"swap_warning_percent"`
	SwapCriticalPercent        float64   `json:"swap_critical_percent"`
	IOWaitWarningPercent       float64   `json:"io_wait_warning_percent"`
	IOWaitCriticalPercent      float64   `json:"io_wait_critical_percent"`
	ExpectedRunningGuests      []string  `json:"expected_running_guests"`
	SortOrder                  int       `json:"sort_order"`
	UpdatedAt                  time.Time `json:"updated_at"`
	Version                    int64     `json:"version"`
}

type ProxmoxClusterUpdateDefinition struct {
	ProxmoxClusterDefinition
	ID                string
	ExpectedUpdatedAt time.Time
}

type ProxmoxClusterOrderDefinition struct {
	ID                string
	Direction         string
	ExpectedUpdatedAt time.Time
	ActorUserID       string
	SourceAddress     string
	CorrelationID     string
	CanReadAudit      bool
}

type ClusterList struct {
	Items []ClusterCard `json:"items"`
}

type ClusterCard struct {
	ID                          string            `json:"id"`
	IntegrationID               string            `json:"integration_id"`
	Name                        string            `json:"name"`
	State                       string            `json:"state"`
	Freshness                   string            `json:"freshness"`
	LastObservedAt              *time.Time        `json:"last_observed_at,omitempty"`
	LastCollection              *time.Time        `json:"last_collection_at,omitempty"`
	Hosts                       []HostSummary     `json:"hosts"`
	Storage                     []StorageSummary  `json:"storage"`
	Totals                      UtilizationTotals `json:"totals"`
	LoadNodes                   []LoadNodeSummary `json:"load_nodes"`
	ConsoleActivityAvailable    bool              `json:"console_activity_available"`
	StorageLoadNodes            []LoadNodeSummary `json:"storage_load_nodes"`
	StorageAttributionAvailable bool              `json:"storage_attribution_available"`
	SortOrder                   int               `json:"-"`
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
	ID                 string `json:"id"`
	IdleVMs            int    `json:"idle_vms"`
	ActiveVMs          int    `json:"active_vms"`
	ConsoleConnections int    `json:"console_connections"`
}
