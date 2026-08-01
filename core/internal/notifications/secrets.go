package notifications

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// FileSecretResolver resolves opaque names beneath one configured directory.
// It rejects traversal and symlinks that escape that directory.
type FileSecretResolver struct{ Root string }

func (resolver FileSecretResolver) Resolve(ctx context.Context, reference string) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if !secretReferencePattern.MatchString(reference) || strings.TrimSpace(resolver.Root) == "" {
		return "", ErrSecretUnavailable
	}
	root, err := filepath.Abs(resolver.Root)
	if err != nil {
		return "", ErrSecretUnavailable
	}
	name, err := filepath.EvalSymlinks(filepath.Join(root, reference))
	if err != nil {
		return "", ErrSecretUnavailable
	}
	relative, err := filepath.Rel(root, name)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", ErrSecretUnavailable
	}
	file, err := os.Open(name)
	if err != nil {
		return "", ErrSecretUnavailable
	}
	defer file.Close()
	value, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil || len(value) == 0 || len(value) > 4096 {
		return "", ErrSecretUnavailable
	}
	secret := strings.TrimSpace(string(value))
	if secret == "" || strings.ContainsAny(secret, "\r\n\x00") {
		return "", ErrSecretUnavailable
	}
	return secret, nil
}

var _ SecretResolver = FileSecretResolver{}
