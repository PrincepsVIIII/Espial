// Package events provides bounded in-process invalidation replay.
package events

import (
	"sync"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/health"
)

const (
	SchemaVersion                  = 1
	StateChanged                   = "resource_state_changed"
	IntegrationChanged             = "integration_changed"
	CollectionChanged              = "collection_changed"
	IncidentChanged                = "incident_changed"
	SuppressionChanged             = "suppression_changed"
	NotificationDestinationChanged = "notification_destination_changed"
	NotificationDeliveryChanged    = "notification_delivery_changed"
	ResyncRequired                 = "resync_required"

	DefaultReplayCapacity     = 1024
	DefaultSubscriberCapacity = 64
	MaxSubscriberCapacity     = 256
)

type Event struct {
	ID            uint64
	Kind          string
	SchemaVersion int
	ResourceID    string
	IntegrationID string
	IncidentID    string
	DeliveryID    string
	State         health.State
	Result        string
	ChangedAt     time.Time
}

type subscriber struct {
	channel chan Event
	resync  bool
}

type Hub struct {
	mu          sync.Mutex
	nextID      uint64
	replay      []Event
	replayLimit int
	subscribers map[uint64]*subscriber
	nextSubID   uint64
	dropped     uint64
}

type Subscription struct {
	Events <-chan Event
	close  func()
	once   sync.Once
}

func NewHub(replayCapacity int) *Hub {
	if replayCapacity <= 0 {
		replayCapacity = DefaultReplayCapacity
	}
	return &Hub{replayLimit: replayCapacity, subscribers: make(map[uint64]*subscriber)}
}

func (hub *Hub) Publish(event Event) Event {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	hub.nextID++
	event.ID = hub.nextID
	event.SchemaVersion = SchemaVersion
	event.ChangedAt = event.ChangedAt.UTC()
	hub.replay = append(hub.replay, event)
	if len(hub.replay) > hub.replayLimit {
		hub.replay = append([]Event(nil), hub.replay[len(hub.replay)-hub.replayLimit:]...)
	}
	for _, target := range hub.subscribers {
		if target.resync {
			continue
		}
		select {
		case target.channel <- event:
		default:
			hub.forceResyncLocked(target, event)
		}
	}
	return event
}

func (hub *Hub) PublishCommitted(changes []health.Change) {
	for _, change := range changes {
		hub.Publish(Event{
			Kind: StateChanged, ResourceID: change.After.ResourceID,
			State: change.After.State, ChangedAt: change.After.UpdatedAt,
		})
	}
}

// Subscribe replays events after the supplied ID. A nil ID begins with future
// events only. A cursor outside the bounded replay window receives one resync event.
func (hub *Hub) Subscribe(afterID *uint64, capacity int) *Subscription {
	if capacity <= 0 {
		capacity = DefaultSubscriberCapacity
	}
	if capacity > MaxSubscriberCapacity {
		capacity = MaxSubscriberCapacity
	}
	hub.mu.Lock()
	defer hub.mu.Unlock()
	hub.nextSubID++
	id := hub.nextSubID
	target := &subscriber{channel: make(chan Event, capacity)}
	hub.subscribers[id] = target
	if afterID != nil {
		if *afterID > hub.nextID || len(hub.replay) > 0 && *afterID+1 < hub.replay[0].ID {
			hub.forceResyncLocked(target, Event{ID: hub.nextID, ChangedAt: time.Now().UTC()})
		} else {
			for _, event := range hub.replay {
				if event.ID <= *afterID {
					continue
				}
				select {
				case target.channel <- event:
				default:
					hub.forceResyncLocked(target, event)
				}
				if target.resync {
					break
				}
			}
		}
	}
	return &Subscription{Events: target.channel, close: func() { hub.unsubscribe(id) }}
}

func (subscription *Subscription) Close() {
	subscription.once.Do(subscription.close)
}

func (hub *Hub) Stats() (subscribers int, dropped uint64, replayed int) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	return len(hub.subscribers), hub.dropped, len(hub.replay)
}

func (hub *Hub) forceResyncLocked(target *subscriber, cause Event) {
	for {
		select {
		case <-target.channel:
		default:
			goto drained
		}
	}
drained:
	target.resync = true
	hub.dropped++
	target.channel <- Event{
		ID: cause.ID, Kind: ResyncRequired, SchemaVersion: SchemaVersion,
		ChangedAt: cause.ChangedAt.UTC(),
	}
}

func (hub *Hub) unsubscribe(id uint64) {
	hub.mu.Lock()
	target, exists := hub.subscribers[id]
	if exists {
		delete(hub.subscribers, id)
		close(target.channel)
	}
	hub.mu.Unlock()
}
