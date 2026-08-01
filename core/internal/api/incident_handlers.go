package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/PrincepsVIIII/Espial/core/internal/incidents"
)

func (server *server) incidentList(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.authorize(w, r, "incidents:read", "incidents.read"); !ok {
		return
	}
	if !server.validQuerySize(w, r) || !server.requireIncidents(w, r) {
		return
	}
	filter, fields := parseIncidentFilter(r.URL.Query())
	if len(fields) > 0 {
		server.validationError(w, r, fields)
		return
	}
	result, err := server.dependencies.Incidents.Incidents(r.Context(), filter)
	if server.handleIncidentReadError(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (server *server) incidentAssignees(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.authorize(w, r, "incidents:operate", "incident.assignees.read"); !ok {
		return
	}
	if !server.validQuerySize(w, r) || !server.requireIncidentWorkflow(w, r) {
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
	result, err := server.dependencies.IncidentWorkflow.Assignees(r.Context(), limit, cursor)
	if errors.Is(err, incidents.ErrInvalidCursor) {
		server.validationError(w, r, []APIFieldError{{Field: "cursor", Code: "invalid"}})
		return
	}
	if err != nil {
		server.internalError(w, r, "incident assignee read failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (server *server) acknowledgeIncident(w http.ResponseWriter, r *http.Request) {
	server.mutateIncident(w, r, incidents.ActionAcknowledge)
}

func (server *server) investigateIncident(w http.ResponseWriter, r *http.Request) {
	server.mutateIncident(w, r, incidents.ActionInvestigate)
}

func (server *server) assignIncident(w http.ResponseWriter, r *http.Request) {
	server.mutateIncident(w, r, incidents.ActionAssign)
}

func (server *server) addIncidentNote(w http.ResponseWriter, r *http.Request) {
	server.mutateIncident(w, r, incidents.ActionNote)
}

func (server *server) resolveIncident(w http.ResponseWriter, r *http.Request) {
	server.mutateIncident(w, r, incidents.ActionResolve)
}

func (server *server) mutateIncident(w http.ResponseWriter, r *http.Request, action incidents.Action) {
	session, ok := server.mutationSession(w, r, "incidents:operate", "incident."+string(action))
	if !ok || !server.requireIncidentWorkflow(w, r) {
		return
	}
	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		server.error(w, r, http.StatusNotFound, "not_found", "The requested incident was not found.")
		return
	}
	expectedVersion, ok := parseIncidentETag(r.Header.Get("If-Match"))
	if !ok {
		if strings.TrimSpace(r.Header.Get("If-Match")) == "" {
			server.error(w, r, http.StatusPreconditionRequired, "precondition_required", "Fetch the current incident before changing it.")
		} else {
			server.validationError(w, r, []APIFieldError{{Field: "If-Match", Code: "invalid"}})
		}
		return
	}
	idempotencyKey := r.Header.Get("Idempotency-Key")
	if strings.TrimSpace(idempotencyKey) == "" {
		server.error(w, r, http.StatusPreconditionRequired, "idempotency_key_required", "Idempotency-Key is required.")
		return
	}
	if len(idempotencyKey) > 128 || strings.TrimSpace(idempotencyKey) != idempotencyKey ||
		strings.IndexFunc(idempotencyKey, unicode.IsControl) >= 0 {
		server.validationError(w, r, []APIFieldError{{Field: "Idempotency-Key", Code: "invalid"}})
		return
	}
	var body struct {
		OwnerUserID string `json:"owner_user_id"`
		Note        string `json:"note"`
	}
	if err := decodeJSONLimit(w, r, &body, 8*1024); err != nil {
		server.decodeError(w, r, err)
		return
	}
	result, err := server.dependencies.IncidentWorkflow.Mutate(r.Context(), incidents.Mutation{
		IncidentID: id, Action: action, OwnerUserID: body.OwnerUserID, Note: body.Note,
		ExpectedVersion: expectedVersion, IdempotencyKey: idempotencyKey,
		ActorUserID: session.User.ID, ActorName: session.User.DisplayName,
		SourceAddress: sourceAddress(r), CorrelationID: requestID(r),
	})
	if server.handleIncidentMutationError(w, r, err) {
		return
	}
	w.Header().Set("ETag", incidentETag(result.Version))
	response := struct {
		incidents.MutationResult
		AuditURL string `json:"audit_url,omitempty"`
	}{MutationResult: result}
	if hasPermission(session.User.Permissions, "audit:read") {
		response.AuditURL = "/audit?correlation_id=" + url.QueryEscape(result.CorrelationID)
	}
	writeJSON(w, http.StatusOK, response)
}

func (server *server) handleIncidentMutationError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	var conflict *incidents.VersionConflictError
	switch {
	case errors.Is(err, incidents.ErrNotFound):
		server.error(w, r, http.StatusNotFound, "not_found", "The requested incident was not found.")
	case errors.As(err, &conflict):
		w.Header().Set("ETag", incidentETag(conflict.CurrentVersion))
		server.error(w, r, http.StatusPreconditionFailed, "precondition_failed", "The incident changed; fetch it and review the current state before retrying.")
	case errors.Is(err, incidents.ErrInvalidTransition):
		server.error(w, r, http.StatusConflict, "invalid_transition", "That action is not valid for the incident's current state.")
	case errors.Is(err, incidents.ErrOwnerNotEligible):
		server.validationError(w, r, []APIFieldError{{Field: "owner_user_id", Code: "not_eligible"}})
	case errors.Is(err, incidents.ErrInvalidNote):
		server.validationError(w, r, []APIFieldError{{Field: "note", Code: "invalid"}})
	case errors.Is(err, incidents.ErrInvalidMutation):
		server.validationError(w, r, []APIFieldError{{Field: "body", Code: "invalid"}})
	case errors.Is(err, incidents.ErrIdempotencyConflict):
		server.error(w, r, http.StatusConflict, "idempotency_conflict", "That Idempotency-Key was already used for a different request.")
	default:
		server.internalError(w, r, "incident mutation failed", err)
	}
	return true
}

func (server *server) requireIncidentWorkflow(w http.ResponseWriter, r *http.Request) bool {
	if server.dependencies.IncidentWorkflow != nil {
		return true
	}
	server.error(w, r, http.StatusServiceUnavailable, "unavailable", "Incident workflow is unavailable.")
	return false
}

func incidentETag(version int64) string { return fmt.Sprintf("\"v%x\"", version) }

func parseIncidentETag(value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if len(value) < 4 || value[0] != '"' || value[len(value)-1] != '"' ||
		!strings.HasPrefix(value[1:], "v") {
		return 0, false
	}
	version, err := strconv.ParseInt(value[2:len(value)-1], 16, 64)
	return version, err == nil && version > 0
}

func (server *server) incidentDetail(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.authorize(w, r, "incidents:read", "incident.read"); !ok {
		return
	}
	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		server.error(w, r, http.StatusNotFound, "not_found", "The requested incident was not found.")
		return
	}
	if !server.requireIncidents(w, r) {
		return
	}
	result, err := server.dependencies.Incidents.Incident(r.Context(), id)
	if server.handleIncidentReadError(w, r, err) {
		return
	}
	w.Header().Set("ETag", incidentETag(result.Version))
	writeJSON(w, http.StatusOK, result)
}

func (server *server) incidentTimeline(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.authorize(w, r, "incidents:read", "incident.timeline.read"); !ok {
		return
	}
	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		server.error(w, r, http.StatusNotFound, "not_found", "The requested incident was not found.")
		return
	}
	if !server.validQuerySize(w, r) || !server.requireIncidents(w, r) {
		return
	}
	filter, fields := parseTimelineFilter(r.URL.Query())
	if len(fields) > 0 {
		server.validationError(w, r, fields)
		return
	}
	result, err := server.dependencies.Incidents.Timeline(r.Context(), id, filter)
	if server.handleIncidentReadError(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (server *server) requireIncidents(w http.ResponseWriter, r *http.Request) bool {
	if server.dependencies.Incidents != nil {
		return true
	}
	server.error(w, r, http.StatusServiceUnavailable, "unavailable", "Incident reads are unavailable.")
	return false
}

func (server *server) handleIncidentReadError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, incidents.ErrNotFound) {
		server.error(w, r, http.StatusNotFound, "not_found", "The requested incident was not found.")
		return true
	}
	if errors.Is(err, incidents.ErrInvalidCursor) {
		server.validationError(w, r, []APIFieldError{{Field: "cursor", Code: "invalid"}})
		return true
	}
	server.internalError(w, r, "incident read failed", err)
	return true
}
