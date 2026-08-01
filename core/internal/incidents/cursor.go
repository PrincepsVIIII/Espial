package incidents

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
)

const maxCursorBytes = 2048

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

type cursor struct {
	Kind        string    `json:"k"`
	Fingerprint string    `json:"f"`
	Snapshot    time.Time `json:"s"`
	OrderedAt   time.Time `json:"t"`
	ID          string    `json:"i"`
}

func encodeCursor(value cursor) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeCursor(raw, kind, fingerprint string) (cursor, error) {
	if raw == "" {
		return cursor{}, nil
	}
	if len(raw) > maxCursorBytes {
		return cursor{}, ErrInvalidCursor
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(decoded) > maxCursorBytes {
		return cursor{}, ErrInvalidCursor
	}
	var value cursor
	decoder := json.NewDecoder(strings.NewReader(string(decoded)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil || value.Kind != kind ||
		value.Fingerprint != fingerprint || value.Snapshot.IsZero() ||
		value.OrderedAt.IsZero() || !uuidPattern.MatchString(value.ID) {
		return cursor{}, ErrInvalidCursor
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return cursor{}, ErrInvalidCursor
	}
	return value, nil
}

func fingerprint(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return base64.RawURLEncoding.EncodeToString(digest[:12])
}

func sorted(values []string) string {
	copyValues := append([]string(nil), values...)
	sort.Strings(copyValues)
	return strings.Join(copyValues, ",")
}
