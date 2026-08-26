package adapters

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type SecretResolver interface {
	Resolve(context.Context, string) (string, error)
}

type SecretWriter interface {
	Write(context.Context, string, string) error
}

type SecretStore interface {
	SecretResolver
	SecretWriter
}

type SecretResolverFunc func(context.Context, string) (string, error)

func (resolve SecretResolverFunc) Resolve(ctx context.Context, reference string) (string, error) {
	return resolve(ctx, reference)
}

var (
	ErrSecretUnavailable = errors.New("secret unavailable")
	secretNamePattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
)

// FileSecretStore resolves and writes opaque secret files under one directory.
// References are names, not paths; traversal and symlink escapes are rejected.
type FileSecretStore struct{ Root string }

func (store FileSecretStore) Resolve(ctx context.Context, reference string) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	path, err := store.path(reference)
	if err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", ErrSecretUnavailable
	}
	defer file.Close()
	value, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil || len(value) == 0 || len(value) > 4096 {
		return "", ErrSecretUnavailable
	}
	secret := strings.TrimSpace(string(value))
	if !validSecretValue(secret) {
		return "", ErrSecretUnavailable
	}
	return secret, nil
}

func (store FileSecretStore) Write(ctx context.Context, reference, value string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	secret := strings.TrimSpace(value)
	if !validSecretValue(secret) {
		return ErrSecretUnavailable
	}
	path, err := store.path(reference)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return ErrSecretUnavailable
	}
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		return ErrSecretUnavailable
	}
	temp := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+"."+hex.EncodeToString(suffix)+".tmp")
	file, err := os.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return ErrSecretUnavailable
	}
	_, writeErr := file.WriteString(secret + "\n")
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(temp)
		return ErrSecretUnavailable
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return ErrSecretUnavailable
	}
	return nil
}

func (store FileSecretStore) path(reference string) (string, error) {
	if !secretNamePattern.MatchString(reference) || strings.TrimSpace(store.Root) == "" {
		return "", ErrSecretUnavailable
	}
	root, err := filepath.Abs(store.Root)
	if err != nil {
		return "", ErrSecretUnavailable
	}
	path := filepath.Join(root, reference)
	resolvedRoot := root
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		resolvedRoot = resolved
	} else if !os.IsNotExist(err) {
		return "", ErrSecretUnavailable
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	relative, err := filepath.Rel(resolvedRoot, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", ErrSecretUnavailable
	}
	return path, nil
}

func validSecretValue(value string) bool {
	return value != "" && len(value) <= 4096 && !strings.ContainsAny(value, "\r\n\x00")
}

func ResolveConfig(
	ctx context.Context,
	manifest Manifest,
	nonsecret map[string]any,
	references map[string]string,
	resolver SecretResolver,
) (map[string]any, []string, error) {
	secretFields := make(map[string]struct{}, len(manifest.SecretFields))
	for _, field := range manifest.SecretFields {
		secretFields[field] = struct{}{}
		if _, exists := nonsecret[field]; exists {
			return nil, nil, runtimeError("secret_in_nonsecret_config")
		}
		if strings.TrimSpace(references[field]) == "" && !optionalSecretField(manifest, field) {
			return nil, nil, runtimeError("secret_reference_missing")
		}
	}
	for field := range references {
		if _, exists := secretFields[field]; !exists {
			return nil, nil, runtimeError("undeclared_secret_reference")
		}
	}
	if len(references) > 0 && resolver == nil {
		return nil, nil, runtimeError("secret_resolver_unavailable")
	}
	resolved := make(map[string]any, len(nonsecret)+len(references))
	for key, value := range nonsecret {
		resolved[key] = value
	}
	redactions := make([]string, 0, len(references))
	for field, reference := range references {
		value, err := resolver.Resolve(ctx, reference)
		if err != nil {
			return nil, nil, runtimeError("secret_resolution_failed")
		}
		resolved[field] = value
		if value != "" {
			redactions = append(redactions, value)
		}
	}
	return resolved, redactions, nil
}

func optionalSecretField(manifest Manifest, field string) bool {
	properties, ok := manifest.ConfigSchema["properties"].(map[string]any)
	if !ok {
		return false
	}
	definition, ok := properties[field].(map[string]any)
	if !ok {
		return false
	}
	value, _ := definition["x-espial-optional-secret"].(bool)
	return value
}

func Redact(value string, secrets []string) string {
	for _, secret := range secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	return value
}
