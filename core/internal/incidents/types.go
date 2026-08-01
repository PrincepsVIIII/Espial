// Package incidents owns incident rules, lifecycle, timeline, and read models.
package incidents

import (
	"errors"
	"time"
)

type Severity string

const (
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

type Status string

const (
	StatusOpen          Status = "open"
	StatusAcknowledged  Status = "acknowledged"
	StatusInvestigating Status = "investigating"
	StatusRecovered     Status = "recovered"
	StatusResolved      Status = "resolved"
)

var (
	ErrNotFound            = errors.New("incident not found")
	ErrInvalidCursor       = errors.New("invalid incident cursor")
	ErrVersionConflict     = errors.New("incident version conflict")
	ErrInvalidTransition   = errors.New("invalid incident transition")
	ErrOwnerNotEligible    = errors.New("incident owner is not eligible")
	ErrInvalidNote         = errors.New("invalid incident note")
	ErrInvalidMutation     = errors.New("invalid incident mutation")
	ErrIdempotencyConflict = errors.New("idempotency key was reused for a different request")
)

const MaxNoteRunes = 2000

type Summary struct {
	ID              string     `json:"id"`
	RuleID          string     `json:"rule_id"`
	RuleName        string     `json:"rule_name"`
	IntegrationID   string     `json:"integration_id"`
	IntegrationName string     `json:"integration_name"`
	ResourceID      string     `json:"resource_id"`
	ResourceName    string     `json:"resource_name"`
	CheckType       string     `json:"check_type"`
	Title           string     `json:"title"`
	Summary         string     `json:"summary"`
	Severity        Severity   `json:"severity"`
	Status          Status     `json:"status"`
	OwnerUserID     string     `json:"owner_user_id,omitempty"`
	OwnerName       string     `json:"owner_name,omitempty"`
	DetectedAt      time.Time  `json:"detected_at"`
	LatestSignalAt  time.Time  `json:"latest_signal_at"`
	AcknowledgedAt  *time.Time `json:"acknowledged_at,omitempty"`
	RecoveredAt     *time.Time `json:"recovered_at,omitempty"`
	ResolvedAt      *time.Time `json:"resolved_at,omitempty"`
	Version         int64      `json:"version"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type Detail struct {
	Summary
	Fingerprint string `json:"fingerprint"`
}

type List struct {
	Items      []Summary `json:"items"`
	NextCursor string    `json:"next_cursor,omitempty"`
}

type TimelineEvent struct {
	ID            string    `json:"id"`
	IncidentID    string    `json:"incident_id"`
	SignalID      string    `json:"signal_id,omitempty"`
	ActorUserID   string    `json:"actor_user_id,omitempty"`
	ActorName     string    `json:"actor_name,omitempty"`
	SubjectUserID string    `json:"subject_user_id,omitempty"`
	SubjectName   string    `json:"subject_name,omitempty"`
	Kind          string    `json:"kind"`
	FromStatus    Status    `json:"from_status,omitempty"`
	ToStatus      Status    `json:"to_status,omitempty"`
	FromSeverity  Severity  `json:"from_severity,omitempty"`
	ToSeverity    Severity  `json:"to_severity,omitempty"`
	Summary       string    `json:"summary"`
	OccurredAt    time.Time `json:"occurred_at"`
}

type Assignee struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

type AssigneeList struct {
	Items      []Assignee `json:"items"`
	NextCursor string     `json:"next_cursor,omitempty"`
}

type Action string

const (
	ActionAcknowledge Action = "acknowledge"
	ActionInvestigate Action = "investigate"
	ActionAssign      Action = "assign"
	ActionNote        Action = "note"
	ActionResolve     Action = "resolve"
)

type Mutation struct {
	IncidentID      string
	Action          Action
	OwnerUserID     string
	Note            string
	ExpectedVersion int64
	IdempotencyKey  string
	ActorUserID     string
	ActorName       string
	SourceAddress   string
	CorrelationID   string
}

type MutationResult struct {
	IncidentID      string `json:"incident_id"`
	Status          Status `json:"status"`
	Version         int64  `json:"version"`
	TimelineEventID string `json:"timeline_event_id"`
	CorrelationID   string `json:"request_id"`
	Replayed        bool   `json:"replayed"`
}

type VersionConflictError struct{ CurrentVersion int64 }

func (err *VersionConflictError) Error() string { return ErrVersionConflict.Error() }
func (err *VersionConflictError) Unwrap() error { return ErrVersionConflict }

type Timeline struct {
	Items      []TimelineEvent `json:"items"`
	NextCursor string          `json:"next_cursor,omitempty"`
}

type Filter struct {
	Limit          int
	Cursor         string
	Severities     []Severity
	Statuses       []Status
	IntegrationIDs []string
	ResourceIDs    []string
	OwnerIDs       []string
	Active         *bool
	From           *time.Time
	To             *time.Time
}

type TimelineFilter struct {
	Limit  int
	Cursor string
}
