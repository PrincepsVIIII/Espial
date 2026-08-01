package webpages

import (
	"errors"
	"testing"
	"time"
)

func TestListCursorIsOpaqueBoundedAndEndpointScoped(t *testing.T) {
	want := pageCursor{Kind: "webpages", OrderedAt: time.Date(2026, 8, 1, 12, 0, 0, 123, time.UTC), ID: "71000000-0000-4000-8000-000000000025"}
	encoded, err := encodeCursor(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeCursor(encoded, "webpages")
	if err != nil || got != want {
		t.Fatalf("round trip = %#v, %v", got, err)
	}
	if _, err := decodeCursor(encoded, "website_monitors"); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("cross-endpoint cursor error = %v", err)
	}
	for _, raw := range []string{"not-base64", encoded + "extra"} {
		if _, err := decodeCursor(raw, "webpages"); !errors.Is(err, ErrInvalidCursor) {
			t.Fatalf("invalid cursor %q error = %v", raw, err)
		}
	}
}

func TestListFilterBounds(t *testing.T) {
	filter, err := normalizeFilter(ListFilter{})
	if err != nil || filter.Limit != DefaultPageLimit {
		t.Fatalf("default filter = %#v, %v", filter, err)
	}
	for _, limit := range []int{-1, MaximumPageLimit + 1} {
		if _, err := normalizeFilter(ListFilter{Limit: limit}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("limit %d error = %v", limit, err)
		}
	}
}
