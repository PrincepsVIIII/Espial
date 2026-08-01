package webpages

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/PrincepsVIIII/Espial/core/internal/adminops"
	"github.com/PrincepsVIIII/Espial/core/internal/audit"
	"github.com/PrincepsVIIII/Espial/core/internal/events"
	"github.com/PrincepsVIIII/Espial/core/internal/monitoring"
	"github.com/PrincepsVIIII/Espial/core/internal/webcheck"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool      *pgxpool.Pool
	hub       *events.Hub
	runtime   Runtime
	policy    *webcheck.Policy
	now       func() time.Time
	resources *monitoring.ReadService
}

var secretReferencePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

func NewService(pool *pgxpool.Pool, hub *events.Hub, runtime Runtime, policy *webcheck.Policy, now func() time.Time) *Service {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{pool: pool, hub: hub, runtime: runtime, policy: policy, now: now, resources: monitoring.NewReadService(pool)}
}

func (service *Service) Monitors(ctx context.Context, filter ListFilter) (MonitorList, error) {
	filter, err := normalizeFilter(filter)
	if err != nil {
		return MonitorList{}, err
	}
	cursor, err := decodeCursor(filter.Cursor, "website_monitors")
	if err != nil {
		return MonitorList{}, err
	}
	var orderedAt any
	var cursorID any
	if filter.Cursor != "" {
		orderedAt, cursorID = cursor.OrderedAt, cursor.ID
	}
	rows, err := service.pool.Query(ctx, `SELECT id::text,updated_at FROM integrations WHERE adapter_id=$1 AND
		($2::timestamptz IS NULL OR (updated_at,id)<($2,$3::uuid)) ORDER BY updated_at DESC,id DESC LIMIT $4`,
		webcheck.AdapterID, orderedAt, cursorID, filter.Limit+1)
	if err != nil {
		return MonitorList{}, err
	}
	type orderedID struct {
		id string
		at time.Time
	}
	ids := []orderedID{}
	for rows.Next() {
		var item orderedID
		if err := rows.Scan(&item.id, &item.at); err != nil {
			rows.Close()
			return MonitorList{}, err
		}
		ids = append(ids, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return MonitorList{}, err
	}
	rows.Close()
	result := MonitorList{Items: []Monitor{}}
	for _, ordered := range ids[:min(len(ids), filter.Limit)] {
		item, err := service.Monitor(ctx, ordered.id)
		if err != nil {
			return MonitorList{}, err
		}
		result.Items = append(result.Items, item)
	}
	if len(ids) > filter.Limit {
		last := ids[filter.Limit-1]
		result.NextCursor, err = encodeCursor(pageCursor{Kind: "website_monitors", OrderedAt: last.at, ID: last.id})
	}
	return result, err
}

func (service *Service) Monitor(ctx context.Context, id string) (Monitor, error) {
	var item Monitor
	var config, references []byte
	err := service.pool.QueryRow(ctx, `SELECT i.id::text,i.display_name,i.enabled,i.interval_seconds,i.config_nonsecret,i.secret_references,
		CASE WHEN NOT i.enabled THEN 'disabled' ELSE COALESCE(ai.state,'not_started') END,i.created_at,i.updated_at
		FROM integrations i LEFT JOIN adapter_instances ai ON ai.integration_id=i.id WHERE i.id=$1 AND i.adapter_id=$2`, id, webcheck.AdapterID).
		Scan(&item.ID, &item.DisplayName, &item.Enabled, &item.IntervalSeconds, &config, &references, &item.RuntimeState, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Monitor{}, ErrNotFound
	}
	if err != nil {
		return Monitor{}, err
	}
	var stored storedConfig
	var refs map[string]string
	if json.Unmarshal(config, &stored) != nil || json.Unmarshal(references, &refs) != nil {
		return Monitor{}, errors.New("decode website monitor")
	}
	defaults := webcheck.WithCertificateDefaults(webcheck.Config{CertificateWarningDays: stored.CertificateWarningDays, CertificateCriticalDays: stored.CertificateCriticalDays, CertificateEscalationDays: stored.CertificateEscalationDays})
	stored.CertificateWarningDays, stored.CertificateCriticalDays, stored.CertificateEscalationDays = defaults.CertificateWarningDays, defaults.CertificateCriticalDays, defaults.CertificateEscalationDays
	item.URL = stored.URL
	item.TimeoutMS = stored.TimeoutMS
	item.WarningLatencyMS = stored.WarningLatencyMS
	item.CertificateWarningDays = stored.CertificateWarningDays
	item.CertificateCriticalDays = stored.CertificateCriticalDays
	item.CertificateEscalationDays = stored.CertificateEscalationDays
	item.AllowedStatuses = append([]int(nil), stored.AllowedStatuses...)
	item.ContentMatchConfigured = stored.ContentMatch != ""
	item.FollowRedirects = stored.FollowRedirects
	item.MaxRedirects = stored.MaxRedirects
	item.SecretHeaderNames = append([]string(nil), stored.HeaderNames...)
	item.CreatedAt = item.CreatedAt.UTC()
	item.UpdatedAt = item.UpdatedAt.UTC()
	item.Version = item.UpdatedAt.UnixMicro()
	return item, nil
}

func (service *Service) Create(ctx context.Context, definition MonitorDefinition, metadata MutationMetadata) (adminops.Receipt, error) {
	return service.mutate(ctx, "create", "", definition, metadata)
}
func (service *Service) Replace(ctx context.Context, id string, definition MonitorDefinition, metadata MutationMetadata) (adminops.Receipt, error) {
	return service.mutate(ctx, "replace", id, definition, metadata)
}

func (service *Service) mutate(ctx context.Context, operation, id string, definition MonitorDefinition, metadata MutationMetadata) (adminops.Receipt, error) {
	stored, refs, err := service.validate(definition)
	if err != nil {
		return adminops.Receipt{}, err
	}
	hash, err := adminops.Hash(struct {
		Operation, ID string
		Version       int64
		Definition    MonitorDefinition
	}{operation, id, metadata.ExpectedVersion, definition})
	if err != nil {
		return adminops.Receipt{}, err
	}
	tx, err := service.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return adminops.Receipt{}, err
	}
	defer tx.Rollback(ctx)
	if replay, found, err := adminops.Replay(ctx, tx, metadata.ActorUserID, "website_monitor", operation, metadata.IdempotencyKey, hash); err != nil {
		if errors.Is(err, adminops.ErrIdempotencyConflict) {
			return adminops.Receipt{}, ErrIdempotencyConflict
		}
		return adminops.Receipt{}, err
	} else if found {
		return replay, tx.Commit(ctx)
	}
	now := service.now().UTC().Truncate(time.Microsecond)
	var before map[string]any
	encoded, _ := json.Marshal(stored)
	encodedRefs, _ := json.Marshal(refs)
	if operation == "create" {
		err = tx.QueryRow(ctx, `INSERT INTO integrations(id,adapter_id,display_name,enabled,config_nonsecret,secret_references,interval_seconds,created_at,updated_at) VALUES(gen_random_uuid(),$1,$2,$3,$4,$5,$6,$7,$7) RETURNING id::text`, webcheck.AdapterID, strings.TrimSpace(definition.DisplayName), definition.Enabled, encoded, encodedRefs, definition.IntervalSeconds, now).Scan(&id)
	} else {
		current, readErr := readMonitorForUpdate(ctx, tx, id)
		if errors.Is(readErr, pgx.ErrNoRows) {
			return adminops.Receipt{}, ErrNotFound
		}
		if readErr != nil {
			return adminops.Receipt{}, readErr
		}
		if current.UpdatedAt.UnixMicro() != metadata.ExpectedVersion {
			return adminops.Receipt{}, ErrConflict
		}
		before = monitorAudit(current)
		if !now.After(current.UpdatedAt) {
			now = current.UpdatedAt.Add(time.Microsecond)
		}
		_, err = tx.Exec(ctx, `UPDATE integrations SET display_name=$2,enabled=$3,config_nonsecret=$4,secret_references=$5,interval_seconds=$6,updated_at=$7 WHERE id=$1`, id, strings.TrimSpace(definition.DisplayName), definition.Enabled, encoded, encodedRefs, definition.IntervalSeconds, now)
	}
	if err != nil {
		return adminops.Receipt{}, fmt.Errorf("save website monitor: %w", err)
	}
	receipt := adminops.Receipt{ID: id, Version: now.UnixMicro(), RequestID: metadata.CorrelationID}
	after := map[string]any{"display_name": strings.TrimSpace(definition.DisplayName), "enabled": definition.Enabled, "interval_seconds": definition.IntervalSeconds, "url_host": mustURL(definition.URL).Hostname(), "secret_header_names": headerNames(definition.SecretHeaders), "content_match_configured": definition.ContentMatch != "", "certificate_warning_days": stored.CertificateWarningDays, "certificate_critical_days": stored.CertificateCriticalDays, "certificate_escalation_days": stored.CertificateEscalationDays}
	if err := audit.Append(ctx, tx, audit.Event{ActorUserID: metadata.ActorUserID, Action: "website_monitor." + operation, TargetType: "website_monitor", TargetID: id, Result: "succeeded", SourceAddress: metadata.SourceAddress, CorrelationID: metadata.CorrelationID, BeforeSummary: before, AfterSummary: after, OccurredAt: now}); err != nil {
		return adminops.Receipt{}, err
	}
	if err := adminops.Save(ctx, tx, metadata.ActorUserID, "website_monitor", operation, metadata.IdempotencyKey, hash, receipt); err != nil {
		return adminops.Receipt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return adminops.Receipt{}, err
	}
	if service.runtime != nil {
		service.runtime.RestartIntegration(id)
	}
	service.publish(id, "configured", now)
	return receipt, nil
}

func (service *Service) Check(ctx context.Context, id string, metadata MutationMetadata) (adminops.Receipt, error) {
	monitor, err := service.Monitor(ctx, id)
	if err != nil {
		return adminops.Receipt{}, err
	}
	if monitor.Version != metadata.ExpectedVersion {
		return adminops.Receipt{}, ErrConflict
	}
	if !monitor.Enabled {
		return adminops.Receipt{}, ErrNotRunning
	}
	hash, _ := adminops.Hash(struct {
		ID      string
		Version int64
	}{id, metadata.ExpectedVersion})
	tx, err := service.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return adminops.Receipt{}, err
	}
	defer tx.Rollback(ctx)
	if replay, found, err := adminops.Replay(ctx, tx, metadata.ActorUserID, "website_monitor", "check", metadata.IdempotencyKey, hash); err != nil {
		if errors.Is(err, adminops.ErrIdempotencyConflict) {
			return adminops.Receipt{}, ErrIdempotencyConflict
		}
		return adminops.Receipt{}, err
	} else if found {
		return replay, tx.Commit(ctx)
	}
	if service.runtime == nil || !service.runtime.RequestCollection(id) {
		return adminops.Receipt{}, ErrNotRunning
	}
	now := service.now().UTC().Truncate(time.Microsecond)
	receipt := adminops.Receipt{ID: id, Version: monitor.Version, RequestID: metadata.CorrelationID}
	if err := audit.Append(ctx, tx, audit.Event{ActorUserID: metadata.ActorUserID, Action: "website_monitor.check", TargetType: "website_monitor", TargetID: id, Result: "succeeded", SourceAddress: metadata.SourceAddress, CorrelationID: metadata.CorrelationID, AfterSummary: map[string]any{"manual_check": true}, OccurredAt: now}); err != nil {
		return adminops.Receipt{}, err
	}
	if err := adminops.Save(ctx, tx, metadata.ActorUserID, "website_monitor", "check", metadata.IdempotencyKey, hash, receipt); err != nil {
		return adminops.Receipt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return adminops.Receipt{}, err
	}
	service.publish(id, "check_requested", now)
	return receipt, nil
}

func (service *Service) Webpages(ctx context.Context, filter ListFilter) (List, error) {
	filter, err := normalizeFilter(filter)
	if err != nil {
		return List{}, err
	}
	cursor, err := decodeCursor(filter.Cursor, "webpages")
	if err != nil {
		return List{}, err
	}
	var orderedAt any
	var cursorID any
	if filter.Cursor != "" {
		orderedAt, cursorID = cursor.OrderedAt, cursor.ID
	}
	rows, err := service.pool.Query(ctx, `SELECT r.id::text,r.last_seen_at FROM resources r JOIN integrations i ON i.id=r.integration_id
		WHERE i.adapter_id=$1 AND r.kind=$2 AND ($3::timestamptz IS NULL OR (r.last_seen_at,r.id)<($3,$4::uuid))
		ORDER BY r.last_seen_at DESC,r.id DESC LIMIT $5`, webcheck.AdapterID, webcheck.ResourceKind, orderedAt, cursorID, filter.Limit+1)
	if err != nil {
		return List{}, err
	}
	type orderedID struct {
		id string
		at time.Time
	}
	ids := []orderedID{}
	for rows.Next() {
		var item orderedID
		if err := rows.Scan(&item.id, &item.at); err != nil {
			rows.Close()
			return List{}, err
		}
		ids = append(ids, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return List{}, err
	}
	rows.Close()
	result := List{Items: []Summary{}}
	for _, ordered := range ids[:min(len(ids), filter.Limit)] {
		detail, err := service.Webpage(ctx, ordered.id)
		if err != nil {
			return List{}, err
		}
		result.Items = append(result.Items, detail.Summary)
	}
	if len(ids) > filter.Limit {
		last := ids[filter.Limit-1]
		result.NextCursor, err = encodeCursor(pageCursor{Kind: "webpages", OrderedAt: last.at, ID: last.id})
	}
	return result, err
}

func (service *Service) Webpage(ctx context.Context, id string) (Detail, error) {
	resource, err := service.resources.Resource(ctx, id)
	if err != nil {
		return Detail{}, ErrNotFound
	}
	var adapterID string
	if err := service.pool.QueryRow(ctx, `SELECT adapter_id FROM integrations WHERE id=$1`, resource.IntegrationID).Scan(&adapterID); err != nil || adapterID != webcheck.AdapterID {
		return Detail{}, ErrNotFound
	}
	result := Detail{Summary: Summary{ID: resource.ID, MonitorID: resource.IntegrationID, DisplayName: resource.DisplayName, URL: resource.SourceURL, State: resource.Health.State, RawState: resource.Health.RawState, Reason: resource.Health.Reason, ObservedAt: resource.Health.ObservedAt, UpdatedAt: resource.Health.UpdatedAt, Stages: Stages{Completed: []string{}}}, FirstSeenAt: resource.FirstSeenAt, LastSeenAt: resource.LastSeenAt}
	if resource.LatestObservation != nil {
		var measurements struct {
			DNS       int64 `json:"dns_ms"`
			TCP       int64 `json:"tcp_ms"`
			TLS       int64 `json:"tls_ms"`
			HTTP      int64 `json:"http_ms"`
			Total     int64 `json:"total_ms"`
			Status    int   `json:"status_code"`
			Body      int64 `json:"body_bytes"`
			Redirects int   `json:"redirects"`
		}
		var metadata struct {
			Reason    string   `json:"reason_code"`
			Completed []string `json:"completed_stages"`
		}
		_ = json.Unmarshal(resource.LatestObservation.Measurements, &measurements)
		_ = json.Unmarshal(resource.LatestObservation.Metadata, &metadata)
		result.ReasonCode = metadata.Reason
		result.Stages = Stages{Completed: metadata.Completed, DNSMS: measurements.DNS, TCPMS: measurements.TCP, TLSMS: measurements.TLS, HTTPMS: measurements.HTTP, TotalMS: measurements.Total, HTTPStatus: measurements.Status, BodyBytes: measurements.Body, Redirects: measurements.Redirects}
	}
	_ = service.pool.QueryRow(ctx, `SELECT id::text FROM incidents WHERE resource_id=$1 AND status<>'resolved' ORDER BY updated_at DESC,id DESC LIMIT 1`, id).Scan(&result.ActiveIncidentID)
	return result, nil
}

type storedConfig struct {
	URL                       string   `json:"url"`
	AllowedStatuses           []int    `json:"allowed_statuses"`
	TimeoutMS                 int      `json:"timeout_ms"`
	WarningLatencyMS          int      `json:"warning_latency_ms,omitempty"`
	CertificateWarningDays    int      `json:"certificate_warning_days,omitempty"`
	CertificateCriticalDays   int      `json:"certificate_critical_days,omitempty"`
	CertificateEscalationDays int      `json:"certificate_escalation_days,omitempty"`
	ContentMatch              string   `json:"content_match,omitempty"`
	FollowRedirects           bool     `json:"follow_redirects"`
	MaxRedirects              int      `json:"max_redirects"`
	ExpectedRefreshSeconds    int      `json:"expected_refresh_seconds"`
	HeaderNames               []string `json:"header_names,omitempty"`
}

func (service *Service) validate(definition MonitorDefinition) (storedConfig, map[string]string, error) {
	definition.DisplayName = strings.TrimSpace(definition.DisplayName)
	if definition.CertificateWarningDays == 0 {
		definition.CertificateWarningDays = webcheck.DefaultWarningDays
	}
	if definition.CertificateCriticalDays == 0 {
		definition.CertificateCriticalDays = webcheck.DefaultCriticalDays
	}
	if definition.CertificateEscalationDays == 0 {
		definition.CertificateEscalationDays = webcheck.DefaultEscalationDays
	}
	if definition.DisplayName == "" || utf8.RuneCountInString(definition.DisplayName) > 128 || definition.IntervalSeconds < 1 || definition.IntervalSeconds > 86400 {
		return storedConfig{}, nil, ErrInvalid
	}
	headerNames := headerNames(definition.SecretHeaders)
	if len(definition.SecretHeaders) > webcheck.MaxSecretHeaders {
		return storedConfig{}, nil, ErrInvalid
	}
	refs := map[string]string{}
	dummy := []string{"", "", "", ""}
	seen := map[string]struct{}{}
	for index, item := range definition.SecretHeaders {
		item.Name = strings.TrimSpace(item.Name)
		if item.Name == "" || !secretReferencePattern.MatchString(item.SecretReference) {
			return storedConfig{}, nil, ErrInvalid
		}
		lower := strings.ToLower(item.Name)
		if _, exists := seen[lower]; exists {
			return storedConfig{}, nil, ErrInvalid
		}
		seen[lower] = struct{}{}
		key := fmt.Sprintf("header_value_%d", index+1)
		refs[key] = item.SecretReference
		dummy[index] = "redacted"
	}
	config := webcheck.Config{URL: definition.URL, AllowedStatuses: definition.AllowedStatuses, TimeoutMS: definition.TimeoutMS, WarningLatencyMS: definition.WarningLatencyMS, CertificateWarningDays: definition.CertificateWarningDays, CertificateCriticalDays: definition.CertificateCriticalDays, CertificateEscalationDays: definition.CertificateEscalationDays, ContentMatch: definition.ContentMatch, FollowRedirects: definition.FollowRedirects, MaxRedirects: definition.MaxRedirects, ExpectedRefreshSeconds: definition.IntervalSeconds, HeaderNames: headerNames, HeaderValue1: dummy[0], HeaderValue2: dummy[1], HeaderValue3: dummy[2], HeaderValue4: dummy[3]}
	if webcheck.ValidateConfig(config) != nil {
		return storedConfig{}, nil, ErrInvalid
	}
	target, _ := url.Parse(definition.URL)
	port := 80
	if target.Scheme == "https" {
		port = 443
	}
	if target.Port() != "" {
		parsed, err := strconv.Atoi(target.Port())
		if err != nil {
			return storedConfig{}, nil, ErrInvalid
		}
		port = parsed
	}
	if service.policy == nil || !service.policy.AllowsTarget(target.Hostname(), port) || !service.policy.AllowsRedirects(definition.MaxRedirects) {
		return storedConfig{}, nil, ErrInvalid
	}
	statuses := append([]int(nil), definition.AllowedStatuses...)
	sort.Ints(statuses)
	return storedConfig{URL: definition.URL, AllowedStatuses: statuses, TimeoutMS: definition.TimeoutMS, WarningLatencyMS: definition.WarningLatencyMS, CertificateWarningDays: definition.CertificateWarningDays, CertificateCriticalDays: definition.CertificateCriticalDays, CertificateEscalationDays: definition.CertificateEscalationDays, ContentMatch: definition.ContentMatch, FollowRedirects: definition.FollowRedirects, MaxRedirects: definition.MaxRedirects, ExpectedRefreshSeconds: definition.IntervalSeconds, HeaderNames: headerNames}, refs, nil
}

func headerNames(headers []SecretHeaderDefinition) []string {
	result := make([]string, len(headers))
	for index, item := range headers {
		result[index] = strings.TrimSpace(item.Name)
	}
	return result
}
func mustURL(value string) *url.URL { parsed, _ := url.Parse(value); return parsed }
func (service *Service) publish(id, result string, at time.Time) {
	if service.hub != nil {
		service.hub.Publish(events.Event{Kind: events.WebpageChanged, MonitorID: id, IntegrationID: id, Result: result, ChangedAt: at})
	}
}

type monitorRow struct {
	DisplayName string
	Enabled     bool
	UpdatedAt   time.Time
}

func readMonitorForUpdate(ctx context.Context, tx pgx.Tx, id string) (monitorRow, error) {
	var item monitorRow
	err := tx.QueryRow(ctx, `SELECT display_name,enabled,updated_at FROM integrations WHERE id=$1 AND adapter_id=$2 FOR UPDATE`, id, webcheck.AdapterID).Scan(&item.DisplayName, &item.Enabled, &item.UpdatedAt)
	return item, err
}
func monitorAudit(item monitorRow) map[string]any {
	return map[string]any{"display_name": item.DisplayName, "enabled": item.Enabled, "version": item.UpdatedAt.UnixMicro()}
}
