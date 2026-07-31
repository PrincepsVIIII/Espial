// Package observations validates and transactionally ingests normalized resources
// and observations.
package observations

import (
	"fmt"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/health"
)

type ResourceInput struct {
	ID          string
	ExternalID  string
	Kind        string
	DisplayName string
	ObservedAt  time.Time
	Attributes  map[string]any
	SourceURL   string
}

type ObservationInput struct {
	ID                     string
	ExternalResourceID     string
	CheckType              string
	State                  health.State
	Summary                string
	ObservedAt             time.Time
	ExpectedRefreshSeconds int
	Measurements           map[string]any
	Metadata               map[string]any
}

type Batch struct {
	Resources    []ResourceInput
	Observations []ObservationInput
}

type Result struct {
	ResourcesUpserted     int
	ObservationsInserted  int
	DuplicateObservations int
	Changes               []health.Change
}

type FieldError struct {
	Record string
	Index  int
	Field  string
	Code   string
}

type ValidationError struct{ Fields []FieldError }

func (err *ValidationError) Error() string {
	return fmt.Sprintf("batch validation failed with %d field error(s)", len(err.Fields))
}

type ConflictError struct {
	Record string
	Index  int
	Code   string
}

func (err *ConflictError) Error() string {
	return fmt.Sprintf("%s %d conflicts with an existing normalized record", err.Record, err.Index)
}

type Publisher interface {
	PublishCommitted([]health.Change)
}

type PublisherFunc func([]health.Change)

func (publish PublisherFunc) PublishCommitted(changes []health.Change) { publish(changes) }
