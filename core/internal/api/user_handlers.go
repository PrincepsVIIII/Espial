package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/auth"
)

func (server *server) managedUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.authorize(w, r, "users:manage", "users.read"); !ok {
		return
	}
	if !server.validQuerySize(w, r) || !server.requireUserAdministration(w, r) {
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
	result, err := server.dependencies.Users.ManagedUsers(r.Context(), auth.ManagedUserFilter{Limit: limit, Cursor: cursor})
	if errors.Is(err, auth.ErrInvalidCursor) {
		server.validationError(w, r, []APIFieldError{{Field: "cursor", Code: "invalid"}})
		return
	}
	if err != nil {
		server.internalError(w, r, "managed user read failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (server *server) managedRoles(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.authorize(w, r, "users:manage", "roles.read"); !ok {
		return
	}
	if !server.requireUserAdministration(w, r) {
		return
	}
	roles, err := server.dependencies.Users.ManagedRoles(r.Context())
	if err != nil {
		server.internalError(w, r, "managed role read failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": roles})
}

func (server *server) createManagedUser(w http.ResponseWriter, r *http.Request) {
	session, ok := server.mutationSession(w, r, "users:manage", "user.create")
	if !ok || !server.requireUserAdministration(w, r) {
		return
	}
	var body struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
		Role        string `json:"role"`
		Password    string `json:"password"`
	}
	if err := decodeJSONLimit(w, r, &body, 16*1024); err != nil {
		server.error(w, r, http.StatusBadRequest, "invalid_request", "Expected complete local user details.")
		return
	}
	user, err := server.dependencies.Users.CreateManagedUser(r.Context(), auth.CreateManagedUser{
		Username: body.Username, DisplayName: body.DisplayName, Email: body.Email,
		Role: body.Role, Password: body.Password,
		Context: administrationContext(session, r),
	})
	if server.handleManagedUserError(w, r, err) {
		return
	}
	w.Header().Set("Location", "/api/v1/users/"+user.ID)
	w.Header().Set("ETag", managedUserETag(user.UpdatedAt))
	writeJSON(w, http.StatusCreated, user)
}

func (server *server) updateManagedUser(w http.ResponseWriter, r *http.Request) {
	session, ok := server.mutationSession(w, r, "users:manage", "user.update")
	if !ok || !server.requireUserAdministration(w, r) {
		return
	}
	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		server.validationError(w, r, []APIFieldError{{Field: "id", Code: "invalid"}})
		return
	}
	expected, ok := parseManagedUserETag(r.Header.Get("If-Match"))
	if !ok {
		server.error(w, r, http.StatusPreconditionRequired, "precondition_required", "Fetch the current user before updating it.")
		return
	}
	var body struct {
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
		Role        string `json:"role"`
		Enabled     *bool  `json:"enabled"`
	}
	if err := decodeJSONLimit(w, r, &body, 16*1024); err != nil || body.Enabled == nil {
		server.error(w, r, http.StatusBadRequest, "invalid_request", "Expected display name, email, role, and enabled state.")
		return
	}
	user, err := server.dependencies.Users.UpdateManagedUser(r.Context(), auth.UpdateManagedUser{
		ID: id, DisplayName: body.DisplayName, Email: body.Email, Role: body.Role,
		Enabled: *body.Enabled, ExpectedUpdatedAt: expected,
		Context: administrationContext(session, r),
	})
	if server.handleManagedUserError(w, r, err) {
		return
	}
	w.Header().Set("ETag", managedUserETag(user.UpdatedAt))
	writeJSON(w, http.StatusOK, user)
}

func (server *server) resetManagedUserPassword(w http.ResponseWriter, r *http.Request) {
	session, ok := server.mutationSession(w, r, "users:manage", "user.password.reset")
	if !ok || !server.requireUserAdministration(w, r) {
		return
	}
	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		server.validationError(w, r, []APIFieldError{{Field: "id", Code: "invalid"}})
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := decodeJSONLimit(w, r, &body, 16*1024); err != nil {
		server.error(w, r, http.StatusBadRequest, "invalid_request", "Expected a replacement password.")
		return
	}
	err := server.dependencies.Users.ResetManagedUserPassword(r.Context(), auth.ResetManagedPassword{
		ID: id, Password: body.Password, Context: administrationContext(session, r),
	})
	if server.handleManagedUserError(w, r, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (server *server) handleManagedUserError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, auth.ErrUserNotFound):
		server.error(w, r, http.StatusNotFound, "not_found", "The requested user was not found.")
	case errors.Is(err, auth.ErrUsernameTaken):
		server.error(w, r, http.StatusConflict, "username_taken", "That username is already in use.")
	case errors.Is(err, auth.ErrUserChanged):
		server.error(w, r, http.StatusPreconditionFailed, "precondition_failed", "The user changed; refresh and retry.")
	case errors.Is(err, auth.ErrSelfLockout):
		server.error(w, r, http.StatusConflict, "self_lockout", "You cannot disable or remove your own administrator access.")
	case errors.Is(err, auth.ErrLastAdministrator):
		server.error(w, r, http.StatusConflict, "last_administrator", "At least one enabled administrator must remain.")
	case errors.Is(err, auth.ErrRoleNotFound):
		server.validationError(w, r, []APIFieldError{{Field: "role", Code: "invalid"}})
	default:
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "password") || strings.Contains(message, "display name") || strings.Contains(message, "email") || strings.Contains(message, "username") {
			server.validationError(w, r, []APIFieldError{{Field: "user", Code: "invalid"}})
		} else {
			server.internalError(w, r, "managed user mutation failed", err)
		}
	}
	return true
}

func (server *server) requireUserAdministration(w http.ResponseWriter, r *http.Request) bool {
	if server.dependencies.Users != nil {
		return true
	}
	server.error(w, r, http.StatusServiceUnavailable, "unavailable", "User administration is unavailable.")
	return false
}

func administrationContext(session auth.Session, r *http.Request) auth.AdministrationContext {
	return auth.AdministrationContext{
		ActorUserID: session.User.ID, SourceAddress: sourceAddress(r), CorrelationID: requestID(r),
	}
}

func managedUserETag(updatedAt time.Time) string {
	return `"` + updatedAt.UTC().Format(time.RFC3339Nano) + `"`
}

func parseManagedUserETag(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if len(value) < 3 || value[0] != '"' || value[len(value)-1] != '"' {
		return time.Time{}, false
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, value[1:len(value)-1])
	if err != nil {
		return time.Time{}, false
	}
	return updatedAt.UTC(), true
}
