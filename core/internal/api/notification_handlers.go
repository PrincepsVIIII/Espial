package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/PrincepsVIIII/Espial/core/internal/adminops"
	"github.com/PrincepsVIIII/Espial/core/internal/notifications"
)

func (server *server) notificationDestinationList(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.authorize(w, r, "notification_destinations:manage", "notification_destinations.read"); !ok {
		return
	}
	if !server.requireNotifications(w, r) {
		return
	}
	if !server.validQuerySize(w, r) {
		return
	}
	values := r.URL.Query()
	fields := rejectUnknownQuery(values, map[string]bool{"limit": true, "cursor": true})
	limit, limitFields := parseLimit(values)
	fields = append(fields, limitFields...)
	cursor, field := singleValue(values, "cursor", 2048)
	if field != nil {
		fields = append(fields, *field)
	}
	if len(fields) > 0 {
		server.validationError(w, r, fields)
		return
	}
	result, err := server.dependencies.Notifications.Destinations(r.Context(), notifications.DestinationFilter{Limit: limit, Cursor: cursor})
	if errors.Is(err, notifications.ErrInvalidCursor) {
		server.validationError(w, r, []APIFieldError{{Field: "cursor", Code: "invalid"}})
		return
	}
	if err != nil {
		server.internalError(w, r, "notification destination list failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (server *server) notificationDestinationDetail(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.authorize(w, r, "notification_destinations:manage", "notification_destination.read"); !ok {
		return
	}
	if !server.requireNotifications(w, r) || !validAdministrativeID(w, r) {
		return
	}
	result, err := server.dependencies.Notifications.Destination(r.Context(), r.PathValue("id"))
	if server.handleNotificationError(w, r, err) {
		return
	}
	w.Header().Set("ETag", incidentETag(result.Version))
	writeJSON(w, http.StatusOK, result)
}

func (server *server) createNotificationDestination(w http.ResponseWriter, r *http.Request) {
	server.mutateNotificationDestination(w, r, false)
}

func (server *server) replaceNotificationDestination(w http.ResponseWriter, r *http.Request) {
	server.mutateNotificationDestination(w, r, true)
}

func (server *server) mutateNotificationDestination(w http.ResponseWriter, r *http.Request, replace bool) {
	session, ok := server.mutationSession(w, r, "notification_destinations:manage", "notification_destination.write")
	if !ok || !server.requireNotifications(w, r) {
		return
	}
	id, version := "", int64(0)
	if replace {
		id = r.PathValue("id")
		if !validAdministrativeID(w, r) {
			return
		}
		var valid bool
		version, valid = parseRequiredVersion(w, r)
		if !valid {
			return
		}
	}
	key, ok := requiredIdempotencyKey(w, r)
	if !ok {
		return
	}
	var body struct {
		DisplayName     string `json:"display_name"`
		DestinationType string `json:"destination_type"`
		Enabled         *bool  `json:"enabled"`
		EndpointHost    string `json:"endpoint_host"`
		EndpointPort    int    `json:"endpoint_port"`
		PathPrefix      string `json:"path_prefix"`
		SecretReference string `json:"secret_reference"`
	}
	if err := decodeJSONLimit(w, r, &body, 8*1024); err != nil {
		server.decodeError(w, r, err)
		return
	}
	if body.Enabled == nil {
		server.validationError(w, r, []APIFieldError{{Field: "enabled", Code: "required"}})
		return
	}
	definition := notifications.DestinationDefinition{DisplayName: body.DisplayName,
		DestinationType: body.DestinationType, Enabled: *body.Enabled,
		EndpointHost: body.EndpointHost, EndpointPort: body.EndpointPort,
		PathPrefix: body.PathPrefix, SecretReference: body.SecretReference}
	metadata := notificationMetadata(session.User.ID, session.User.DisplayName,
		sourceAddress(r), requestID(r), key, version)
	var receipt adminops.Receipt
	var err error
	if replace {
		receipt, err = server.dependencies.Notifications.ReplaceDestination(r.Context(), id, definition, metadata)
	} else {
		receipt, err = server.dependencies.Notifications.CreateDestination(r.Context(), definition, metadata)
	}
	if server.handleNotificationError(w, r, err) {
		return
	}
	completeReceipt(&receipt, session.User.Permissions)
	w.Header().Set("ETag", incidentETag(receipt.Version))
	if replace {
		writeJSON(w, http.StatusOK, receipt)
		return
	}
	w.Header().Set("Location", "/api/v1/notification-destinations/"+receipt.ID)
	writeJSON(w, http.StatusCreated, receipt)
}

func (server *server) testNotificationDestination(w http.ResponseWriter, r *http.Request) {
	session, ok := server.mutationSession(w, r, "notification_destinations:manage", "notification_destination.test")
	if !ok || !server.requireNotifications(w, r) || !validAdministrativeID(w, r) {
		return
	}
	version, ok := parseRequiredVersion(w, r)
	if !ok {
		return
	}
	key, ok := requiredIdempotencyKey(w, r)
	if !ok {
		return
	}
	if err := decodeJSONLimit(w, r, &struct{}{}, 1024); err != nil {
		server.decodeError(w, r, err)
		return
	}
	receipt, err := server.dependencies.Notifications.TestDestination(r.Context(), r.PathValue("id"),
		notificationMetadata(session.User.ID, session.User.DisplayName, sourceAddress(r), requestID(r), key, version))
	if server.handleNotificationError(w, r, err) {
		return
	}
	completeReceipt(&receipt, session.User.Permissions)
	w.Header().Set("ETag", incidentETag(receipt.Version))
	writeJSON(w, http.StatusAccepted, receipt)
}

func (server *server) notificationDeliveryList(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.authorize(w, r, "notification_destinations:manage", "notification_deliveries.read"); !ok {
		return
	}
	server.deliveries(w, r, "")
}

func (server *server) incidentDeliveryList(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.authorize(w, r, "incidents:read", "incident.deliveries.read"); !ok {
		return
	}
	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		server.error(w, r, http.StatusNotFound, "not_found", "The requested incident was not found.")
		return
	}
	if !server.requireIncidents(w, r) || !server.requireNotifications(w, r) {
		return
	}
	if _, err := server.dependencies.Incidents.Incident(r.Context(), id); err != nil {
		server.handleIncidentReadError(w, r, err)
		return
	}
	server.deliveries(w, r, id)
}

func (server *server) deliveries(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !server.validQuerySize(w, r) || !server.requireNotifications(w, r) {
		return
	}
	values := r.URL.Query()
	allowed := map[string]bool{"limit": true, "cursor": true, "state": true}
	if incidentID == "" {
		allowed["incident"] = true
		allowed["destination"] = true
	}
	fields := rejectUnknownQuery(values, allowed)
	limit, limitFields := parseLimit(values)
	fields = append(fields, limitFields...)
	cursor, cursorField := singleValue(values, "cursor", 2048)
	if cursorField != nil {
		fields = append(fields, *cursorField)
	}
	if incidentID == "" {
		var field *APIFieldError
		incidentID, field = singleValue(values, "incident", 36)
		if field != nil {
			fields = append(fields, *field)
		}
	}
	destinationID, field := singleValue(values, "destination", 36)
	if field != nil && incidentID == "" {
		fields = append(fields, *field)
	}
	if incidentID != "" && !uuidPattern.MatchString(incidentID) {
		fields = append(fields, APIFieldError{Field: "incident", Code: "invalid"})
	}
	if destinationID != "" && !uuidPattern.MatchString(destinationID) {
		fields = append(fields, APIFieldError{Field: "destination", Code: "invalid"})
	}
	states := []notifications.State{}
	for _, value := range values["state"] {
		state := notifications.State(value)
		if !validDeliveryState(state) {
			fields = append(fields, APIFieldError{Field: "state", Code: "invalid"})
			continue
		}
		states = append(states, state)
	}
	if len(fields) > 0 {
		server.validationError(w, r, fields)
		return
	}
	result, err := server.dependencies.Notifications.Deliveries(r.Context(), notifications.DeliveryFilter{
		Limit: limit, Cursor: cursor, IncidentID: incidentID, DestinationID: destinationID, States: states,
	})
	if errors.Is(err, notifications.ErrInvalidCursor) {
		server.validationError(w, r, []APIFieldError{{Field: "cursor", Code: "invalid"}})
		return
	}
	if err != nil {
		server.internalError(w, r, "notification delivery list failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func validDeliveryState(state notifications.State) bool {
	switch state {
	case notifications.StateQueued, notifications.StateAttempting, notifications.StateDelivered,
		notifications.StateRetryWait, notifications.StateFailed, notifications.StateDeadLetter,
		notifications.StateSuppressed:
		return true
	default:
		return false
	}
}

func notificationMetadata(actorID, actorName, source, correlation, key string, version int64) notifications.MutationMetadata {
	return notifications.MutationMetadata{ExpectedVersion: version, IdempotencyKey: key,
		ActorUserID: actorID, ActorName: actorName, SourceAddress: source, CorrelationID: correlation}
}

func (server *server) requireNotifications(w http.ResponseWriter, r *http.Request) bool {
	if server.dependencies.Notifications != nil {
		return true
	}
	server.unavailable(w, r, "Notification delivery is unavailable.")
	return false
}

func (server *server) handleNotificationError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, notifications.ErrNotFound):
		server.error(w, r, http.StatusNotFound, "not_found", "The requested notification destination was not found.")
	case errors.Is(err, notifications.ErrConflict):
		server.error(w, r, http.StatusPreconditionFailed, "precondition_failed", "The notification destination changed; fetch it and retry.")
	case errors.Is(err, notifications.ErrIdempotencyConflict):
		server.error(w, r, http.StatusConflict, "idempotency_conflict", "That Idempotency-Key was already used for a different request.")
	case errors.Is(err, notifications.ErrNetworkPolicy):
		server.validationError(w, r, []APIFieldError{{Field: "endpoint_host", Code: "not_approved"}})
	case errors.Is(err, notifications.ErrSecretUnavailable):
		server.validationError(w, r, []APIFieldError{{Field: "secret_reference", Code: "unavailable"}})
	case errors.Is(err, notifications.ErrInvalid), strings.Contains(err.Error(), "invalid notification"):
		server.validationError(w, r, []APIFieldError{{Field: "body", Code: "invalid"}})
	default:
		server.internalError(w, r, "notification operation failed", err)
	}
	return true
}
