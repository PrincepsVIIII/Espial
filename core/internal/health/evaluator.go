package health

import "time"

const minimumGrace = 30 * time.Second

// TransitionTimes returns the persisted freshness schedule for an observation.
func TransitionTimes(observation Observation) (time.Time, time.Time) {
	refresh := observation.ExpectedRefresh
	grace := refresh / 2
	if grace < minimumGrace {
		grace = minimumGrace
	}
	staleAt := observation.ObservedAt.UTC().Add(refresh + grace)
	unknownAt := observation.ObservedAt.UTC().Add(3 * refresh)
	if unknownAt.Before(staleAt) {
		unknownAt = staleAt
	}
	return staleAt, unknownAt
}

// Evaluate derives current health from the newest observation at one injected
// instant. It performs no I/O and never reads the wall clock.
func Evaluate(resourceID string, observation Observation, previous *Current, now time.Time) Current {
	now = now.UTC()
	observedAt := observation.ObservedAt.UTC()
	observationID := observation.ID
	result := Current{
		ResourceID:    resourceID,
		State:         observation.State,
		Reason:        observation.Summary,
		ObservationID: &observationID,
		ObservedAt:    timePointer(observedAt),
		UpdatedAt:     now,
	}
	if previous != nil && previous.LastSuccessAt != nil {
		result.LastSuccessAt = timePointer(previous.LastSuccessAt.UTC())
	}
	if observation.State.PositiveDetermination() &&
		(result.LastSuccessAt == nil || observedAt.After(*result.LastSuccessAt)) {
		result.LastSuccessAt = timePointer(observedAt)
	}

	if observation.State == Disabled {
		return result
	}

	staleAt, unknownAt := TransitionTimes(observation)
	result.StaleAt = timePointer(staleAt)
	result.UnknownAt = timePointer(unknownAt)

	if observation.State == Unknown {
		return result
	}
	if !now.Before(unknownAt) {
		result.State = Unknown
		result.Reason = "observation exceeded the unknown freshness threshold"
		return result
	}
	if !now.Before(staleAt) {
		result.State = Stale
		result.Reason = "observation exceeded the expected refresh window"
	}
	return result
}

func NoObservation(resourceID string, now time.Time) Current {
	return Current{
		ResourceID: resourceID,
		State:      Unknown,
		Reason:     "no valid observation",
		UpdatedAt:  now.UTC(),
	}
}

// CompareObservations returns 1 when left wins, -1 when right wins, and 0 when
// their deterministic ordering keys are identical.
func CompareObservations(left, right Observation) int {
	if left.ObservedAt.After(right.ObservedAt) {
		return 1
	}
	if left.ObservedAt.Before(right.ObservedAt) {
		return -1
	}
	if left.ReceivedAt.After(right.ReceivedAt) {
		return 1
	}
	if left.ReceivedAt.Before(right.ReceivedAt) {
		return -1
	}
	if left.ID > right.ID {
		return 1
	}
	if left.ID < right.ID {
		return -1
	}
	return 0
}

func timePointer(value time.Time) *time.Time {
	value = value.UTC()
	return &value
}
