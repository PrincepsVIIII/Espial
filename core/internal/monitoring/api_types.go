package monitoring

import (
	"encoding/json"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/health"
)

type StateCount struct {
	State health.State `json:"state"`
	Count int64        `json:"count"`
}

type IntegrationStateCount struct {
	State string `json:"state"`
	Count int64  `json:"count"`
}

type RecentStateChange struct {
	ResourceID    string       `json:"resource_id"`
	IntegrationID string       `json:"integration_id"`
	DisplayName   string       `json:"display_name"`
	State         health.State `json:"state"`
	Reason        string       `json:"reason"`
	ChangedAt     time.Time    `json:"changed_at"`
}

type ActiveIncidentCount struct {
	Severity string `json:"severity"`
	Count    int64  `json:"count"`
}

type ActiveIncidentSummary struct {
	ID              string    `json:"id"`
	Title           string    `json:"title"`
	Severity        string    `json:"severity"`
	Status          string    `json:"status"`
	IntegrationName string    `json:"integration_name"`
	ResourceName    string    `json:"resource_name"`
	DetectedAt      time.Time `json:"detected_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type CertificateWarningSummary struct {
	Warning  int64 `json:"warning"`
	Critical int64 `json:"critical"`
	Unknown  int64 `json:"unknown"`
}

type Overview struct {
	GeneratedAt          time.Time                 `json:"generated_at"`
	ResourceCounts       []StateCount              `json:"resource_counts"`
	IntegrationCounts    []IntegrationStateCount   `json:"integration_counts"`
	StaleCount           int64                     `json:"stale_count"`
	UnknownCount         int64                     `json:"unknown_count"`
	RecentChanges        []RecentStateChange       `json:"recent_state_changes"`
	ActiveIncidentCounts []ActiveIncidentCount     `json:"active_incident_counts"`
	ActiveIncidents      []ActiveIncidentSummary   `json:"active_incidents"`
	CertificateWarnings  CertificateWarningSummary `json:"certificate_warnings"`
}

type CurrentHealthView struct {
	State         health.State           `json:"state"`
	Reason        string                 `json:"reason"`
	RawState      health.State           `json:"raw_state"`
	RawReason     string                 `json:"raw_reason"`
	Maintenance   *MaintenanceHealthView `json:"maintenance,omitempty"`
	ObservationID string                 `json:"observation_id,omitempty"`
	ObservedAt    *time.Time             `json:"observed_at,omitempty"`
	LastSuccessAt *time.Time             `json:"last_success_at,omitempty"`
	StaleAt       *time.Time             `json:"stale_at,omitempty"`
	UnknownAt     *time.Time             `json:"unknown_at,omitempty"`
	UpdatedAt     time.Time              `json:"updated_at"`
}

type MaintenanceHealthView struct {
	ID     string    `json:"id"`
	Reason string    `json:"reason"`
	EndsAt time.Time `json:"ends_at"`
}

type ObservationView struct {
	ID                     string          `json:"id"`
	CheckType              string          `json:"check_type"`
	State                  health.State    `json:"state"`
	Summary                string          `json:"summary"`
	ObservedAt             time.Time       `json:"observed_at"`
	ReceivedAt             time.Time       `json:"received_at"`
	ExpectedRefreshSeconds int             `json:"expected_refresh_seconds"`
	Measurements           json.RawMessage `json:"measurements"`
	Metadata               json.RawMessage `json:"metadata"`
}

type ResourceView struct {
	ID                string            `json:"id"`
	IntegrationID     string            `json:"integration_id"`
	IntegrationName   string            `json:"integration_name"`
	ExternalID        string            `json:"external_id"`
	Kind              string            `json:"kind"`
	DisplayName       string            `json:"display_name"`
	Attributes        json.RawMessage   `json:"attributes"`
	SourceURL         string            `json:"source_url,omitempty"`
	FirstSeenAt       time.Time         `json:"first_seen_at"`
	LastSeenAt        time.Time         `json:"last_seen_at"`
	Health            CurrentHealthView `json:"health"`
	LatestObservation *ObservationView  `json:"latest_observation,omitempty"`
}

type ResourceList struct {
	Items      []ResourceView `json:"items"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

type ResourceFilter struct {
	Limit          int
	Cursor         string
	States         []health.State
	Kinds          []string
	IntegrationIDs []string
	Stale          *bool
}

type AdapterInstanceView struct {
	AdapterVersion      string     `json:"adapter_version"`
	ProtocolVersion     string     `json:"protocol_version,omitempty"`
	State               string     `json:"state"`
	LastStartedAt       *time.Time `json:"last_started_at,omitempty"`
	LastHealthyAt       *time.Time `json:"last_healthy_at,omitempty"`
	LastStoppedAt       *time.Time `json:"last_stopped_at,omitempty"`
	LastErrorAt         *time.Time `json:"last_error_at,omitempty"`
	LastErrorCode       string     `json:"last_error_code,omitempty"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	NextRestartAt       *time.Time `json:"next_restart_at,omitempty"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type CollectionRunView struct {
	StartedAt             time.Time `json:"started_at"`
	CompletedAt           time.Time `json:"completed_at"`
	DurationMS            int64     `json:"duration_ms"`
	Result                string    `json:"result"`
	ErrorCode             string    `json:"error_code,omitempty"`
	ResourceCount         int       `json:"resource_count"`
	ObservationCount      int       `json:"observation_count"`
	ObservationsInserted  int       `json:"observations_inserted"`
	DuplicateObservations int       `json:"duplicate_observations"`
}

type IntegrationView struct {
	ID                  string               `json:"id"`
	AdapterID           string               `json:"adapter_id"`
	DisplayName         string               `json:"display_name"`
	Enabled             bool                 `json:"enabled"`
	IntervalSeconds     int                  `json:"interval_seconds"`
	ConfigKeys          []string             `json:"config_keys"`
	SecretReferenceKeys []string             `json:"secret_reference_keys"`
	RuntimeState        string               `json:"runtime_state"`
	ResourceCount       int64                `json:"resource_count"`
	StaleCount          int64                `json:"stale_count"`
	UnknownCount        int64                `json:"unknown_count"`
	Instance            *AdapterInstanceView `json:"instance,omitempty"`
	LastCollection      *CollectionRunView   `json:"last_collection,omitempty"`
	CreatedAt           time.Time            `json:"created_at"`
	UpdatedAt           time.Time            `json:"updated_at"`
}

type IntegrationList struct {
	Items      []IntegrationView `json:"items"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

type IntegrationFilter struct {
	Limit         int
	Cursor        string
	AdapterIDs    []string
	RuntimeStates []string
	Enabled       *bool
}

type AuditEventView struct {
	ID            string          `json:"id"`
	ActorUserID   string          `json:"actor_user_id,omitempty"`
	ActorUsername string          `json:"actor_username,omitempty"`
	Action        string          `json:"action"`
	TargetType    string          `json:"target_type"`
	TargetID      string          `json:"target_id,omitempty"`
	Result        string          `json:"result"`
	SourceAddress string          `json:"source_address,omitempty"`
	CorrelationID string          `json:"correlation_id"`
	BeforeSummary json.RawMessage `json:"before_summary,omitempty"`
	AfterSummary  json.RawMessage `json:"after_summary,omitempty"`
	OccurredAt    time.Time       `json:"occurred_at"`
}

type AuditList struct {
	Items      []AuditEventView `json:"items"`
	NextCursor string           `json:"next_cursor,omitempty"`
	From       time.Time        `json:"from"`
	To         time.Time        `json:"to"`
}

type AuditFilter struct {
	Limit         int
	Cursor        string
	From          time.Time
	To            time.Time
	FromExplicit  bool
	ToExplicit    bool
	Actions       []string
	Results       []string
	TargetTypes   []string
	ActorUserID   string
	CorrelationID string
}

type CreateIntegration struct {
	AdapterID        string
	DisplayName      string
	Enabled          bool
	Interval         time.Duration
	ConfigNonsecret  map[string]any
	SecretReferences map[string]string
	ActorUserID      string
	SourceAddress    string
	CorrelationID    string
}
