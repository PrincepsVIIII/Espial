package api

import (
	"errors"
	"net/http"

	"github.com/PrincepsVIIII/Espial/core/internal/adminops"
	"github.com/PrincepsVIIII/Espial/core/internal/webpages"
)

func (server *server) websiteMonitorList(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.authorize(w, r, "website_monitors:manage", "website_monitors.read"); !ok {
		return
	}
	if !server.requireWebsites(w, r) {
		return
	}
	filter, ok := server.websiteListFilter(w, r)
	if !ok {
		return
	}
	result, err := server.dependencies.Websites.Monitors(r.Context(), filter)
	if errors.Is(err, webpages.ErrInvalidCursor) {
		server.validationError(w, r, []APIFieldError{{Field: "cursor", Code: "invalid"}})
		return
	}
	if err != nil {
		server.internalError(w, r, "website monitor list failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (server *server) websiteMonitorDetail(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.authorize(w, r, "website_monitors:manage", "website_monitor.read"); !ok {
		return
	}
	if !server.requireWebsites(w, r) || !validAdministrativeID(w, r) {
		return
	}
	result, err := server.dependencies.Websites.Monitor(r.Context(), r.PathValue("id"))
	if server.handleWebsiteError(w, r, err) {
		return
	}
	w.Header().Set("ETag", incidentETag(result.Version))
	writeJSON(w, http.StatusOK, result)
}
func (server *server) createWebsiteMonitor(w http.ResponseWriter, r *http.Request) {
	server.mutateWebsiteMonitor(w, r, false)
}
func (server *server) replaceWebsiteMonitor(w http.ResponseWriter, r *http.Request) {
	server.mutateWebsiteMonitor(w, r, true)
}
func (server *server) mutateWebsiteMonitor(w http.ResponseWriter, r *http.Request, replace bool) {
	session, ok := server.mutationSession(w, r, "website_monitors:manage", "website_monitor.write")
	if !ok || !server.requireWebsites(w, r) {
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
		DisplayName      string                            `json:"display_name"`
		Enabled          *bool                             `json:"enabled"`
		URL              string                            `json:"url"`
		IntervalSeconds  int                               `json:"interval_seconds"`
		TimeoutMS        int                               `json:"timeout_ms"`
		WarningLatencyMS int                               `json:"warning_latency_ms"`
		AllowedStatuses  []int                             `json:"allowed_statuses"`
		ContentMatch     string                            `json:"content_match"`
		FollowRedirects  *bool                             `json:"follow_redirects"`
		MaxRedirects     int                               `json:"max_redirects"`
		SecretHeaders    []webpages.SecretHeaderDefinition `json:"secret_headers"`
	}
	if err := decodeJSONLimit(w, r, &body, 16*1024); err != nil {
		server.decodeError(w, r, err)
		return
	}
	if body.Enabled == nil || body.FollowRedirects == nil {
		server.validationError(w, r, []APIFieldError{{Field: "body", Code: "required_fields"}})
		return
	}
	definition := webpages.MonitorDefinition{DisplayName: body.DisplayName, Enabled: *body.Enabled, URL: body.URL, IntervalSeconds: body.IntervalSeconds, TimeoutMS: body.TimeoutMS, WarningLatencyMS: body.WarningLatencyMS, AllowedStatuses: body.AllowedStatuses, ContentMatch: body.ContentMatch, FollowRedirects: *body.FollowRedirects, MaxRedirects: body.MaxRedirects, SecretHeaders: body.SecretHeaders}
	metadata := websiteMetadata(session.User.ID, session.User.DisplayName, sourceAddress(r), requestID(r), key, version)
	var receipt adminops.Receipt
	var err error
	if replace {
		receipt, err = server.dependencies.Websites.Replace(r.Context(), id, definition, metadata)
	} else {
		receipt, err = server.dependencies.Websites.Create(r.Context(), definition, metadata)
	}
	if server.handleWebsiteError(w, r, err) {
		return
	}
	completeReceipt(&receipt, session.User.Permissions)
	w.Header().Set("ETag", incidentETag(receipt.Version))
	if replace {
		writeJSON(w, http.StatusOK, receipt)
		return
	}
	w.Header().Set("Location", "/api/v1/website-monitors/"+receipt.ID)
	writeJSON(w, http.StatusCreated, receipt)
}
func (server *server) checkWebsiteMonitor(w http.ResponseWriter, r *http.Request) {
	session, ok := server.mutationSession(w, r, "website_monitors:manage", "website_monitor.check")
	if !ok || !server.requireWebsites(w, r) || !validAdministrativeID(w, r) {
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
	receipt, err := server.dependencies.Websites.Check(r.Context(), r.PathValue("id"), websiteMetadata(session.User.ID, session.User.DisplayName, sourceAddress(r), requestID(r), key, version))
	if server.handleWebsiteError(w, r, err) {
		return
	}
	completeReceipt(&receipt, session.User.Permissions)
	w.Header().Set("ETag", incidentETag(receipt.Version))
	writeJSON(w, http.StatusAccepted, receipt)
}
func (server *server) webpageList(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.authorize(w, r, "webpages:read", "webpages.read"); !ok {
		return
	}
	if !server.requireWebsites(w, r) {
		return
	}
	filter, ok := server.websiteListFilter(w, r)
	if !ok {
		return
	}
	result, err := server.dependencies.Websites.Webpages(r.Context(), filter)
	if errors.Is(err, webpages.ErrInvalidCursor) {
		server.validationError(w, r, []APIFieldError{{Field: "cursor", Code: "invalid"}})
		return
	}
	if err != nil {
		server.internalError(w, r, "webpage list failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (server *server) websiteListFilter(w http.ResponseWriter, r *http.Request) (webpages.ListFilter, bool) {
	if !server.validQuerySize(w, r) {
		return webpages.ListFilter{}, false
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
		return webpages.ListFilter{}, false
	}
	return webpages.ListFilter{Limit: limit, Cursor: cursor}, true
}
func (server *server) webpageDetail(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.authorize(w, r, "webpages:read", "webpage.read"); !ok {
		return
	}
	if !server.requireWebsites(w, r) || !validAdministrativeID(w, r) {
		return
	}
	result, err := server.dependencies.Websites.Webpage(r.Context(), r.PathValue("id"))
	if server.handleWebsiteError(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func websiteMetadata(actorID, actorName, source, correlation, key string, version int64) webpages.MutationMetadata {
	return webpages.MutationMetadata{ExpectedVersion: version, IdempotencyKey: key, ActorUserID: actorID, ActorName: actorName, SourceAddress: source, CorrelationID: correlation}
}
func (server *server) requireWebsites(w http.ResponseWriter, r *http.Request) bool {
	if server.dependencies.Websites != nil {
		return true
	}
	server.unavailable(w, r, "Website monitoring is unavailable.")
	return false
}
func (server *server) handleWebsiteError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, webpages.ErrNotFound):
		server.error(w, r, http.StatusNotFound, "not_found", "The requested website record was not found.")
	case errors.Is(err, webpages.ErrConflict):
		server.error(w, r, http.StatusPreconditionFailed, "precondition_failed", "The website monitor changed; fetch it and retry.")
	case errors.Is(err, webpages.ErrIdempotencyConflict):
		server.error(w, r, http.StatusConflict, "idempotency_conflict", "That Idempotency-Key was already used for a different request.")
	case errors.Is(err, webpages.ErrNotRunning):
		server.error(w, r, http.StatusConflict, "monitor_not_running", "The website monitor is disabled or has not started.")
	case errors.Is(err, webpages.ErrInvalid):
		server.validationError(w, r, []APIFieldError{{Field: "body", Code: "invalid"}})
	default:
		server.internalError(w, r, "website operation failed", err)
	}
	return true
}
