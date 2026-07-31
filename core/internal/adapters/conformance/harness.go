// Package conformance provides a reusable black-box check for adapter executables.
package conformance

import (
	"context"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/adapters"
	"github.com/PrincepsVIIII/Espial/core/internal/observations"
)

type Result struct {
	Manifest adapters.Manifest
	Version  string
	Batch    observations.Batch
}

func Run(
	ctx context.Context,
	descriptor adapters.Descriptor,
	integration adapters.Integration,
	resolver adapters.SecretResolver,
	options adapters.ProcessOptions,
	receivedAt time.Time,
) (Result, error) {
	session, err := adapters.StartSession(ctx, descriptor, integration, resolver, options)
	if err != nil {
		return Result{}, err
	}
	defer session.Close(context.Background())
	_, batch, err := session.Collect(ctx, receivedAt)
	if err != nil {
		return Result{}, err
	}
	if err := session.Health(ctx); err != nil {
		return Result{}, err
	}
	if err := session.Close(ctx); err != nil {
		return Result{}, err
	}
	return Result{Manifest: session.Manifest, Version: session.Version, Batch: batch}, nil
}
