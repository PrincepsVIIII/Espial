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

	"github.com/PrincepsVIIII/Espial/core/internal/auth"
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

type Dependencies struct {
	Logger        *slog.Logger
	Ready         Readiness
	Auth          AuthService
	PublicURL     *url.URL
	SecureCookies bool
}

// New creates the Phase 1 HTTP handler.
func New(dependencies Dependencies) http.Handler {
	server := &server{dependencies: dependencies}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health/live", healthLive)
	mux.HandleFunc("GET /api/v1/health/ready", healthReady(dependencies.Ready))
	mux.HandleFunc("GET /api/v1/auth/capabilities", server.capabilities)
	mux.HandleFunc("POST /api/v1/auth/local/login", server.login)
	mux.HandleFunc("GET /api/v1/auth/session", server.currentSession)
	mux.HandleFunc("POST /api/v1/auth/logout", server.logout)
	mux.HandleFunc("GET /api/v1/admin/ping", server.adminPing)
	return middleware(dependencies.Logger, mux)
}

type server struct{ dependencies Dependencies }

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
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message, "request_id": requestID(r)}})
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
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
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
