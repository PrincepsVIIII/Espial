package operations

import (
	"strings"
	"testing"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/notifications"
	"github.com/PrincepsVIIII/Espial/core/internal/signals"
)

func TestRenderPrometheusUsesOnlyBoundedLabels(t *testing.T) {
	text := RenderPrometheus(Snapshot{
		Signals:             signals.Metrics{QueueDepth: 4, OldestAge: 3 * time.Second},
		Notifications:       notifications.Metrics{Queued: 2, DeadLetters: 1},
		IncidentsBySeverity: map[string]int64{"critical": 1, "warning": 2, "private-host": 99},
		IncidentsByStatus:   map[string]int64{"open": 1},
		WebpagesByState:     map[string]int64{"healthy": 3}, CertificatesByState: map[string]int64{"warning": 2},
		WebsiteLatency: map[string]float64{"tls": 125}, CertificateExpiry: map[string]int64{"0_7": 1},
	})
	for _, expected := range []string{
		"espial_monitoring_signals_queued 4", `espial_notification_intents{state="dead_letter"} 1`,
		`espial_incidents_active{severity="critical"} 1`, `espial_certificates{state="warning"} 2`,
		`espial_website_check_stage_latency_seconds{stage="tls"} 0.125000`, `espial_certificate_expiry{bucket="0_7"} 1`,
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("metrics missing %q:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "private-host") {
		t.Fatalf("unbounded label escaped into metrics: %s", text)
	}
}
