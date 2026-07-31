package monitoring

import (
	"errors"
	"testing"

	"github.com/PrincepsVIIII/Espial/core/internal/adapters"
	"github.com/PrincepsVIIII/Espial/core/internal/observations"
)

func TestCollectionFailuresUseStableSafeCategories(t *testing.T) {
	invalid := &adapters.RuntimeError{Code: "invalid_collection"}
	if result, code := classifyCollectionError(invalid), safeCollectionCode(invalid); result != CollectionRejected || code != "invalid_collection" {
		t.Fatalf("invalid collection = %s %s", result, code)
	}
	unsafe := errors.New("remote payload contained a secret")
	if result, code := classifyCollectionError(unsafe), safeCollectionCode(unsafe); result != CollectionFailed || code != "collection_failed" {
		t.Fatalf("unsafe failure = %s %s", result, code)
	}
	validation := &observations.ValidationError{}
	if result, code := classifyIngestionError(validation), safeIngestionCode(validation); result != CollectionRejected || code != "validation_failed" {
		t.Fatalf("validation = %s %s", result, code)
	}
	conflict := &observations.ConflictError{Code: "idempotency_conflict"}
	if result, code := classifyIngestionError(conflict), safeIngestionCode(conflict); result != CollectionRejected || code != "idempotency_conflict" {
		t.Fatalf("conflict = %s %s", result, code)
	}
}
