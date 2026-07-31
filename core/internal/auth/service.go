package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const bootstrapLockID int64 = 0x45535049414c4155 // "ESPIALAU"

type Options struct {
	Hasher          PasswordHasher
	SessionIdle     time.Duration
	SessionAbsolute time.Duration
	FailureLimit    int
	LockoutDuration time.Duration
	LoginLimiter    *LoginLimiter
	Now             func() time.Time
}

func DefaultOptions() Options {
	return Options{
		Hasher:          DefaultPasswordHasher(),
		SessionIdle:     30 * time.Minute,
		SessionAbsolute: 12 * time.Hour,
		FailureLimit:    5,
		LockoutDuration: 15 * time.Minute,
		LoginLimiter:    NewLoginLimiter(10, time.Minute),
		Now:             time.Now,
	}
}

type Service struct {
	pool      *pgxpool.Pool
	options   Options
	dummyHash string
}

func NewService(pool *pgxpool.Pool, options Options) (*Service, error) {
	defaults := DefaultOptions()
	if options.Hasher.Memory == 0 {
		options.Hasher = defaults.Hasher
	}
	if options.SessionIdle <= 0 {
		options.SessionIdle = defaults.SessionIdle
	}
	if options.SessionAbsolute <= 0 {
		options.SessionAbsolute = defaults.SessionAbsolute
	}
	if options.FailureLimit <= 0 {
		options.FailureLimit = defaults.FailureLimit
	}
	if options.LockoutDuration <= 0 {
		options.LockoutDuration = defaults.LockoutDuration
	}
	if options.LoginLimiter == nil {
		options.LoginLimiter = defaults.LoginLimiter
	}
	if options.Now == nil {
		options.Now = defaults.Now
	}
	dummyHash, err := options.Hasher.Hash("this password is never accepted")
	if err != nil {
		return nil, fmt.Errorf("create comparison hash: %w", err)
	}
	return &Service{pool: pool, options: options, dummyHash: dummyHash}, nil
}

func (service *Service) BootstrapAdmin(ctx context.Context, username, password, requestID, sourceAddress string) (User, error) {
	username = NormalizeUsername(username)
	if err := validateUsername(username); err != nil {
		return User{}, err
	}
	passwordHash, err := service.options.Hasher.Hash(password)
	if err != nil {
		return User{}, err
	}

	tx, err := service.pool.Begin(ctx)
	if err != nil {
		return User{}, fmt.Errorf("begin administrator bootstrap: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", bootstrapLockID); err != nil {
		return User{}, fmt.Errorf("lock administrator bootstrap: %w", err)
	}

	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM user_roles ur
			JOIN roles r ON r.id = ur.role_id
			WHERE r.name = 'administrator'
		)`).Scan(&exists); err != nil {
		return User{}, fmt.Errorf("check administrator bootstrap: %w", err)
	}
	if exists {
		return User{}, ErrAlreadyBootstrapped
	}

	var userID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (id, username, display_name, identity_provider, external_subject)
		VALUES (gen_random_uuid(), $1, $1, 'local', $1)
		RETURNING id`, username).Scan(&userID); err != nil {
		return User{}, fmt.Errorf("create local administrator: %w", err)
	}
	if _, err := tx.Exec(ctx,
		"INSERT INTO local_credentials (user_id, password_hash) VALUES ($1, $2)",
		userID,
		passwordHash,
	); err != nil {
		return User{}, fmt.Errorf("store local administrator credential: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		SELECT $1, id FROM roles WHERE name = 'administrator'`, userID); err != nil {
		return User{}, fmt.Errorf("grant administrator role: %w", err)
	}
	if err := insertAudit(ctx, tx, auditEvent{
		ActorUserID:   userID,
		Action:        "auth.local.bootstrap",
		TargetType:    "user",
		TargetID:      userID,
		Result:        "succeeded",
		SourceAddress: sourceAddress,
		CorrelationID: requestID,
	}); err != nil {
		return User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, fmt.Errorf("commit administrator bootstrap: %w", err)
	}
	return User{
		ID:          userID,
		Username:    username,
		DisplayName: username,
		Roles:       []string{"administrator"},
		Permissions: administratorPermissions(),
	}, nil
}

func (service *Service) Login(ctx context.Context, username, password, sourceAddress, requestID string) (Session, error) {
	now := service.options.Now().UTC()
	if !service.options.LoginLimiter.Allow(sourceAddress, now) {
		return Session{}, ErrRateLimited
	}
	username = NormalizeUsername(username)

	credential, err := service.credential(ctx, username)
	if errors.Is(err, pgx.ErrNoRows) {
		_, _ = service.options.Hasher.Verify(password, service.dummyHash)
		if err := service.audit(ctx, auditEvent{
			Action: "auth.login.failed", TargetType: "login_identity", TargetID: username,
			Result: "failed", SourceAddress: sourceAddress, CorrelationID: requestID,
		}); err != nil {
			return Session{}, err
		}
		return Session{}, ErrInvalidCredentials
	}
	if err != nil {
		return Session{}, err
	}

	valid, verifyErr := service.options.Hasher.Verify(password, credential.PasswordHash)
	if verifyErr != nil {
		return Session{}, fmt.Errorf("verify stored credential: %w", verifyErr)
	}
	locked := credential.LockedUntil != nil && credential.LockedUntil.After(now)
	if !valid || !credential.Enabled || locked {
		if !valid && !locked {
			if err := service.recordFailure(ctx, credential.UserID, now); err != nil {
				return Session{}, err
			}
		}
		if err := service.audit(ctx, auditEvent{
			Action: "auth.login.failed", TargetType: "user", TargetID: credential.UserID,
			Result: "failed", SourceAddress: sourceAddress, CorrelationID: requestID,
		}); err != nil {
			return Session{}, err
		}
		return Session{}, ErrInvalidCredentials
	}

	session, err := service.createSession(ctx, credential, now, sourceAddress, requestID)
	if err != nil {
		return Session{}, err
	}
	return session, nil
}

func (service *Service) Authenticate(ctx context.Context, rawToken string) (Session, error) {
	if rawToken == "" {
		return Session{}, ErrSessionNotFound
	}
	now := service.options.Now().UTC()
	newIdleExpiry := now.Add(service.options.SessionIdle)

	var session Session
	var csrfDigest []byte
	err := service.pool.QueryRow(ctx, `
		UPDATE sessions s
		SET last_seen_at = $2, expires_at = LEAST($3, s.absolute_expires_at)
		FROM users u
		WHERE s.token_digest = $1
		  AND u.id = s.user_id
		  AND u.enabled
		  AND s.revoked_at IS NULL
		  AND s.expires_at > $2
		  AND s.absolute_expires_at > $2
		RETURNING s.id, u.id, u.username, u.display_name, s.csrf_digest,
		          s.expires_at, s.absolute_expires_at`,
		digestSecret(rawToken), now, newIdleExpiry,
	).Scan(
		&session.ID,
		&session.User.ID,
		&session.User.Username,
		&session.User.DisplayName,
		&csrfDigest,
		&session.ExpiresAt,
		&session.AbsoluteExpiry,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrSessionNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("load session: %w", err)
	}
	session.CSRFDigest = csrfDigest

	session.User.Roles, session.User.Permissions, err = service.roles(ctx, session.User.ID)
	if err != nil {
		return Session{}, err
	}
	return session, nil
}

func (service *Service) VerifyCSRF(session Session, rawToken string) bool {
	if rawToken == "" {
		return false
	}
	return subtle.ConstantTimeCompare(session.CSRFDigest, digestSecret(rawToken)) == 1
}

func (service *Service) Logout(ctx context.Context, session Session, sourceAddress, requestID string) error {
	tx, err := service.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin logout: %w", err)
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx,
		"UPDATE sessions SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL",
		session.ID,
	)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	if result.RowsAffected() > 0 {
		if err := insertAudit(ctx, tx, auditEvent{
			ActorUserID: session.User.ID, Action: "auth.logout", TargetType: "session", TargetID: session.ID,
			Result: "succeeded", SourceAddress: sourceAddress, CorrelationID: requestID,
		}); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit logout: %w", err)
	}
	return nil
}

// RotateSession replaces both browser secrets while preserving the session lifetime.
func (service *Service) RotateSession(ctx context.Context, session Session, sourceAddress, requestID string) (Session, error) {
	rawToken, tokenDigest, err := newSecret()
	if err != nil {
		return Session{}, fmt.Errorf("generate rotated session token: %w", err)
	}
	csrfToken, csrfDigest, err := newSecret()
	if err != nil {
		return Session{}, fmt.Errorf("generate rotated CSRF token: %w", err)
	}
	tx, err := service.pool.Begin(ctx)
	if err != nil {
		return Session{}, fmt.Errorf("begin session rotation: %w", err)
	}
	defer tx.Rollback(ctx)
	now := service.options.Now().UTC()
	result, err := tx.Exec(ctx, `
		UPDATE sessions SET token_digest = $2, csrf_digest = $3
		WHERE id = $1 AND revoked_at IS NULL AND expires_at > $4 AND absolute_expires_at > $4`,
		session.ID, tokenDigest, csrfDigest, now,
	)
	if err != nil {
		return Session{}, fmt.Errorf("rotate session: %w", err)
	}
	if result.RowsAffected() != 1 {
		return Session{}, ErrSessionNotFound
	}
	if err := insertAudit(ctx, tx, auditEvent{
		ActorUserID: session.User.ID, Action: "auth.session.rotated", TargetType: "session", TargetID: session.ID,
		Result: "succeeded", SourceAddress: sourceAddress, CorrelationID: requestID,
	}); err != nil {
		return Session{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Session{}, fmt.Errorf("commit session rotation: %w", err)
	}
	session.Token, session.CSRFToken, session.CSRFDigest = rawToken, csrfToken, csrfDigest
	return session, nil
}

func (service *Service) RevokeUserSessions(ctx context.Context, userID, actorID, sourceAddress, requestID string) error {
	tx, err := service.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin user session revocation: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx,
		"UPDATE sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL",
		userID,
	); err != nil {
		return fmt.Errorf("revoke user sessions: %w", err)
	}
	if err := insertAudit(ctx, tx, auditEvent{
		ActorUserID: actorID, Action: "auth.session.revoked", TargetType: "user", TargetID: userID,
		Result: "succeeded", SourceAddress: sourceAddress, CorrelationID: requestID,
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit user session revocation: %w", err)
	}
	return nil
}

func (service *Service) RecordDenied(ctx context.Context, user User, action, sourceAddress, requestID string) error {
	return service.audit(ctx, auditEvent{
		ActorUserID: user.ID, Action: action, TargetType: "permission", TargetID: action,
		Result: "denied", SourceAddress: sourceAddress, CorrelationID: requestID,
	})
}

func (service *Service) DeleteExpiredSessions(ctx context.Context, now time.Time) error {
	_, err := service.pool.Exec(ctx, `
		DELETE FROM sessions
		WHERE expires_at < $1 OR absolute_expires_at < $1
		   OR (revoked_at IS NOT NULL AND revoked_at < $1 - interval '24 hours')`, now)
	if err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}
	return nil
}

type credential struct {
	UserID       string
	Username     string
	DisplayName  string
	Enabled      bool
	PasswordHash string
	LockedUntil  *time.Time
}

func (service *Service) credential(ctx context.Context, username string) (credential, error) {
	var value credential
	err := service.pool.QueryRow(ctx, `
		SELECT u.id, u.username, u.display_name, u.enabled, c.password_hash, c.locked_until
		FROM users u
		JOIN local_credentials c ON c.user_id = u.id
		WHERE lower(u.username) = lower($1) AND u.identity_provider = 'local'`, username,
	).Scan(&value.UserID, &value.Username, &value.DisplayName, &value.Enabled, &value.PasswordHash, &value.LockedUntil)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return credential{}, fmt.Errorf("load local credential: %w", err)
	}
	return value, err
}

func (service *Service) recordFailure(ctx context.Context, userID string, now time.Time) error {
	_, err := service.pool.Exec(ctx, `
		UPDATE local_credentials
		SET failed_attempts = failed_attempts + 1,
		    locked_until = CASE
		        WHEN failed_attempts + 1 >= $2 THEN $3
		        ELSE locked_until
		    END
		WHERE user_id = $1`, userID, service.options.FailureLimit, now.Add(service.options.LockoutDuration))
	if err != nil {
		return fmt.Errorf("record login failure: %w", err)
	}
	return nil
}

func (service *Service) createSession(ctx context.Context, credential credential, now time.Time, sourceAddress, requestID string) (Session, error) {
	rawToken, tokenDigest, err := newSecret()
	if err != nil {
		return Session{}, fmt.Errorf("generate session token: %w", err)
	}
	csrfToken, csrfDigest, err := newSecret()
	if err != nil {
		return Session{}, fmt.Errorf("generate CSRF token: %w", err)
	}

	tx, err := service.pool.Begin(ctx)
	if err != nil {
		return Session{}, fmt.Errorf("begin login: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx,
		"UPDATE local_credentials SET failed_attempts = 0, locked_until = NULL WHERE user_id = $1",
		credential.UserID,
	); err != nil {
		return Session{}, fmt.Errorf("reset login failures: %w", err)
	}

	idleExpiry := now.Add(service.options.SessionIdle)
	absoluteExpiry := now.Add(service.options.SessionAbsolute)
	var sessionID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO sessions (
			id, user_id, token_digest, csrf_digest, source_address,
			last_seen_at, expires_at, absolute_expires_at
		) VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7)
		RETURNING id`,
		credential.UserID, tokenDigest, csrfDigest, nullableAddress(sourceAddress),
		now, idleExpiry, absoluteExpiry,
	).Scan(&sessionID); err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}
	for _, action := range []string{"auth.login.succeeded", "auth.local.used"} {
		if err := insertAudit(ctx, tx, auditEvent{
			ActorUserID: credential.UserID, Action: action, TargetType: "session", TargetID: sessionID,
			Result: "succeeded", SourceAddress: sourceAddress, CorrelationID: requestID,
		}); err != nil {
			return Session{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Session{}, fmt.Errorf("commit login: %w", err)
	}

	roles, permissions, err := service.roles(ctx, credential.UserID)
	if err != nil {
		return Session{}, err
	}
	return Session{
		ID: sessionID, Token: rawToken, CSRFToken: csrfToken, CSRFDigest: csrfDigest,
		ExpiresAt: idleExpiry, AbsoluteExpiry: absoluteExpiry,
		User: User{
			ID: credential.UserID, Username: credential.Username, DisplayName: credential.DisplayName,
			Roles: roles, Permissions: permissions,
		},
	}, nil
}

func (service *Service) roles(ctx context.Context, userID string) ([]string, []string, error) {
	rows, err := service.pool.Query(ctx, `
		SELECT r.name, permission.value
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		LEFT JOIN LATERAL jsonb_array_elements_text(r.permissions) permission(value) ON true
		WHERE ur.user_id = $1`, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("load user roles: %w", err)
	}
	defer rows.Close()

	roleSet := make(map[string]struct{})
	permissionSet := make(map[string]struct{})
	for rows.Next() {
		var role string
		var permission *string
		if err := rows.Scan(&role, &permission); err != nil {
			return nil, nil, fmt.Errorf("scan user role: %w", err)
		}
		roleSet[role] = struct{}{}
		if permission != nil {
			permissionSet[*permission] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("read user roles: %w", err)
	}

	roles := keys(roleSet)
	permissions := keys(permissionSet)
	return roles, permissions, nil
}

type auditEvent struct {
	ActorUserID   string
	Action        string
	TargetType    string
	TargetID      string
	Result        string
	SourceAddress string
	CorrelationID string
}

func (service *Service) audit(ctx context.Context, event auditEvent) error {
	_, err := service.pool.Exec(ctx, auditSQL, auditArguments(event)...)
	if err != nil {
		return fmt.Errorf("write audit event: %w", err)
	}
	return nil
}

const auditSQL = `
	INSERT INTO audit_events (
		id, actor_user_id, action, target_type, target_id, result, source_address, correlation_id
	) VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7)`

func insertAudit(ctx context.Context, tx pgx.Tx, event auditEvent) error {
	if _, err := tx.Exec(ctx, auditSQL, auditArguments(event)...); err != nil {
		return fmt.Errorf("write audit event: %w", err)
	}
	return nil
}

func auditArguments(event auditEvent) []any {
	var actor any
	if event.ActorUserID != "" {
		actor = event.ActorUserID
	}
	if event.CorrelationID == "" {
		event.CorrelationID = "system"
	}
	return []any{
		actor, event.Action, event.TargetType, nullableText(event.TargetID), event.Result,
		nullableAddress(event.SourceAddress), event.CorrelationID,
	}
}

func nullableText(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableAddress(value string) any {
	if net.ParseIP(value) == nil {
		return nil
	}
	return value
}

func validateUsername(username string) error {
	length := utf8.RuneCountInString(username)
	if length < 3 || length > 64 {
		return errors.New("username must contain between 3 and 64 characters")
	}
	for _, character := range username {
		if unicode.IsLetter(character) || unicode.IsDigit(character) || strings.ContainsRune("._-", character) {
			continue
		}
		return errors.New("username may contain letters, numbers, dot, underscore, and hyphen")
	}
	return nil
}

func keys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func administratorPermissions() []string {
	return []string{
		"audit:read", "incidents:operate", "integrations:manage", "integrations:read",
		"overview:read", "resources:read", "roles:manage", "users:manage",
	}
}
