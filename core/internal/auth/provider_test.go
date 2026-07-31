package auth

import (
	"context"
	"errors"
	"testing"
)

func TestDisabledProviderCannotAcceptIdentity(t *testing.T) {
	provider := DisabledProvider{}
	if provider.Name() != "sso" {
		t.Fatalf("name = %q", provider.Name())
	}
	if _, err := provider.BeginLogin(context.Background(), "/overview"); !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("begin login: %v", err)
	}
	if _, err := provider.CompleteLogin(context.Background(), map[string]string{"subject": "someone"}); !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("complete login: %v", err)
	}
}
