package operations

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/notifications"
)

func TestCollectorReportsBoundedDatabaseState(t *testing.T) {
	pool := loadTestPool(t)
	seedLoadDomain(t, pool, 1, 1)
	now := time.Now().UTC().Add(time.Second)
	collector := NewCollector(pool, notifications.NewService(pool, nil, nil, nil, nil), func() time.Time { return now })
	snapshot, err := collector.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Signals.QueueDepth != 1 || snapshot.Notifications.Queued != 1 || snapshot.ActiveMaintenance != 0 || snapshot.ActiveSilences != 0 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	text := RenderPrometheus(snapshot)
	for _, prohibited := range []string{loadIntegrationID, loadResourceID, "mattermost.example.invalid", "load-token"} {
		if strings.Contains(text, prohibited) {
			t.Fatalf("metrics leaked %q: %s", prohibited, text)
		}
	}
}
