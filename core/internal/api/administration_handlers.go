package api

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/PrincepsVIIII/Espial/core/internal/adminops"
	"github.com/PrincepsVIIII/Espial/core/internal/health"
	"github.com/PrincepsVIIII/Espial/core/internal/incidents"
	"github.com/PrincepsVIIII/Espial/core/internal/suppressions"
)

type ruleWriteBody struct {
	Name          string `json:"name"`
	Enabled       *bool  `json:"enabled"`
	Priority      int    `json:"priority"`
	IntegrationID string `json:"integration_id"`
	ResourceID    string `json:"resource_id"`
	ResourceKind  string `json:"resource_kind"`
	CheckType     string `json:"check_type"`
	ReasonCode    string `json:"reason_code"`
	Conditions    []struct {
		State          health.State       `json:"state"`
		Severity       incidents.Severity `json:"severity"`
		MinOccurrences int                `json:"min_occurrences"`
		ForSeconds     int                `json:"for_seconds"`
	} `json:"conditions"`
	RecoveryState          health.State `json:"recovery_state"`
	RecoveryMinOccurrences int          `json:"recovery_min_occurrences"`
	RecoveryForSeconds     int          `json:"recovery_for_seconds"`
}

func (body ruleWriteBody) input() (incidents.RuleDefinition, bool) {
	conditions := make([]incidents.RuleCondition, len(body.Conditions))
	for index, item := range body.Conditions {
		conditions[index] = incidents.RuleCondition{State: item.State, Severity: item.Severity, MinOccurrences: item.MinOccurrences, For: time.Duration(item.ForSeconds) * time.Second}
	}
	enabled := false
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	return incidents.RuleDefinition{Name: body.Name, Priority: body.Priority, IntegrationID: body.IntegrationID, ResourceID: body.ResourceID, ResourceKind: body.ResourceKind, CheckType: body.CheckType, ReasonCode: body.ReasonCode, Conditions: conditions, RecoveryState: body.RecoveryState, RecoveryMinOccurrences: body.RecoveryMinOccurrences, RecoveryFor: time.Duration(body.RecoveryForSeconds) * time.Second}, enabled
}

func (server *server) incidentRuleList(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.authorize(w, r, "incident_rules:manage", "incident_rules.read"); !ok {
		return
	}
	if server.dependencies.IncidentRules == nil {
		server.unavailable(w, r, "Incident rule management is unavailable.")
		return
	}
	result, err := server.dependencies.IncidentRules.List(r.Context())
	if err != nil {
		server.internalError(w, r, "incident rule list failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (server *server) incidentRuleDetail(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.authorize(w, r, "incident_rules:manage", "incident_rule.read"); !ok {
		return
	}
	if !validAdministrativeID(w, r) || server.dependencies.IncidentRules == nil {
		if server.dependencies.IncidentRules == nil {
			server.unavailable(w, r, "Incident rule management is unavailable.")
		}
		return
	}
	result, err := server.dependencies.IncidentRules.Detail(r.Context(), r.PathValue("id"))
	if errors.Is(err, incidents.ErrRuleNotFound) {
		server.error(w, r, http.StatusNotFound, "not_found", "The requested incident rule was not found.")
		return
	}
	if err != nil {
		server.internalError(w, r, "incident rule read failed", err)
		return
	}
	w.Header().Set("ETag", incidentETag(result.Version))
	writeJSON(w, http.StatusOK, result)
}
func (server *server) createIncidentRule(w http.ResponseWriter, r *http.Request) {
	server.mutateIncidentRule(w, r, false)
}
func (server *server) replaceIncidentRule(w http.ResponseWriter, r *http.Request) {
	server.mutateIncidentRule(w, r, true)
}
func (server *server) mutateIncidentRule(w http.ResponseWriter, r *http.Request, replace bool) {
	session, ok := server.mutationSession(w, r, "incident_rules:manage", "incident_rule.write")
	if !ok {
		return
	}
	if server.dependencies.IncidentRules == nil {
		server.unavailable(w, r, "Incident rule management is unavailable.")
		return
	}
	id := ""
	version := int64(0)
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
	var body ruleWriteBody
	if err := decodeJSONLimit(w, r, &body, 32*1024); err != nil {
		server.decodeError(w, r, err)
		return
	}
	definition, enabled := body.input()
	if body.Enabled == nil {
		server.validationError(w, r, []APIFieldError{{Field: "enabled", Code: "required"}})
		return
	}
	input := incidents.RuleWrite{Definition: definition, Enabled: enabled, ExpectedVersion: version, IdempotencyKey: key, ActorUserID: session.User.ID, ActorName: session.User.DisplayName, SourceAddress: sourceAddress(r), CorrelationID: requestID(r)}
	var receipt adminops.Receipt
	var err error
	if replace {
		receipt, err = server.dependencies.IncidentRules.Replace(r.Context(), id, input)
	} else {
		receipt, err = server.dependencies.IncidentRules.Create(r.Context(), input)
	}
	if server.handleAdministrativeError(w, r, err, "incident rule") {
		return
	}
	completeReceipt(&receipt, session.User.Permissions)
	w.Header().Set("ETag", incidentETag(receipt.Version))
	if !replace {
		w.Header().Set("Location", "/api/v1/incident-rules/"+receipt.ID)
		writeJSON(w, http.StatusCreated, receipt)
	} else {
		writeJSON(w, http.StatusOK, receipt)
	}
}
func (server *server) previewIncidentRule(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.mutationSession(w, r, "incident_rules:manage", "incident_rule.preview"); !ok {
		return
	}
	if server.dependencies.IncidentRules == nil {
		server.unavailable(w, r, "Incident rule management is unavailable.")
		return
	}
	var body incidents.RulePreviewInput
	if err := decodeJSONLimit(w, r, &body, 8*1024); err != nil {
		server.decodeError(w, r, err)
		return
	}
	result, err := server.dependencies.IncidentRules.Preview(r.Context(), body)
	if server.handleAdministrativeError(w, r, err, "incident rule preview") {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (server *server) maintenanceWindowList(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.authorize(w, r, "suppressions:manage", "maintenance_windows.read"); !ok {
		return
	}
	if server.dependencies.Suppressions == nil {
		server.unavailable(w, r, "Suppression management is unavailable.")
		return
	}
	result, err := server.dependencies.Suppressions.MaintenanceWindows(r.Context())
	if err != nil {
		server.internalError(w, r, "maintenance window list failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (server *server) maintenanceWindowDetail(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.authorize(w, r, "suppressions:manage", "maintenance_window.read"); !ok {
		return
	}
	if server.dependencies.Suppressions == nil {
		server.unavailable(w, r, "Suppression management is unavailable.")
		return
	}
	if !validAdministrativeID(w, r) {
		return
	}
	result, err := server.dependencies.Suppressions.MaintenanceWindow(r.Context(), r.PathValue("id"))
	if server.handleAdministrativeError(w, r, err, "maintenance window") {
		return
	}
	w.Header().Set("ETag", incidentETag(result.Version))
	writeJSON(w, http.StatusOK, result)
}
func (server *server) createMaintenanceWindow(w http.ResponseWriter, r *http.Request) {
	server.mutateMaintenanceWindow(w, r, false)
}
func (server *server) replaceMaintenanceWindow(w http.ResponseWriter, r *http.Request) {
	server.mutateMaintenanceWindow(w, r, true)
}
func (server *server) mutateMaintenanceWindow(w http.ResponseWriter, r *http.Request, replace bool) {
	session, ok := server.mutationSession(w, r, "suppressions:manage", "maintenance_window.write")
	if !ok {
		return
	}
	if server.dependencies.Suppressions == nil {
		server.unavailable(w, r, "Suppression management is unavailable.")
		return
	}
	id := ""
	version := int64(0)
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
	var body suppressions.MaintenanceDefinition
	if err := decodeJSONLimit(w, r, &body, 16*1024); err != nil {
		server.decodeError(w, r, err)
		return
	}
	metadata := suppressionMetadata(session.User.ID, session.User.DisplayName, sourceAddress(r), requestID(r), key, version)
	var receipt adminops.Receipt
	var err error
	if replace {
		receipt, err = server.dependencies.Suppressions.ReplaceMaintenance(r.Context(), id, body, metadata)
	} else {
		receipt, err = server.dependencies.Suppressions.CreateMaintenance(r.Context(), body, metadata)
	}
	if server.handleAdministrativeError(w, r, err, "maintenance window") {
		return
	}
	completeReceipt(&receipt, session.User.Permissions)
	w.Header().Set("ETag", incidentETag(receipt.Version))
	if !replace {
		w.Header().Set("Location", "/api/v1/maintenance-windows/"+receipt.ID)
		writeJSON(w, http.StatusCreated, receipt)
	} else {
		writeJSON(w, http.StatusOK, receipt)
	}
}
func (server *server) revokeMaintenanceWindow(w http.ResponseWriter, r *http.Request) {
	server.revokeSuppression(w, r, true)
}

func (server *server) silenceList(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.authorize(w, r, "suppressions:manage", "silences.read"); !ok {
		return
	}
	if server.dependencies.Suppressions == nil {
		server.unavailable(w, r, "Suppression management is unavailable.")
		return
	}
	result, err := server.dependencies.Suppressions.Silences(r.Context())
	if err != nil {
		server.internalError(w, r, "silence list failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (server *server) silenceDetail(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.authorize(w, r, "suppressions:manage", "silence.read"); !ok {
		return
	}
	if server.dependencies.Suppressions == nil {
		server.unavailable(w, r, "Suppression management is unavailable.")
		return
	}
	if !validAdministrativeID(w, r) {
		return
	}
	result, err := server.dependencies.Suppressions.Silence(r.Context(), r.PathValue("id"))
	if server.handleAdministrativeError(w, r, err, "silence") {
		return
	}
	w.Header().Set("ETag", incidentETag(result.Version))
	writeJSON(w, http.StatusOK, result)
}
func (server *server) createSilence(w http.ResponseWriter, r *http.Request) {
	server.mutateSilence(w, r, false)
}
func (server *server) replaceSilence(w http.ResponseWriter, r *http.Request) {
	server.mutateSilence(w, r, true)
}
func (server *server) mutateSilence(w http.ResponseWriter, r *http.Request, replace bool) {
	session, ok := server.mutationSession(w, r, "suppressions:manage", "silence.write")
	if !ok {
		return
	}
	if server.dependencies.Suppressions == nil {
		server.unavailable(w, r, "Suppression management is unavailable.")
		return
	}
	id := ""
	version := int64(0)
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
	var body suppressions.SilenceDefinition
	if err := decodeJSONLimit(w, r, &body, 16*1024); err != nil {
		server.decodeError(w, r, err)
		return
	}
	metadata := suppressionMetadata(session.User.ID, session.User.DisplayName, sourceAddress(r), requestID(r), key, version)
	var receipt adminops.Receipt
	var err error
	if replace {
		receipt, err = server.dependencies.Suppressions.ReplaceSilence(r.Context(), id, body, metadata)
	} else {
		receipt, err = server.dependencies.Suppressions.CreateSilence(r.Context(), body, metadata)
	}
	if server.handleAdministrativeError(w, r, err, "silence") {
		return
	}
	completeReceipt(&receipt, session.User.Permissions)
	w.Header().Set("ETag", incidentETag(receipt.Version))
	if !replace {
		w.Header().Set("Location", "/api/v1/silences/"+receipt.ID)
		writeJSON(w, http.StatusCreated, receipt)
	} else {
		writeJSON(w, http.StatusOK, receipt)
	}
}
func (server *server) revokeSilence(w http.ResponseWriter, r *http.Request) {
	server.revokeSuppression(w, r, false)
}

func (server *server) revokeSuppression(w http.ResponseWriter, r *http.Request, maintenance bool) {
	permission, action := "suppressions:manage", "silence.revoke"
	if maintenance {
		action = "maintenance_window.revoke"
	}
	session, ok := server.mutationSession(w, r, permission, action)
	if !ok {
		return
	}
	if server.dependencies.Suppressions == nil {
		server.unavailable(w, r, "Suppression management is unavailable.")
		return
	}
	if !validAdministrativeID(w, r) {
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
	metadata := suppressionMetadata(session.User.ID, session.User.DisplayName, sourceAddress(r), requestID(r), key, version)
	var receipt adminops.Receipt
	var err error
	if maintenance {
		receipt, err = server.dependencies.Suppressions.RevokeMaintenance(r.Context(), r.PathValue("id"), metadata)
	} else {
		receipt, err = server.dependencies.Suppressions.RevokeSilence(r.Context(), r.PathValue("id"), metadata)
	}
	if server.handleAdministrativeError(w, r, err, "suppression") {
		return
	}
	completeReceipt(&receipt, session.User.Permissions)
	w.Header().Set("ETag", incidentETag(receipt.Version))
	writeJSON(w, http.StatusOK, receipt)
}

func validAdministrativeID(w http.ResponseWriter, r *http.Request) bool {
	if uuidPattern.MatchString(r.PathValue("id")) {
		return true
	}
	r.Context()
	writeJSON(w, http.StatusNotFound, map[string]any{"error": map[string]string{"code": "not_found", "message": "The requested record was not found.", "request_id": requestID(r)}})
	return false
}
func parseRequiredVersion(w http.ResponseWriter, r *http.Request) (int64, bool) {
	value := strings.TrimSpace(r.Header.Get("If-Match"))
	if value == "" {
		r.Context()
		writeJSON(w, http.StatusPreconditionRequired, map[string]any{"error": map[string]string{"code": "precondition_required", "message": "Fetch the current record before changing it.", "request_id": requestID(r)}})
		return 0, false
	}
	version, ok := parseIncidentETag(value)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "validation_failed", "message": "If-Match is invalid.", "request_id": requestID(r)}})
		return 0, false
	}
	return version, true
}
func requiredIdempotencyKey(w http.ResponseWriter, r *http.Request) (string, bool) {
	key := r.Header.Get("Idempotency-Key")
	if strings.TrimSpace(key) == "" {
		writeJSON(w, http.StatusPreconditionRequired, map[string]any{"error": map[string]string{"code": "idempotency_key_required", "message": "Idempotency-Key is required.", "request_id": requestID(r)}})
		return "", false
	}
	if len(key) > 128 || strings.TrimSpace(key) != key || strings.IndexFunc(key, unicode.IsControl) >= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "validation_failed", "message": "Idempotency-Key is invalid.", "request_id": requestID(r)}})
		return "", false
	}
	return key, true
}
func suppressionMetadata(actorID, actorName, source, correlation, key string, version int64) suppressions.MutationMetadata {
	return suppressions.MutationMetadata{ExpectedVersion: version, IdempotencyKey: key, ActorUserID: actorID, ActorName: actorName, SourceAddress: source, CorrelationID: correlation}
}
func completeReceipt(receipt *adminops.Receipt, permissions []string) {
	if hasPermission(permissions, "audit:read") {
		receipt.AuditURL = "/audit?correlation_id=" + url.QueryEscape(receipt.RequestID)
	}
}
func (server *server) handleAdministrativeError(w http.ResponseWriter, r *http.Request, err error, label string) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, incidents.ErrRuleNotFound), errors.Is(err, suppressions.ErrNotFound):
		server.error(w, r, http.StatusNotFound, "not_found", "The requested "+label+" was not found.")
	case errors.Is(err, incidents.ErrRuleConflict), errors.Is(err, suppressions.ErrConflict):
		server.error(w, r, http.StatusPreconditionFailed, "precondition_failed", "The "+label+" changed; fetch it and retry.")
	case errors.Is(err, adminops.ErrIdempotencyConflict):
		server.error(w, r, http.StatusConflict, "idempotency_conflict", "That Idempotency-Key was already used for a different request.")
	case errors.Is(err, incidents.ErrRuleScope), errors.Is(err, suppressions.ErrInvalid), strings.HasPrefix(err.Error(), "invalid incident rule"):
		server.validationError(w, r, []APIFieldError{{Field: "body", Code: "invalid"}})
	default:
		server.internalError(w, r, label+" operation failed", err)
	}
	return true
}
func (server *server) unavailable(w http.ResponseWriter, r *http.Request, message string) {
	server.error(w, r, http.StatusServiceUnavailable, "unavailable", message)
}
