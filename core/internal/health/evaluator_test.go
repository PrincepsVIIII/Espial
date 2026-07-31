package health

import (
	"testing"
	"time"
)

func TestEvaluateFreshnessBoundaries(t *testing.T) {
	observed := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	observation := Observation{
		ID: "00000000-0000-4000-8000-000000000001", ResourceID: "resource",
		State: Healthy, Summary: "responding", ObservedAt: observed,
		ReceivedAt: observed, ExpectedRefresh: 5 * time.Minute,
	}

	tests := []struct {
		name  string
		now   time.Time
		state State
	}{
		{"before stale", observed.Add(7*time.Minute + 29*time.Second), Healthy},
		{"at stale", observed.Add(7*time.Minute + 30*time.Second), Stale},
		{"before unknown", observed.Add(15*time.Minute - time.Nanosecond), Stale},
		{"at unknown", observed.Add(15 * time.Minute), Unknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current := Evaluate("resource", observation, nil, test.now)
			if current.State != test.state {
				t.Fatalf("state = %q, want %q", current.State, test.state)
			}
		})
	}
}

func TestTransitionTimesUseMinimumGrace(t *testing.T) {
	observed := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	staleAt, unknownAt := TransitionTimes(Observation{
		ObservedAt: observed, ExpectedRefresh: 10 * time.Second,
	})
	if want := observed.Add(40 * time.Second); !staleAt.Equal(want) {
		t.Fatalf("stale at = %s, want %s", staleAt, want)
	}
	if !unknownAt.Equal(staleAt) {
		t.Fatalf("unknown at = %s, want stale at %s", unknownAt, staleAt)
	}
}

func TestExplicitUnknownRetainsLastSuccess(t *testing.T) {
	lastSuccess := time.Date(2026, 7, 31, 11, 55, 0, 0, time.UTC)
	previous := Current{LastSuccessAt: &lastSuccess}
	observed := lastSuccess.Add(5 * time.Minute)
	current := Evaluate("resource", Observation{
		ID: "00000000-0000-4000-8000-000000000002", State: Unknown,
		Summary: "source cannot determine health", ObservedAt: observed,
		ExpectedRefresh: 5 * time.Minute,
	}, &previous, observed)
	if current.State != Unknown || current.Reason != "source cannot determine health" {
		t.Fatalf("current = %#v", current)
	}
	if current.LastSuccessAt == nil || !current.LastSuccessAt.Equal(lastSuccess) {
		t.Fatalf("last success = %v, want %s", current.LastSuccessAt, lastSuccess)
	}
}

func TestDisabledDoesNotAge(t *testing.T) {
	observed := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	current := Evaluate("resource", Observation{
		ID: "00000000-0000-4000-8000-000000000003", State: Disabled,
		Summary: "collection disabled", ObservedAt: observed,
		ExpectedRefresh: time.Minute,
	}, nil, observed.Add(365*24*time.Hour))
	if current.State != Disabled || current.StaleAt != nil || current.UnknownAt != nil {
		t.Fatalf("disabled current = %#v", current)
	}
}

func TestMaintenanceAgesAndCountsAsSuccess(t *testing.T) {
	observed := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	observation := Observation{
		ID: "00000000-0000-4000-8000-000000000004", State: Maintenance,
		Summary: "maintenance", ObservedAt: observed, ExpectedRefresh: time.Minute,
	}
	current := Evaluate("resource", observation, nil, observed.Add(2*time.Minute))
	if current.State != Stale {
		t.Fatalf("state = %q, want stale", current.State)
	}
	if current.LastSuccessAt == nil || !current.LastSuccessAt.Equal(observed) {
		t.Fatalf("last success = %v, want %s", current.LastSuccessAt, observed)
	}
}

func TestCompareObservationsUsesAllTieBreakers(t *testing.T) {
	base := Observation{ID: "a", ObservedAt: time.Unix(100, 0), ReceivedAt: time.Unix(200, 0)}
	newerObserved := base
	newerObserved.ObservedAt = base.ObservedAt.Add(time.Second)
	if CompareObservations(newerObserved, base) != 1 {
		t.Fatal("newer observed time did not win")
	}
	newerReceived := base
	newerReceived.ReceivedAt = base.ReceivedAt.Add(time.Second)
	if CompareObservations(newerReceived, base) != 1 {
		t.Fatal("newer received time did not win")
	}
	higherID := base
	higherID.ID = "b"
	if CompareObservations(higherID, base) != 1 {
		t.Fatal("higher UUID did not win final tie")
	}
}

func TestNoObservationIsUnknown(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	current := NoObservation("resource", now)
	if current.State != Unknown || current.ObservationID != nil || current.ObservedAt != nil {
		t.Fatalf("current = %#v", current)
	}
}
