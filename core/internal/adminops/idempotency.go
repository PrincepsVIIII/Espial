// Package adminops contains shared administrative mutation contracts.
package adminops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

var ErrIdempotencyConflict = errors.New("idempotency key already used for another request")

type Receipt struct {
	ID        string `json:"id"`
	Version   int64  `json:"version"`
	RequestID string `json:"request_id"`
	Replayed  bool   `json:"replayed"`
	AuditURL  string `json:"audit_url,omitempty"`
}

func Hash(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode idempotent request: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

// Replay serializes uses of a key and returns an earlier result when the request
// hash is identical. Callers must invoke Save before committing a new mutation.
func Replay(ctx context.Context, tx pgx.Tx, actorID, targetType, operation, key, hash string) (Receipt, bool, error) {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, actorID+":"+targetType+":"+operation+":"+key); err != nil {
		return Receipt{}, false, fmt.Errorf("lock idempotency key: %w", err)
	}
	var storedHash string
	var result Receipt
	err := tx.QueryRow(ctx, `
		SELECT request_hash, target_id::text, result_version, correlation_id
		FROM administrative_mutation_idempotency
		WHERE actor_user_id = $1 AND target_type = $2 AND operation = $3 AND idempotency_key = $4
	`, actorID, targetType, operation, key).Scan(&storedHash, &result.ID, &result.Version, &result.RequestID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Receipt{}, false, nil
	}
	if err != nil {
		return Receipt{}, false, fmt.Errorf("read idempotency result: %w", err)
	}
	if storedHash != hash {
		return Receipt{}, false, ErrIdempotencyConflict
	}
	result.Replayed = true
	return result, true, nil
}

func Save(ctx context.Context, tx pgx.Tx, actorID, targetType, operation, key, hash string, result Receipt) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO administrative_mutation_idempotency (
			actor_user_id, target_type, operation, idempotency_key, request_hash,
			target_id, result_version, correlation_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, actorID, targetType, operation, key, hash, result.ID, result.Version, result.RequestID)
	if err != nil {
		return fmt.Errorf("save idempotency result: %w", err)
	}
	return nil
}
