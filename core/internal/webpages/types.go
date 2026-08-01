// Package webpages owns website-monitor administration and authoritative website read models.
package webpages

import (
	"errors"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/health"
)

var (
	ErrNotFound            = errors.New("website monitor or webpage not found")
	ErrInvalid             = errors.New("website monitor is invalid")
	ErrConflict            = errors.New("website monitor version conflict")
	ErrIdempotencyConflict = errors.New("website monitor idempotency conflict")
	ErrNotRunning          = errors.New("website monitor is not running")
	ErrInvalidCursor       = errors.New("website list cursor is invalid")
)

type ListFilter struct {
	Limit  int
	Cursor string
}

type SecretHeaderDefinition struct {
	Name            string `json:"name"`
	SecretReference string `json:"secret_reference"`
}

type MonitorDefinition struct {
	DisplayName      string                   `json:"display_name"`
	Enabled          bool                     `json:"enabled"`
	URL              string                   `json:"url"`
	IntervalSeconds  int                      `json:"interval_seconds"`
	TimeoutMS        int                      `json:"timeout_ms"`
	WarningLatencyMS int                      `json:"warning_latency_ms,omitempty"`
	AllowedStatuses  []int                    `json:"allowed_statuses"`
	ContentMatch     string                   `json:"content_match,omitempty"`
	FollowRedirects  bool                     `json:"follow_redirects"`
	MaxRedirects     int                      `json:"max_redirects"`
	SecretHeaders    []SecretHeaderDefinition `json:"secret_headers,omitempty"`
}

type Monitor struct {
	ID                     string    `json:"id"`
	DisplayName            string    `json:"display_name"`
	Enabled                bool      `json:"enabled"`
	URL                    string    `json:"url"`
	IntervalSeconds        int       `json:"interval_seconds"`
	TimeoutMS              int       `json:"timeout_ms"`
	WarningLatencyMS       int       `json:"warning_latency_ms,omitempty"`
	AllowedStatuses        []int     `json:"allowed_statuses"`
	ContentMatchConfigured bool      `json:"content_match_configured"`
	FollowRedirects        bool      `json:"follow_redirects"`
	MaxRedirects           int       `json:"max_redirects"`
	SecretHeaderNames      []string  `json:"secret_header_names"`
	RuntimeState           string    `json:"runtime_state"`
	Version                int64     `json:"version"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type MonitorList struct {
	Items      []Monitor `json:"items"`
	NextCursor string    `json:"next_cursor,omitempty"`
}

type Stages struct {
	Completed  []string `json:"completed"`
	DNSMS      int64    `json:"dns_ms"`
	TCPMS      int64    `json:"tcp_ms"`
	TLSMS      int64    `json:"tls_ms"`
	HTTPMS     int64    `json:"http_ms"`
	TotalMS    int64    `json:"total_ms"`
	HTTPStatus int      `json:"http_status,omitempty"`
	BodyBytes  int64    `json:"body_bytes"`
	Redirects  int      `json:"redirects"`
}

type Summary struct {
	ID               string       `json:"id"`
	MonitorID        string       `json:"monitor_id"`
	DisplayName      string       `json:"display_name"`
	URL              string       `json:"url"`
	State            health.State `json:"state"`
	RawState         health.State `json:"raw_state"`
	Reason           string       `json:"reason"`
	ReasonCode       string       `json:"reason_code,omitempty"`
	ObservedAt       *time.Time   `json:"observed_at,omitempty"`
	UpdatedAt        time.Time    `json:"updated_at"`
	Stages           Stages       `json:"stages"`
	ActiveIncidentID string       `json:"active_incident_id,omitempty"`
}

type Detail struct {
	Summary
	FirstSeenAt time.Time `json:"first_seen_at"`
	LastSeenAt  time.Time `json:"last_seen_at"`
}
type List struct {
	Items      []Summary `json:"items"`
	NextCursor string    `json:"next_cursor,omitempty"`
}

type MutationMetadata struct {
	ExpectedVersion int64
	IdempotencyKey  string
	ActorUserID     string
	ActorName       string
	SourceAddress   string
	CorrelationID   string
}

type Runtime interface {
	RequestCollection(string) bool
	RestartIntegration(string)
}
