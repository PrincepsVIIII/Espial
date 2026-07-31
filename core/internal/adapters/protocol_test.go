package adapters

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

const testRequestID = "40000000-0000-4000-8000-000000000001"

func TestValidateEnvelopeKindMatrix(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	deadline := now.Add(time.Minute)
	tests := []struct {
		name     string
		envelope Envelope
		code     string
	}{
		{"request", Envelope{ProtocolVersion: ProtocolV1, Kind: KindRequest, Operation: OperationCollect, RequestID: testRequestID, SentAt: now, Deadline: &deadline, Payload: json.RawMessage(`{}`)}, ""},
		{"response payload", Envelope{ProtocolVersion: ProtocolV1, Kind: KindResponse, Operation: OperationCollect, RequestID: testRequestID, SentAt: now, Payload: json.RawMessage(`{}`)}, ""},
		{"response error", Envelope{ProtocolVersion: ProtocolV1, Kind: KindResponse, Operation: OperationCollect, RequestID: testRequestID, SentAt: now, Error: &RemoteError{Code: "collection_failed", Message: "safe", Retryable: true}}, ""},
		{"notification", Envelope{ProtocolVersion: ProtocolV1, Kind: KindNotification, Operation: OperationReady, SentAt: now, Payload: json.RawMessage(`{}`)}, ""},
		{"response both", Envelope{ProtocolVersion: ProtocolV1, Kind: KindResponse, Operation: OperationCollect, RequestID: testRequestID, SentAt: now, Payload: json.RawMessage(`{}`), Error: &RemoteError{Code: "bad", Message: "bad"}}, "invalid_envelope"},
		{"response neither", Envelope{ProtocolVersion: ProtocolV1, Kind: KindResponse, Operation: OperationCollect, RequestID: testRequestID, SentAt: now}, "invalid_envelope"},
		{"notification request id", Envelope{ProtocolVersion: ProtocolV1, Kind: KindNotification, Operation: OperationReady, RequestID: testRequestID, SentAt: now}, "invalid_envelope"},
		{"request notification operation", Envelope{ProtocolVersion: ProtocolV1, Kind: KindRequest, Operation: OperationReady, RequestID: testRequestID, SentAt: now, Deadline: &deadline}, "invalid_envelope"},
		{"invalid uuid", Envelope{ProtocolVersion: ProtocolV1, Kind: KindRequest, Operation: OperationHealth, RequestID: "bad", SentAt: now, Deadline: &deadline}, "invalid_envelope"},
		{"invalid version", Envelope{ProtocolVersion: "01.0", Kind: KindNotification, Operation: OperationReady, SentAt: now}, "invalid_protocol_version"},
		{"array payload", Envelope{ProtocolVersion: ProtocolV1, Kind: KindNotification, Operation: OperationReady, SentAt: now, Payload: json.RawMessage(`[]`)}, "invalid_envelope"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateEnvelope(test.envelope)
			if test.code == "" && err != nil {
				t.Fatalf("validate: %v", err)
			}
			if test.code != "" {
				requireRuntimeCode(t, err, test.code)
			}
		})
	}
}

func TestDecodeEnvelopeRejectsUnknownFieldsAndTrailingJSON(t *testing.T) {
	valid := `{"protocol_version":"1.0","kind":"notification","operation":"ready","sent_at":"2026-07-31T12:00:00Z"}`
	if _, err := DecodeEnvelope([]byte(valid)); err != nil {
		t.Fatal(err)
	}
	for _, encoded := range []string{
		strings.TrimSuffix(valid, "}") + `,"surprise":true}`,
		valid + `{}`,
	} {
		_, err := DecodeEnvelope([]byte(encoded))
		requireRuntimeCode(t, err, "invalid_envelope")
	}
}

func TestValidateManifestAndVersionNegotiation(t *testing.T) {
	manifest := validManifest()
	if err := ValidateManifest(manifest); err != nil {
		t.Fatal(err)
	}
	manifest.ProtocolVersions = []string{"1.0", "1.1"}
	version, err := NegotiateVersion([]string{"1.0", "1.1", "2.0"}, manifest.ProtocolVersions)
	if err != nil || version != "1.1" {
		t.Fatalf("version = %q, err = %v", version, err)
	}
	version, err = NegotiateVersion([]string{"1.0"}, []string{"1.0", "1.1"})
	if err != nil || version != "1.0" {
		t.Fatalf("additive version = %q, err = %v", version, err)
	}
	_, err = NegotiateVersion([]string{"1.0"}, []string{"1.1"})
	requireRuntimeCode(t, err, "unsupported_protocol_minor")
	_, err = NegotiateVersion([]string{"1.0"}, []string{"2.0"})
	requireRuntimeCode(t, err, "unsupported_protocol_major")
}

func TestManifestRejectsIdentityDuplicatesAndUnknownFields(t *testing.T) {
	tests := []func(*Manifest){
		func(value *Manifest) { value.AdapterID = "Wrong" },
		func(value *Manifest) { value.ProtocolVersions = []string{"1.0", "1.0"} },
		func(value *Manifest) { value.Capabilities = []string{"root_shell"} },
		func(value *Manifest) { value.ResourceTypes = []string{"Host"} },
		func(value *Manifest) { value.SecretFields = []string{"token", "token"} },
		func(value *Manifest) { value.ConfigSchema = map[string]any{"type": "array"} },
	}
	for index, mutate := range tests {
		manifest := validManifest()
		mutate(&manifest)
		if err := ValidateManifest(manifest); err == nil {
			t.Fatalf("invalid manifest %d accepted", index)
		}
	}
	encoded, _ := json.Marshal(validManifest())
	encoded = append(encoded[:len(encoded)-1], []byte(`,"unknown":true}`)...)
	_, err := DecodeManifest(encoded)
	requireRuntimeCode(t, err, "invalid_manifest")
}

func TestSliceCapabilityBoundaryAllowsOnlyReadOnlyCollection(t *testing.T) {
	manifest := validManifest()
	if err := validateSliceCapabilities(manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Capabilities = []string{"collect", "events"}
	requireRuntimeCode(t, validateSliceCapabilities(manifest), "unsupported_capability")
	manifest = validManifest()
	manifest.ReadOnly = false
	requireRuntimeCode(t, validateSliceCapabilities(manifest), "unsupported_capability")
}

func validManifest() Manifest {
	return Manifest{
		AdapterID: "org.ubnetdef.espial.sample", DisplayName: "Espial sample adapter",
		AdapterVersion: "0.1.0", ProtocolVersions: []string{ProtocolV1},
		IntegrationCategory: "sample", ResourceTypes: []string{"host"},
		CheckTypes: []string{"sample.availability"}, Capabilities: []string{"collect"},
		ReadOnly: true, ConfigSchema: map[string]any{"type": "object"},
	}
}

func requireRuntimeCode(t *testing.T, err error, code string) {
	t.Helper()
	var runtime *RuntimeError
	if !errors.As(err, &runtime) || runtime.Code != code {
		t.Fatalf("error = %v, want runtime code %q", err, code)
	}
}
