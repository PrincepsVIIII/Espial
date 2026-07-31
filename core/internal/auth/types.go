package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"time"
)

var (
	ErrAlreadyBootstrapped = errors.New("an administrator already exists")
	ErrInvalidCredentials  = errors.New("invalid username or password")
	ErrRateLimited         = errors.New("too many login attempts")
	ErrSessionNotFound     = errors.New("session not found")
	ErrUserNotFound        = errors.New("local user not found")
	ErrRoleNotFound        = errors.New("role not found")
	ErrUsernameTaken       = errors.New("username already exists")
	ErrInvalidCursor       = errors.New("invalid user cursor")
	ErrUserChanged         = errors.New("user changed since it was read")
	ErrSelfLockout         = errors.New("administrators cannot remove their own access")
	ErrLastAdministrator   = errors.New("at least one enabled administrator is required")
)

type User struct {
	ID          string   `json:"id"`
	Username    string   `json:"username"`
	DisplayName string   `json:"display_name"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
}

type ManagedUser struct {
	ID               string     `json:"id"`
	Username         string     `json:"username"`
	DisplayName      string     `json:"display_name"`
	Email            string     `json:"email,omitempty"`
	IdentityProvider string     `json:"identity_provider"`
	Enabled          bool       `json:"enabled"`
	Roles            []string   `json:"roles"`
	ActiveSessions   int64      `json:"active_sessions"`
	PasswordChanged  *time.Time `json:"password_changed_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type ManagedUserList struct {
	Items      []ManagedUser `json:"items"`
	NextCursor string        `json:"next_cursor,omitempty"`
}

type ManagedUserFilter struct {
	Limit  int
	Cursor string
}

type RoleView struct {
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
}

type AdministrationContext struct {
	ActorUserID   string
	SourceAddress string
	CorrelationID string
}

type CreateManagedUser struct {
	Username    string
	DisplayName string
	Email       string
	Role        string
	Password    string
	Context     AdministrationContext
}

type UpdateManagedUser struct {
	ID                string
	DisplayName       string
	Email             string
	Role              string
	Enabled           bool
	ExpectedUpdatedAt time.Time
	Context           AdministrationContext
}

type ResetManagedPassword struct {
	ID       string
	Password string
	Context  AdministrationContext
}

func (user User) HasPermission(permission string) bool {
	for _, candidate := range user.Permissions {
		if candidate == permission {
			return true
		}
	}
	return false
}

type Session struct {
	ID             string
	Token          string
	CSRFToken      string
	CSRFDigest     []byte
	User           User
	ExpiresAt      time.Time
	AbsoluteExpiry time.Time
}

type ExternalIdentity struct {
	Provider    string
	Subject     string
	DisplayName string
	Email       string
	Groups      []string
}

type IdentityProvider interface {
	Name() string
	BeginLogin(context.Context, string) (string, error)
	CompleteLogin(context.Context, map[string]string) (ExternalIdentity, error)
}

func NormalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func newSecret() (string, []byte, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", nil, err
	}
	raw := base64.RawURLEncoding.EncodeToString(value)
	digest := sha256.Sum256([]byte(raw))
	return raw, digest[:], nil
}

func digestSecret(raw string) []byte {
	digest := sha256.Sum256([]byte(raw))
	return digest[:]
}
