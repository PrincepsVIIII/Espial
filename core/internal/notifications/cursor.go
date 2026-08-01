package notifications

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

type pageCursor struct {
	Kind        string    `json:"kind"`
	Fingerprint string    `json:"fingerprint"`
	Timestamp   time.Time `json:"timestamp"`
	ID          string    `json:"id"`
}

func encodePageCursor(value pageCursor) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodePageCursor(raw, kind, fingerprint string) (pageCursor, error) {
	if raw == "" {
		return pageCursor{}, nil
	}
	if len(raw) > 2048 {
		return pageCursor{}, ErrInvalidCursor
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(decoded) > 2048 {
		return pageCursor{}, ErrInvalidCursor
	}
	var value pageCursor
	if err := json.Unmarshal(decoded, &value); err != nil || value.Kind != kind ||
		value.Fingerprint != fingerprint || value.Timestamp.IsZero() ||
		!destinationUUIDPattern.MatchString(value.ID) {
		return pageCursor{}, ErrInvalidCursor
	}
	value.Timestamp = value.Timestamp.UTC()
	return value, nil
}

func deliveryFingerprint(filter DeliveryFilter) string {
	states := make([]string, len(filter.States))
	for index, state := range filter.States {
		states[index] = string(state)
	}
	value := strings.Join([]string{filter.IncidentID, filter.DestinationID, strings.Join(states, ",")}, "|")
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
