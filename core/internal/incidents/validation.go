package incidents

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/PrincepsVIIII/Espial/core/internal/health"
)

var (
	ruleIdentifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,126}$`)
	ruleUUIDPattern       = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-8][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
)

type RuleDefinition struct {
	Name                   string
	Priority               int
	IntegrationID          string
	ResourceID             string
	ResourceKind           string
	CheckType              string
	ReasonCode             string
	RecoveryState          health.State
	RecoveryMinOccurrences int
	RecoveryFor            time.Duration
	Conditions             []RuleCondition
}

type RuleCondition struct {
	State          health.State
	Severity       Severity
	MinOccurrences int
	For            time.Duration
}

// ValidateRule applies the same bounded domain accepted by the database before a
// later administrative slice persists a rule.
func ValidateRule(rule RuleDefinition) error {
	if strings.TrimSpace(rule.Name) != rule.Name || rule.Name == "" || utf8.RuneCountInString(rule.Name) > 128 {
		return errors.New("invalid incident rule name")
	}
	if rule.Priority < 0 || rule.Priority > 10000 {
		return errors.New("invalid incident rule priority")
	}
	if rule.IntegrationID != "" && !ruleUUIDPattern.MatchString(rule.IntegrationID) ||
		rule.ResourceID != "" && !ruleUUIDPattern.MatchString(rule.ResourceID) {
		return errors.New("invalid incident rule scope ID")
	}
	for _, value := range []string{rule.ResourceKind, rule.CheckType, rule.ReasonCode} {
		if value != "" && !ruleIdentifierPattern.MatchString(value) {
			return errors.New("invalid incident rule match field")
		}
	}
	if !validRuleState(rule.RecoveryState) || rule.RecoveryMinOccurrences < 1 ||
		rule.RecoveryMinOccurrences > 1000 || rule.RecoveryFor < 0 || rule.RecoveryFor > 30*24*time.Hour {
		return errors.New("invalid incident rule recovery")
	}
	if len(rule.Conditions) < 1 || len(rule.Conditions) > 4 {
		return errors.New("invalid incident rule conditions")
	}
	seen := make(map[health.State]bool, len(rule.Conditions))
	for _, condition := range rule.Conditions {
		if seen[condition.State] || !validConditionState(condition.State) ||
			condition.Severity != SeverityWarning && condition.Severity != SeverityCritical ||
			condition.MinOccurrences < 1 || condition.MinOccurrences > 1000 ||
			condition.For < 0 || condition.For > 30*24*time.Hour {
			return errors.New("invalid incident rule condition")
		}
		seen[condition.State] = true
	}
	return nil
}

func validRuleState(state health.State) bool {
	switch state {
	case health.Healthy, health.Warning, health.Critical, health.Unknown,
		health.Stale, health.Maintenance, health.Disabled:
		return true
	default:
		return false
	}
}

func validConditionState(state health.State) bool {
	switch state {
	case health.Warning, health.Critical, health.Unknown, health.Stale:
		return true
	default:
		return false
	}
}
