// Package sampleadapter implements Espial's deterministic reference adapter.
package sampleadapter

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/adapters"
	"github.com/PrincepsVIIII/Espial/core/internal/health"
)

const AdapterID = "org.ubnetdef.espial.sample"

type Config struct {
	Scenario              string `json:"scenario"`
	Count                 int    `json:"count"`
	DelayMS               int    `json:"delay_ms"`
	FaultMode             string `json:"fault_mode"`
	ExpectedRefreshSecond int    `json:"expected_refresh_seconds"`
}

type ExitError struct{ Code int }

func (err *ExitError) Error() string { return "sample adapter requested process exit" }

func Manifest() adapters.Manifest {
	return adapters.Manifest{
		AdapterID: AdapterID, DisplayName: "Espial sample adapter", AdapterVersion: "0.1.0",
		ProtocolVersions: []string{adapters.ProtocolV1}, IntegrationCategory: "sample",
		ResourceTypes: []string{"host"}, CheckTypes: []string{"sample.availability"},
		Capabilities: []string{"collect"}, ReadOnly: true,
		ConfigSchema: map[string]any{
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{
				"scenario":   map[string]any{"enum": []string{"healthy", "warning", "critical"}},
				"count":      map[string]any{"type": "integer", "minimum": 1, "maximum": 10},
				"delay_ms":   map[string]any{"type": "integer", "minimum": 0, "maximum": 5000},
				"fault_mode": map[string]any{"enum": validFaults()},
				"expected_refresh_seconds": map[string]any{
					"type": "integer", "minimum": 1, "maximum": 86400,
				},
			},
		},
	}
}

func Run(input io.Reader, output io.Writer, diagnostic io.Writer, startupFault string) error {
	codec := adapters.NewCodec(input, output, adapters.MaxLineBytes)
	ready := adapters.Envelope{
		ProtocolVersion: adapters.ProtocolV1, Kind: adapters.KindNotification,
		Operation: adapters.OperationReady, SentAt: time.Now().UTC(), Payload: json.RawMessage(`{}`),
	}
	switch startupFault {
	case "none", "":
		if err := codec.Write(ready); err != nil {
			return err
		}
	case "malformed_ready":
		_, _ = io.WriteString(output, "not-json\n")
	case "duplicate_ready":
		if err := codec.Write(ready); err != nil {
			return err
		}
		if err := codec.Write(ready); err != nil {
			return err
		}
	case "wrong_major":
		ready.ProtocolVersion = "2.0"
		if err := codec.Write(ready); err != nil {
			return err
		}
	case "no_ready":
		select {}
	default:
		return errors.New("unknown startup fault")
	}

	active := defaultConfig()
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
			config, err := decodeConfig(request.Payload)
			if err != nil {
				if writeErr := respondError(codec, request, "invalid_config"); writeErr != nil {
					return writeErr
				}
				continue
			}
			active = config
			if err := respond(codec, request, map[string]any{"valid": true}); err != nil {
				return err
			}
		case adapters.OperationHealth:
			if err := respond(codec, request, map[string]any{"status": "healthy"}); err != nil {
				return err
			}
		case adapters.OperationCollect:
			config, err := decodeConfig(request.Payload)
			if err != nil {
				if writeErr := respondError(codec, request, "invalid_config"); writeErr != nil {
					return writeErr
				}
				continue
			}
			active = config
			if err := collect(codec, output, diagnostic, request, config); err != nil {
				return err
			}
		case adapters.OperationShutdown:
			if active.FaultMode == "refuse_shutdown" {
				select {}
			}
			if err := respond(codec, request, map[string]any{"stopping": true}); err != nil {
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

func collect(codec *adapters.Codec, output, diagnostic io.Writer, request adapters.Envelope, config Config) error {
	if config.DelayMS > 0 {
		time.Sleep(time.Duration(config.DelayMS) * time.Millisecond)
	}
	switch config.FaultMode {
	case "malformed":
		_, _ = io.WriteString(output, "not-json\n")
		return nil
	case "oversized":
		_, _ = io.WriteString(output, strings.Repeat("x", adapters.MaxLineBytes+1)+"\n")
		return nil
	case "partial":
		_, _ = io.WriteString(output, `{"protocol_version":"1.0"`)
		return &ExitError{Code: 18}
	case "crash_before_response":
		return &ExitError{Code: 17}
	case "stderr_flood":
		_, _ = io.WriteString(diagnostic, strings.Repeat("diagnostic-noise\n", 8192))
	case "terminal_error":
		return respondError(codec, request, "sample_failure")
	case "unsolicited":
		wrong := request
		wrong.Kind = adapters.KindResponse
		wrong.Deadline = nil
		wrong.RequestID = "40000000-0000-4000-8000-000000000099"
		wrong.Payload = json.RawMessage(`{}`)
		if err := codec.Write(wrong); err != nil {
			return err
		}
	case "wrong_operation":
		wrong := request
		wrong.Kind = adapters.KindResponse
		wrong.Deadline = nil
		wrong.Operation = adapters.OperationHealth
		wrong.Payload = json.RawMessage(`{}`)
		if err := codec.Write(wrong); err != nil {
			return err
		}
	}
	payload := collectionPayload(request.SentAt, config)
	if err := respond(codec, request, payload); err != nil {
		return err
	}
	if config.FaultMode == "duplicate_response" {
		return respond(codec, request, payload)
	}
	return nil
}

func collectionPayload(observedAt time.Time, config Config) adapters.CollectionPayload {
	result := adapters.CollectionPayload{
		Resources:    make([]adapters.CollectionResource, config.Count),
		Observations: make([]adapters.CollectionObservation, config.Count),
	}
	state := health.State(config.Scenario)
	for index := 0; index < config.Count; index++ {
		externalID := fmt.Sprintf("sample-node-%02d", index+1)
		result.Resources[index] = adapters.CollectionResource{
			ExternalID: externalID, Kind: "host", DisplayName: fmt.Sprintf("Sample node %02d", index+1),
			ObservedAt: observedAt.UTC(), Attributes: map[string]any{"sample": true, "ordinal": index + 1},
		}
		result.Observations[index] = adapters.CollectionObservation{
			ExternalResourceID: externalID, CheckType: "sample.availability", State: state,
			Summary: fmt.Sprintf("Sample node reports %s.", state), ObservedAt: observedAt.UTC(),
			ExpectedRefreshSeconds: config.ExpectedRefreshSecond, Measurements: map[string]any{"ordinal": index + 1},
			Metadata: map[string]any{"adapter": AdapterID},
		}
	}
	return result
}

func decodeConfig(payload json.RawMessage) (Config, error) {
	var wrapper struct {
		Config json.RawMessage `json:"config"`
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wrapper); err != nil || len(wrapper.Config) == 0 {
		return Config{}, errors.New("invalid config wrapper")
	}
	config := defaultConfig()
	decoder = json.NewDecoder(bytes.NewReader(wrapper.Config))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return Config{}, err
	}
	if !contains(validScenarios(), config.Scenario) || config.Count < 1 || config.Count > 10 ||
		config.DelayMS < 0 || config.DelayMS > 5000 || !contains(validFaults(), config.FaultMode) ||
		config.ExpectedRefreshSecond < 1 || config.ExpectedRefreshSecond > 86400 {
		return Config{}, errors.New("invalid config")
	}
	return config, nil
}

func defaultConfig() Config {
	return Config{Scenario: "healthy", Count: 1, FaultMode: "none", ExpectedRefreshSecond: 300}
}

func validScenarios() []string { return []string{"healthy", "warning", "critical"} }

func validFaults() []string {
	return []string{"none", "malformed", "oversized", "partial", "crash_before_response", "duplicate_response", "stderr_flood", "refuse_shutdown", "terminal_error", "unsolicited", "wrong_operation"}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func respond(codec *adapters.Codec, request adapters.Envelope, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return codec.Write(adapters.Envelope{
		ProtocolVersion: request.ProtocolVersion, Kind: adapters.KindResponse,
		Operation: request.Operation, RequestID: request.RequestID, SentAt: request.SentAt,
		Payload: encoded,
	})
}

func respondError(codec *adapters.Codec, request adapters.Envelope, code string) error {
	return codec.Write(adapters.Envelope{
		ProtocolVersion: request.ProtocolVersion, Kind: adapters.KindResponse,
		Operation: request.Operation, RequestID: request.RequestID, SentAt: request.SentAt,
		Error: &adapters.RemoteError{Code: code, Message: "sample adapter request failed", Retryable: false},
	})
}
