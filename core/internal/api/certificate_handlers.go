package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/PrincepsVIIII/Espial/core/internal/certificates"
	"github.com/PrincepsVIIII/Espial/core/internal/health"
)

func (server *server) certificateList(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.authorize(w, r, "webpages:read", "certificates.read"); !ok {
		return
	}
	if !server.requireCertificates(w, r) {
		return
	}
	filter, ok := server.certificateFilter(w, r)
	if !ok {
		return
	}
	result, err := server.dependencies.Certificates.Certificates(r.Context(), filter)
	if errors.Is(err, certificates.ErrInvalidCursor) {
		server.validationError(w, r, []APIFieldError{{Field: "cursor", Code: "invalid"}})
		return
	}
	if errors.Is(err, certificates.ErrInvalidFilter) {
		server.validationError(w, r, []APIFieldError{{Field: "query", Code: "invalid"}})
		return
	}
	if err != nil {
		server.internalError(w, r, "certificate list failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (server *server) certificateDetail(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.authorize(w, r, "webpages:read", "certificate.read"); !ok {
		return
	}
	if !server.requireCertificates(w, r) || !validAdministrativeID(w, r) {
		return
	}
	result, err := server.dependencies.Certificates.Certificate(r.Context(), r.PathValue("id"))
	if errors.Is(err, certificates.ErrNotFound) {
		server.error(w, r, http.StatusNotFound, "not_found", "The requested certificate was not found.")
		return
	}
	if err != nil {
		server.internalError(w, r, "certificate detail failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (server *server) certificateFilter(w http.ResponseWriter, r *http.Request) (certificates.Filter, bool) {
	if !server.validQuerySize(w, r) {
		return certificates.Filter{}, false
	}
	values := r.URL.Query()
	fields := rejectUnknownQuery(values, map[string]bool{"limit": true, "cursor": true, "state": true, "hostname_valid": true, "expiry_days": true})
	limit, limitFields := parseLimit(values)
	fields = append(fields, limitFields...)
	cursor, field := singleValue(values, "cursor", 2048)
	if field != nil {
		fields = append(fields, *field)
	}
	states := []health.State{}
	seen := map[string]bool{}
	for _, raw := range values["state"] {
		if seen[raw] || raw != "healthy" && raw != "warning" && raw != "critical" && raw != "unknown" {
			fields = append(fields, APIFieldError{Field: "state", Code: "invalid"})
			continue
		}
		seen[raw] = true
		states = append(states, health.State(raw))
	}
	var hostname *bool
	if raw, problem := singleValue(values, "hostname_valid", 5); problem != nil {
		fields = append(fields, *problem)
	} else if raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			fields = append(fields, APIFieldError{Field: "hostname_valid", Code: "invalid"})
		} else {
			hostname = &value
		}
	}
	var expiry *int
	if raw, problem := singleValue(values, "expiry_days", 4); problem != nil {
		fields = append(fields, *problem)
	} else if raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 || value > 3650 {
			fields = append(fields, APIFieldError{Field: "expiry_days", Code: "invalid"})
		} else {
			expiry = &value
		}
	}
	if len(fields) > 0 {
		server.validationError(w, r, fields)
		return certificates.Filter{}, false
	}
	return certificates.Filter{Limit: limit, Cursor: cursor, States: states, HostnameValid: hostname, ExpiryDays: expiry}, true
}

func (server *server) requireCertificates(w http.ResponseWriter, r *http.Request) bool {
	if server.dependencies.Certificates != nil {
		return true
	}
	server.unavailable(w, r, "Certificate monitoring is unavailable.")
	return false
}
