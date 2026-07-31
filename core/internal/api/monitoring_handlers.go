package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/PrincepsVIIII/Espial/core/internal/auth"
	"github.com/PrincepsVIIII/Espial/core/internal/monitoring"
)

func (server *server) overview(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.authorize(w, r, "overview:read", "overview.read"); !ok {
		return
	}
	if !server.requireMonitoring(w, r) {
		return
	}
	result, err := server.dependencies.Monitoring.Overview(r.Context())
	if err != nil {
		server.internalError(w, r, "overview read failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (server *server) resources(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.authorize(w, r, "resources:read", "resources.read"); !ok {
		return
	}
	if !server.validQuerySize(w, r) || !server.requireMonitoring(w, r) {
		return
	}
	filter, fields := parseResourceFilter(r.URL.Query())
	if len(fields) > 0 {
		server.validationError(w, r, fields)
		return
	}
	result, err := server.dependencies.Monitoring.Resources(r.Context(), filter)
	if server.handleReadError(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (server *server) resource(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.authorize(w, r, "resources:read", "resource.read"); !ok {
		return
	}
	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		server.error(w, r, http.StatusNotFound, "not_found", "The requested resource was not found.")
		return
	}
	if !server.requireMonitoring(w, r) {
		return
	}
	result, err := server.dependencies.Monitoring.Resource(r.Context(), id)
	if server.handleReadError(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (server *server) integrations(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.authorize(w, r, "integrations:read", "integrations.read"); !ok {
		return
	}
	if !server.validQuerySize(w, r) || !server.requireMonitoring(w, r) {
		return
	}
	filter, fields := parseIntegrationFilter(r.URL.Query())
	if len(fields) > 0 {
		server.validationError(w, r, fields)
		return
	}
	result, err := server.dependencies.Monitoring.Integrations(r.Context(), filter)
	if server.handleReadError(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (server *server) integration(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.authorize(w, r, "integrations:read", "integration.read"); !ok {
		return
	}
	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		server.error(w, r, http.StatusNotFound, "not_found", "The requested integration was not found.")
		return
	}
	if !server.requireMonitoring(w, r) {
		return
	}
	result, err := server.dependencies.Monitoring.Integration(r.Context(), id)
	if server.handleReadError(w, r, err) {
		return
	}
	w.Header().Set("ETag", integrationETag(result.UpdatedAt))
	writeJSON(w, http.StatusOK, result)
}

func (server *server) auditEvents(w http.ResponseWriter, r *http.Request) {
	session, ok := server.authorize(w, r, "audit:read", "audit.read")
	if !ok {
		return
	}
	if !server.validQuerySize(w, r) || !server.requireMonitoring(w, r) {
		return
	}
	filter, fields := parseAuditFilter(r.URL.Query(), server.dependencies.Now().UTC())
	if len(fields) > 0 {
		server.validationError(w, r, fields)
		return
	}
	result, err := server.dependencies.Monitoring.Audit(r.Context(), filter)
	if server.handleReadError(w, r, err) {
		return
	}
	filter.From, filter.To = result.From, result.To
	if err := server.dependencies.Monitoring.RecordAuditRead(
		r.Context(), session.User.ID, sourceAddress(r), requestID(r), filter,
	); err != nil {
		server.internalError(w, r, "audit read record failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (server *server) createIntegration(w http.ResponseWriter, r *http.Request) {
	session, ok := server.mutationSession(w, r, "integrations:manage", "integration.create")
	if !ok {
		return
	}
	if server.dependencies.Integrations == nil {
		server.error(w, r, http.StatusServiceUnavailable, "unavailable", "Integration management is unavailable.")
		return
	}
	var body struct {
		AdapterID        string            `json:"adapter_id"`
		DisplayName      string            `json:"display_name"`
		Enabled          *bool             `json:"enabled"`
		IntervalSeconds  *int              `json:"interval_seconds"`
		ConfigNonsecret  map[string]any    `json:"config_nonsecret"`
		SecretReferences map[string]string `json:"secret_references"`
	}
	if err := decodeJSONLimit(w, r, &body, 128*1024); err != nil {
		server.decodeError(w, r, err)
		return
	}
	var fields []APIFieldError
	if !identifierPattern.MatchString(body.AdapterID) {
		fields = append(fields, APIFieldError{Field: "adapter_id", Code: "invalid"})
	}
	if strings.TrimSpace(body.DisplayName) == "" || utf8.RuneCountInString(body.DisplayName) > 128 {
		fields = append(fields, APIFieldError{Field: "display_name", Code: "invalid"})
	}
	if body.Enabled == nil {
		fields = append(fields, APIFieldError{Field: "enabled", Code: "required"})
	}
	if body.IntervalSeconds == nil {
		fields = append(fields, APIFieldError{Field: "interval_seconds", Code: "required"})
	} else if *body.IntervalSeconds < 1 || *body.IntervalSeconds > 86400 {
		fields = append(fields, APIFieldError{Field: "interval_seconds", Code: "out_of_range"})
	}
	if body.ConfigNonsecret == nil {
		fields = append(fields, APIFieldError{Field: "config_nonsecret", Code: "required"})
	}
	if body.SecretReferences == nil {
		fields = append(fields, APIFieldError{Field: "secret_references", Code: "required"})
	}
	if len(fields) > 0 {
		server.validationError(w, r, fields)
		return
	}
	id, updatedAt, err := server.dependencies.Integrations.Create(r.Context(), monitoring.CreateIntegration{
		AdapterID: body.AdapterID, DisplayName: body.DisplayName, Enabled: *body.Enabled,
		Interval:        time.Duration(*body.IntervalSeconds) * time.Second,
		ConfigNonsecret: body.ConfigNonsecret, SecretReferences: body.SecretReferences,
		ActorUserID: session.User.ID, SourceAddress: sourceAddress(r), CorrelationID: requestID(r),
	})
	if err != nil {
		var operational *monitoring.Error
		if errors.As(err, &operational) && operational.Code == "adapter_not_registered" {
			server.error(w, r, http.StatusConflict, operational.Code, "The requested adapter is not registered in Core.")
			return
		}
		if errors.As(err, &operational) {
			server.validationError(w, r, []APIFieldError{{Field: "integration", Code: operational.Code}})
			return
		}
		server.internalError(w, r, "integration creation failed", err)
		return
	}
	w.Header().Set("Location", "/api/v1/integrations/"+id)
	w.Header().Set("ETag", integrationETag(updatedAt))
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "updated_at": updatedAt.UTC()})
}

func (server *server) updateIntegration(w http.ResponseWriter, r *http.Request) {
	session, ok := server.mutationSession(w, r, "integrations:manage", "integration.configuration.update")
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		server.error(w, r, http.StatusNotFound, "not_found", "The requested integration was not found.")
		return
	}
	if server.dependencies.Integrations == nil {
		server.error(w, r, http.StatusServiceUnavailable, "unavailable", "Integration management is unavailable.")
		return
	}
	ifMatch := strings.TrimSpace(r.Header.Get("If-Match"))
	if ifMatch == "" {
		server.error(w, r, http.StatusPreconditionRequired, "precondition_required", "If-Match is required.")
		return
	}
	expected, err := parseIntegrationETag(ifMatch)
	if err != nil {
		server.errorFields(w, r, http.StatusBadRequest, "validation_failed", "Request validation failed.", []APIFieldError{{Field: "If-Match", Code: "invalid"}})
		return
	}
	var body struct {
		Enabled          *bool             `json:"enabled"`
		IntervalSeconds  *int              `json:"interval_seconds"`
		ConfigNonsecret  map[string]any    `json:"config_nonsecret"`
		SecretReferences map[string]string `json:"secret_references"`
	}
	if err := decodeJSONLimit(w, r, &body, 128*1024); err != nil {
		server.decodeError(w, r, err)
		return
	}
	if body.Enabled == nil || body.IntervalSeconds == nil || body.ConfigNonsecret == nil || body.SecretReferences == nil {
		server.validationError(w, r, []APIFieldError{{Field: "body", Code: "required_fields"}})
		return
	}
	if *body.IntervalSeconds < 1 || *body.IntervalSeconds > 86400 {
		server.validationError(w, r, []APIFieldError{{Field: "interval_seconds", Code: "out_of_range"}})
		return
	}
	updatedAt, err := server.dependencies.Integrations.Update(r.Context(), monitoring.IntegrationConfigUpdate{
		IntegrationID: id, Enabled: *body.Enabled,
		Interval:        time.Duration(*body.IntervalSeconds) * time.Second,
		ConfigNonsecret: body.ConfigNonsecret, SecretReferences: body.SecretReferences,
		ExpectedUpdatedAt: expected, ActorUserID: session.User.ID,
		SourceAddress: sourceAddress(r), CorrelationID: requestID(r),
	})
	if err != nil {
		var operational *monitoring.Error
		if errors.As(err, &operational) {
			switch operational.Code {
			case "integration_not_found":
				server.error(w, r, http.StatusNotFound, "not_found", "The requested integration was not found.")
			case "integration_config_conflict":
				server.error(w, r, http.StatusPreconditionFailed, "precondition_failed", "The integration changed; fetch it and retry.")
			default:
				server.validationError(w, r, []APIFieldError{{Field: "integration", Code: operational.Code}})
			}
			return
		}
		server.internalError(w, r, "integration update failed", err)
		return
	}
	w.Header().Set("ETag", integrationETag(updatedAt))
	w.WriteHeader(http.StatusNoContent)
}

func (server *server) authorize(w http.ResponseWriter, r *http.Request, permission, action string) (auth.Session, bool) {
	session, ok := server.session(w, r)
	if !ok {
		return auth.Session{}, false
	}
	if !hasPermission(session.User.Permissions, permission) {
		_ = server.dependencies.Auth.RecordDenied(r.Context(), session.User, action, sourceAddress(r), requestID(r))
		server.error(w, r, http.StatusForbidden, "forbidden", "You do not have permission to perform this action.")
		return auth.Session{}, false
	}
	return session, true
}

func (server *server) mutationSession(w http.ResponseWriter, r *http.Request, permission, action string) (auth.Session, bool) {
	if !server.trustedOrigin(r) {
		server.error(w, r, http.StatusForbidden, "origin_rejected", "Request origin was not accepted.")
		return auth.Session{}, false
	}
	session, ok := server.authorize(w, r, permission, action)
	if !ok {
		return auth.Session{}, false
	}
	csrf, err := r.Cookie(csrfCookie)
	if err != nil || csrf.Value != r.Header.Get("X-CSRF-Token") || !server.dependencies.Auth.VerifyCSRF(session, csrf.Value) {
		server.error(w, r, http.StatusForbidden, "csrf_rejected", "CSRF validation failed.")
		return auth.Session{}, false
	}
	return session, true
}

func (server *server) validationError(w http.ResponseWriter, r *http.Request, fields []APIFieldError) {
	server.errorFields(w, r, http.StatusBadRequest, "validation_failed", "Request validation failed.", fields)
}

func (server *server) handleReadError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, monitoring.ErrNotFound) {
		server.error(w, r, http.StatusNotFound, "not_found", "The requested record was not found.")
		return true
	}
	var operational *monitoring.Error
	if errors.As(err, &operational) && operational.Code == "invalid_cursor" {
		server.validationError(w, r, []APIFieldError{{Field: "cursor", Code: "invalid"}})
		return true
	}
	server.internalError(w, r, "monitoring read failed", err)
	return true
}

func (server *server) internalError(w http.ResponseWriter, r *http.Request, message string, err error) {
	server.dependencies.Logger.ErrorContext(r.Context(), message, "request_id", requestID(r), "error", err)
	server.error(w, r, http.StatusInternalServerError, "internal_error", "The request could not be completed.")
}

func (server *server) validQuerySize(w http.ResponseWriter, r *http.Request) bool {
	if len(r.URL.RawQuery) <= maxQueryBytes {
		return true
	}
	server.errorFields(w, r, http.StatusRequestURITooLong, "request_too_large", "The query string is too large.", []APIFieldError{{Field: "query", Code: "too_large"}})
	return false
}

func (server *server) requireMonitoring(w http.ResponseWriter, r *http.Request) bool {
	if server.dependencies.Monitoring != nil {
		return true
	}
	server.error(w, r, http.StatusServiceUnavailable, "unavailable", "Monitoring reads are unavailable.")
	return false
}

func (server *server) decodeError(w http.ResponseWriter, r *http.Request, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		server.error(w, r, http.StatusRequestEntityTooLarge, "request_too_large", "The request body is too large.")
		return
	}
	server.validationError(w, r, []APIFieldError{{Field: "body", Code: "invalid"}})
}

func integrationETag(updatedAt time.Time) string {
	return fmt.Sprintf("\"%x\"", updatedAt.UTC().UnixMicro())
}

func parseIntegrationETag(value string) (time.Time, error) {
	if len(value) < 3 || value[0] != '"' || value[len(value)-1] != '"' || strings.HasPrefix(value, "W/") {
		return time.Time{}, errors.New("invalid ETag")
	}
	microseconds, err := strconv.ParseInt(value[1:len(value)-1], 16, 64)
	if err != nil || microseconds <= 0 {
		return time.Time{}, errors.New("invalid ETag")
	}
	return time.UnixMicro(microseconds).UTC(), nil
}
