package monitoring

import (
	"context"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/adapters"
	"github.com/PrincepsVIIII/Espial/core/internal/audit"
	"github.com/PrincepsVIIII/Espial/core/internal/events"
)

type LifecycleAudit struct {
	writer *audit.Writer
	hub    *events.Hub
}

func NewLifecycleAudit(writer *audit.Writer, hub *events.Hub) *LifecycleAudit {
	return &LifecycleAudit{writer: writer, hub: hub}
}

func (observer *LifecycleAudit) Starting(ctx context.Context, integration adapters.Integration, at time.Time) error {
	return observer.record(ctx, integration, "integration.adapter.starting", "starting", nil, at)
}

func (observer *LifecycleAudit) Healthy(
	ctx context.Context,
	integration adapters.Integration,
	instance adapters.Instance,
	recovered bool,
	at time.Time,
) error {
	action := "integration.adapter.healthy"
	if recovered {
		action = "integration.adapter.recovered"
	}
	return observer.record(ctx, integration, action, "healthy", map[string]any{
		"adapter_version":  instance.AdapterVersion,
		"protocol_version": instance.ProtocolVersion,
		"recovered":        recovered,
	}, at)
}

func (observer *LifecycleAudit) Failed(
	ctx context.Context,
	integration adapters.Integration,
	instance adapters.Instance,
	at time.Time,
) error {
	return observer.record(ctx, integration, "integration.adapter.failed", "unhealthy", map[string]any{
		"error_code":           instance.LastErrorCode,
		"consecutive_failures": instance.ConsecutiveFailures,
	}, at)
}

func (observer *LifecycleAudit) Stopped(ctx context.Context, integration adapters.Integration, at time.Time) error {
	return observer.record(ctx, integration, "integration.adapter.stopped", "stopped", nil, at)
}

func (observer *LifecycleAudit) record(
	ctx context.Context,
	integration adapters.Integration,
	action, state string,
	summary map[string]any,
	at time.Time,
) error {
	correlationID, err := newCorrelationID()
	if err != nil {
		return &Error{Code: "correlation_id_failed"}
	}
	if summary == nil {
		summary = make(map[string]any)
	}
	summary["state"] = state
	if err := observer.writer.Append(ctx, audit.Event{
		Action: action, TargetType: "integration", TargetID: integration.ID,
		Result: "succeeded", CorrelationID: correlationID,
		AfterSummary: summary, OccurredAt: at,
	}); err != nil {
		return err
	}
	observer.hub.Publish(events.Event{
		Kind: events.IntegrationChanged, IntegrationID: integration.ID,
		State: "", Result: state, ChangedAt: at,
	})
	return nil
}
