package migrations

import "testing"

func TestAllReturnsOrderedMigrations(t *testing.T) {
	items, err := All()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	if len(items) != 5 {
		t.Fatalf("migration count = %d", len(items))
	}
	for index, item := range items {
		if item.Version != index+1 {
			t.Fatalf("migration %d has version %d", index, item.Version)
		}
		if item.SQL == "" {
			t.Fatalf("migration %d is empty", item.Version)
		}
	}
}
