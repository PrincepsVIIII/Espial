package audit

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type captureDatabase struct {
	arguments []any
	err       error
}

func (database *captureDatabase) Exec(_ context.Context, _ string, arguments ...any) (pgconn.CommandTag, error) {
	database.arguments = arguments
	return pgconn.CommandTag{}, database.err
}

func TestAppendValidatesAndEncodesSafeEvent(t *testing.T) {
	database := &captureDatabase{}
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	err := Append(context.Background(), database, Event{
		Action: "integration.collection.succeeded", TargetType: "integration",
		TargetID: "id", Result: "succeeded", CorrelationID: "correlation", OccurredAt: now,
		AfterSummary: map[string]any{"resource_count": 2},
	})
	if err != nil || len(database.arguments) != 10 || database.arguments[1] != "integration.collection.succeeded" {
		t.Fatalf("arguments=%#v err=%v", database.arguments, err)
	}
}

func TestAppendRejectsUnsafeFieldsAndSummary(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	base := Event{Action: "valid.action", TargetType: "integration", Result: "succeeded", CorrelationID: "correlation", OccurredAt: now}
	tests := []func(*Event){
		func(event *Event) { event.Action = "Invalid action" },
		func(event *Event) { event.TargetType = "" },
		func(event *Event) { event.Result = "maybe" },
		func(event *Event) { event.CorrelationID = "" },
		func(event *Event) { event.SourceAddress = "not-an-ip" },
		func(event *Event) { event.AfterSummary = map[string]any{"bad": make(chan int)} },
		func(event *Event) { event.AfterSummary = map[string]any{"large": strings.Repeat("x", MaxSummaryBytes)} },
	}
	for index, mutate := range tests {
		event := base
		mutate(&event)
		if err := Append(context.Background(), &captureDatabase{}, event); err == nil {
			t.Fatalf("invalid event %d accepted", index)
		}
	}
}

func TestAppendDoesNotExposeDatabaseErrorDetailsInValidation(t *testing.T) {
	database := &captureDatabase{err: errors.New("database unavailable")}
	err := Append(context.Background(), database, Event{
		Action: "valid.action", TargetType: "integration", Result: "failed",
		CorrelationID: "correlation", OccurredAt: time.Now(),
	})
	if err == nil {
		t.Fatal("database failure succeeded")
	}
}
