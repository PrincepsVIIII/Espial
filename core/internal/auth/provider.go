package auth

import (
	"context"
	"errors"
)

var ErrProviderUnavailable = errors.New("identity provider is not configured")

type DisabledProvider struct{}

func (DisabledProvider) Name() string { return "sso" }

func (DisabledProvider) BeginLogin(context.Context, string) (string, error) {
	return "", ErrProviderUnavailable
}

func (DisabledProvider) CompleteLogin(context.Context, map[string]string) (ExternalIdentity, error) {
	return ExternalIdentity{}, ErrProviderUnavailable
}
