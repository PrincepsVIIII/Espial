package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestBootstrapIsOneTimeAndConcurrent(t *testing.T) {
	pool := authTestPool(t)
	service := testService(t, pool, nil)
	ctx := context.Background()
	errorsSeen := make(chan error, 2)
	var wait sync.WaitGroup
	for _, username := range []string{"first-admin", "second-admin"} {
		wait.Add(1)
		go func(name string) {
			defer wait.Done()
			_, err := service.BootstrapAdmin(ctx, name, "A valid password phrase 90210", "test", "127.0.0.1")
			errorsSeen <- err
		}(username)
	}
	wait.Wait()
	close(errorsSeen)
	succeeded, rejected := 0, 0
	for err := range errorsSeen {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrAlreadyBootstrapped):
			rejected++
		default:
			t.Fatalf("bootstrap: %v", err)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("succeeded=%d rejected=%d", succeeded, rejected)
	}
}

func TestLoginLockoutSessionExpiryRoleRefreshAndRevocation(t *testing.T) {
	pool := authTestPool(t)
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	service := testService(t, pool, func() time.Time { return now })
	ctx := context.Background()
	user, err := service.BootstrapAdmin(ctx, "local-admin", "A valid password phrase 90210", "bootstrap", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}

	for range 5 {
		if _, err := service.Login(ctx, user.Username, "A wrong password phrase 123", "10.0.0.1", "failed"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("failed login: %v", err)
		}
	}
	if _, err := service.Login(ctx, user.Username, "A valid password phrase 90210", "10.0.0.2", "locked"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("locked login: %v", err)
	}

	now = now.Add(16 * time.Minute)
	session, err := service.Login(ctx, user.Username, "A valid password phrase 90210", "10.0.0.2", "success")
	if err != nil {
		t.Fatalf("login after lockout: %v", err)
	}
	if !service.VerifyCSRF(session, session.CSRFToken) {
		t.Fatal("generated CSRF token did not verify")
	}
	var tokenStored bool
	if err := pool.QueryRow(ctx, "SELECT token_digest <> convert_to($2, 'UTF8') FROM sessions WHERE id = $1", session.ID, session.Token).Scan(&tokenStored); err != nil || !tokenStored {
		t.Fatalf("session token was not stored as a digest: %v", err)
	}

	if _, err := service.Authenticate(ctx, session.Token); err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	oldToken := session.Token
	session, err = service.RotateSession(ctx, session, "127.0.0.1", "rotate")
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if _, err := service.Authenticate(ctx, oldToken); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("old rotated token: %v", err)
	}
	if _, err := service.Authenticate(ctx, session.Token); err != nil {
		t.Fatalf("rotated token: %v", err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM user_roles WHERE user_id = $1", user.ID); err != nil {
		t.Fatalf("remove role: %v", err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO user_roles (user_id, role_id) SELECT $1, id FROM roles WHERE name = 'viewer'", user.ID); err != nil {
		t.Fatalf("change role: %v", err)
	}
	refreshed, err := service.Authenticate(ctx, session.Token)
	if err != nil || len(refreshed.User.Roles) != 1 || refreshed.User.Roles[0] != "viewer" || refreshed.User.HasPermission("audit:read") {
		t.Fatalf("roles were not refreshed: %#v, %v", refreshed.User, err)
	}
	if err := service.RevokeUserSessions(ctx, user.ID, user.ID, "127.0.0.1", "revoke"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(ctx, session.Token); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("revoked session: %v", err)
	}

	newSession, err := service.Login(ctx, user.Username, "A valid password phrase 90210", "10.0.0.3", "success-2")
	if err != nil {
		t.Fatal(err)
	}
	now = newSession.ExpiresAt.Add(time.Second)
	if _, err := service.Authenticate(ctx, newSession.Token); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("idle-expired session: %v", err)
	}

	now = time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	disabledSession, err := service.Login(ctx, user.Username, "A valid password phrase 90210", "10.0.0.4", "disabled-test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE users SET enabled = false WHERE id = $1", user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(ctx, disabledSession.Token); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("disabled user session: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE users SET enabled = true WHERE id = $1", user.ID); err != nil {
		t.Fatal(err)
	}
	absoluteSession, err := service.Login(ctx, user.Username, "A valid password phrase 90210", "10.0.0.5", "absolute-test")
	if err != nil {
		t.Fatal(err)
	}
	now = absoluteSession.AbsoluteExpiry.Add(time.Second)
	if _, err := service.Authenticate(ctx, absoluteSession.Token); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("absolute-expired session: %v", err)
	}

	var localUsed int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE action = 'auth.local.used' AND result = 'succeeded'").Scan(&localUsed); err != nil || localUsed != 4 {
		t.Fatalf("local auth audit count=%d error=%v", localUsed, err)
	}
}

func TestLocalUserAdministrationIsAuditedAndRevokesSessions(t *testing.T) {
	pool := authTestPool(t)
	service := testService(t, pool, nil)
	ctx := context.Background()
	user, err := service.CreateLocalUser(ctx, "phase-viewer", "A viewer password phrase 90210", "viewer", "create-user")
	if err != nil {
		t.Fatal(err)
	}
	if len(user.Roles) != 1 || user.Roles[0] != "viewer" || user.HasPermission("audit:read") {
		t.Fatalf("created user = %#v", user)
	}
	if _, err := service.CreateLocalUser(ctx, "phase-viewer", "Another viewer password 90210", "viewer", "duplicate"); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("duplicate user error = %v", err)
	}
	if _, err := service.CreateLocalUser(ctx, "missing-role", "A missing role password 90210", "owner", "missing-role"); !errors.Is(err, ErrRoleNotFound) {
		t.Fatalf("missing role error = %v", err)
	}

	session, err := service.Login(ctx, user.Username, "A viewer password phrase 90210", "127.0.0.1", "viewer-login")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.AssignRole(ctx, user.Username, "operator", "assign-role"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(ctx, session.Token); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("role assignment did not revoke session: %v", err)
	}

	if err := service.ResetLocalPassword(ctx, user.Username, "A replacement password phrase 90210", "reset-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Login(ctx, user.Username, "A viewer password phrase 90210", "127.0.0.1", "old-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password login = %v", err)
	}
	if err := service.SetLocalUserEnabled(ctx, user.Username, false, "disable-user"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Login(ctx, user.Username, "A replacement password phrase 90210", "127.0.0.1", "disabled-login"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("disabled login = %v", err)
	}
	if err := service.SetLocalUserEnabled(ctx, user.Username, true, "enable-user"); err != nil {
		t.Fatal(err)
	}

	var actions int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM audit_events
		WHERE action IN ('auth.local.user.created', 'auth.role.assigned', 'auth.password.reset', 'auth.user.disabled', 'auth.user.enabled')
		  AND target_id = $1`, user.ID).Scan(&actions); err != nil || actions != 5 {
		t.Fatalf("administrative audit events = %d, %v", actions, err)
	}
}

func testService(t *testing.T, pool *pgxpool.Pool, now func() time.Time) *Service {
	t.Helper()
	options := DefaultOptions()
	options.Hasher = PasswordHasher{Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 16}
	options.LoginLimiter = NewLoginLimiter(100, time.Minute)
	if now != nil {
		options.Now = now
	}
	service, err := NewService(pool, options)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func authTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("ESPIAL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ESPIAL_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	base, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := base.Ping(ctx); err != nil {
		base.Close()
		t.Fatal(err)
	}
	schema := fmt.Sprintf("espial_auth_test_%d", time.Now().UnixNano())
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := base.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		base.Close()
		t.Fatal(err)
	}
	configuration, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	configuration.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := base.Exec(cleanup, "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Errorf("drop schema: %v", err)
		}
		base.Close()
	})
	return pool
}
