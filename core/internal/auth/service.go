package auth

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const bootstrapLockID int64 = 0x45535049414c4155 // "ESPIALAU"
const managedUserLockID int64 = 0x45535049414c5553 // "ESPIALUS"

const (
	defaultManagedUserLimit = 50
	maximumManagedUserLimit = 200
)

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

// ManagedUsers returns a stable, bounded administration view without credentials.
func (service *Service) ManagedUsers(ctx context.Context, filter ManagedUserFilter) (ManagedUserList, error) {
	if filter.Limit <= 0 {
		filter.Limit = defaultManagedUserLimit
	}
	if filter.Limit > maximumManagedUserLimit {
		filter.Limit = maximumManagedUserLimit
	}
	cursorUsername, cursorID, err := decodeManagedUserCursor(filter.Cursor)
	if err != nil {
		return ManagedUserList{}, ErrInvalidCursor
	}
	rows, err := service.pool.Query(ctx, `
		SELECT u.id::text, u.username, u.display_name, u.email, u.identity_provider,
			u.enabled, COALESCE(array_agg(DISTINCT r.name) FILTER (WHERE r.name IS NOT NULL), '{}'),
			count(DISTINCT s.id) FILTER (WHERE s.revoked_at IS NULL AND s.expires_at > now()),
			c.password_changed_at, u.created_at, u.updated_at
		FROM users u
		LEFT JOIN user_roles ur ON ur.user_id = u.id
		LEFT JOIN roles r ON r.id = ur.role_id
		LEFT JOIN sessions s ON s.user_id = u.id
		LEFT JOIN local_credentials c ON c.user_id = u.id
		WHERE ($1 = '' OR (lower(u.username), u.id::text) > ($1, $2))
		GROUP BY u.id, c.password_changed_at
		ORDER BY lower(u.username), u.id::text
		LIMIT $3
	`, cursorUsername, cursorID, filter.Limit+1)
	if err != nil {
		return ManagedUserList{}, fmt.Errorf("list managed users: %w", err)
	}
	defer rows.Close()
	items := make([]ManagedUser, 0, filter.Limit+1)
	for rows.Next() {
		var item ManagedUser
		var email *string
		if err := rows.Scan(
			&item.ID, &item.Username, &item.DisplayName, &email, &item.IdentityProvider,
			&item.Enabled, &item.Roles, &item.ActiveSessions, &item.PasswordChanged,
			&item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return ManagedUserList{}, fmt.Errorf("scan managed user: %w", err)
		}
		if email != nil {
			item.Email = *email
		}
		item.CreatedAt, item.UpdatedAt = item.CreatedAt.UTC(), item.UpdatedAt.UTC()
		if item.PasswordChanged != nil {
			changed := item.PasswordChanged.UTC()
			item.PasswordChanged = &changed
		}
		if item.Roles == nil {
			item.Roles = []string{}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ManagedUserList{}, fmt.Errorf("read managed users: %w", err)
	}
	result := ManagedUserList{Items: items}
	if len(result.Items) > filter.Limit {
		result.Items = result.Items[:filter.Limit]
		last := result.Items[len(result.Items)-1]
		result.NextCursor = encodeManagedUserCursor(last.Username, last.ID)
	}
	return result, nil
}

func (service *Service) ManagedRoles(ctx context.Context) ([]RoleView, error) {
	rows, err := service.pool.Query(ctx, `
		SELECT name, ARRAY(SELECT jsonb_array_elements_text(permissions) ORDER BY 1)
		FROM roles ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list managed roles: %w", err)
	}
	defer rows.Close()
	roles := make([]RoleView, 0, 4)
	for rows.Next() {
		var role RoleView
		if err := rows.Scan(&role.Name, &role.Permissions); err != nil {
			return nil, fmt.Errorf("scan managed role: %w", err)
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read managed roles: %w", err)
	}
	return roles, nil
}

// CreateManagedUser creates a local account and its redacted audit evidence in one transaction.
func (service *Service) CreateManagedUser(ctx context.Context, input CreateManagedUser) (ManagedUser, error) {
	input.Username = NormalizeUsername(input.Username)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Email = strings.TrimSpace(input.Email)
	input.Role = strings.ToLower(strings.TrimSpace(input.Role))
	if err := validateUsername(input.Username); err != nil {
		return ManagedUser{}, err
	}
	if err := validateManagedIdentity(input.DisplayName, input.Email); err != nil {
		return ManagedUser{}, err
	}
	passwordHash, err := service.options.Hasher.Hash(input.Password)
	if err != nil {
		return ManagedUser{}, err
	}
	tx, err := service.pool.Begin(ctx)
	if err != nil {
		return ManagedUser{}, fmt.Errorf("begin managed user creation: %w", err)
	}
	defer tx.Rollback(ctx)
	var roleID string
	if err := tx.QueryRow(ctx, "SELECT id::text FROM roles WHERE name = $1", input.Role).Scan(&roleID); errors.Is(err, pgx.ErrNoRows) {
		return ManagedUser{}, ErrRoleNotFound
	} else if err != nil {
		return ManagedUser{}, fmt.Errorf("load managed user role: %w", err)
	}
	var user ManagedUser
	var email *string
	err = tx.QueryRow(ctx, `
		INSERT INTO users (id, username, display_name, email, identity_provider, external_subject)
		VALUES (gen_random_uuid(), $1, $2, NULLIF($3, ''), 'local', $1)
		ON CONFLICT DO NOTHING
		RETURNING id::text, username, display_name, email, identity_provider, enabled, created_at, updated_at
	`, input.Username, input.DisplayName, input.Email).Scan(
		&user.ID, &user.Username, &user.DisplayName, &email, &user.IdentityProvider,
		&user.Enabled, &user.CreatedAt, &user.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ManagedUser{}, ErrUsernameTaken
	}
	if err != nil {
		return ManagedUser{}, fmt.Errorf("create managed user: %w", err)
	}
	if email != nil {
		user.Email = *email
	}
	if _, err := tx.Exec(ctx, "INSERT INTO local_credentials (user_id, password_hash) VALUES ($1, $2)", user.ID, passwordHash); err != nil {
		return ManagedUser{}, fmt.Errorf("store managed credential: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)", user.ID, roleID); err != nil {
		return ManagedUser{}, fmt.Errorf("grant managed role: %w", err)
	}
	if err := insertAudit(ctx, tx, auditEvent{
		ActorUserID: input.Context.ActorUserID, Action: "auth.local.user.created",
		TargetType: "user", TargetID: user.ID, Result: "succeeded",
		SourceAddress: input.Context.SourceAddress, CorrelationID: input.Context.CorrelationID,
		AfterSummary: map[string]any{
			"username": user.Username, "display_name": user.DisplayName, "email": user.Email,
			"role": input.Role, "enabled": true,
		},
	}); err != nil {
		return ManagedUser{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ManagedUser{}, fmt.Errorf("commit managed user creation: %w", err)
	}
	user.Roles = []string{input.Role}
	user.CreatedAt, user.UpdatedAt = user.CreatedAt.UTC(), user.UpdatedAt.UTC()
	return user, nil
}

// UpdateManagedUser replaces editable identity, role, and enablement state atomically.
func (service *Service) UpdateManagedUser(ctx context.Context, input UpdateManagedUser) (ManagedUser, error) {
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Email = strings.TrimSpace(input.Email)
	input.Role = strings.ToLower(strings.TrimSpace(input.Role))
	if err := validateManagedIdentity(input.DisplayName, input.Email); err != nil {
		return ManagedUser{}, err
	}
	tx, err := service.pool.Begin(ctx)
	if err != nil {
		return ManagedUser{}, fmt.Errorf("begin managed user update: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", managedUserLockID); err != nil {
		return ManagedUser{}, fmt.Errorf("lock managed user update: %w", err)
	}
	var before ManagedUser
	var email *string
	var currentRole string
	err = tx.QueryRow(ctx, `
		SELECT u.id::text, u.username, u.display_name, u.email, u.identity_provider,
			u.enabled, COALESCE((SELECT r.name FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = u.id ORDER BY r.name LIMIT 1), ''),
			u.created_at, u.updated_at
		FROM users u WHERE u.id::text = $1 FOR UPDATE
	`, input.ID).Scan(
		&before.ID, &before.Username, &before.DisplayName, &email, &before.IdentityProvider,
		&before.Enabled, &currentRole, &before.CreatedAt, &before.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ManagedUser{}, ErrUserNotFound
	}
	if err != nil {
		return ManagedUser{}, fmt.Errorf("load managed user: %w", err)
	}
	if email != nil {
		before.Email = *email
	}
	before.Roles = []string{currentRole}
	if input.ExpectedUpdatedAt.IsZero() || !before.UpdatedAt.UTC().Equal(input.ExpectedUpdatedAt.UTC()) {
		return ManagedUser{}, ErrUserChanged
	}
	if input.ID == input.Context.ActorUserID && (!input.Enabled || input.Role != "administrator") {
		return ManagedUser{}, ErrSelfLockout
	}
	if currentRole == "administrator" && before.Enabled && (!input.Enabled || input.Role != "administrator") {
		var others int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM users u JOIN user_roles ur ON ur.user_id = u.id
			JOIN roles r ON r.id = ur.role_id
			WHERE u.enabled AND r.name = 'administrator' AND u.id::text <> $1
		`, input.ID).Scan(&others); err != nil {
			return ManagedUser{}, fmt.Errorf("count enabled administrators: %w", err)
		}
		if others == 0 {
			return ManagedUser{}, ErrLastAdministrator
		}
	}
	var roleID string
	if err := tx.QueryRow(ctx, "SELECT id::text FROM roles WHERE name = $1", input.Role).Scan(&roleID); errors.Is(err, pgx.ErrNoRows) {
		return ManagedUser{}, ErrRoleNotFound
	} else if err != nil {
		return ManagedUser{}, fmt.Errorf("load replacement role: %w", err)
	}
	var updated ManagedUser
	email = nil
	err = tx.QueryRow(ctx, `
		UPDATE users SET display_name = $2, email = NULLIF($3, ''), enabled = $4, updated_at = now()
		WHERE id::text = $1
		RETURNING id::text, username, display_name, email, identity_provider, enabled, created_at, updated_at
	`, input.ID, input.DisplayName, input.Email, input.Enabled).Scan(
		&updated.ID, &updated.Username, &updated.DisplayName, &email, &updated.IdentityProvider,
		&updated.Enabled, &updated.CreatedAt, &updated.UpdatedAt,
	)
	if err != nil {
		return ManagedUser{}, fmt.Errorf("update managed user: %w", err)
	}
	if email != nil {
		updated.Email = *email
	}
	if currentRole != input.Role {
		if _, err := tx.Exec(ctx, "DELETE FROM user_roles WHERE user_id::text = $1", input.ID); err != nil {
			return ManagedUser{}, fmt.Errorf("remove managed roles: %w", err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)", input.ID, roleID); err != nil {
			return ManagedUser{}, fmt.Errorf("assign managed role: %w", err)
		}
	}
	if currentRole != input.Role || before.Enabled != input.Enabled {
		if _, err := tx.Exec(ctx, "UPDATE sessions SET revoked_at = now() WHERE user_id::text = $1 AND revoked_at IS NULL", input.ID); err != nil {
			return ManagedUser{}, fmt.Errorf("revoke sessions after managed user update: %w", err)
		}
	}
	if err := insertAudit(ctx, tx, auditEvent{
		ActorUserID: input.Context.ActorUserID, Action: "auth.user.updated",
		TargetType: "user", TargetID: input.ID, Result: "succeeded",
		SourceAddress: input.Context.SourceAddress, CorrelationID: input.Context.CorrelationID,
		BeforeSummary: managedUserSummary(before, currentRole),
		AfterSummary:  managedUserSummary(updated, input.Role),
	}); err != nil {
		return ManagedUser{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ManagedUser{}, fmt.Errorf("commit managed user update: %w", err)
	}
	updated.Roles = []string{input.Role}
	updated.CreatedAt, updated.UpdatedAt = updated.CreatedAt.UTC(), updated.UpdatedAt.UTC()
	return updated, nil
}

// ResetManagedUserPassword replaces a local credential and records the acting administrator.
func (service *Service) ResetManagedUserPassword(ctx context.Context, input ResetManagedPassword) error {
	passwordHash, err := service.options.Hasher.Hash(input.Password)
	if err != nil {
		return err
	}
	tx, err := service.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin managed password reset: %w", err)
	}
	defer tx.Rollback(ctx)
	var username, provider string
	if err := tx.QueryRow(ctx, "SELECT username, identity_provider FROM users WHERE id::text = $1 FOR UPDATE", input.ID).Scan(&username, &provider); errors.Is(err, pgx.ErrNoRows) {
		return ErrUserNotFound
	} else if err != nil {
		return fmt.Errorf("load password reset target: %w", err)
	}
	if provider != "local" {
		return ErrUserNotFound
	}
	result, err := tx.Exec(ctx, `
		UPDATE local_credentials SET password_hash = $2, password_changed_at = now(),
			failed_attempts = 0, locked_until = NULL WHERE user_id::text = $1
	`, input.ID, passwordHash)
	if err != nil {
		return fmt.Errorf("reset managed password: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrUserNotFound
	}
	if _, err := tx.Exec(ctx, "UPDATE sessions SET revoked_at = now() WHERE user_id::text = $1 AND revoked_at IS NULL", input.ID); err != nil {
		return fmt.Errorf("revoke sessions after managed password reset: %w", err)
	}
	if err := insertAudit(ctx, tx, auditEvent{
		ActorUserID: input.Context.ActorUserID, Action: "auth.password.reset",
		TargetType: "user", TargetID: input.ID, Result: "succeeded",
		SourceAddress: input.Context.SourceAddress, CorrelationID: input.Context.CorrelationID,
		AfterSummary: map[string]any{"username": username, "sessions_revoked": true},
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit managed password reset: %w", err)
	}
	return nil
}

// CreateLocalUser creates an audited local account with one built-in role. This is
// intentionally exposed to the host-side administrative CLI, not the public API.
func (service *Service) CreateLocalUser(ctx context.Context, username, password, role, requestID string) (User, error) {
	username = NormalizeUsername(username)
	role = strings.ToLower(strings.TrimSpace(role))
	if err := validateUsername(username); err != nil {
		return User{}, err
	}
	passwordHash, err := service.options.Hasher.Hash(password)
	if err != nil {
		return User{}, err
	}

	tx, err := service.pool.Begin(ctx)
	if err != nil {
		return User{}, fmt.Errorf("begin local user creation: %w", err)
	}
	defer tx.Rollback(ctx)
	var roleID string
	var permissions []string
	if err := tx.QueryRow(ctx, `
		SELECT id::text, ARRAY(SELECT jsonb_array_elements_text(permissions) ORDER BY 1)
		FROM roles WHERE name = $1`, role).Scan(&roleID, &permissions); errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrRoleNotFound
	} else if err != nil {
		return User{}, fmt.Errorf("load local user role: %w", err)
	}
	var userID string
	err = tx.QueryRow(ctx, `
		INSERT INTO users (id, username, display_name, identity_provider, external_subject)
		VALUES (gen_random_uuid(), $1, $1, 'local', $1)
		ON CONFLICT DO NOTHING
		RETURNING id::text`, username).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUsernameTaken
	}
	if err != nil {
		return User{}, fmt.Errorf("create local user: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO local_credentials (user_id, password_hash) VALUES ($1, $2)", userID, passwordHash); err != nil {
		return User{}, fmt.Errorf("store local user credential: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)", userID, roleID); err != nil {
		return User{}, fmt.Errorf("grant local user role: %w", err)
	}
	if err := insertAudit(ctx, tx, auditEvent{
		Action: "auth.local.user.created", TargetType: "user", TargetID: userID,
		Result: "succeeded", CorrelationID: requestID,
	}); err != nil {
		return User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, fmt.Errorf("commit local user creation: %w", err)
	}
	return User{ID: userID, Username: username, DisplayName: username, Roles: []string{role}, Permissions: permissions}, nil
}

// AssignRole replaces a local user's roles with one built-in role and revokes all
// sessions so the new authorization boundary is immediate.
func (service *Service) AssignRole(ctx context.Context, username, role, requestID string) error {
	username = NormalizeUsername(username)
	role = strings.ToLower(strings.TrimSpace(role))
	tx, err := service.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin role assignment: %w", err)
	}
	defer tx.Rollback(ctx)
	userID, err := localUserID(ctx, tx, username)
	if err != nil {
		return err
	}
	var roleID string
	if err := tx.QueryRow(ctx, "SELECT id::text FROM roles WHERE name = $1", role).Scan(&roleID); errors.Is(err, pgx.ErrNoRows) {
		return ErrRoleNotFound
	} else if err != nil {
		return fmt.Errorf("load role: %w", err)
	}
	if _, err := tx.Exec(ctx, "DELETE FROM user_roles WHERE user_id = $1", userID); err != nil {
		return fmt.Errorf("remove existing roles: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)", userID, roleID); err != nil {
		return fmt.Errorf("assign role: %w", err)
	}
	if _, err := tx.Exec(ctx, "UPDATE sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL", userID); err != nil {
		return fmt.Errorf("revoke sessions after role assignment: %w", err)
	}
	if err := insertAudit(ctx, tx, auditEvent{
		Action: "auth.role.assigned", TargetType: "user", TargetID: userID,
		Result: "succeeded", CorrelationID: requestID,
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit role assignment: %w", err)
	}
	return nil
}

// ResetLocalPassword replaces a password, clears lockout state, and revokes all
// existing sessions without ever returning or logging the credential.
func (service *Service) ResetLocalPassword(ctx context.Context, username, password, requestID string) error {
	username = NormalizeUsername(username)
	passwordHash, err := service.options.Hasher.Hash(password)
	if err != nil {
		return err
	}
	tx, err := service.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin password reset: %w", err)
	}
	defer tx.Rollback(ctx)
	userID, err := localUserID(ctx, tx, username)
	if err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `
		UPDATE local_credentials
		SET password_hash = $2, password_changed_at = now(), failed_attempts = 0, locked_until = NULL
		WHERE user_id = $1`, userID, passwordHash)
	if err != nil {
		return fmt.Errorf("reset local password: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrUserNotFound
	}
	if _, err := tx.Exec(ctx, "UPDATE sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL", userID); err != nil {
		return fmt.Errorf("revoke sessions after password reset: %w", err)
	}
	if err := insertAudit(ctx, tx, auditEvent{
		Action: "auth.password.reset", TargetType: "user", TargetID: userID,
		Result: "succeeded", CorrelationID: requestID,
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit password reset: %w", err)
	}
	return nil
}

// SetLocalUserEnabled changes account availability and revokes sessions. It is safe
// to call repeatedly and still leaves an audit record of the administrative action.
func (service *Service) SetLocalUserEnabled(ctx context.Context, username string, enabled bool, requestID string) error {
	username = NormalizeUsername(username)
	tx, err := service.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin user status change: %w", err)
	}
	defer tx.Rollback(ctx)
	userID, err := localUserID(ctx, tx, username)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "UPDATE users SET enabled = $2, updated_at = now() WHERE id = $1", userID, enabled); err != nil {
		return fmt.Errorf("change local user status: %w", err)
	}
	if _, err := tx.Exec(ctx, "UPDATE sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL", userID); err != nil {
		return fmt.Errorf("revoke sessions after user status change: %w", err)
	}
	action := "auth.user.disabled"
	if enabled {
		action = "auth.user.enabled"
	}
	if err := insertAudit(ctx, tx, auditEvent{
		Action: action, TargetType: "user", TargetID: userID,
		Result: "succeeded", CorrelationID: requestID,
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit user status change: %w", err)
	}
	return nil
}

func localUserID(ctx context.Context, tx pgx.Tx, username string) (string, error) {
	var userID string
	err := tx.QueryRow(ctx, `
		SELECT id::text FROM users
		WHERE lower(username) = lower($1) AND identity_provider = 'local'
		FOR UPDATE`, username).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrUserNotFound
	}
	if err != nil {
		return "", fmt.Errorf("load local user: %w", err)
	}
	return userID, nil
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
	BeforeSummary map[string]any
	AfterSummary  map[string]any
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
			id, actor_user_id, action, target_type, target_id, result, source_address,
			correlation_id, before_summary, after_summary
		) VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9::jsonb)`

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
	var before, after any
	if event.BeforeSummary != nil {
		if encoded, err := json.Marshal(event.BeforeSummary); err == nil {
			before = string(encoded)
		}
	}
	if event.AfterSummary != nil {
		if encoded, err := json.Marshal(event.AfterSummary); err == nil {
			after = string(encoded)
		}
	}
	return []any{
		actor, event.Action, event.TargetType, nullableText(event.TargetID), event.Result,
		nullableAddress(event.SourceAddress), event.CorrelationID, before, after,
	}
}

func validateManagedIdentity(displayName, email string) error {
	if !utf8.ValidString(displayName) || utf8.RuneCountInString(displayName) < 1 || utf8.RuneCountInString(displayName) > 128 {
		return errors.New("display name must contain between 1 and 128 Unicode characters")
	}
	if email == "" {
		return nil
	}
	if len(email) > 254 {
		return errors.New("email address is too long")
	}
	address, err := mail.ParseAddress(email)
	if err != nil || !strings.EqualFold(address.Address, email) {
		return errors.New("email address is invalid")
	}
	return nil
}

func managedUserSummary(user ManagedUser, role string) map[string]any {
	return map[string]any{
		"username": user.Username, "display_name": user.DisplayName, "email": user.Email,
		"role": role, "enabled": user.Enabled,
	}
}

type managedUserCursor struct {
	Username string `json:"username"`
	ID       string `json:"id"`
}

func encodeManagedUserCursor(username, id string) string {
	encoded, _ := json.Marshal(managedUserCursor{Username: strings.ToLower(username), ID: id})
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func decodeManagedUserCursor(raw string) (string, string, error) {
	if raw == "" {
		return "", "", nil
	}
	if len(raw) > 2048 {
		return "", "", errors.New("invalid user cursor")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return "", "", errors.New("invalid user cursor")
	}
	var cursor managedUserCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil || cursor.Username == "" || cursor.ID == "" {
		return "", "", errors.New("invalid user cursor")
	}
	return cursor.Username, cursor.ID, nil
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
