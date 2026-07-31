package auth

import (
	"testing"
	"time"
)

func TestLoginLimiter(t *testing.T) {
	limiter := NewLoginLimiter(2, time.Minute)
	now := time.Unix(100, 0)
	if !limiter.Allow("address", now) || !limiter.Allow("address", now) {
		t.Fatal("allowed attempts were rejected")
	}
	if limiter.Allow("address", now) {
		t.Fatal("excess attempt was allowed")
	}
	if !limiter.Allow("address", now.Add(time.Minute)) {
		t.Fatal("new window was rejected")
	}
}
