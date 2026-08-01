package webpages

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
)

const (
	DefaultPageLimit = 50
	MaximumPageLimit = 200
	maxCursorBytes   = 2048
)

var cursorUUIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

type pageCursor struct {
	Kind      string    `json:"k"`
	OrderedAt time.Time `json:"t"`
	ID        string    `json:"i"`
}

func encodeCursor(value pageCursor) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeCursor(raw, kind string) (pageCursor, error) {
	if raw == "" {
		return pageCursor{}, nil
	}
	if len(raw) > maxCursorBytes {
		return pageCursor{}, ErrInvalidCursor
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(decoded) > maxCursorBytes {
		return pageCursor{}, ErrInvalidCursor
	}
	var value pageCursor
	decoder := json.NewDecoder(strings.NewReader(string(decoded)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil || value.Kind != kind || value.OrderedAt.IsZero() || !cursorUUIDPattern.MatchString(value.ID) {
		return pageCursor{}, ErrInvalidCursor
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return pageCursor{}, ErrInvalidCursor
	}
	return value, nil
}

func normalizeFilter(filter ListFilter) (ListFilter, error) {
	if filter.Limit == 0 {
		filter.Limit = DefaultPageLimit
	}
	if filter.Limit < 1 || filter.Limit > MaximumPageLimit {
		return ListFilter{}, ErrInvalid
	}
	return filter, nil
}
