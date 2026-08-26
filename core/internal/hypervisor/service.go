package hypervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/adapters"
	"github.com/PrincepsVIIII/Espial/core/internal/adminops"
	"github.com/PrincepsVIIII/Espial/core/internal/monitoring"
	"github.com/PrincepsVIIII/Espial/core/internal/proxmox"
	"github.com/jackc/pgx/v5/pgxpool"
)

type IntegrationCreator interface {
	Create(context.Context, monitoring.CreateIntegration) (string, time.Time, error)
}

type Service struct {
	pool      *pgxpool.Pool
	creator   IntegrationCreator
	secrets   adapters.SecretStore
	policy    proxmox.Policy
	newClient func(proxmox.Config) (proxmox.API, error)
	now       func() time.Time
}

func NewService(pool *pgxpool.Pool, creator IntegrationCreator, secrets adapters.SecretStore, policy proxmox.Policy, now func() time.Time) *Service {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{
		pool: pool, creator: creator, secrets: secrets, policy: policy, now: now,
		newClient: func(config proxmox.Config) (proxmox.API, error) {
			return proxmox.NewClient(config, proxmoxClientOptions(policy, config))
		},
	}
}

func (service *Service) CreateProxmoxCluster(ctx context.Context, definition ProxmoxClusterDefinition) (adminops.Receipt, error) {
	if service.creator == nil || service.secrets == nil {
		return adminops.Receipt{}, fmt.Errorf("hypervisor management unavailable")
	}
	config := proxmox.WithDefaults(proxmox.Config{
		BaseURL: definition.BaseURL, DeploymentID: definition.DeploymentID, AllowInsecureHTTP: definition.AllowInsecureHTTP,
		TokenID: definition.TokenID, TokenSecret: definition.TokenSecret,
		ExpectedRefreshSeconds: definition.ExpectedRefreshSeconds,
		RequestTimeoutSeconds:  definition.RequestTimeoutSeconds,
		StorageWarningPercent:  definition.StorageWarningPercent,
		StorageCriticalPercent: definition.StorageCriticalPercent,
		SwapWarningPercent:     definition.SwapWarningPercent,
		SwapCriticalPercent:    definition.SwapCriticalPercent,
		IOWaitWarningPercent:   definition.IOWaitWarningPercent,
		IOWaitCriticalPercent:  definition.IOWaitCriticalPercent,
		ExpectedRunningGuests:  definition.ExpectedRunningGuests,
	})
	if !definition.ApproveTarget {
		return adminops.Receipt{}, &monitoring.Error{Code: "target_approval_required"}
	}
	host, port, err := approvedTarget(config.BaseURL)
	if err != nil {
		return adminops.Receipt{}, &monitoring.Error{Code: "invalid_proxmox_config"}
	}
	config.ApprovedHost = host
	config.ApprovedPort = port
	if err := proxmox.ValidateConfig(config); err != nil {
		return adminops.Receipt{}, &monitoring.Error{Code: "invalid_proxmox_config"}
	}
	client, err := service.newClient(config)
	if err != nil {
		return adminops.Receipt{}, &monitoring.Error{Code: proxmoxCreateErrorCode(err)}
	}
	validationContext, cancel := context.WithTimeout(ctx, proxmox.RequestTimeout(config))
	defer cancel()
	if _, err := proxmox.Collect(validationContext, config, client, service.now()); err != nil {
		return adminops.Receipt{}, &monitoring.Error{Code: proxmoxCreateErrorCode(err)}
	}
	tokenSecretReference := tokenReference(config.DeploymentID, definition.CorrelationID)
	if err := service.secrets.Write(ctx, tokenSecretReference, config.TokenSecret); err != nil {
		return adminops.Receipt{}, &monitoring.Error{Code: "secret_store_unavailable"}
	}
	nonsecret := map[string]any{
		"base_url":                 config.BaseURL,
		"allow_insecure_http":      config.AllowInsecureHTTP,
		"approved_host":            config.ApprovedHost,
		"approved_port":            config.ApprovedPort,
		"deployment_id":            config.DeploymentID,
		"token_id":                 config.TokenID,
		"expected_refresh_seconds": config.ExpectedRefreshSeconds,
		"request_timeout_seconds":  config.RequestTimeoutSeconds,
		"storage_warning_percent":  config.StorageWarningPercent,
		"storage_critical_percent": config.StorageCriticalPercent,
		"swap_warning_percent":     config.SwapWarningPercent,
		"swap_critical_percent":    config.SwapCriticalPercent,
		"io_wait_warning_percent":  config.IOWaitWarningPercent,
		"io_wait_critical_percent": config.IOWaitCriticalPercent,
		"expected_running_guests":  config.ExpectedRunningGuests,
	}
	id, updatedAt, err := service.creator.Create(ctx, monitoring.CreateIntegration{
		AdapterID: proxmox.AdapterID, DisplayName: strings.TrimSpace(definition.DisplayName), Enabled: true, Typed: true,
		Interval:        time.Duration(intervalSeconds(definition.IntervalSeconds)) * time.Second,
		ConfigNonsecret: nonsecret, SecretReferences: map[string]string{"token_secret": tokenSecretReference},
		ActorUserID: definition.ActorUserID, SourceAddress: definition.SourceAddress, CorrelationID: definition.CorrelationID,
	})
	if err != nil {
		return adminops.Receipt{}, err
	}
	receipt := adminops.Receipt{ID: id, Version: updatedAt.UnixMicro(), RequestID: definition.CorrelationID}
	if definition.CanReadAudit {
		receipt.AuditURL = "/audit?correlation_id=" + definition.CorrelationID
	}
	return receipt, nil
}

func approvedTarget(baseURL string) (string, int, error) {
	target, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || target.Hostname() == "" {
		return "", 0, fmt.Errorf("invalid proxmox target")
	}
	port := target.Port()
	if port == "" {
		port = "443"
	}
	parsed, err := strconv.Atoi(port)
	if err != nil {
		return "", 0, err
	}
	return strings.ToLower(target.Hostname()), parsed, nil
}

func proxmoxClientOptions(policy proxmox.Policy, config proxmox.Config) proxmox.ClientOptions {
	hosts := append([]string(nil), policy.ApprovedHosts...)
	if len(hosts) == 0 && config.ApprovedHost != "" {
		hosts = []string{config.ApprovedHost}
	}
	ports := append([]int(nil), policy.AllowedPorts...)
	if len(ports) == 0 && config.ApprovedPort > 0 {
		ports = []int{config.ApprovedPort}
	}
	return proxmox.ClientOptions{ApprovedHosts: hosts, AllowedPorts: ports}
}

func (service *Service) ProxmoxClusters(ctx context.Context) (ClusterList, error) {
	rows, err := service.pool.Query(ctx, `
		SELECT i.id::text, i.display_name, r.id::text, r.external_id, r.kind, r.display_name,
			r.attributes, ch.state, ch.observed_at, o.measurements, o.metadata, run.completed_at
		FROM integrations i
		LEFT JOIN resources r ON r.integration_id = i.id
		LEFT JOIN current_health ch ON ch.resource_id = r.id
		LEFT JOIN observations o ON o.id = ch.observation_id
		LEFT JOIN LATERAL (
			SELECT completed_at FROM integration_collection_runs
			WHERE integration_id = i.id ORDER BY completed_at DESC LIMIT 1
		) run ON true
		WHERE i.adapter_id = $1
		ORDER BY i.display_name, r.kind, r.display_name
	`, proxmox.AdapterID)
	if err != nil {
		return ClusterList{}, fmt.Errorf("read proxmox clusters: %w", err)
	}
	defer rows.Close()
	clusters := map[string]*ClusterCard{}
	order := []string{}
	for rows.Next() {
		var integrationID, integrationName string
		var resourceID, kind, displayName *string
		var attributes, measurements, metadata []byte
		var state *string
		var observedAt, lastCollection *time.Time
		var externalID *string
		if err := rows.Scan(&integrationID, &integrationName, &resourceID, &externalID, &kind, &displayName, &attributes, &state, &observedAt, &measurements, &metadata, &lastCollection); err != nil {
			return ClusterList{}, fmt.Errorf("scan proxmox cluster: %w", err)
		}
		cluster := clusters[integrationID]
		if cluster == nil {
			cluster = &ClusterCard{ID: integrationID, IntegrationID: integrationID, Name: integrationName, State: "unknown", Freshness: "unknown", LastCollection: lastCollection}
			clusters[integrationID] = cluster
			order = append(order, integrationID)
		}
		if kind == nil {
			continue
		}
		attr := object(attributes)
		measures := object(measurements)
		meta := object(metadata)
		resourceState := valueString(state, "unknown")
		if observedAt != nil && (cluster.LastObservedAt == nil || observedAt.After(*cluster.LastObservedAt)) {
			cluster.LastObservedAt = observedAt
		}
		switch *kind {
		case proxmox.ClusterKind:
			cluster.ID = valueString(resourceID, integrationID)
			cluster.State = resourceState
			cluster.Freshness = freshness(resourceState)
			cluster.LoadNodes = loadNodes(meta["active_vms_by_node"])
		case proxmox.NodeKind:
			cluster.Hosts = append(cluster.Hosts, HostSummary{
				ID:         valueString(displayName, attrString(attr, "node")),
				CPUPercent: number(measures["cpu_percent"]), RAMPercent: percent(number(measures["memory_used_bytes"]), number(measures["memory_total_bytes"])),
				CPUCores: number(measures["cpu_cores"]), RAMBytes: number(measures["memory_total_bytes"]),
				SwapPercent: number(measures["swap_percent"]), SwapIndicator: attrString(meta, "swap_indicator"), HealthState: resourceState,
			})
		case proxmox.StorageKind:
			cluster.Storage = append(cluster.Storage, StorageSummary{
				ID: valueString(displayName, attrString(attr, "storage")), Scope: attrString(attr, "scope"),
				UsedBytes: number(measures["used_bytes"]), TotalBytes: number(measures["total_bytes"]),
				UsedPercent: number(measures["used_percent"]), IOWaitPercent: number(measures["io_wait_percent"]),
				IOWaitIndicator: attrString(meta, "io_wait_indicator"), HealthState: resourceState,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return ClusterList{}, fmt.Errorf("read proxmox clusters: %w", err)
	}
	items := make([]ClusterCard, 0, len(order))
	for _, id := range order {
		cluster := clusters[id]
		sort.Slice(cluster.Hosts, func(i, j int) bool { return cluster.Hosts[i].ID < cluster.Hosts[j].ID })
		sort.Slice(cluster.Storage, func(i, j int) bool { return cluster.Storage[i].ID < cluster.Storage[j].ID })
		sort.Slice(cluster.LoadNodes, func(i, j int) bool { return cluster.LoadNodes[i].ID < cluster.LoadNodes[j].ID })
		cluster.Totals = totals(*cluster)
		items = append(items, *cluster)
	}
	return ClusterList{Items: items}, nil
}

func intervalSeconds(value int) int {
	if value <= 0 {
		return proxmox.DefaultExpectedRefreshSeconds
	}
	return value
}

func tokenReference(deploymentID, correlationID string) string {
	suffix := strings.ReplaceAll(correlationID, "-", "")
	if len(suffix) > 12 {
		suffix = suffix[:12]
	}
	if suffix == "" {
		suffix = fmt.Sprint(time.Now().UTC().UnixNano())
	}
	return "proxmox_" + strings.ReplaceAll(deploymentID, "-", "_") + "_" + suffix + "_token"
}

func proxmoxCreateErrorCode(err error) string {
	if err == nil {
		return ""
	}
	switch err.Error() {
	case "target_not_approved", "unauthorized", "forbidden", "throttled", "timeout", "temporary_upstream_failure", "malformed_data", "oversized_response":
		return err.Error()
	default:
		return "proxmox_validation_failed"
	}
}

func object(value []byte) map[string]any {
	if len(value) == 0 {
		return map[string]any{}
	}
	var result map[string]any
	if json.Unmarshal(value, &result) != nil {
		return map[string]any{}
	}
	return result
}

func attrString(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}

func valueString(value *string, fallback string) string {
	if value != nil && *value != "" {
		return *value
	}
	return fallback
}

func number(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case json.Number:
		result, _ := typed.Float64()
		return result
	default:
		return 0
	}
}

func percent(used, total float64) float64 {
	if total <= 0 {
		return 0
	}
	return used / total * 100
}

func freshness(state string) string {
	switch state {
	case "stale", "unknown", "maintenance":
		return state
	default:
		return "fresh"
	}
}

func loadNodes(value any) []LoadNodeSummary {
	values, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	result := make([]LoadNodeSummary, 0, len(values))
	for key, count := range values {
		result = append(result, LoadNodeSummary{ID: key, ActiveVMs: int(number(count))})
	}
	return result
}

func totals(cluster ClusterCard) UtilizationTotals {
	var cpuUsed, cpuMax, ramUsed, ramMax, storageUsed, storageMax float64
	for _, host := range cluster.Hosts {
		cpuUsed += host.CPUPercent * host.CPUCores / 100
		cpuMax += host.CPUCores
		ramUsed += host.RAMPercent * host.RAMBytes / 100
		ramMax += host.RAMBytes
	}
	for _, storage := range cluster.Storage {
		storageUsed += storage.UsedBytes
		storageMax += storage.TotalBytes
	}
	return UtilizationTotals{CPUPercent: percent(cpuUsed, cpuMax), RAMPercent: percent(ramUsed, ramMax), StoragePercent: percent(storageUsed, storageMax)}
}
