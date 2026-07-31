// Package scheduler owns deterministic, bounded collection dispatch.
package scheduler

import (
	"encoding/binary"
	"hash/fnv"
	"time"
)

type Jitter interface {
	Offset(integrationID string, sequence uint64, interval time.Duration) time.Duration
}

type StableJitter struct{ Seed uint64 }

// Offset returns a startup offset in [0, spread] for sequence zero and a periodic
// offset in [-spread, spread] afterward. Spread is 10% of the interval, capped at
// 30 seconds.
func (jitter StableJitter) Offset(integrationID string, sequence uint64, interval time.Duration) time.Duration {
	spread := interval / 10
	if spread > 30*time.Second {
		spread = 30 * time.Second
	}
	if spread <= 0 {
		return 0
	}
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(integrationID))
	var encoded [16]byte
	binary.BigEndian.PutUint64(encoded[:8], jitter.Seed)
	binary.BigEndian.PutUint64(encoded[8:], sequence)
	_, _ = hash.Write(encoded[:])
	value := hash.Sum64()
	if sequence == 0 {
		return time.Duration(value % uint64(spread+1))
	}
	width := uint64(2*spread + 1)
	return time.Duration(value%width) - spread
}
