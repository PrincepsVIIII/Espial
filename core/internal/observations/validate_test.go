package observations

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/health"
)

func TestValidateBatchAcceptsNormalizedInput(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	if err := ValidateBatch(validBatch(now), now); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestValidateBatchAcceptsEveryObservedState(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	for _, state := range []health.State{
		health.Healthy, health.Warning, health.Critical, health.Unknown,
		health.Maintenance, health.Disabled,
	} {
		t.Run(string(state), func(t *testing.T) {
			batch := validBatch(now)
			batch.Observations[0].State = state
			if err := ValidateBatch(batch, now); err != nil {
				t.Fatalf("validate %q: %v", state, err)
			}
		})
	}
}

func TestValidateBatchAllowsFutureSkewBoundary(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	batch := validBatch(now)
	batch.Resources[0].ObservedAt = now.Add(MaxFutureSkew)
	batch.Observations[0].ObservedAt = now.Add(MaxFutureSkew)
	if err := ValidateBatch(batch, now); err != nil {
		t.Fatalf("validate at skew boundary: %v", err)
	}
	batch.Observations[0].ObservedAt = now.Add(MaxFutureSkew + time.Nanosecond)
	requireFieldCode(t, ValidateBatch(batch, now), "future_skew")
}

func TestValidateBatchRejectsDomainViolations(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		mutate func(*Batch)
		code   string
	}{
		{"resource whitespace", func(batch *Batch) { batch.Resources[0].ExternalID = " node" }, "ambiguous_whitespace"},
		{"resource kind", func(batch *Batch) { batch.Resources[0].Kind = "Host" }, "invalid_format"},
		{"source scheme", func(batch *Batch) { batch.Resources[0].SourceURL = "file:///etc/passwd" }, "invalid_url"},
		{"observation stale state", func(batch *Batch) { batch.Observations[0].State = health.Stale }, "invalid_state"},
		{"refresh zero", func(batch *Batch) { batch.Observations[0].ExpectedRefreshSeconds = 0 }, "out_of_bounds"},
		{"measurement object", func(batch *Batch) { batch.Observations[0].Measurements["bad"] = map[string]any{"nested": true} }, "invalid_value"},
		{"measurement nan", func(batch *Batch) { batch.Observations[0].Measurements["bad"] = math.NaN() }, "invalid_value"},
		{"metadata json", func(batch *Batch) { batch.Observations[0].Metadata["bad"] = make(chan int) }, "invalid_json"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			batch := validBatch(now)
			test.mutate(&batch)
			requireFieldCode(t, ValidateBatch(batch, now), test.code)
		})
	}
}

func TestValidateBatchRejectsLimitsAndDuplicates(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	batch := Batch{Resources: make([]ResourceInput, MaxBatchRecords+1)}
	requireFieldCode(t, ValidateBatch(batch, now), "limit_exceeded")

	batch = validBatch(now)
	batch.Resources = append(batch.Resources, batch.Resources[0])
	requireFieldCode(t, ValidateBatch(batch, now), "duplicate")

	batch = validBatch(now)
	batch.Resources[0].Attributes["large"] = strings.Repeat("x", MaxNestedJSONBytes)
	requireFieldCode(t, ValidateBatch(batch, now), "encoded_size_exceeded")
}

func TestValidateBatchAcceptsAndRejectsExactLimits(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)

	batch := Batch{Resources: make([]ResourceInput, MaxBatchRecords)}
	for index := range batch.Resources {
		batch.Resources[index] = ResourceInput{
			ExternalID: fmt.Sprintf("node-%04d", index), Kind: "host",
			DisplayName: fmt.Sprintf("Node %04d", index), ObservedAt: now,
		}
	}
	if err := ValidateBatch(batch, now); err != nil {
		t.Fatalf("validate exact batch limit: %v", err)
	}

	batch = validBatch(now)
	batch.Observations[0].ExpectedRefreshSeconds = 1
	if err := ValidateBatch(batch, now); err != nil {
		t.Fatalf("validate minimum refresh: %v", err)
	}
	batch.Observations[0].ExpectedRefreshSeconds = 86400
	if err := ValidateBatch(batch, now); err != nil {
		t.Fatalf("validate maximum refresh: %v", err)
	}
	batch.Observations[0].ExpectedRefreshSeconds = 86401
	requireFieldCode(t, ValidateBatch(batch, now), "out_of_bounds")

	batch = validBatch(now)
	batch.Resources[0].Attributes = numberedProperties(MaxResourceProperties)
	batch.Observations[0].Measurements = numberedProperties(MaxObservationProperties)
	batch.Observations[0].Metadata = numberedProperties(MaxObservationProperties)
	if err := ValidateBatch(batch, now); err != nil {
		t.Fatalf("validate exact property limits: %v", err)
	}
	batch.Observations[0].Metadata["over"] = true
	requireFieldCode(t, ValidateBatch(batch, now), "property_limit_exceeded")

	batch = validBatch(now)
	batch.Resources[0].Attributes = map[string]any{
		"x": strings.Repeat("x", MaxNestedJSONBytes-len(`{"x":""}`)),
	}
	if err := ValidateBatch(batch, now); err != nil {
		t.Fatalf("validate exact encoded-size limit: %v", err)
	}
	batch.Resources[0].Attributes["x"] = strings.Repeat("x", MaxNestedJSONBytes-len(`{"x":""}`)+1)
	requireFieldCode(t, ValidateBatch(batch, now), "encoded_size_exceeded")
}

func TestValidateBatchRejectsMissingAndOversizedFields(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		mutate func(*Batch)
		code   string
	}{
		{"bad resource uuid", func(batch *Batch) { batch.Resources[0].ID = "not-a-uuid" }, "invalid_uuid"},
		{"missing external id", func(batch *Batch) { batch.Resources[0].ExternalID = " " }, "required"},
		{"long external id", func(batch *Batch) { batch.Resources[0].ExternalID = strings.Repeat("x", 513) }, "too_long"},
		{"missing display name", func(batch *Batch) { batch.Resources[0].DisplayName = " " }, "required"},
		{"long display name", func(batch *Batch) { batch.Resources[0].DisplayName = strings.Repeat("x", 257) }, "too_long"},
		{"missing resource time", func(batch *Batch) { batch.Resources[0].ObservedAt = time.Time{} }, "required"},
		{"bad observation uuid", func(batch *Batch) { batch.Observations[0].ID = "not-a-uuid" }, "invalid_uuid"},
		{"missing check type", func(batch *Batch) { batch.Observations[0].CheckType = "" }, "invalid_format"},
		{"missing summary", func(batch *Batch) { batch.Observations[0].Summary = " " }, "required"},
		{"long summary", func(batch *Batch) { batch.Observations[0].Summary = strings.Repeat("x", 1025) }, "too_long"},
		{"missing observation time", func(batch *Batch) { batch.Observations[0].ObservedAt = time.Time{} }, "required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			batch := validBatch(now)
			test.mutate(&batch)
			requireFieldCode(t, ValidateBatch(batch, now), test.code)
		})
	}
}

func TestValidationErrorDoesNotEchoPayload(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	batch := validBatch(now)
	batch.Observations[0].Summary = ""
	err := ValidateBatch(batch, now)
	if err == nil || strings.Contains(err.Error(), "sample") {
		t.Fatalf("unsafe validation error: %v", err)
	}
}

func validBatch(now time.Time) Batch {
	return Batch{
		Resources: []ResourceInput{{
			ExternalID: "sample-node-01", Kind: "host", DisplayName: "Sample node 01",
			ObservedAt: now, Attributes: map[string]any{"environment": "sample"},
			SourceURL: "https://example.invalid/nodes/01",
		}},
		Observations: []ObservationInput{{
			ExternalResourceID: "sample-node-01", CheckType: "sample.availability",
			State: health.Healthy, Summary: "Sample node is responding.", ObservedAt: now,
			ExpectedRefreshSeconds: 300, Measurements: map[string]any{"latency_ms": 12.5},
			Metadata: map[string]any{"source": "sample"},
		}},
	}
}

func requireFieldCode(t *testing.T, err error, code string) {
	t.Helper()
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want ValidationError", err)
	}
	for _, field := range validation.Fields {
		if field.Code == code {
			return
		}
	}
	t.Fatalf("field errors = %#v, want code %q", validation.Fields, code)
}

func numberedProperties(count int) map[string]any {
	properties := make(map[string]any, count)
	for index := 0; index < count; index++ {
		properties[fmt.Sprintf("field_%03d", index)] = index
	}
	return properties
}
