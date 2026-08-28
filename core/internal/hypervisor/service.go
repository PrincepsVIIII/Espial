package hypervisor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/adapters"
	"github.com/PrincepsVIIII/Espial/core/internal/adminops"
	"github.com/PrincepsVIIII/Espial/core/internal/audit"
	"github.com/PrincepsVIIII/Espial/core/internal/monitoring"
	"github.com/PrincepsVIIII/Espial/core/internal/proxmox"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type IntegrationCreator interface {
	Create(context.Context, monitoring.CreateIntegration) (string, time.Time, error)
}

type IntegrationRestarter interface {
	RestartIntegration(string)
}

type Service struct {
	pool      *pgxpool.Pool
	creator   IntegrationCreator
	secrets   adapters.SecretStore
	policy    proxmox.Policy
	newClient func(proxmox.Config) (proxmox.API, error)
	now       func() time.Time
	restarter IntegrationRestarter
}

func NewService(pool *pgxpool.Pool, creator IntegrationCreator, secrets adapters.SecretStore, policy proxmox.Policy, now func() time.Time, restarters ...IntegrationRestarter) *Service {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	service := &Service{
		pool: pool, creator: creator, secrets: secrets, policy: policy, now: now,
		newClient: func(config proxmox.Config) (proxmox.API, error) {
			return proxmox.NewClient(config, proxmoxClientOptions(policy, config))
		},
	}
	if len(restarters) > 0 {
		service.restarter = restarters[0]
	}
	return service
}

func (service *Service) CreateProxmoxCluster(ctx context.Context, definition ProxmoxClusterDefinition) (adminops.Receipt, error) {
	if service.creator == nil || service.secrets == nil {
		return adminops.Receipt{}, fmt.Errorf("hypervisor management unavailable")
	}
	config := proxmox.WithDefaults(proxmox.Config{
		BaseURL: definition.BaseURL, DeploymentID: definition.DeploymentID, AllowInsecureHTTP: definition.AllowInsecureHTTP,
		TrustSelfSignedCertificate: definition.TrustSelfSignedCertificate,
		AllowUnverifiedTLS:         definition.AllowUnverifiedTLS,
		TokenID:                    definition.TokenID, TokenSecret: definition.TokenSecret,
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
	if config.TrustSelfSignedCertificate {
		fingerprinter, ok := client.(interface{ CertificateFingerprint() string })
		if !ok || fingerprinter.CertificateFingerprint() == "" {
			return adminops.Receipt{}, &monitoring.Error{Code: "tls_certificate_missing"}
		}
		config.TrustedCertificateSHA256 = fingerprinter.CertificateFingerprint()
	}
	tokenSecretReference := tokenReference(config.DeploymentID, definition.CorrelationID)
	if err := service.secrets.Write(ctx, tokenSecretReference, config.TokenSecret); err != nil {
		return adminops.Receipt{}, &monitoring.Error{Code: "secret_store_unavailable"}
	}
	nonsecret := nonsecretConfig(config)
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

type proxmoxClusterRecord struct {
	ID               string
	DisplayName      string
	Enabled          bool
	IntervalSeconds  int
	SortOrder        int
	Config           map[string]any
	SecretReferences map[string]string
	UpdatedAt        time.Time
}

// ProxmoxClusterConfiguration returns the dedicated, non-secret editor view.
// It deliberately requires callers to use the typed configuration route rather
// than the generic integration configuration API.
func (service *Service) ProxmoxClusterConfiguration(ctx context.Context, id string) (ProxmoxClusterConfiguration, error) {
	record, err := service.loadProxmoxCluster(ctx, service.pool, id, false)
	if err != nil {
		return ProxmoxClusterConfiguration{}, err
	}
	return configurationFromRecord(record)
}

// UpdateProxmoxCluster validates the replacement configuration against Proxmox,
// writes an append-only redacted audit event, and restarts collection after the
// transaction commits. An empty TokenSecret retains the stored secret.
func (service *Service) UpdateProxmoxCluster(ctx context.Context, definition ProxmoxClusterUpdateDefinition) (adminops.Receipt, error) {
	if service.pool == nil || service.secrets == nil || definition.ID == "" || definition.ExpectedUpdatedAt.IsZero() {
		return adminops.Receipt{}, &monitoring.Error{Code: "invalid_integration_config"}
	}
	record, err := service.loadProxmoxCluster(ctx, service.pool, definition.ID, false)
	if err != nil {
		return adminops.Receipt{}, err
	}
	if !record.UpdatedAt.Equal(definition.ExpectedUpdatedAt) {
		return adminops.Receipt{}, &monitoring.Error{Code: "integration_config_conflict"}
	}
	previous, err := configFromRecord(record)
	if err != nil {
		return adminops.Receipt{}, err
	}
	tokenSecret := strings.TrimSpace(definition.TokenSecret)
	if tokenSecret == "" {
		reference := strings.TrimSpace(record.SecretReferences["token_secret"])
		if reference == "" {
			return adminops.Receipt{}, &monitoring.Error{Code: "secret_store_unavailable"}
		}
		tokenSecret, err = service.secrets.Resolve(ctx, reference)
		if err != nil {
			return adminops.Receipt{}, &monitoring.Error{Code: "secret_store_unavailable"}
		}
	}
	config, err := service.validateProxmoxDefinition(ctx, definition.ProxmoxClusterDefinition, tokenSecret, previous)
	if err != nil {
		return adminops.Receipt{}, err
	}

	tx, err := service.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return adminops.Receipt{}, fmt.Errorf("begin proxmox configuration update: %w", err)
	}
	defer tx.Rollback(ctx)
	locked, err := service.loadProxmoxCluster(ctx, tx, definition.ID, true)
	if err != nil {
		return adminops.Receipt{}, err
	}
	if !locked.UpdatedAt.Equal(definition.ExpectedUpdatedAt) {
		return adminops.Receipt{}, &monitoring.Error{Code: "integration_config_conflict"}
	}
	if strings.TrimSpace(definition.TokenSecret) != "" {
		reference := strings.TrimSpace(locked.SecretReferences["token_secret"])
		if reference == "" || service.secrets.Write(ctx, reference, tokenSecret) != nil {
			return adminops.Receipt{}, &monitoring.Error{Code: "secret_store_unavailable"}
		}
	}
	nonsecret, err := json.Marshal(nonsecretConfig(config))
	if err != nil {
		return adminops.Receipt{}, &monitoring.Error{Code: "invalid_integration_config"}
	}
	updatedAt := service.now().UTC().Truncate(time.Microsecond)
	if !updatedAt.After(locked.UpdatedAt) {
		updatedAt = locked.UpdatedAt.Add(time.Microsecond)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE integrations SET display_name = $2, interval_seconds = $3,
			config_nonsecret = $4::jsonb, updated_at = $5
		WHERE id = $1
	`, definition.ID, strings.TrimSpace(definition.DisplayName), intervalSeconds(definition.IntervalSeconds), string(nonsecret), updatedAt); err != nil {
		return adminops.Receipt{}, fmt.Errorf("update proxmox configuration: %w", err)
	}
	if err := audit.Append(ctx, tx, audit.Event{
		ActorUserID: definition.ActorUserID, Action: "integration.configuration.updated",
		TargetType: "integration", TargetID: definition.ID, Result: "succeeded",
		SourceAddress: definition.SourceAddress, CorrelationID: definition.CorrelationID,
		BeforeSummary: configurationSummary(locked),
		AfterSummary: map[string]any{
			"enabled": locked.Enabled, "interval_seconds": intervalSeconds(definition.IntervalSeconds),
			"config_keys":           sortedMapKeys(nonsecretConfig(config)),
			"secret_reference_keys": sortedMapKeys(locked.SecretReferences),
		},
		OccurredAt: updatedAt,
	}); err != nil {
		return adminops.Receipt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return adminops.Receipt{}, fmt.Errorf("commit proxmox configuration update: %w", err)
	}
	if service.restarter != nil {
		service.restarter.RestartIntegration(definition.ID)
	}
	receipt := adminops.Receipt{ID: definition.ID, Version: updatedAt.UnixMicro(), RequestID: definition.CorrelationID}
	if definition.CanReadAudit {
		receipt.AuditURL = "/audit?correlation_id=" + definition.CorrelationID
	}
	return receipt, nil
}

// ReorderProxmoxCluster moves one cluster by one visible position. The complete
// typed-cluster order is normalized in one transaction, including clusters that
// existed before an explicit order was stored.
func (service *Service) ReorderProxmoxCluster(ctx context.Context, definition ProxmoxClusterOrderDefinition) (adminops.Receipt, error) {
	if service.pool == nil || definition.ID == "" || definition.ExpectedUpdatedAt.IsZero() || (definition.Direction != "up" && definition.Direction != "down") {
		return adminops.Receipt{}, &monitoring.Error{Code: "invalid_integration_config"}
	}
	tx, err := service.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return adminops.Receipt{}, fmt.Errorf("begin proxmox cluster reorder: %w", err)
	}
	defer tx.Rollback(ctx)
	ordered, err := loadOrderedProxmoxClusters(ctx, tx)
	if err != nil {
		return adminops.Receipt{}, err
	}
	current := -1
	for index, item := range ordered {
		if item.ID == definition.ID {
			current = index
			break
		}
	}
	if current < 0 {
		return adminops.Receipt{}, &monitoring.Error{Code: "integration_not_found"}
	}
	locked := ordered[current]
	if !locked.UpdatedAt.Equal(definition.ExpectedUpdatedAt) {
		return adminops.Receipt{}, &monitoring.Error{Code: "integration_config_conflict"}
	}
	target := current - 1
	if definition.Direction == "down" {
		target = current + 1
	}
	if target < 0 || target >= len(ordered) {
		return adminops.Receipt{}, &monitoring.Error{Code: "invalid_cluster_order"}
	}
	ordered[current], ordered[target] = ordered[target], ordered[current]
	updatedAt := service.now().UTC().Truncate(time.Microsecond)
	if !updatedAt.After(locked.UpdatedAt) {
		updatedAt = locked.UpdatedAt.Add(time.Microsecond)
	}
	for index, item := range ordered {
		if item.ID == definition.ID {
			if _, err := tx.Exec(ctx, `UPDATE integrations SET sort_order = $2, updated_at = $3 WHERE id = $1`, item.ID, index, updatedAt); err != nil {
				return adminops.Receipt{}, fmt.Errorf("update proxmox cluster order: %w", err)
			}
			continue
		}
		if _, err := tx.Exec(ctx, `UPDATE integrations SET sort_order = $2 WHERE id = $1`, item.ID, index); err != nil {
			return adminops.Receipt{}, fmt.Errorf("normalize proxmox cluster order: %w", err)
		}
	}
	if err := audit.Append(ctx, tx, audit.Event{
		ActorUserID: definition.ActorUserID, Action: "integration.order.updated",
		TargetType: "integration", TargetID: definition.ID, Result: "succeeded",
		SourceAddress: definition.SourceAddress, CorrelationID: definition.CorrelationID,
		BeforeSummary: map[string]any{"sort_order": locked.SortOrder},
		AfterSummary:  map[string]any{"sort_order": target}, OccurredAt: updatedAt,
	}); err != nil {
		return adminops.Receipt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return adminops.Receipt{}, fmt.Errorf("commit proxmox cluster reorder: %w", err)
	}
	receipt := adminops.Receipt{ID: definition.ID, Version: updatedAt.UnixMicro(), RequestID: definition.CorrelationID}
	if definition.CanReadAudit {
		receipt.AuditURL = "/audit?correlation_id=" + definition.CorrelationID
	}
	return receipt, nil
}

type proxmoxClusterQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (service *Service) loadProxmoxCluster(ctx context.Context, database proxmoxClusterQuerier, id string, lock bool) (proxmoxClusterRecord, error) {
	query := `SELECT id::text, display_name, enabled, interval_seconds, sort_order, config_nonsecret, secret_references, updated_at
		FROM integrations WHERE id = $1 AND adapter_id = $2`
	if lock {
		query += " FOR UPDATE"
	}
	var record proxmoxClusterRecord
	if err := database.QueryRow(ctx, query, id, proxmox.AdapterID).Scan(
		&record.ID, &record.DisplayName, &record.Enabled, &record.IntervalSeconds, &record.SortOrder,
		&record.Config, &record.SecretReferences, &record.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return proxmoxClusterRecord{}, &monitoring.Error{Code: "integration_not_found"}
		}
		return proxmoxClusterRecord{}, fmt.Errorf("load proxmox configuration: %w", err)
	}
	return record, nil
}

func loadOrderedProxmoxClusters(ctx context.Context, tx pgx.Tx) ([]proxmoxClusterRecord, error) {
	rows, err := tx.Query(ctx, `SELECT id::text, display_name, enabled, interval_seconds, sort_order, config_nonsecret, secret_references, updated_at
		FROM integrations WHERE adapter_id = $1 ORDER BY sort_order, display_name, id FOR UPDATE`, proxmox.AdapterID)
	if err != nil {
		return nil, fmt.Errorf("load proxmox cluster order: %w", err)
	}
	defer rows.Close()
	result := []proxmoxClusterRecord{}
	for rows.Next() {
		var record proxmoxClusterRecord
		if err := rows.Scan(&record.ID, &record.DisplayName, &record.Enabled, &record.IntervalSeconds, &record.SortOrder, &record.Config, &record.SecretReferences, &record.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan proxmox cluster order: %w", err)
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read proxmox cluster order: %w", err)
	}
	return result, nil
}

func configurationFromRecord(record proxmoxClusterRecord) (ProxmoxClusterConfiguration, error) {
	config, err := configFromRecord(record)
	if err != nil {
		return ProxmoxClusterConfiguration{}, err
	}
	return ProxmoxClusterConfiguration{
		ID: record.ID, DisplayName: record.DisplayName, BaseURL: config.BaseURL, ApproveTarget: true,
		AllowInsecureHTTP: config.AllowInsecureHTTP, TrustSelfSignedCertificate: config.TrustSelfSignedCertificate,
		AllowUnverifiedTLS: config.AllowUnverifiedTLS,
		DeploymentID:       config.DeploymentID, TokenID: config.TokenID,
		TokenSecretConfigured: strings.TrimSpace(record.SecretReferences["token_secret"]) != "",
		IntervalSeconds:       record.IntervalSeconds, ExpectedRefreshSeconds: config.ExpectedRefreshSeconds,
		RequestTimeoutSeconds: config.RequestTimeoutSeconds, StorageWarningPercent: config.StorageWarningPercent,
		StorageCriticalPercent: config.StorageCriticalPercent, SwapWarningPercent: config.SwapWarningPercent,
		SwapCriticalPercent: config.SwapCriticalPercent, IOWaitWarningPercent: config.IOWaitWarningPercent,
		IOWaitCriticalPercent: config.IOWaitCriticalPercent, ExpectedRunningGuests: config.ExpectedRunningGuests,
		SortOrder: record.SortOrder,
		UpdatedAt: record.UpdatedAt, Version: record.UpdatedAt.UnixMicro(),
	}, nil
}

func configFromRecord(record proxmoxClusterRecord) (proxmox.Config, error) {
	encoded, err := json.Marshal(record.Config)
	if err != nil {
		return proxmox.Config{}, &monitoring.Error{Code: "invalid_integration_config"}
	}
	var config proxmox.Config
	if err := json.Unmarshal(encoded, &config); err != nil {
		return proxmox.Config{}, &monitoring.Error{Code: "invalid_integration_config"}
	}
	return proxmox.WithDefaults(config), nil
}

func (service *Service) validateProxmoxDefinition(ctx context.Context, definition ProxmoxClusterDefinition, tokenSecret string, previous proxmox.Config) (proxmox.Config, error) {
	if !definition.ApproveTarget {
		return proxmox.Config{}, &monitoring.Error{Code: "target_approval_required"}
	}
	config := proxmox.WithDefaults(proxmox.Config{
		BaseURL: definition.BaseURL, DeploymentID: definition.DeploymentID, AllowInsecureHTTP: definition.AllowInsecureHTTP,
		TrustSelfSignedCertificate: definition.TrustSelfSignedCertificate, AllowUnverifiedTLS: definition.AllowUnverifiedTLS,
		TokenID: definition.TokenID, TokenSecret: tokenSecret,
		ExpectedRefreshSeconds: definition.ExpectedRefreshSeconds, RequestTimeoutSeconds: definition.RequestTimeoutSeconds,
		StorageWarningPercent: definition.StorageWarningPercent, StorageCriticalPercent: definition.StorageCriticalPercent,
		SwapWarningPercent: definition.SwapWarningPercent, SwapCriticalPercent: definition.SwapCriticalPercent,
		IOWaitWarningPercent: definition.IOWaitWarningPercent, IOWaitCriticalPercent: definition.IOWaitCriticalPercent,
		ExpectedRunningGuests: definition.ExpectedRunningGuests,
	})
	host, port, err := approvedTarget(config.BaseURL)
	if err != nil {
		return proxmox.Config{}, &monitoring.Error{Code: "invalid_proxmox_config"}
	}
	config.ApprovedHost, config.ApprovedPort = host, port
	if config.TrustSelfSignedCertificate && strings.TrimSpace(previous.BaseURL) == strings.TrimSpace(config.BaseURL) && previous.TrustSelfSignedCertificate {
		config.TrustedCertificateSHA256 = previous.TrustedCertificateSHA256
	}
	if err := proxmox.ValidateConfig(config); err != nil {
		return proxmox.Config{}, &monitoring.Error{Code: "invalid_proxmox_config"}
	}
	client, err := service.newClient(config)
	if err != nil {
		return proxmox.Config{}, &monitoring.Error{Code: proxmoxCreateErrorCode(err)}
	}
	validationContext, cancel := context.WithTimeout(ctx, proxmox.RequestTimeout(config))
	defer cancel()
	if _, err := proxmox.Collect(validationContext, config, client, service.now()); err != nil {
		return proxmox.Config{}, &monitoring.Error{Code: proxmoxCreateErrorCode(err)}
	}
	if config.TrustSelfSignedCertificate && config.TrustedCertificateSHA256 == "" {
		fingerprinter, ok := client.(interface{ CertificateFingerprint() string })
		if !ok || fingerprinter.CertificateFingerprint() == "" {
			return proxmox.Config{}, &monitoring.Error{Code: "tls_certificate_missing"}
		}
		config.TrustedCertificateSHA256 = fingerprinter.CertificateFingerprint()
	}
	return config, nil
}

func nonsecretConfig(config proxmox.Config) map[string]any {
	return map[string]any{
		"base_url": config.BaseURL, "allow_insecure_http": config.AllowInsecureHTTP,
		"trust_self_signed_certificate": config.TrustSelfSignedCertificate,
		"allow_unverified_tls":          config.AllowUnverifiedTLS,
		"trusted_certificate_sha256":    config.TrustedCertificateSHA256,
		"approved_host":                 config.ApprovedHost, "approved_port": config.ApprovedPort,
		"deployment_id": config.DeploymentID, "token_id": config.TokenID,
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
}

func configurationSummary(record proxmoxClusterRecord) map[string]any {
	return map[string]any{
		"enabled": record.Enabled, "interval_seconds": record.IntervalSeconds,
		"config_keys": sortedMapKeys(record.Config), "secret_reference_keys": sortedMapKeys(record.SecretReferences),
	}
}

func sortedMapKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
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
		SELECT i.id::text, i.display_name, i.sort_order, r.id::text, r.external_id, r.kind, r.display_name,
			r.attributes, ch.state, ch.observed_at, o.measurements, o.metadata,
			capacity.measurements, capacity.metadata, run.completed_at
		FROM integrations i
		LEFT JOIN resources r ON r.integration_id = i.id
		LEFT JOIN current_resource_health ch ON ch.resource_id = r.id
		LEFT JOIN observations o ON o.id = ch.observation_id
		LEFT JOIN current_health capacity_health
			ON capacity_health.resource_id = r.id AND capacity_health.check_type = 'proxmox.storage.capacity'
		LEFT JOIN observations capacity ON capacity.id = capacity_health.observation_id
		LEFT JOIN LATERAL (
			SELECT completed_at FROM integration_collection_runs
			WHERE integration_id = i.id ORDER BY completed_at DESC LIMIT 1
		) run ON true
		WHERE i.adapter_id = $1
		ORDER BY i.sort_order, i.display_name, r.kind, r.display_name
	`, proxmox.AdapterID)
	if err != nil {
		return ClusterList{}, fmt.Errorf("read proxmox clusters: %w", err)
	}
	defer rows.Close()
	clusters := map[string]*ClusterCard{}
	order := []string{}
	for rows.Next() {
		var integrationID, integrationName string
		var sortOrder int
		var resourceID, kind, displayName *string
		var attributes, measurements, metadata, capacityMeasurements, capacityMetadata []byte
		var state *string
		var observedAt, lastCollection *time.Time
		var externalID *string
		if err := rows.Scan(&integrationID, &integrationName, &sortOrder, &resourceID, &externalID, &kind, &displayName, &attributes, &state, &observedAt, &measurements, &metadata, &capacityMeasurements, &capacityMetadata, &lastCollection); err != nil {
			return ClusterList{}, fmt.Errorf("scan proxmox cluster: %w", err)
		}
		cluster := clusters[integrationID]
		if cluster == nil {
			cluster = &ClusterCard{ID: integrationID, IntegrationID: integrationID, Name: integrationName, State: "unknown", Freshness: "unknown", LastCollection: lastCollection, SortOrder: sortOrder}
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
			cluster.LoadNodes = loadNodes(meta["idle_vms_by_node"], meta["active_vms_by_node"])
			cluster.ConsoleActivityAvailable = boolValue(meta["console_activity_available"])
		case proxmox.NodeKind:
			cluster.Hosts = append(cluster.Hosts, HostSummary{
				ID:         valueString(displayName, attrString(attr, "node")),
				CPUPercent: number(measures["cpu_percent"]), RAMPercent: percent(number(measures["memory_used_bytes"]), number(measures["memory_total_bytes"])),
				CPUCores: number(measures["cpu_cores"]), RAMBytes: number(measures["memory_total_bytes"]),
				SwapPercent: number(measures["swap_percent"]), SwapIndicator: attrString(meta, "swap_indicator"), HealthState: resourceState,
			})
		case proxmox.StorageKind:
			if capacityMeasurements != nil {
				measures = object(capacityMeasurements)
				meta = object(capacityMetadata)
			}
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
		cluster.LoadNodes = completeLoadNodes(cluster.Hosts, cluster.LoadNodes)
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
	case "target_not_approved", "unauthorized", "forbidden", "throttled", "timeout", "temporary_upstream_failure", "malformed_data", "oversized_response",
		"tls_certificate_untrusted", "tls_hostname_mismatch", "tls_certificate_invalid", "tls_certificate_changed", "tls_certificate_missing", "tls_handshake_failed", "dns_failed", "connection_failed",
		"proxmox_api_not_found", "upstream_request_rejected":
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

func boolValue(value any) bool {
	result, _ := value.(bool)
	return result
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

func loadNodes(idleValue, activeValue any) []LoadNodeSummary {
	idle, _ := idleValue.(map[string]any)
	active, _ := activeValue.(map[string]any)
	ids := map[string]struct{}{}
	for id := range idle {
		ids[id] = struct{}{}
	}
	for id := range active {
		ids[id] = struct{}{}
	}
	result := make([]LoadNodeSummary, 0, len(ids))
	for id := range ids {
		result = append(result, LoadNodeSummary{ID: id, IdleVMs: int(number(idle[id])), ActiveVMs: int(number(active[id]))})
	}
	return result
}

func completeLoadNodes(hosts []HostSummary, loads []LoadNodeSummary) []LoadNodeSummary {
	counts := make(map[string]LoadNodeSummary, len(hosts)+len(loads))
	for _, host := range hosts {
		counts[host.ID] = LoadNodeSummary{ID: host.ID}
	}
	for _, load := range loads {
		if load.IdleVMs < 0 {
			load.IdleVMs = 0
		}
		if load.ActiveVMs < 0 {
			load.ActiveVMs = 0
		}
		counts[load.ID] = load
	}
	result := make([]LoadNodeSummary, 0, len(counts))
	for _, load := range counts {
		result = append(result, load)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
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
