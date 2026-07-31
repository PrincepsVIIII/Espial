package adapters

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/health"
	"github.com/PrincepsVIIII/Espial/core/internal/observations"
)

type CollectionPayload struct {
	Resources    []CollectionResource    `json:"resources"`
	Observations []CollectionObservation `json:"observations"`
	NextCursor   string                  `json:"next_cursor,omitempty"`
}

type CollectionResource struct {
	ID          string         `json:"id,omitempty"`
	ExternalID  string         `json:"external_id"`
	Kind        string         `json:"kind"`
	DisplayName string         `json:"display_name"`
	ObservedAt  time.Time      `json:"observed_at"`
	Attributes  map[string]any `json:"attributes,omitempty"`
	SourceURL   string         `json:"source_url,omitempty"`
}

type CollectionObservation struct {
	ID                     string         `json:"id,omitempty"`
	ExternalResourceID     string         `json:"external_resource_id"`
	CheckType              string         `json:"check_type"`
	State                  health.State   `json:"state"`
	Summary                string         `json:"summary"`
	ObservedAt             time.Time      `json:"observed_at"`
	ExpectedRefreshSeconds int            `json:"expected_refresh_seconds"`
	Measurements           map[string]any `json:"measurements,omitempty"`
	Metadata               map[string]any `json:"metadata,omitempty"`
}

func DecodeCollection(payload json.RawMessage, receivedAt time.Time) (CollectionPayload, observations.Batch, error) {
	var collection CollectionPayload
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&collection); err != nil {
		return CollectionPayload{}, observations.Batch{}, runtimeError("invalid_collection")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return CollectionPayload{}, observations.Batch{}, runtimeError("invalid_collection")
	}
	batch := observations.Batch{
		Resources:    make([]observations.ResourceInput, len(collection.Resources)),
		Observations: make([]observations.ObservationInput, len(collection.Observations)),
	}
	for index, resource := range collection.Resources {
		batch.Resources[index] = observations.ResourceInput{
			ID: resource.ID, ExternalID: resource.ExternalID, Kind: resource.Kind,
			DisplayName: resource.DisplayName, ObservedAt: resource.ObservedAt,
			Attributes: resource.Attributes, SourceURL: resource.SourceURL,
		}
	}
	for index, observation := range collection.Observations {
		batch.Observations[index] = observations.ObservationInput{
			ID: observation.ID, ExternalResourceID: observation.ExternalResourceID,
			CheckType: observation.CheckType, State: observation.State, Summary: observation.Summary,
			ObservedAt: observation.ObservedAt, ExpectedRefreshSeconds: observation.ExpectedRefreshSeconds,
			Measurements: observation.Measurements, Metadata: observation.Metadata,
		}
	}
	if err := observations.ValidateBatch(batch, receivedAt); err != nil {
		return CollectionPayload{}, observations.Batch{}, runtimeError("invalid_collection")
	}
	return collection, batch, nil
}
