package adapters

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

var (
	uuidPattern       = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-8][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
	identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9.-]{0,126}$`)
	kindPattern       = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)
	checkPattern      = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,126}$`)
	errorCodePattern  = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,126}$`)
)

func DecodeEnvelope(data []byte) (Envelope, error) {
	var envelope Envelope
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return Envelope{}, runtimeError("invalid_envelope")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Envelope{}, runtimeError("invalid_envelope")
	}
	if err := ValidateEnvelope(envelope); err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}

func ValidateEnvelope(envelope Envelope) error {
	if _, _, ok := parseVersion(envelope.ProtocolVersion); !ok {
		return runtimeError("invalid_protocol_version")
	}
	if envelope.SentAt.IsZero() {
		return runtimeError("invalid_envelope")
	}
	if !validOperation(envelope.Operation) {
		return runtimeError("unsupported_operation")
	}
	payload := len(envelope.Payload) > 0
	if payload && !validJSONObject(envelope.Payload) {
		return runtimeError("invalid_envelope")
	}
	switch envelope.Kind {
	case KindRequest:
		if !uuidPattern.MatchString(envelope.RequestID) || envelope.Deadline == nil ||
			envelope.Deadline.IsZero() || envelope.Error != nil || !requestOperation(envelope.Operation) {
			return runtimeError("invalid_envelope")
		}
	case KindResponse:
		if !uuidPattern.MatchString(envelope.RequestID) || envelope.Deadline != nil ||
			!requestOperation(envelope.Operation) || payload == (envelope.Error != nil) {
			return runtimeError("invalid_envelope")
		}
	case KindNotification:
		if envelope.RequestID != "" || envelope.Deadline != nil || envelope.Error != nil ||
			!notificationOperation(envelope.Operation) {
			return runtimeError("invalid_envelope")
		}
	default:
		return runtimeError("invalid_envelope")
	}
	if envelope.Error != nil {
		if !errorCodePattern.MatchString(envelope.Error.Code) ||
			strings.TrimSpace(envelope.Error.Message) == "" ||
			utf8.RuneCountInString(envelope.Error.Message) > 1024 {
			return runtimeError("invalid_envelope")
		}
	}
	return nil
}

func DecodeManifest(payload json.RawMessage) (Manifest, error) {
	var manifest Manifest
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, runtimeError("invalid_manifest")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Manifest{}, runtimeError("invalid_manifest")
	}
	if err := ValidateManifest(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func ValidateManifest(manifest Manifest) error {
	if !identifierPattern.MatchString(manifest.AdapterID) ||
		strings.TrimSpace(manifest.DisplayName) == "" || utf8.RuneCountInString(manifest.DisplayName) > 128 ||
		strings.TrimSpace(manifest.AdapterVersion) == "" || utf8.RuneCountInString(manifest.AdapterVersion) > 64 ||
		!kindPattern.MatchString(manifest.IntegrationCategory) || len(manifest.ProtocolVersions) == 0 ||
		len(manifest.Capabilities) == 0 || manifest.ConfigSchema == nil {
		return runtimeError("invalid_manifest")
	}
	if duplicateOrInvalid(manifest.ProtocolVersions, func(value string) bool {
		_, _, ok := parseVersion(value)
		return ok
	}) || duplicateOrInvalid(manifest.ResourceTypes, kindPattern.MatchString) ||
		duplicateOrInvalid(manifest.CheckTypes, checkPattern.MatchString) ||
		duplicateOrInvalid(manifest.SecretFields, func(value string) bool {
			return strings.TrimSpace(value) == value && value != "" && utf8.RuneCountInString(value) <= 128
		}) || duplicateOrInvalid(manifest.Capabilities, validCapability) {
		return runtimeError("invalid_manifest")
	}
	encodedSchema, err := json.Marshal(manifest.ConfigSchema)
	if err != nil || len(encodedSchema) > 64*1024 || manifest.ConfigSchema["type"] != "object" {
		return runtimeError("invalid_manifest")
	}
	return nil
}

func validJSONObject(value json.RawMessage) bool {
	var object map[string]any
	return json.Unmarshal(value, &object) == nil && object != nil
}

func NegotiateVersion(core, adapter []string) (string, error) {
	common := make([]string, 0)
	coreSet := make(map[string]struct{}, len(core))
	majorOne := false
	for _, version := range core {
		major, _, ok := parseVersion(version)
		if !ok {
			return "", runtimeError("invalid_protocol_version")
		}
		coreSet[version] = struct{}{}
		majorOne = majorOne || major == 1
	}
	adapterMajorOne := false
	for _, version := range adapter {
		major, _, ok := parseVersion(version)
		if !ok {
			return "", runtimeError("invalid_manifest")
		}
		adapterMajorOne = adapterMajorOne || major == 1
		if _, exists := coreSet[version]; exists {
			common = append(common, version)
		}
	}
	if len(common) == 0 {
		if !majorOne || !adapterMajorOne {
			return "", runtimeError("unsupported_protocol_major")
		}
		return "", runtimeError("unsupported_protocol_minor")
	}
	sort.Slice(common, func(i, j int) bool {
		majorI, minorI, _ := parseVersion(common[i])
		majorJ, minorJ, _ := parseVersion(common[j])
		return majorI > majorJ || majorI == majorJ && minorI > minorJ
	})
	return common[0], nil
}

func parseVersion(value string) (int, int, bool) {
	majorText, minorText, found := strings.Cut(value, ".")
	if !found || strings.Contains(minorText, ".") || majorText == "" || minorText == "" ||
		(len(majorText) > 1 && majorText[0] == '0') || (len(minorText) > 1 && minorText[0] == '0') {
		return 0, 0, false
	}
	major, majorErr := strconv.Atoi(majorText)
	minor, minorErr := strconv.Atoi(minorText)
	return major, minor, majorErr == nil && minorErr == nil && major >= 0 && minor >= 0
}

func validOperation(operation string) bool {
	return requestOperation(operation) || notificationOperation(operation)
}

func requestOperation(operation string) bool {
	switch operation {
	case OperationManifest, OperationValidateConfig, OperationCollect, OperationHealth, OperationShutdown:
		return true
	default:
		return false
	}
}

func notificationOperation(operation string) bool {
	return operation == OperationReady || operation == OperationEvent || operation == OperationLog
}

func validCapability(value string) bool {
	switch value {
	case "collect", "events", "notifications", "actions":
		return true
	default:
		return false
	}
}

func duplicateOrInvalid(values []string, valid func(string) bool) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !valid(value) {
			return true
		}
		if _, exists := seen[value]; exists {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}
