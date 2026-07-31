package api

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/health"
	"github.com/PrincepsVIIII/Espial/core/internal/monitoring"
)

const (
	maxQueryBytes   = 8192
	maxFilterValues = 32
)

var (
	uuidPattern       = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-8][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
	identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,126}$`)
)

type queryErrors []APIFieldError

func parseResourceFilter(values url.Values) (monitoring.ResourceFilter, queryErrors) {
	allowed := map[string]bool{"limit": true, "cursor": true, "state": true, "kind": true, "integration": true, "stale": true}
	if fields := rejectUnknownQuery(values, allowed); len(fields) > 0 {
		return monitoring.ResourceFilter{}, fields
	}
	limit, fields := parseLimit(values)
	filter := monitoring.ResourceFilter{Limit: limit}
	if cursor, field := singleValue(values, "cursor", 2048); field != nil {
		fields = append(fields, *field)
	} else {
		filter.Cursor = cursor
	}
	if len(values["state"]) > maxFilterValues {
		fields = append(fields, APIFieldError{Field: "state", Code: "too_many"})
	} else {
		seen := map[health.State]bool{}
		for _, raw := range values["state"] {
			state := health.State(raw)
			if !validReadState(state) {
				fields = append(fields, APIFieldError{Field: "state", Code: "invalid"})
				continue
			}
			if !seen[state] {
				seen[state] = true
				filter.States = append(filter.States, state)
			}
		}
	}
	filter.Kinds, fields = parseRepeated(values, "kind", identifierPattern.MatchString, fields)
	filter.IntegrationIDs, fields = parseRepeated(values, "integration", uuidPattern.MatchString, fields)
	if raw, field := singleValue(values, "stale", 5); field != nil {
		fields = append(fields, *field)
	} else if raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			fields = append(fields, APIFieldError{Field: "stale", Code: "invalid"})
		} else {
			filter.Stale = &parsed
		}
	}
	return filter, fields
}

func parseIntegrationFilter(values url.Values) (monitoring.IntegrationFilter, queryErrors) {
	allowed := map[string]bool{"limit": true, "cursor": true, "adapter_id": true, "runtime_state": true, "enabled": true}
	if fields := rejectUnknownQuery(values, allowed); len(fields) > 0 {
		return monitoring.IntegrationFilter{}, fields
	}
	limit, fields := parseLimit(values)
	filter := monitoring.IntegrationFilter{Limit: limit}
	if cursor, field := singleValue(values, "cursor", 2048); field != nil {
		fields = append(fields, *field)
	} else {
		filter.Cursor = cursor
	}
	filter.AdapterIDs, fields = parseRepeated(values, "adapter_id", identifierPattern.MatchString, fields)
	validRuntime := func(value string) bool {
		switch value {
		case "starting", "healthy", "unhealthy", "stopped", "disabled", "not_started":
			return true
		default:
			return false
		}
	}
	filter.RuntimeStates, fields = parseRepeated(values, "runtime_state", validRuntime, fields)
	if raw, field := singleValue(values, "enabled", 5); field != nil {
		fields = append(fields, *field)
	} else if raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			fields = append(fields, APIFieldError{Field: "enabled", Code: "invalid"})
		} else {
			filter.Enabled = &parsed
		}
	}
	return filter, fields
}

func parseAuditFilter(values url.Values, now time.Time) (monitoring.AuditFilter, queryErrors) {
	allowed := map[string]bool{
		"limit": true, "cursor": true, "from": true, "to": true, "action": true,
		"result": true, "target_type": true, "actor_user_id": true, "correlation_id": true,
	}
	if fields := rejectUnknownQuery(values, allowed); len(fields) > 0 {
		return monitoring.AuditFilter{}, fields
	}
	limit, fields := parseLimit(values)
	filter := monitoring.AuditFilter{Limit: limit, To: now.UTC(), From: now.UTC().Add(-24 * time.Hour)}
	if cursor, field := singleValue(values, "cursor", 2048); field != nil {
		fields = append(fields, *field)
	} else {
		filter.Cursor = cursor
	}
	for name, destination := range map[string]struct {
		value *time.Time
		set   *bool
	}{
		"from": {&filter.From, &filter.FromExplicit},
		"to":   {&filter.To, &filter.ToExplicit},
	} {
		raw, field := singleValue(values, name, 64)
		if field != nil {
			fields = append(fields, *field)
			continue
		}
		if raw == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			fields = append(fields, APIFieldError{Field: name, Code: "invalid"})
			continue
		}
		*destination.value = parsed.UTC()
		*destination.set = true
	}
	if !filter.From.Before(filter.To) || filter.To.Sub(filter.From) > monitoring.MaximumAuditRange || filter.To.After(now.Add(time.Minute)) {
		fields = append(fields, APIFieldError{Field: "from", Code: "invalid_range"})
	}
	filter.Actions, fields = parseRepeated(values, "action", identifierPattern.MatchString, fields)
	validResult := func(value string) bool { return value == "succeeded" || value == "failed" || value == "denied" }
	filter.Results, fields = parseRepeated(values, "result", validResult, fields)
	filter.TargetTypes, fields = parseRepeated(values, "target_type", identifierPattern.MatchString, fields)
	if actor, field := singleValue(values, "actor_user_id", 36); field != nil {
		fields = append(fields, *field)
	} else if actor != "" && !uuidPattern.MatchString(actor) {
		fields = append(fields, APIFieldError{Field: "actor_user_id", Code: "invalid"})
	} else {
		filter.ActorUserID = actor
	}
	if correlation, field := singleValue(values, "correlation_id", 128); field != nil {
		fields = append(fields, *field)
	} else {
		filter.CorrelationID = correlation
	}
	return filter, fields
}

func parseLimit(values url.Values) (int, queryErrors) {
	raw, field := singleValue(values, "limit", 3)
	if field != nil {
		return 0, queryErrors{*field}
	}
	if raw == "" {
		return monitoring.DefaultPageLimit, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > monitoring.MaximumPageLimit {
		return 0, queryErrors{{Field: "limit", Code: "out_of_range"}}
	}
	return limit, nil
}

func singleValue(values url.Values, name string, maximum int) (string, *APIFieldError) {
	items := values[name]
	if len(items) > 1 {
		return "", &APIFieldError{Field: name, Code: "multiple"}
	}
	if len(items) == 0 {
		return "", nil
	}
	value := strings.TrimSpace(items[0])
	if len(value) > maximum {
		return "", &APIFieldError{Field: name, Code: "too_long"}
	}
	return value, nil
}

func parseRepeated(values url.Values, name string, valid func(string) bool, fields queryErrors) ([]string, queryErrors) {
	if len(values[name]) > maxFilterValues {
		return nil, append(fields, APIFieldError{Field: name, Code: "too_many"})
	}
	result := make([]string, 0, len(values[name]))
	seen := make(map[string]bool, len(values[name]))
	for _, raw := range values[name] {
		value := strings.TrimSpace(raw)
		if len(value) > 128 || !valid(value) {
			fields = append(fields, APIFieldError{Field: name, Code: "invalid"})
			continue
		}
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result, fields
}

func rejectUnknownQuery(values url.Values, allowed map[string]bool) queryErrors {
	var fields queryErrors
	for name := range values {
		if !allowed[name] {
			fields = append(fields, APIFieldError{Field: name, Code: "unknown"})
		}
	}
	return fields
}

func validReadState(state health.State) bool {
	return state == health.Healthy || state == health.Warning || state == health.Critical ||
		state == health.Unknown || state == health.Stale || state == health.Maintenance || state == health.Disabled
}
