// Package operations exposes bounded, low-cardinality operational evidence.
package operations

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/notifications"
	"github.com/PrincepsVIIII/Espial/core/internal/signals"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	incidentSeverities = []string{"warning", "critical"}
	incidentStatuses   = []string{"open", "acknowledged", "investigating", "recovered", "resolved"}
	healthStates       = []string{"healthy", "warning", "critical", "unknown", "stale", "maintenance", "disabled"}
	certificateStates  = []string{"healthy", "warning", "critical", "unknown"}
	websiteStages      = []string{"dns", "tcp", "tls", "http", "total"}
	expiryBuckets      = []string{"expired", "0_7", "8_14", "15_30", "over_30", "unknown"}
)

type Snapshot struct {
	Signals             signals.Metrics
	Notifications       notifications.Metrics
	IncidentsBySeverity map[string]int64
	IncidentsByStatus   map[string]int64
	WebpagesByState     map[string]int64
	CertificatesByState map[string]int64
	WebsiteLatency      map[string]float64
	CertificateExpiry   map[string]int64
	ActiveMaintenance   int64
	ActiveSilences      int64
}

type Collector struct {
	pool          *pgxpool.Pool
	signals       *signals.Store
	notifications *notifications.Service
	now           func() time.Time
}

func NewCollector(pool *pgxpool.Pool, notificationService *notifications.Service, now func() time.Time) *Collector {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Collector{pool: pool, signals: signals.NewStore(pool), notifications: notificationService, now: now}
}

func (collector *Collector) Snapshot(ctx context.Context) (Snapshot, error) {
	now := collector.now().UTC().Truncate(time.Microsecond)
	signalMetrics, err := collector.signals.Metrics(ctx, now)
	if err != nil {
		return Snapshot{}, err
	}
	notificationMetrics, err := collector.notifications.Metrics(ctx, now)
	if err != nil {
		return Snapshot{}, err
	}
	result := Snapshot{
		Signals: signalMetrics, Notifications: notificationMetrics,
		IncidentsBySeverity: zeroMap(incidentSeverities), IncidentsByStatus: zeroMap(incidentStatuses),
		WebpagesByState: zeroMap(healthStates), CertificatesByState: zeroMap(certificateStates),
		WebsiteLatency: zeroFloatMap(websiteStages), CertificateExpiry: zeroMap(expiryBuckets),
	}
	rows, err := collector.pool.Query(ctx, `SELECT severity,status,count(*) FROM incidents GROUP BY severity,status`)
	if err != nil {
		return Snapshot{}, fmt.Errorf("read incident metrics: %w", err)
	}
	for rows.Next() {
		var severity, status string
		var count int64
		if err := rows.Scan(&severity, &status, &count); err != nil {
			rows.Close()
			return Snapshot{}, fmt.Errorf("scan incident metrics: %w", err)
		}
		if _, ok := result.IncidentsBySeverity[severity]; ok && status != "resolved" {
			result.IncidentsBySeverity[severity] += count
		}
		if _, ok := result.IncidentsByStatus[status]; ok {
			result.IncidentsByStatus[status] += count
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Snapshot{}, fmt.Errorf("iterate incident metrics: %w", err)
	}
	rows.Close()

	if err := collector.pool.QueryRow(ctx, `SELECT
		count(*) FILTER (WHERE enabled AND revoked_at IS NULL AND starts_at <= $1 AND ends_at > $1),
		(SELECT count(*) FROM silences WHERE enabled AND revoked_at IS NULL AND starts_at <= $1 AND expires_at > $1)
		FROM maintenance_windows`, now).Scan(&result.ActiveMaintenance, &result.ActiveSilences); err != nil {
		return Snapshot{}, fmt.Errorf("read suppression metrics: %w", err)
	}
	if err := scanCounts(ctx, collector.pool, `SELECT health.state,count(*) FROM resources resource
		JOIN current_health health ON health.resource_id=resource.id WHERE resource.kind='webpage' GROUP BY health.state`, result.WebpagesByState); err != nil {
		return Snapshot{}, fmt.Errorf("read webpage metrics: %w", err)
	}
	if err := scanCounts(ctx, collector.pool, `WITH latest AS (
		SELECT DISTINCT ON (resource_id) resource_id,certificate_state FROM certificate_observations
		ORDER BY resource_id,observed_at DESC,observation_id DESC
	) SELECT certificate_state,count(*) FROM latest GROUP BY certificate_state`, result.CertificatesByState); err != nil {
		return Snapshot{}, fmt.Errorf("read certificate metrics: %w", err)
	}
	var dnsMS, tcpMS, tlsMS, httpMS, totalMS float64
	if err := collector.pool.QueryRow(ctx, `WITH latest AS (
		SELECT DISTINCT ON (resource_id) measurements FROM observations
		WHERE check_type='website.availability' ORDER BY resource_id,observed_at DESC,received_at DESC,id DESC
	) SELECT
		COALESCE(avg((measurements->>'dns_ms')::double precision),0),
		COALESCE(avg((measurements->>'tcp_ms')::double precision),0),
		COALESCE(avg((measurements->>'tls_ms')::double precision),0),
		COALESCE(avg((measurements->>'http_ms')::double precision),0),
		COALESCE(avg((measurements->>'total_ms')::double precision),0)
		FROM latest`).Scan(&dnsMS, &tcpMS, &tlsMS, &httpMS, &totalMS); err != nil {
		return Snapshot{}, fmt.Errorf("read website latency metrics: %w", err)
	}
	result.WebsiteLatency["dns"], result.WebsiteLatency["tcp"], result.WebsiteLatency["tls"] = dnsMS, tcpMS, tlsMS
	result.WebsiteLatency["http"], result.WebsiteLatency["total"] = httpMS, totalMS
	var expired, zeroToSeven, eightToFourteen, fifteenToThirty, overThirty, unknown int64
	if err := collector.pool.QueryRow(ctx, `WITH latest AS (
		SELECT DISTINCT ON (resource_id) days_remaining FROM certificate_observations
		ORDER BY resource_id,observed_at DESC,observation_id DESC
	) SELECT
		count(*) FILTER (WHERE days_remaining < 0),
		count(*) FILTER (WHERE days_remaining BETWEEN 0 AND 7),
		count(*) FILTER (WHERE days_remaining BETWEEN 8 AND 14),
		count(*) FILTER (WHERE days_remaining BETWEEN 15 AND 30),
		count(*) FILTER (WHERE days_remaining > 30),
		count(*) FILTER (WHERE days_remaining IS NULL)
		FROM latest`).Scan(&expired, &zeroToSeven, &eightToFourteen, &fifteenToThirty, &overThirty, &unknown); err != nil {
		return Snapshot{}, fmt.Errorf("read certificate expiry metrics: %w", err)
	}
	result.CertificateExpiry["expired"], result.CertificateExpiry["0_7"] = expired, zeroToSeven
	result.CertificateExpiry["8_14"], result.CertificateExpiry["15_30"] = eightToFourteen, fifteenToThirty
	result.CertificateExpiry["over_30"], result.CertificateExpiry["unknown"] = overThirty, unknown
	return result, nil
}

func RenderPrometheus(snapshot Snapshot) string {
	var output strings.Builder
	metric(&output, "espial_monitoring_signals_queued", snapshot.Signals.QueueDepth)
	metricFloat(&output, "espial_monitoring_signals_oldest_age_seconds", snapshot.Signals.OldestAge.Seconds())
	metric(&output, "espial_monitoring_signals_claimed", snapshot.Signals.Claimed)
	metric(&output, "espial_monitoring_signals_retried", snapshot.Signals.Retried)
	metric(&output, "espial_monitoring_signals_dead_letter", snapshot.Signals.DeadLetters)
	metricFloat(&output, "espial_monitoring_signals_processing_latency_seconds", snapshot.Signals.AverageProcessingLatency.Seconds())
	for _, entry := range []struct {
		state string
		value int64
	}{{"queued", snapshot.Notifications.Queued}, {"attempting", snapshot.Notifications.Attempting}, {"retry_wait", snapshot.Notifications.RetryWaiting}, {"delivered", snapshot.Notifications.Delivered}, {"failed", snapshot.Notifications.Failed}, {"dead_letter", snapshot.Notifications.DeadLetters}, {"suppressed", snapshot.Notifications.Suppressed}} {
		metricLabel(&output, "espial_notification_intents", "state", entry.state, entry.value)
	}
	metricFloat(&output, "espial_notification_oldest_due_age_seconds", snapshot.Notifications.OldestDueAge.Seconds())
	metric(&output, "espial_notification_attempts_total", snapshot.Notifications.AttemptTotal)
	mapMetrics(&output, "espial_incidents_active", "severity", incidentSeverities, snapshot.IncidentsBySeverity)
	mapMetrics(&output, "espial_incidents", "status", incidentStatuses, snapshot.IncidentsByStatus)
	mapMetrics(&output, "espial_webpages", "state", healthStates, snapshot.WebpagesByState)
	mapMetrics(&output, "espial_certificates", "state", certificateStates, snapshot.CertificatesByState)
	mapFloatMetrics(&output, "espial_website_check_stage_latency_seconds", "stage", websiteStages, snapshot.WebsiteLatency, 0.001)
	mapMetrics(&output, "espial_certificate_expiry", "bucket", expiryBuckets, snapshot.CertificateExpiry)
	metric(&output, "espial_maintenance_windows_active", snapshot.ActiveMaintenance)
	metric(&output, "espial_silences_active", snapshot.ActiveSilences)
	return output.String()
}

func zeroMap(keys []string) map[string]int64 {
	result := make(map[string]int64, len(keys))
	for _, key := range keys {
		result[key] = 0
	}
	return result
}

func zeroFloatMap(keys []string) map[string]float64 {
	result := make(map[string]float64, len(keys))
	for _, key := range keys {
		result[key] = 0
	}
	return result
}

func scanCounts(ctx context.Context, pool *pgxpool.Pool, query string, destination map[string]int64) error {
	rows, err := pool.Query(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var count int64
		if err := rows.Scan(&key, &count); err != nil {
			return err
		}
		if _, ok := destination[key]; ok {
			destination[key] = count
		}
	}
	return rows.Err()
}

func metric(output *strings.Builder, name string, value int64) {
	fmt.Fprintf(output, "%s %d\n", name, value)
}

func metricFloat(output *strings.Builder, name string, value float64) {
	fmt.Fprintf(output, "%s %.6f\n", name, value)
}

func metricLabel(output *strings.Builder, name, label, value string, count int64) {
	fmt.Fprintf(output, "%s{%s=%q} %d\n", name, label, value, count)
}

func mapMetrics(output *strings.Builder, name, label string, order []string, values map[string]int64) {
	keys := append([]string(nil), order...)
	sort.Strings(keys)
	for _, key := range keys {
		metricLabel(output, name, label, key, values[key])
	}
}

func mapFloatMetrics(output *strings.Builder, name, label string, order []string, values map[string]float64, scale float64) {
	keys := append([]string(nil), order...)
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(output, "%s{%s=%q} %.6f\n", name, label, key, values[key]*scale)
	}
}
