// Package api exposes Espial Core's versioned HTTP API.
package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/adminops"
	"github.com/PrincepsVIIII/Espial/core/internal/auth"
	"github.com/PrincepsVIIII/Espial/core/internal/certificates"
	"github.com/PrincepsVIIII/Espial/core/internal/events"
	"github.com/PrincepsVIIII/Espial/core/internal/incidents"
	"github.com/PrincepsVIIII/Espial/core/internal/monitoring"
	"github.com/PrincepsVIIII/Espial/core/internal/notifications"
	"github.com/PrincepsVIIII/Espial/core/internal/suppressions"
	"github.com/PrincepsVIIII/Espial/core/internal/webpages"
)

const (
	sessionCookie = "espial_session"
	csrfCookie    = "espial_csrf"
)

type Readiness func(context.Context) error

type AuthService interface {
	Login(context.Context, string, string, string, string) (auth.Session, error)
	Authenticate(context.Context, string) (auth.Session, error)
	VerifyCSRF(auth.Session, string) bool
	Logout(context.Context, auth.Session, string, string) error
	RecordDenied(context.Context, auth.User, string, string, string) error
}

type MonitoringReader interface {
	Overview(context.Context) (monitoring.Overview, error)
	Resources(context.Context, monitoring.ResourceFilter) (monitoring.ResourceList, error)
	Resource(context.Context, string) (monitoring.ResourceView, error)
	Integrations(context.Context, monitoring.IntegrationFilter) (monitoring.IntegrationList, error)
	Integration(context.Context, string) (monitoring.IntegrationView, error)
	Audit(context.Context, monitoring.AuditFilter) (monitoring.AuditList, error)
	RecordAuditRead(context.Context, string, string, string, monitoring.AuditFilter) error
}

type IntegrationManager interface {
	Create(context.Context, monitoring.CreateIntegration) (string, time.Time, error)
	Update(context.Context, monitoring.IntegrationConfigUpdate) (time.Time, error)
}

type IncidentReader interface {
	Incidents(context.Context, incidents.Filter) (incidents.List, error)
	Incident(context.Context, string) (incidents.Detail, error)
	Timeline(context.Context, string, incidents.TimelineFilter) (incidents.Timeline, error)
}

type IncidentWorkflow interface {
	Assignees(context.Context, int, string) (incidents.AssigneeList, error)
	Mutate(context.Context, incidents.Mutation) (incidents.MutationResult, error)
}

type IncidentRuleManager interface {
	List(context.Context) (incidents.RuleList, error)
	Detail(context.Context, string) (incidents.RuleView, error)
	Create(context.Context, incidents.RuleWrite) (adminops.Receipt, error)
	Replace(context.Context, string, incidents.RuleWrite) (adminops.Receipt, error)
	Preview(context.Context, incidents.RulePreviewInput) (incidents.RulePreview, error)
}

type SuppressionManager interface {
	MaintenanceWindows(context.Context) (suppressions.MaintenanceList, error)
	MaintenanceWindow(context.Context, string) (suppressions.MaintenanceWindow, error)
	CreateMaintenance(context.Context, suppressions.MaintenanceDefinition, suppressions.MutationMetadata) (adminops.Receipt, error)
	ReplaceMaintenance(context.Context, string, suppressions.MaintenanceDefinition, suppressions.MutationMetadata) (adminops.Receipt, error)
	RevokeMaintenance(context.Context, string, suppressions.MutationMetadata) (adminops.Receipt, error)
	Silences(context.Context) (suppressions.SilenceList, error)
	Silence(context.Context, string) (suppressions.Silence, error)
	CreateSilence(context.Context, suppressions.SilenceDefinition, suppressions.MutationMetadata) (adminops.Receipt, error)
	ReplaceSilence(context.Context, string, suppressions.SilenceDefinition, suppressions.MutationMetadata) (adminops.Receipt, error)
	RevokeSilence(context.Context, string, suppressions.MutationMetadata) (adminops.Receipt, error)
}

type NotificationManager interface {
	Destinations(context.Context, notifications.DestinationFilter) (notifications.DestinationList, error)
	Destination(context.Context, string) (notifications.Destination, error)
	CreateDestination(context.Context, notifications.DestinationDefinition, notifications.MutationMetadata) (adminops.Receipt, error)
	ReplaceDestination(context.Context, string, notifications.DestinationDefinition, notifications.MutationMetadata) (adminops.Receipt, error)
	TestDestination(context.Context, string, notifications.MutationMetadata) (adminops.Receipt, error)
	Deliveries(context.Context, notifications.DeliveryFilter) (notifications.DeliveryList, error)
}

type WebsiteManager interface {
	Monitors(context.Context, webpages.ListFilter) (webpages.MonitorList, error)
	Monitor(context.Context, string) (webpages.Monitor, error)
	Create(context.Context, webpages.MonitorDefinition, webpages.MutationMetadata) (adminops.Receipt, error)
	Replace(context.Context, string, webpages.MonitorDefinition, webpages.MutationMetadata) (adminops.Receipt, error)
	Check(context.Context, string, webpages.MutationMetadata) (adminops.Receipt, error)
	Webpages(context.Context, webpages.ListFilter) (webpages.List, error)
	Webpage(context.Context, string) (webpages.Detail, error)
}

type CertificateReader interface {
	Certificates(context.Context, certificates.Filter) (certificates.List, error)
	Certificate(context.Context, string) (certificates.Detail, error)
}

type UserAdministrator interface {
	ManagedUsers(context.Context, auth.ManagedUserFilter) (auth.ManagedUserList, error)
	ManagedRoles(context.Context) ([]auth.RoleView, error)
	CreateManagedUser(context.Context, auth.CreateManagedUser) (auth.ManagedUser, error)
	UpdateManagedUser(context.Context, auth.UpdateManagedUser) (auth.ManagedUser, error)
	ResetManagedUserPassword(context.Context, auth.ResetManagedPassword) error
}

type EventSource interface {
	Subscribe(*uint64, int) *events.Subscription
}

type Dependencies struct {
	Logger           *slog.Logger
	Ready            Readiness
	Auth             AuthService
	PublicURL        *url.URL
	SecureCookies    bool
	Monitoring       MonitoringReader
	Incidents        IncidentReader
	IncidentWorkflow IncidentWorkflow
	IncidentRules    IncidentRuleManager
	Suppressions     SuppressionManager
	Notifications    NotificationManager
	Websites         WebsiteManager
	Certificates     CertificateReader
	Integrations     IntegrationManager
	Users            UserAdministrator
	Events           EventSource
	SSEHeartbeat     time.Duration
	SSEMaxClients    int
	Now              func() time.Time
}

// New creates the Phase 1 HTTP handler.
func New(dependencies Dependencies) http.Handler {
	if dependencies.Logger == nil {
		dependencies.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if dependencies.SSEHeartbeat <= 0 {
		dependencies.SSEHeartbeat = 15 * time.Second
	}
	if dependencies.SSEMaxClients <= 0 {
		dependencies.SSEMaxClients = 100
	}
	if dependencies.Now == nil {
		dependencies.Now = func() time.Time { return time.Now().UTC() }
	}
	server := &server{dependencies: dependencies, sseSlots: make(chan struct{}, dependencies.SSEMaxClients)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health/live", healthLive)
	mux.HandleFunc("GET /api/v1/health/ready", healthReady(dependencies.Ready))
	mux.HandleFunc("GET /api/v1/auth/capabilities", server.capabilities)
	mux.HandleFunc("POST /api/v1/auth/local/login", server.login)
	mux.HandleFunc("GET /api/v1/auth/session", server.currentSession)
	mux.HandleFunc("POST /api/v1/auth/logout", server.logout)
	mux.HandleFunc("GET /api/v1/admin/ping", server.adminPing)
	mux.HandleFunc("GET /api/v1/overview", server.overview)
	mux.HandleFunc("GET /api/v1/resources", server.resources)
	mux.HandleFunc("GET /api/v1/resources/{id}", server.resource)
	mux.HandleFunc("GET /api/v1/integrations", server.integrations)
	mux.HandleFunc("POST /api/v1/integrations", server.createIntegration)
	mux.HandleFunc("GET /api/v1/integrations/{id}", server.integration)
	mux.HandleFunc("PUT /api/v1/integrations/{id}/configuration", server.updateIntegration)
	mux.HandleFunc("GET /api/v1/audit", server.auditEvents)
	mux.HandleFunc("GET /api/v1/incidents", server.incidentList)
	mux.HandleFunc("GET /api/v1/incidents/{id}", server.incidentDetail)
	mux.HandleFunc("GET /api/v1/incidents/{id}/timeline", server.incidentTimeline)
	mux.HandleFunc("POST /api/v1/incidents/{id}/acknowledge", server.acknowledgeIncident)
	mux.HandleFunc("POST /api/v1/incidents/{id}/investigate", server.investigateIncident)
	mux.HandleFunc("PUT /api/v1/incidents/{id}/owner", server.assignIncident)
	mux.HandleFunc("POST /api/v1/incidents/{id}/notes", server.addIncidentNote)
	mux.HandleFunc("POST /api/v1/incidents/{id}/resolve", server.resolveIncident)
	mux.HandleFunc("GET /api/v1/incident-assignees", server.incidentAssignees)
	mux.HandleFunc("GET /api/v1/incident-rules", server.incidentRuleList)
	mux.HandleFunc("POST /api/v1/incident-rules", server.createIncidentRule)
	mux.HandleFunc("POST /api/v1/incident-rules/preview", server.previewIncidentRule)
	mux.HandleFunc("GET /api/v1/incident-rules/{id}", server.incidentRuleDetail)
	mux.HandleFunc("PUT /api/v1/incident-rules/{id}", server.replaceIncidentRule)
	mux.HandleFunc("GET /api/v1/maintenance-windows", server.maintenanceWindowList)
	mux.HandleFunc("POST /api/v1/maintenance-windows", server.createMaintenanceWindow)
	mux.HandleFunc("GET /api/v1/maintenance-windows/{id}", server.maintenanceWindowDetail)
	mux.HandleFunc("PUT /api/v1/maintenance-windows/{id}", server.replaceMaintenanceWindow)
	mux.HandleFunc("POST /api/v1/maintenance-windows/{id}/revoke", server.revokeMaintenanceWindow)
	mux.HandleFunc("GET /api/v1/silences", server.silenceList)
	mux.HandleFunc("POST /api/v1/silences", server.createSilence)
	mux.HandleFunc("GET /api/v1/silences/{id}", server.silenceDetail)
	mux.HandleFunc("PUT /api/v1/silences/{id}", server.replaceSilence)
	mux.HandleFunc("POST /api/v1/silences/{id}/revoke", server.revokeSilence)
	mux.HandleFunc("GET /api/v1/notification-destinations", server.notificationDestinationList)
	mux.HandleFunc("POST /api/v1/notification-destinations", server.createNotificationDestination)
	mux.HandleFunc("GET /api/v1/notification-destinations/{id}", server.notificationDestinationDetail)
	mux.HandleFunc("PUT /api/v1/notification-destinations/{id}", server.replaceNotificationDestination)
	mux.HandleFunc("POST /api/v1/notification-destinations/{id}/test", server.testNotificationDestination)
	mux.HandleFunc("GET /api/v1/notification-deliveries", server.notificationDeliveryList)
	mux.HandleFunc("GET /api/v1/incidents/{id}/deliveries", server.incidentDeliveryList)
	mux.HandleFunc("GET /api/v1/website-monitors", server.websiteMonitorList)
	mux.HandleFunc("POST /api/v1/website-monitors", server.createWebsiteMonitor)
	mux.HandleFunc("GET /api/v1/website-monitors/{id}", server.websiteMonitorDetail)
	mux.HandleFunc("PUT /api/v1/website-monitors/{id}", server.replaceWebsiteMonitor)
	mux.HandleFunc("POST /api/v1/website-monitors/{id}/check", server.checkWebsiteMonitor)
	mux.HandleFunc("GET /api/v1/webpages", server.webpageList)
	mux.HandleFunc("GET /api/v1/webpages/{id}", server.webpageDetail)
	mux.HandleFunc("GET /api/v1/certificates", server.certificateList)
	mux.HandleFunc("GET /api/v1/certificates/{id}", server.certificateDetail)
	mux.HandleFunc("GET /api/v1/roles", server.managedRoles)
	mux.HandleFunc("GET /api/v1/users", server.managedUsers)
	mux.HandleFunc("POST /api/v1/users", server.createManagedUser)
	mux.HandleFunc("PUT /api/v1/users/{id}", server.updateManagedUser)
	mux.HandleFunc("POST /api/v1/users/{id}/password", server.resetManagedUserPassword)
	mux.HandleFunc("GET /api/v1/events/stream", server.eventStream)
	return middleware(dependencies.Logger, mux)
}

type server struct {
	dependencies Dependencies
	sseSlots     chan struct{}
}

func healthLive(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func healthReady(ready Readiness) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := ready(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func (server *server) capabilities(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"local": true, "sso": false})
}

func (server *server) login(w http.ResponseWriter, r *http.Request) {
	if !server.trustedOrigin(r) {
		server.error(w, r, http.StatusForbidden, "origin_rejected", "Request origin was not accepted.")
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		server.error(w, r, http.StatusBadRequest, "invalid_request", "Expected a username and password.")
		return
	}
	session, err := server.dependencies.Auth.Login(r.Context(), body.Username, body.Password, sourceAddress(r), requestID(r))
	if errors.Is(err, auth.ErrRateLimited) {
		w.Header().Set("Retry-After", "60")
		server.error(w, r, http.StatusTooManyRequests, "rate_limited", "Too many sign-in attempts. Try again later.")
		return
	}
	if errors.Is(err, auth.ErrInvalidCredentials) {
		server.error(w, r, http.StatusUnauthorized, "invalid_credentials", "The username or password was not accepted.")
		return
	}
	if err != nil {
		server.dependencies.Logger.ErrorContext(r.Context(), "local login failed", "request_id", requestID(r), "error", err)
		server.error(w, r, http.StatusInternalServerError, "internal_error", "The request could not be completed.")
		return
	}
	server.setCookies(w, session)
	writeJSON(w, http.StatusOK, sessionResponse(session))
}

func (server *server) currentSession(w http.ResponseWriter, r *http.Request) {
	session, ok := server.session(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, sessionResponse(session))
}

func (server *server) logout(w http.ResponseWriter, r *http.Request) {
	if !server.trustedOrigin(r) {
		server.error(w, r, http.StatusForbidden, "origin_rejected", "Request origin was not accepted.")
		return
	}
	session, ok := server.session(w, r)
	if !ok {
		return
	}
	csrf, err := r.Cookie(csrfCookie)
	if err != nil || csrf.Value != r.Header.Get("X-CSRF-Token") || !server.dependencies.Auth.VerifyCSRF(session, csrf.Value) {
		server.error(w, r, http.StatusForbidden, "csrf_rejected", "CSRF validation failed.")
		return
	}
	if err := server.dependencies.Auth.Logout(r.Context(), session, sourceAddress(r), requestID(r)); err != nil {
		server.error(w, r, http.StatusInternalServerError, "internal_error", "The request could not be completed.")
		return
	}
	server.clearCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

func (server *server) adminPing(w http.ResponseWriter, r *http.Request) {
	session, ok := server.session(w, r)
	if !ok {
		return
	}
	if !hasPermission(session.User.Permissions, "audit:read") {
		_ = server.dependencies.Auth.RecordDenied(r.Context(), session.User, "admin.ping", sourceAddress(r), requestID(r))
		server.error(w, r, http.StatusForbidden, "forbidden", "You do not have permission to perform this action.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (server *server) session(w http.ResponseWriter, r *http.Request) (auth.Session, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		server.unauthorized(w, r)
		return auth.Session{}, false
	}
	session, err := server.dependencies.Auth.Authenticate(r.Context(), cookie.Value)
	if err != nil {
		server.unauthorized(w, r)
		return auth.Session{}, false
	}
	return session, true
}

func (server *server) unauthorized(w http.ResponseWriter, r *http.Request) {
	server.clearCookies(w)
	server.error(w, r, http.StatusUnauthorized, "unauthenticated", "Sign in is required.")
}

func (server *server) trustedOrigin(r *http.Request) bool {
	if server.dependencies.PublicURL == nil {
		return false
	}
	expected := server.dependencies.PublicURL.Scheme + "://" + server.dependencies.PublicURL.Host
	return r.Header.Get("Origin") == expected
}

func (server *server) setCookies(w http.ResponseWriter, session auth.Session) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: session.Token, Path: "/", HttpOnly: true, Secure: server.dependencies.SecureCookies, SameSite: http.SameSiteLaxMode, Expires: session.AbsoluteExpiry})
	http.SetCookie(w, &http.Cookie{Name: csrfCookie, Value: session.CSRFToken, Path: "/", HttpOnly: false, Secure: server.dependencies.SecureCookies, SameSite: http.SameSiteLaxMode, Expires: session.AbsoluteExpiry})
}

func (server *server) clearCookies(w http.ResponseWriter) {
	for _, name := range []string{sessionCookie, csrfCookie} {
		http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", HttpOnly: name == sessionCookie, Secure: server.dependencies.SecureCookies, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	}
}

func sessionResponse(session auth.Session) any {
	return struct {
		User         auth.User       `json:"user"`
		ExpiresAt    time.Time       `json:"expires_at"`
		Capabilities map[string]bool `json:"capabilities"`
	}{session.User, session.ExpiresAt.UTC(), map[string]bool{"local": true, "sso": false}}
}

func (server *server) error(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	server.errorFields(w, r, status, code, message, nil)
}

type APIFieldError struct {
	Field string `json:"field"`
	Code  string `json:"code"`
}

type apiErrorBody struct {
	Error struct {
		Code      string          `json:"code"`
		Message   string          `json:"message"`
		RequestID string          `json:"request_id"`
		Fields    []APIFieldError `json:"fields,omitempty"`
	} `json:"error"`
}

func (server *server) errorFields(w http.ResponseWriter, r *http.Request, status int, code, message string, fields []APIFieldError) {
	var body apiErrorBody
	body.Error.Code, body.Error.Message, body.Error.RequestID = code, message, requestID(r)
	body.Error.Fields = fields
	writeJSON(w, status, body)
}

type contextKey string

const requestIDKey contextKey = "request_id"

func middleware(logger *slog.Logger, next http.Handler) http.Handler {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if id == "" || len(id) > 128 {
			id = newRequestID()
		}
		r = r.WithContext(context.WithValue(r.Context(), requestIDKey, id))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Request-ID", id)
		started := time.Now()
		next.ServeHTTP(w, r)
		logger.InfoContext(r.Context(), "http request", "method", r.Method, "path", r.URL.Path, "request_id", id, "duration_ms", time.Since(started).Milliseconds())
	})
}

func requestID(r *http.Request) string {
	value, _ := r.Context().Value(requestIDKey).(string)
	return value
}

func sourceAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	if net.ParseIP(r.RemoteAddr) != nil {
		return r.RemoteAddr
	}
	return ""
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	return decodeJSONLimit(w, r, destination, 4096)
}

func decodeJSONLimit(w http.ResponseWriter, r *http.Request, destination any, limit int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("expected one JSON object")
	}
	return nil
}

func hasPermission(permissions []string, expected string) bool {
	for _, permission := range permissions {
		if permission == expected {
			return true
		}
	}
	return false
}

func newRequestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "unavailable"
	}
	return hex.EncodeToString(value[:])
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
