package events

import (
	"testing"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/health"
)

func TestHubPublishesAndReplaysInOrder(t *testing.T) {
	hub := NewHub(3)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	for index := 0; index < 3; index++ {
		hub.Publish(Event{Kind: CollectionChanged, IntegrationID: "integration", ChangedAt: now.Add(time.Duration(index) * time.Second)})
	}
	after := uint64(1)
	subscription := hub.Subscribe(&after, 3)
	defer subscription.Close()
	for want := uint64(2); want <= 3; want++ {
		event := <-subscription.Events
		if event.ID != want || event.SchemaVersion != SchemaVersion {
			t.Fatalf("event = %#v, want ID %d", event, want)
		}
	}
}

func TestOldCursorAndSlowSubscriberReceiveResync(t *testing.T) {
	hub := NewHub(2)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	for index := 0; index < 3; index++ {
		hub.Publish(Event{Kind: CollectionChanged, ChangedAt: now})
	}
	old := uint64(0)
	oldSubscription := hub.Subscribe(&old, 2)
	defer oldSubscription.Close()
	if event := <-oldSubscription.Events; event.Kind != ResyncRequired {
		t.Fatalf("old cursor event = %#v", event)
	}

	slow := hub.Subscribe(nil, 1)
	defer slow.Close()
	hub.Publish(Event{Kind: CollectionChanged, ChangedAt: now})
	hub.Publish(Event{Kind: CollectionChanged, ChangedAt: now})
	if event := <-slow.Events; event.Kind != ResyncRequired {
		t.Fatalf("slow subscriber event = %#v", event)
	}
	_, dropped, replayed := hub.Stats()
	if dropped != 2 || replayed != 2 {
		t.Fatalf("stats dropped=%d replayed=%d", dropped, replayed)
	}
}

func TestHealthPublisherEmitsPostCommitStateInvalidations(t *testing.T) {
	hub := NewHub(4)
	subscription := hub.Subscribe(nil, 2)
	defer subscription.Close()
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	hub.PublishCommitted([]health.Change{{After: health.Current{
		ResourceID: "resource", State: health.Stale, UpdatedAt: now,
	}}})
	event := <-subscription.Events
	if event.Kind != StateChanged || event.ResourceID != "resource" || event.State != health.Stale || !event.ChangedAt.Equal(now) {
		t.Fatalf("event = %#v", event)
	}
}

func TestClosedSubscriptionIsRemoved(t *testing.T) {
	hub := NewHub(2)
	subscription := hub.Subscribe(nil, 1)
	subscription.Close()
	subscription.Close()
	subscribers, _, _ := hub.Stats()
	if subscribers != 0 {
		t.Fatalf("subscriber count = %d", subscribers)
	}
}
