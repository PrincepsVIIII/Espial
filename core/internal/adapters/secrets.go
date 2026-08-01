package adapters

import (
	"context"
	"strings"
)

type SecretResolver interface {
	Resolve(context.Context, string) (string, error)
}

type SecretResolverFunc func(context.Context, string) (string, error)

func (resolve SecretResolverFunc) Resolve(ctx context.Context, reference string) (string, error) {
	return resolve(ctx, reference)
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
