package notifications

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileSecretResolverBoundsAndContainment(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "valid-token"), []byte("opaque-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolver := FileSecretResolver{Root: root}
	value, err := resolver.Resolve(context.Background(), "valid-token")
	if err != nil || value != "opaque-token" {
		t.Fatalf("resolved secret = %q, %v", value, err)
	}
	outside := filepath.Join(t.TempDir(), "outside-token")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escaped-token")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "too-large"), []byte(strings.Repeat("x", 4097)), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, reference := range []string{"../outside-token", "escaped-token", "too-large", "missing"} {
		if _, err := resolver.Resolve(context.Background(), reference); !errors.Is(err, ErrSecretUnavailable) {
			t.Errorf("Resolve(%q) error = %v", reference, err)
		}
	}
}
