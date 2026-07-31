package adapters

import (
	"context"
	"encoding/json"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/observations"
)

type Session struct {
	Process     *Process
	Manifest    Manifest
	Version     string
	integration Integration
	resolver    SecretResolver
}

func StartSession(
	ctx context.Context,
	descriptor Descriptor,
	integration Integration,
	resolver SecretResolver,
	options ProcessOptions,
) (*Session, error) {
	options = normalizeProcessOptions(options)
	if integration.AdapterID != descriptor.AdapterID {
		return nil, runtimeError("adapter_identity_mismatch")
	}
	process, err := StartProcess(ctx, descriptor, options)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*Session, error) {
		stopContext, cancel := context.WithTimeout(context.Background(), options.ShutdownTimeout+options.TerminationTimeout+time.Second)
		defer cancel()
		_ = process.Stop(stopContext)
		return nil, err
	}
	payload, err := process.Call(ctx, OperationManifest, map[string]any{})
	if err != nil {
		return fail(err)
	}
	manifest, err := DecodeManifest(payload)
	if err != nil {
		return fail(err)
	}
	if manifest.AdapterID != descriptor.AdapterID {
		return fail(runtimeError("adapter_identity_mismatch"))
	}
	if err := validateSliceCapabilities(manifest); err != nil {
		return fail(err)
	}
	version, err := NegotiateVersion([]string{ProtocolV1}, manifest.ProtocolVersions)
	if err != nil {
		return fail(err)
	}
	process.SetProtocolVersion(version)
	resolved, secrets, err := ResolveConfig(ctx, manifest, integration.ConfigNonsecret, integration.SecretReferences, resolver)
	if err != nil {
		return fail(err)
	}
	process.SetDiagnosticSecrets(secrets)
	validationPayload, err := process.Call(ctx, OperationValidateConfig, map[string]any{"config": resolved})
	if err != nil {
		return fail(err)
	}
	if err := requireBooleanResponse(validationPayload, "valid"); err != nil {
		return fail(err)
	}
	healthPayload, err := process.Call(ctx, OperationHealth, map[string]any{})
	if err != nil {
		return fail(err)
	}
	if err := requireStringResponse(healthPayload, "status", "healthy"); err != nil {
		return fail(err)
	}
	return &Session{Process: process, Manifest: manifest, Version: version, integration: integration, resolver: resolver}, nil
}

func (session *Session) Collect(ctx context.Context, receivedAt time.Time) (CollectionPayload, observations.Batch, error) {
	resolved, secrets, err := ResolveConfig(ctx, session.Manifest, session.integration.ConfigNonsecret, session.integration.SecretReferences, session.resolver)
	if err != nil {
		return CollectionPayload{}, observations.Batch{}, err
	}
	session.Process.SetDiagnosticSecrets(secrets)
	payload, err := session.Process.Call(ctx, OperationCollect, map[string]any{"config": resolved})
	if err != nil {
		return CollectionPayload{}, observations.Batch{}, err
	}
	return DecodeCollection(payload, receivedAt)
}

func (session *Session) Health(ctx context.Context) error {
	payload, err := session.Process.Call(ctx, OperationHealth, map[string]any{})
	if err != nil {
		return err
	}
	return requireStringResponse(payload, "status", "healthy")
}

func requireBooleanResponse(payload json.RawMessage, field string) error {
	var value map[string]any
	if json.Unmarshal(payload, &value) != nil || len(value) != 1 || value[field] != true {
		return runtimeError("invalid_operation_response")
	}
	return nil
}

func requireStringResponse(payload json.RawMessage, field, expected string) error {
	var value map[string]any
	if json.Unmarshal(payload, &value) != nil || len(value) != 1 || value[field] != expected {
		return runtimeError("invalid_operation_response")
	}
	return nil
}

func (session *Session) Close(ctx context.Context) error { return session.Process.Stop(ctx) }

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func validateSliceCapabilities(manifest Manifest) error {
	if !manifest.ReadOnly || len(manifest.Capabilities) != 1 || !contains(manifest.Capabilities, "collect") {
		return runtimeError("unsupported_capability")
	}
	return nil
}
