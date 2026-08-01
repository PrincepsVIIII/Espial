package webcheck

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strconv"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/adapters"
)

func Manifest() adapters.Manifest {
	properties := map[string]any{
		"url":                         map[string]any{"type": "string", "format": "uri", "maxLength": 2048},
		"allowed_statuses":            map[string]any{"type": "array", "minItems": 1, "maxItems": 100, "uniqueItems": true, "items": map[string]any{"type": "integer", "minimum": 100, "maximum": 599}},
		"timeout_ms":                  map[string]any{"type": "integer", "minimum": 100, "maximum": 60000},
		"warning_latency_ms":          map[string]any{"type": "integer", "minimum": 0, "maximum": 59999},
		"content_match":               map[string]any{"type": "string", "maxLength": MaxContentBytes},
		"follow_redirects":            map[string]any{"type": "boolean"},
		"max_redirects":               map[string]any{"type": "integer", "minimum": 0, "maximum": 5},
		"expected_refresh_seconds":    map[string]any{"type": "integer", "minimum": 1, "maximum": 86400},
		"certificate_warning_days":    map[string]any{"type": "integer", "minimum": 1, "maximum": 3650},
		"certificate_critical_days":   map[string]any{"type": "integer", "minimum": 1, "maximum": 3649},
		"certificate_escalation_days": map[string]any{"type": "integer", "minimum": 1, "maximum": 3648},
		"header_names":                map[string]any{"type": "array", "maxItems": MaxSecretHeaders, "uniqueItems": true, "items": map[string]any{"type": "string", "minLength": 1, "maxLength": 64}},
	}
	secretFields := make([]string, MaxSecretHeaders)
	for index := 0; index < MaxSecretHeaders; index++ {
		name := "header_value_" + strconv.Itoa(index+1)
		secretFields[index] = name
		properties[name] = map[string]any{"type": "string", "maxLength": 4096, "x-espial-optional-secret": true}
	}
	return adapters.Manifest{
		AdapterID: AdapterID, DisplayName: "Espial website availability", AdapterVersion: "0.1.0",
		ProtocolVersions: []string{adapters.ProtocolV1}, IntegrationCategory: "website",
		ResourceTypes: []string{ResourceKind, CertificateResourceKind}, CheckTypes: []string{CheckType, CertificateCheckType},
		Capabilities: []string{"collect"}, ReadOnly: true, SecretFields: secretFields,
		ConfigSchema: map[string]any{"type": "object", "additionalProperties": false,
			"required":   []string{"url", "allowed_statuses", "timeout_ms", "follow_redirects", "max_redirects", "expected_refresh_seconds"},
			"properties": properties},
	}
}

func Run(input io.Reader, output io.Writer, checker *Checker) error {
	if checker == nil || checker.policy == nil {
		return errors.New("webcheck policy is required")
	}
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
			if err == nil {
				err = validatePolicy(checker.policy, config)
			}
			if err != nil {
				if writeErr := respondError(codec, request, "invalid_config"); writeErr != nil {
					return writeErr
				}
				continue
			}
			if err := respond(codec, request, map[string]bool{"valid": true}); err != nil {
				return err
			}
		case adapters.OperationHealth:
			if err := respond(codec, request, map[string]string{"status": "healthy"}); err != nil {
				return err
			}
		case adapters.OperationCollect:
			config, err := DecodeConfig(request.Payload)
			if err == nil {
				err = validatePolicy(checker.policy, config)
			}
			if err != nil {
				if writeErr := respondError(codec, request, "invalid_config"); writeErr != nil {
					return writeErr
				}
				continue
			}
			checkContext, cancel := envelopeContext(request)
			result := checker.Check(checkContext, config)
			cancel()
			config = WithCertificateDefaults(config)
			payload := adapters.CollectionPayload{
				Resources: []adapters.CollectionResource{{ExternalID: config.URL, Kind: ResourceKind,
					DisplayName: displayName(config.URL), ObservedAt: result.ObservedAt,
					Attributes: map[string]any{"scheme": mustURL(config.URL).Scheme, "host": mustURL(config.URL).Hostname()}, SourceURL: config.URL}},
				Observations: []adapters.CollectionObservation{{ExternalResourceID: config.URL, CheckType: CheckType,
					State: result.State, Summary: result.Summary, ObservedAt: result.ObservedAt,
					ExpectedRefreshSeconds: config.ExpectedRefreshSeconds,
					Measurements:           result.Measurements, Metadata: result.Metadata}},
			}
			if result.Certificate != nil {
				certificate := result.Certificate
				externalID := "certificate:" + certificate.Endpoint
				payload.Resources = append(payload.Resources, adapters.CollectionResource{ExternalID: externalID,
					Kind: CertificateResourceKind, DisplayName: certificate.Endpoint, ObservedAt: result.ObservedAt,
					Attributes: map[string]any{"endpoint": certificate.Endpoint, "source": "webcheck"}, SourceURL: config.URL})
				metadata := map[string]any{"reason_code": certificate.ReasonCode, "endpoint": certificate.Endpoint,
					"subject": certificate.Subject, "san_summary": certificate.SANSummary, "issuer": certificate.Issuer,
					"serial_number": certificate.SerialNumber, "fingerprint_sha256": certificate.FingerprintSHA256}
				if certificate.NotBefore != nil {
					metadata["not_before"] = certificate.NotBefore.Format(time.RFC3339Nano)
				}
				if certificate.NotAfter != nil {
					metadata["not_after"] = certificate.NotAfter.Format(time.RFC3339Nano)
				}
				if certificate.HostnameValid != nil {
					metadata["hostname_valid"] = *certificate.HostnameValid
				}
				if certificate.ChainValid != nil {
					metadata["chain_valid"] = *certificate.ChainValid
				}
				measurements := map[string]any{"warning_days": config.CertificateWarningDays, "critical_days": config.CertificateCriticalDays, "escalation_days": config.CertificateEscalationDays}
				if certificate.DaysRemaining != nil {
					measurements["days_remaining"] = *certificate.DaysRemaining
				}
				payload.Observations = append(payload.Observations, adapters.CollectionObservation{ExternalResourceID: externalID,
					CheckType: CertificateCheckType, State: certificate.State, Summary: certificate.Summary, ObservedAt: result.ObservedAt,
					ExpectedRefreshSeconds: config.ExpectedRefreshSeconds, Measurements: measurements, Metadata: metadata})
			}
			if err := respond(codec, request, payload); err != nil {
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

func validatePolicy(policy *Policy, config Config) error {
	target, _ := url.Parse(config.URL)
	port, err := targetPort(target.Scheme, target.Port())
	if err != nil || !policy.AllowsTarget(target.Hostname(), port) || config.MaxRedirects > policy.maxRedirects {
		return errors.New("target is not approved")
	}
	return nil
}

func envelopeContext(request adapters.Envelope) (context.Context, context.CancelFunc) {
	if request.Deadline != nil {
		return context.WithDeadline(context.Background(), *request.Deadline)
	}
	return context.WithCancel(context.Background())
}

func displayName(value string) string {
	target := mustURL(value)
	return target.Hostname() + target.EscapedPath()
}
func mustURL(value string) *url.URL { parsed, _ := url.Parse(value); return parsed }

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
		Error: &adapters.RemoteError{Code: code, Message: "webcheck request failed", Retryable: false}})
}
