package adapters

import (
	"context"
	"time"
)

// Workload owns collection activity while one adapter session remains healthy.
type Workload interface {
	Run(context.Context, Integration, *Session) error
}

type LifecycleObserver interface {
	Starting(context.Context, Integration, time.Time) error
	Healthy(context.Context, Integration, Instance, bool, time.Time) error
	Failed(context.Context, Integration, Instance, time.Time) error
	Stopped(context.Context, Integration, time.Time) error
}
