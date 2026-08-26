package proxmox

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/adapters"
)

func Manifest() adapters.Manifest {
	return adapters.Manifest{
		AdapterID: AdapterID, DisplayName: "Espial Proxmox VE", AdapterVersion: "0.1.0",
		ProtocolVersions: []string{adapters.ProtocolV1}, IntegrationCategory: "hypervisor",
		ResourceTypes: []string{ClusterKind, NodeKind, QEMUKind, LXCKind, StorageKind},
		CheckTypes:    []string{ClusterQuorumCheck, NodeAvailabilityCheck, GuestRuntimeCheck, StorageAvailabilityCheck, StorageCapacityCheck},
		Capabilities:  []string{"collect"}, ReadOnly: true, SecretFields: []string{"token_secret"},
		ConfigSchema: map[string]any{"type": "object", "additionalProperties": false,
			"required": []string{"base_url", "deployment_id", "token_id", "token_secret"},
			"properties": map[string]any{
				"base_url":                      map[string]any{"type": "string", "format": "uri", "maxLength": 2048},
				"allow_insecure_http":           map[string]any{"type": "boolean"},
				"trust_self_signed_certificate": map[string]any{"type": "boolean"},
				"trusted_certificate_sha256":    map[string]any{"type": "string", "pattern": "^[0-9a-f]{64}$"},
				"approved_host":                 map[string]any{"type": "string", "minLength": 1, "maxLength": 253},
				"approved_port":                 map[string]any{"type": "integer", "minimum": 1, "maximum": 65535},
				"deployment_id":                 map[string]any{"type": "string", "pattern": "^[a-z][a-z0-9-]{1,62}$"},
				"token_id":                      map[string]any{"type": "string", "minLength": 3, "maxLength": 160},
				"token_secret":                  map[string]any{"type": "string", "maxLength": 4096},
				"expected_refresh_seconds":      map[string]any{"type": "integer", "minimum": 1, "maximum": 86400},
				"request_timeout_seconds":       map[string]any{"type": "integer", "minimum": 1, "maximum": 60},
				"expected_running_guests":       map[string]any{"type": "array", "maxItems": 10000, "uniqueItems": true, "items": map[string]any{"type": "string", "pattern": "^(qemu|lxc):[0-9]{1,10}$"}},
				"storage_warning_percent":       percentSchema(),
				"storage_critical_percent":      percentSchema(),
				"swap_warning_percent":          percentSchema(),
				"swap_critical_percent":         percentSchema(),
				"io_wait_warning_percent":       percentSchema(),
				"io_wait_critical_percent":      percentSchema(),
			}},
	}
}

func Run(input io.Reader, output io.Writer, policy Policy) error {
	codec := adapters.NewCodec(input, output, adapters.MaxLineBytes)
	if err := codec.Write(adapters.Envelope{ProtocolVersion: adapters.ProtocolV1, Kind: adapters.KindNotification,
		Operation: adapters.OperationReady, SentAt: time.Now().UTC(), Payload: json.RawMessage(`{}`)}); err != nil {
		return err
	}
	for {
		request, err := codec.Read()
		if err != nil {
			return err
		}
		if request.Kind != adapters.KindRequest {
			return errors.New("expected request")
		}
		switch request.Operation {
		case adapters.OperationManifest:
			if err := respond(codec, request, Manifest()); err != nil {
				return err
			}
		case adapters.OperationValidateConfig:
			config, err := DecodeConfig(request.Payload)
			if err != nil {
				if writeErr := respondError(codec, request, "invalid_config"); writeErr != nil {
					return writeErr
				}
				continue
			}
			if err := ValidateLive(request, config, policy); err != nil {
				if writeErr := respondError(codec, request, safeErrorCode(err)); writeErr != nil {
					return writeErr
				}
				continue
			}
			if err := respond(codec, request, map[string]bool{"valid": true}); err != nil {
				return err
			}
		case adapters.OperationCollect:
			config, err := DecodeConfig(request.Payload)
			if err != nil {
				if writeErr := respondError(codec, request, "invalid_config"); writeErr != nil {
					return writeErr
				}
				continue
			}
			ctx, cancel := envelopeContext(request, RequestTimeout(config))
			client, err := NewClient(config, clientOptions(policy, config))
			if err == nil {
				payload, err := Collect(ctx, config, client, time.Now().UTC())
				cancel()
				if err == nil {
					if err := respond(codec, request, payload); err != nil {
						return err
					}
					continue
				}
			} else {
				cancel()
			}
			if writeErr := respondError(codec, request, safeErrorCode(err)); writeErr != nil {
				return writeErr
			}
		case adapters.OperationHealth:
			if err := respond(codec, request, map[string]string{"status": "healthy"}); err != nil {
				return err
			}
		case adapters.OperationShutdown:
			if err := respond(codec, request, map[string]bool{"stopping": true}); err != nil {
				return err
			}
			return nil
		default:
			if err := respondError(codec, request, "unsupported_operation"); err != nil {
				return err
			}
		}
	}
}

func ValidateLive(request adapters.Envelope, config Config, policy Policy) error {
	ctx, cancel := envelopeContext(request, RequestTimeout(config))
	defer cancel()
	client, err := NewClient(config, clientOptions(policy, config))
	if err != nil {
		return err
	}
	_, err = Collect(ctx, config, client, time.Now().UTC())
	return err
}

func clientOptions(policy Policy, config Config) ClientOptions {
	hosts := append([]string(nil), policy.ApprovedHosts...)
	if len(hosts) == 0 && config.ApprovedHost != "" {
		hosts = []string{config.ApprovedHost}
	}
	ports := append([]int(nil), policy.AllowedPorts...)
	if len(ports) == 0 && config.ApprovedPort > 0 {
		ports = []int{config.ApprovedPort}
	}
	return ClientOptions{ApprovedHosts: hosts, AllowedPorts: ports}
}

func envelopeContext(request adapters.Envelope, fallback time.Duration) (context.Context, context.CancelFunc) {
	if request.Deadline != nil {
		return context.WithDeadline(context.Background(), *request.Deadline)
	}
	return context.WithTimeout(context.Background(), fallback)
}

func respond(codec *adapters.Codec, request adapters.Envelope, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return codec.Write(adapters.Envelope{ProtocolVersion: request.ProtocolVersion, Kind: adapters.KindResponse,
		Operation: request.Operation, RequestID: request.RequestID, SentAt: time.Now().UTC(), Payload: encoded})
}

func respondError(codec *adapters.Codec, request adapters.Envelope, code string) error {
	return codec.Write(adapters.Envelope{ProtocolVersion: request.ProtocolVersion, Kind: adapters.KindResponse,
		Operation: request.Operation, RequestID: request.RequestID, SentAt: time.Now().UTC(),
		Error: &adapters.RemoteError{Code: code, Message: "proxmox request failed", Retryable: false}})
}

func percentSchema() map[string]any {
	return map[string]any{"type": "number", "minimum": 0, "maximum": 100}
}

func safeErrorCode(err error) string {
	if err == nil {
		return ""
	}
	switch err.Error() {
	case "invalid proxmox config":
		return "invalid_config"
	case "unauthorized", "forbidden", "throttled", "timeout", "temporary_upstream_failure", "malformed_data", "oversized_response", "target_not_approved",
		"tls_certificate_untrusted", "tls_hostname_mismatch", "tls_certificate_invalid", "tls_certificate_changed", "tls_certificate_missing", "tls_handshake_failed", "dns_failed", "connection_failed",
		"proxmox_api_not_found", "upstream_request_rejected":
		return err.Error()
	default:
		return "proxmox_collection_failed"
	}
}
