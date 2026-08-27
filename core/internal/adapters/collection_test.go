package adapters

import (
	"errors"
	"testing"
	"time"
)

func TestDecodeCollectionReportsSafeStructuralValidationCategory(t *testing.T) {
	payload := []byte(`{
		"resources": [
			{"external_id":"duplicate","kind":"storage","display_name":"first","observed_at":"2026-08-27T00:00:00Z"},
			{"external_id":"duplicate","kind":"storage","display_name":"second","observed_at":"2026-08-27T00:00:00Z"}
		],
		"observations": []
	}`)
	_, _, err := DecodeCollection(payload, time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC))
	var runtime *RuntimeError
	if !errors.As(err, &runtime) {
		t.Fatalf("DecodeCollection error = %v", err)
	}
	if runtime.Code != "invalid_collection_resource_external_id_duplicate" {
		t.Fatalf("runtime code = %q", runtime.Code)
	}
}
