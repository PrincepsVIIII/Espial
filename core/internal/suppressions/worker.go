package suppressions

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Worker struct {
	service *Service
	poll    time.Duration
}

func NewWorker(service *Service, poll time.Duration) *Worker {
	if poll <= 0 {
		poll = time.Second
	}
	return &Worker{service: service, poll: poll}
}

func (worker *Worker) Run(ctx context.Context) error {
	for {
		if _, err := worker.ProcessOnce(ctx); err != nil {
			return err
		}
		timer := time.NewTimer(worker.poll)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// ProcessOnce expires a bounded control batch. Window expiry and its synthetic
// re-evaluation signals commit atomically, making restart recovery idempotent.
func (worker *Worker) ProcessOnce(ctx context.Context) (int, error) {
	now := worker.service.now().UTC().Truncate(time.Microsecond)
	processed := 0
	for processed < 50 {
		found, err := worker.expireWindow(ctx, now)
		if err != nil {
			return processed, err
		}
		if !found {
			break
		}
		processed++
	}
	command, err := worker.service.pool.Exec(ctx, `UPDATE silences SET expired_at=$1,updated_at=$1 WHERE id IN (SELECT id FROM silences WHERE expired_at IS NULL AND expires_at<=$1 ORDER BY expires_at,id LIMIT 50 FOR UPDATE SKIP LOCKED)`, now)
	if err != nil {
		return processed, err
	}
	processed += int(command.RowsAffected())
	return processed, nil
}
func (worker *Worker) expireWindow(ctx context.Context, now time.Time) (bool, error) {
	tx, err := worker.service.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	item, err := scanMaintenance(tx.QueryRow(ctx, maintenanceSelect+` WHERE expired_at IS NULL AND ends_at<=$1 ORDER BY ends_at,id FOR UPDATE SKIP LOCKED LIMIT 1`, now))
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if item.Enabled && item.RevokedAt == nil {
		if err := appendExitSignals(ctx, tx, item, item.EndsAt); err != nil {
			return false, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE maintenance_windows SET expired_at=$2,updated_at=GREATEST(updated_at,$2) WHERE id=$1`, item.ID, now); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	worker.service.publish(item.ID, now)
	return true, nil
}
