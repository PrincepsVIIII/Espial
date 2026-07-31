package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/events"
	"github.com/PrincepsVIIII/Espial/core/internal/health"
)

type liveInvalidation struct {
	SchemaVersion int          `json:"schema_version"`
	ResourceID    string       `json:"resource_id,omitempty"`
	IntegrationID string       `json:"integration_id,omitempty"`
	State         health.State `json:"state,omitempty"`
	Result        string       `json:"result,omitempty"`
	ChangedAt     time.Time    `json:"changed_at"`
}

func (server *server) eventStream(w http.ResponseWriter, r *http.Request) {
	if server.dependencies.Events == nil {
		server.error(w, r, http.StatusServiceUnavailable, "unavailable", "Live events are unavailable.")
		return
	}
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		server.unauthorized(w, r)
		return
	}
	session, err := server.dependencies.Auth.Authenticate(r.Context(), cookie.Value)
	if err != nil {
		server.unauthorized(w, r)
		return
	}
	if !hasPermission(session.User.Permissions, "overview:read") {
		_ = server.dependencies.Auth.RecordDenied(r.Context(), session.User, "events.stream", sourceAddress(r), requestID(r))
		server.error(w, r, http.StatusForbidden, "forbidden", "You do not have permission to perform this action.")
		return
	}
	select {
	case server.sseSlots <- struct{}{}:
		defer func() { <-server.sseSlots }()
	default:
		w.Header().Set("Retry-After", "15")
		server.error(w, r, http.StatusServiceUnavailable, "stream_limit_reached", "Live event capacity is currently full.")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		server.error(w, r, http.StatusInternalServerError, "stream_unsupported", "Live events are unavailable.")
		return
	}
	var afterID *uint64
	if raw := strings.TrimSpace(r.Header.Get("Last-Event-ID")); raw != "" {
		if len(raw) > 20 {
			server.validationError(w, r, []APIFieldError{{Field: "Last-Event-ID", Code: "invalid"}})
			return
		}
		parsed, parseErr := strconv.ParseUint(raw, 10, 64)
		if parseErr != nil {
			server.validationError(w, r, []APIFieldError{{Field: "Last-Event-ID", Code: "invalid"}})
			return
		}
		afterID = &parsed
	}
	subscription := server.dependencies.Events.Subscribe(afterID, events.DefaultSubscriberCapacity)
	defer subscription.Close()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if _, err := fmt.Fprint(w, ": connected\n\n"); err != nil {
		return
	}
	flusher.Flush()

	heartbeat := time.NewTicker(server.dependencies.SSEHeartbeat)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case event, open := <-subscription.Events:
			if !open || writeSSEEvent(w, event) != nil {
				return
			}
			flusher.Flush()
			if event.Kind == events.ResyncRequired {
				return
			}
		case <-heartbeat.C:
			revalidationTimeout := server.dependencies.SSEHeartbeat / 2
			if revalidationTimeout > 2*time.Second {
				revalidationTimeout = 2 * time.Second
			}
			if revalidationTimeout <= 0 {
				revalidationTimeout = time.Second
			}
			authContext, cancel := context.WithTimeout(r.Context(), revalidationTimeout)
			current, authenticateErr := server.dependencies.Auth.Authenticate(authContext, cookie.Value)
			cancel()
			if authenticateErr != nil || !hasPermission(current.User.Permissions, "overview:read") {
				return
			}
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func writeSSEEvent(w http.ResponseWriter, event events.Event) error {
	if event.Kind == "" || strings.ContainsAny(event.Kind, "\r\n") {
		return errors.New("invalid event kind")
	}
	data, err := json.Marshal(liveInvalidation{
		SchemaVersion: event.SchemaVersion, ResourceID: event.ResourceID,
		IntegrationID: event.IntegrationID, State: event.State, Result: event.Result,
		ChangedAt: event.ChangedAt.UTC(),
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", event.ID, event.Kind, data)
	return err
}
