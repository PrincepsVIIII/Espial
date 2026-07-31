package monitoring

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

const maxCursorBytes = 2048

type pageCursor struct {
	Kind        string    `json:"k"`
	Fingerprint string    `json:"f"`
	Snapshot    time.Time `json:"s"`
	OrderedAt   time.Time `json:"t"`
	ID          string    `json:"i"`
	RangeFrom   time.Time `json:"a,omitempty"`
	RangeTo     time.Time `json:"z,omitempty"`
}

func encodeCursor(cursor pageCursor) (string, error) {
	encoded, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeCursor(raw, kind, fingerprint string) (pageCursor, error) {
	if raw == "" {
		return pageCursor{}, nil
	}
	if len(raw) > maxCursorBytes {
		return pageCursor{}, errors.New("invalid cursor")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(decoded) > maxCursorBytes {
		return pageCursor{}, errors.New("invalid cursor")
	}
	var cursor pageCursor
	decoder := json.NewDecoder(strings.NewReader(string(decoded)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cursor); err != nil || cursor.Kind != kind ||
		cursor.Fingerprint != fingerprint || cursor.Snapshot.IsZero() ||
		cursor.OrderedAt.IsZero() || !integrationUUIDPattern.MatchString(cursor.ID) {
		return pageCursor{}, errors.New("invalid cursor")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return pageCursor{}, errors.New("invalid cursor")
	}
	return cursor, nil
}

func filterFingerprint(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return base64.RawURLEncoding.EncodeToString(digest[:12])
}
