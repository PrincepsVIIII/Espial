// Package adapters owns Espial's trusted adapter process boundary.
package adapters

import (
	"encoding/json"
	"fmt"
	"time"
)

const (
	ProtocolV1 = "1.0"

	KindRequest      = "request"
	KindResponse     = "response"
	KindNotification = "notification"

	OperationManifest       = "manifest"
	OperationValidateConfig = "validate_config"
	OperationCollect        = "collect"
	OperationHealth         = "health"
	OperationShutdown       = "shutdown"
	OperationReady          = "ready"
	OperationEvent          = "event"
	OperationLog            = "log"
)

type Envelope struct {
	ProtocolVersion string          `json:"protocol_version"`
	Kind            string          `json:"kind"`
	Operation       string          `json:"operation"`
	RequestID       string          `json:"request_id,omitempty"`
	SentAt          time.Time       `json:"sent_at"`
	Deadline        *time.Time      `json:"deadline,omitempty"`
	Payload         json.RawMessage `json:"payload,omitempty"`
	Error           *RemoteError    `json:"error,omitempty"`
}

type RemoteError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type Manifest struct {
	AdapterID           string         `json:"adapter_id"`
	DisplayName         string         `json:"display_name"`
	AdapterVersion      string         `json:"adapter_version"`
	ProtocolVersions    []string       `json:"protocol_versions"`
	IntegrationCategory string         `json:"integration_category"`
	ResourceTypes       []string       `json:"resource_types,omitempty"`
	CheckTypes          []string       `json:"check_types,omitempty"`
	Capabilities        []string       `json:"capabilities"`
	ReadOnly            bool           `json:"read_only"`
	ConfigSchema        map[string]any `json:"config_schema"`
	SecretFields        []string       `json:"secret_fields,omitempty"`
}

type Descriptor struct {
	AdapterID        string
	Executable       string
	Arguments        []string
	WorkingDirectory string
	Environment      map[string]string
}

type Integration struct {
	ID               string
	AdapterID        string
	ConfigNonsecret  map[string]any
	SecretReferences map[string]string
}

type Instance struct {
	IntegrationID       string
	AdapterVersion      string
	ProtocolVersion     string
	State               string
	LastStartedAt       *time.Time
	LastHealthyAt       *time.Time
	LastStoppedAt       *time.Time
	LastErrorAt         *time.Time
	LastErrorCode       string
	ConsecutiveFailures int
	NextRestartAt       *time.Time
	UpdatedAt           time.Time
}

// RuntimeError intentionally exposes only a stable category. Adapter payloads,
// diagnostics, paths, configuration, and causes never appear in Error().
type RuntimeError struct{ Code string }

func (err *RuntimeError) Error() string { return fmt.Sprintf("adapter runtime failure: %s", err.Code) }

func runtimeError(code string) error { return &RuntimeError{Code: code} }
