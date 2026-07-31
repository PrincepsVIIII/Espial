// Package health owns Espial's normalized health and freshness decisions.
package health

import "time"

type State string

const (
	Healthy     State = "healthy"
	Warning     State = "warning"
	Critical    State = "critical"
	Unknown     State = "unknown"
	Stale       State = "stale"
	Maintenance State = "maintenance"
	Disabled    State = "disabled"
)

// ValidObserved reports whether a source may report state directly. Stale is
// intentionally omitted because only Core derives it from freshness.
func (state State) ValidObserved() bool {
	switch state {
	case Healthy, Warning, Critical, Unknown, Maintenance, Disabled:
		return true
	default:
		return false
	}
}

// PositiveDetermination reports whether an observation established a state rather
// than explicitly saying that no trustworthy determination was possible.
func (state State) PositiveDetermination() bool {
	return state.ValidObserved() && state != Unknown
}

type Observation struct {
	ID              string
	ResourceID      string
	State           State
	Summary         string
	ObservedAt      time.Time
	ReceivedAt      time.Time
	ExpectedRefresh time.Duration
}

type Current struct {
	ResourceID    string
	State         State
	Reason        string
	ObservationID *string
	ObservedAt    *time.Time
	LastSuccessAt *time.Time
	StaleAt       *time.Time
	UnknownAt     *time.Time
	UpdatedAt     time.Time
}

type Change struct {
	Before *Current
	After  Current
}
