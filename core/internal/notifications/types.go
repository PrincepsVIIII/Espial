// Package notifications owns destination-independent notification intents,
// delivery attempts, routing, retries, and redacted read models.
package notifications

import (
	"context"
	"errors"
	"time"
)

const DestinationMattermost = "mattermost"

var (
	ErrNotFound            = errors.New("notification destination not found")
	ErrConflict            = errors.New("notification destination version conflict")
	ErrInvalid             = errors.New("invalid notification destination")
	ErrSecretUnavailable   = errors.New("notification secret unavailable")
	ErrNetworkPolicy       = errors.New("notification network policy rejected destination")
	ErrIdempotencyConflict = errors.New("notification idempotency key conflict")
	ErrInvalidCursor       = errors.New("invalid notification cursor")
)

type State string

const (
	StateQueued     State = "queued"
	StateAttempting State = "attempting"
	StateDelivered  State = "delivered"
	StateRetryWait  State = "retry_wait"
	StateFailed     State = "failed"
	StateDeadLetter State = "dead_letter"
	StateSuppressed State = "suppressed"
)

type Destination struct {
	ID              string    `json:"id"`
	DisplayName     string    `json:"display_name"`
	DestinationType string    `json:"destination_type"`
	Enabled         bool      `json:"enabled"`
	Version         int64     `json:"version"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type DestinationList struct {
	Items      []Destination `json:"items"`
	NextCursor string        `json:"next_cursor,omitempty"`
}

type DestinationFilter struct {
	Limit  int
	Cursor string
}

type DestinationDefinition struct {
	DisplayName     string `json:"display_name"`
	DestinationType string `json:"destination_type"`
	Enabled         bool   `json:"enabled"`
	EndpointHost    string `json:"endpoint_host"`
	EndpointPort    int    `json:"endpoint_port"`
	PathPrefix      string `json:"path_prefix"`
	SecretReference string `json:"secret_reference"`
}

type MutationMetadata struct {
	ExpectedVersion int64
	IdempotencyKey  string
	ActorUserID     string
	ActorName       string
	SourceAddress   string
	CorrelationID   string
}

type Delivery struct {
	ID                  string     `json:"id"`
	IncidentID          string     `json:"incident_id,omitempty"`
	IncidentEventID     string     `json:"incident_event_id,omitempty"`
	DestinationID       string     `json:"destination_id"`
	DestinationName     string     `json:"destination_name"`
	DestinationType     string     `json:"destination_type"`
	EventKind           string     `json:"event_kind"`
	Test                bool       `json:"test"`
	State               State      `json:"state"`
	AttemptCount        int        `json:"attempt_count"`
	EventOccurredAt     time.Time  `json:"event_occurred_at"`
	LastAttemptAt       *time.Time `json:"last_attempt_at,omitempty"`
	AvailableAt         time.Time  `json:"available_at"`
	DeliveredAt         *time.Time `json:"delivered_at,omitempty"`
	TerminalAt          *time.Time `json:"terminal_at,omitempty"`
	SuppressedSilenceID string     `json:"suppressed_silence_id,omitempty"`
	LastErrorCode       string     `json:"last_error_code,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type DeliveryList struct {
	Items      []Delivery `json:"items"`
	NextCursor string     `json:"next_cursor,omitempty"`
}

type DeliveryFilter struct {
	Limit         int
	IncidentID    string
	DestinationID string
	States        []State
	Cursor        string
}

type SecretResolver interface {
	Resolve(context.Context, string) (string, error)
}

type SecretResolverFunc func(context.Context, string) (string, error)

func (resolver SecretResolverFunc) Resolve(ctx context.Context, reference string) (string, error) {
	return resolver(ctx, reference)
}

type Target struct {
	Host       string
	Port       int
	PathPrefix string
}

type DestinationValidator interface {
	Validate(context.Context, Target) error
}

type Message struct {
	EventID     string
	IncidentID  string
	Kind        string
	Title       string
	Summary     string
	Severity    string
	Status      string
	OccurredAt  time.Time
	IncidentURL string
	Test        bool
}

type DeliveryRequest struct {
	Target       Target
	WebhookToken string
	Message      Message
}

type DeliveryResult struct {
	Delivered         bool
	Retryable         bool
	HTTPStatus        int
	ErrorCode         string
	ProviderRequestID string
	RetryAfter        time.Duration
}

type Driver interface {
	Deliver(context.Context, DeliveryRequest) DeliveryResult
}

type Metrics struct {
	Queued       int64
	Attempting   int64
	RetryWaiting int64
	Delivered    int64
	Failed       int64
	DeadLetters  int64
	Suppressed   int64
	OldestDueAge time.Duration
	AttemptTotal int64
}
