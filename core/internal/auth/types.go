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
)

type User struct {
	ID          string   `json:"id"`
	Username    string   `json:"username"`
	DisplayName string   `json:"display_name"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
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
