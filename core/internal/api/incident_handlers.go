package api

import (
	"errors"
	"fmt"
	"net/http"

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
	w.Header().Set("ETag", fmt.Sprintf("\"v%x\"", result.Version))
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
