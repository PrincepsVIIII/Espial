package suppressions

import (
	"errors"
	"testing"
	"time"
)

func TestMaintenanceAndSilenceScopeValidation(t *testing.T) {
	start := time.Now().UTC()
	validWindow := MaintenanceDefinition{Reason: "Planned work", CheckType: "availability", StartsAt: start, EndsAt: start.Add(time.Hour), Enabled: true}
	if err := ValidateMaintenance(validWindow); err != nil {
		t.Fatalf("valid maintenance: %v", err)
	}
	for name, mutate := range map[string]func(*MaintenanceDefinition){
		"empty scope":    func(value *MaintenanceDefinition) { value.CheckType = "" },
		"reversed range": func(value *MaintenanceDefinition) { value.EndsAt = value.StartsAt },
		"invalid check":  func(value *MaintenanceDefinition) { value.CheckType = "Availability" },
	} {
		t.Run(name, func(t *testing.T) {
			value := validWindow
			mutate(&value)
			if !errors.Is(ValidateMaintenance(value), ErrInvalid) {
				t.Fatal("invalid maintenance was accepted")
			}
		})
	}

	validSilence := SilenceDefinition{Reason: "Known noise", IncidentID: "50000000-0000-4000-8000-000000000001", StartsAt: start, ExpiresAt: start.Add(time.Hour), Enabled: true}
	if err := ValidateSilence(validSilence); err != nil {
		t.Fatalf("valid silence: %v", err)
	}
	validSilence.ResourceID = "60000000-0000-4000-8000-000000000001"
	if !errors.Is(ValidateSilence(validSilence), ErrInvalid) {
		t.Fatal("multi-scope silence was accepted")
	}
}
