package incidents

import (
	"testing"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/health"
)

func TestValidateRuleAcceptsBoundedDefaultPolicy(t *testing.T) {
	rule := RuleDefinition{
		Name: "Default health", Priority: 100, CheckType: "availability",
		RecoveryState: health.Healthy, RecoveryMinOccurrences: 2,
		Conditions: []RuleCondition{
			{State: health.Critical, Severity: SeverityCritical, MinOccurrences: 1},
			{State: health.Warning, Severity: SeverityWarning, MinOccurrences: 2, For: time.Minute},
		},
	}
	if err := ValidateRule(rule); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRuleRejectsUnboundedOrAmbiguousPolicy(t *testing.T) {
	valid := RuleDefinition{
		Name: "Health", Priority: 100, RecoveryState: health.Healthy,
		RecoveryMinOccurrences: 2,
		Conditions:             []RuleCondition{{State: health.Critical, Severity: SeverityCritical, MinOccurrences: 1}},
	}
	for name, mutate := range map[string]func(*RuleDefinition){
		"whitespace name": func(rule *RuleDefinition) { rule.Name = " Health" },
		"bad scope":       func(rule *RuleDefinition) { rule.ResourceID = "not-a-uuid" },
		"bad match":       func(rule *RuleDefinition) { rule.CheckType = "availability OR true" },
		"bad recovery":    func(rule *RuleDefinition) { rule.RecoveryMinOccurrences = 0 },
		"duplicate state": func(rule *RuleDefinition) { rule.Conditions = append(rule.Conditions, rule.Conditions[0]) },
		"unbounded delay": func(rule *RuleDefinition) { rule.Conditions[0].For = 31 * 24 * time.Hour },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			candidate.Conditions = append([]RuleCondition(nil), valid.Conditions...)
			mutate(&candidate)
			if err := ValidateRule(candidate); err == nil {
				t.Fatal("invalid rule was accepted")
			}
		})
	}
}
