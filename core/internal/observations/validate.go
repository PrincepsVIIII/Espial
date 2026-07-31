package observations

import (
	"encoding/json"
	"math"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxFutureSkew            = 5 * time.Minute
	MaxBatchRecords          = 1000
	MaxNestedJSONBytes       = 64 * 1024
	MaxResourceProperties    = 128
	MaxObservationProperties = 64
)

var (
	resourceKindPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)
	checkTypePattern    = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,126}$`)
	uuidPattern         = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-8][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
)

func ValidateBatch(batch Batch, receivedAt time.Time) error {
	errors := make([]FieldError, 0)
	if len(batch.Resources)+len(batch.Observations) > MaxBatchRecords {
		errors = append(errors, FieldError{Record: "batch", Index: -1, Field: "records", Code: "limit_exceeded"})
	}

	resources := make(map[string]int, len(batch.Resources))
	for index, resource := range batch.Resources {
		validateResource(resource, index, receivedAt, &errors)
		if prior, exists := resources[resource.ExternalID]; exists {
			errors = append(errors, FieldError{Record: "resource", Index: index, Field: "external_id", Code: "duplicate"})
			_ = prior
		} else {
			resources[resource.ExternalID] = index
		}
	}
	for index, observation := range batch.Observations {
		validateObservation(observation, index, receivedAt, &errors)
	}
	if len(errors) > 0 {
		return &ValidationError{Fields: errors}
	}
	return nil
}

func validateResource(resource ResourceInput, index int, receivedAt time.Time, errors *[]FieldError) {
	if resource.ID != "" && !uuidPattern.MatchString(resource.ID) {
		addFieldError(errors, "resource", index, "id", "invalid_uuid")
	}
	validateIdentity(resource.ExternalID, 512, "resource", index, "external_id", errors)
	if !resourceKindPattern.MatchString(resource.Kind) {
		addFieldError(errors, "resource", index, "kind", "invalid_format")
	}
	validateText(resource.DisplayName, 256, "resource", index, "display_name", errors)
	validateObservedAt(resource.ObservedAt, receivedAt, "resource", index, errors)
	if len(resource.Attributes) > MaxResourceProperties {
		addFieldError(errors, "resource", index, "attributes", "property_limit_exceeded")
	}
	validateJSONSize(resource.Attributes, "resource", index, "attributes", errors)
	if resource.SourceURL != "" {
		parsed, err := url.Parse(resource.SourceURL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || len(resource.SourceURL) > 2048 {
			addFieldError(errors, "resource", index, "source_url", "invalid_url")
		}
	}
}

func validateObservation(observation ObservationInput, index int, receivedAt time.Time, errors *[]FieldError) {
	if observation.ID != "" && !uuidPattern.MatchString(observation.ID) {
		addFieldError(errors, "observation", index, "id", "invalid_uuid")
	}
	validateIdentity(observation.ExternalResourceID, 512, "observation", index, "external_resource_id", errors)
	if !checkTypePattern.MatchString(observation.CheckType) {
		addFieldError(errors, "observation", index, "check_type", "invalid_format")
	}
	if !observation.State.ValidObserved() {
		addFieldError(errors, "observation", index, "state", "invalid_state")
	}
	validateText(observation.Summary, 1024, "observation", index, "summary", errors)
	validateObservedAt(observation.ObservedAt, receivedAt, "observation", index, errors)
	if observation.ExpectedRefreshSeconds < 1 || observation.ExpectedRefreshSeconds > 86400 {
		addFieldError(errors, "observation", index, "expected_refresh_seconds", "out_of_bounds")
	}
	if len(observation.Measurements) > MaxObservationProperties {
		addFieldError(errors, "observation", index, "measurements", "property_limit_exceeded")
	}
	for _, value := range observation.Measurements {
		if !validMeasurement(value) {
			addFieldError(errors, "observation", index, "measurements", "invalid_value")
			break
		}
	}
	validateJSONSize(observation.Measurements, "observation", index, "measurements", errors)
	if len(observation.Metadata) > MaxObservationProperties {
		addFieldError(errors, "observation", index, "metadata", "property_limit_exceeded")
	}
	validateJSONSize(observation.Metadata, "observation", index, "metadata", errors)
}

func validateIdentity(value string, maximum int, record string, index int, field string, errors *[]FieldError) {
	if strings.TrimSpace(value) == "" {
		addFieldError(errors, record, index, field, "required")
		return
	}
	if strings.TrimSpace(value) != value {
		addFieldError(errors, record, index, field, "ambiguous_whitespace")
	}
	if utf8.RuneCountInString(value) > maximum {
		addFieldError(errors, record, index, field, "too_long")
	}
}

func validateText(value string, maximum int, record string, index int, field string, errors *[]FieldError) {
	if strings.TrimSpace(value) == "" {
		addFieldError(errors, record, index, field, "required")
		return
	}
	if utf8.RuneCountInString(value) > maximum {
		addFieldError(errors, record, index, field, "too_long")
	}
}

func validateObservedAt(value, receivedAt time.Time, record string, index int, errors *[]FieldError) {
	if value.IsZero() {
		addFieldError(errors, record, index, "observed_at", "required")
		return
	}
	if value.After(receivedAt.UTC().Add(MaxFutureSkew)) {
		addFieldError(errors, record, index, "observed_at", "future_skew")
	}
}

func validateJSONSize(value map[string]any, record string, index int, field string, errors *[]FieldError) {
	encoded, err := json.Marshal(value)
	if err != nil {
		addFieldError(errors, record, index, field, "invalid_json")
		return
	}
	if len(encoded) > MaxNestedJSONBytes {
		addFieldError(errors, record, index, field, "encoded_size_exceeded")
	}
}

func validMeasurement(value any) bool {
	switch value := value.(type) {
	case string, bool, json.Number,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		return true
	case float32:
		return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
	case float64:
		return !math.IsNaN(value) && !math.IsInf(value, 0)
	default:
		return false
	}
}

func addFieldError(errors *[]FieldError, record string, index int, field, code string) {
	*errors = append(*errors, FieldError{Record: record, Index: index, Field: field, Code: code})
}
