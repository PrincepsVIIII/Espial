package proxmox

import "testing"

func TestValidateConfigFieldsReportsSpecificInvalidInputs(t *testing.T) {
	config := Config{
		BaseURL:                "http://pve.example.test:8006/api2/json",
		DeploymentID:           "Prod_1",
		TokenID:                "x",
		TokenSecret:            "line\nbreak",
		ExpectedRefreshSeconds: -1,
		RequestTimeoutSeconds:  61,
		StorageWarningPercent:  95,
		StorageCriticalPercent: 90,
		SwapWarningPercent:     25,
		SwapCriticalPercent:    10,
		IOWaitWarningPercent:   15,
		IOWaitCriticalPercent:  5,
		ExpectedRunningGuests:  []string{"100"},
	}

	fields := ValidateConfigFields(config)
	got := map[string]string{}
	for _, field := range fields {
		got[field.Field] = field.Code
	}

	want := map[string]string{
		"base_url":                 "invalid",
		"deployment_id":            "invalid",
		"token_id":                 "invalid",
		"token_secret":             "invalid",
		"expected_refresh_seconds": "out_of_range",
		"request_timeout_seconds":  "out_of_range",
		"storage_thresholds":       "invalid_range",
		"swap_thresholds":          "invalid_range",
		"io_wait_thresholds":       "invalid_range",
		"expected_running_guests":  "invalid",
	}
	for field, code := range want {
		if got[field] != code {
			t.Fatalf("expected %s=%s, got %q in %#v", field, code, got[field], fields)
		}
	}
}

func TestValidateConfigFieldsAcceptsAddClusterDefaults(t *testing.T) {
	config := Config{
		BaseURL:      " https://cdr-debug.cse.buffalo.edu:8006 ",
		DeploymentID: " prod-1 ",
		TokenID:      " root@pam!espial ",
		TokenSecret:  "secret-value",
	}

	if fields := ValidateConfigFields(config); len(fields) != 0 {
		t.Fatalf("expected valid config, got %#v", fields)
	}
}

func TestValidateConfigFieldsRequiresHTTPApproval(t *testing.T) {
	config := Config{
		BaseURL:      "http://cdr-debug.cse.buffalo.edu:8006",
		DeploymentID: "prod-1",
		TokenID:      "root@pam!espial",
		TokenSecret:  "secret-value",
	}

	fields := ValidateConfigFields(config)
	for _, field := range fields {
		if field.Field == "base_url" && field.Code == "requires_https" {
			return
		}
	}
	t.Fatalf("expected base_url requires_https, got %#v", fields)
}

func TestValidateConfigFieldsAcceptsApprovedHTTP(t *testing.T) {
	config := Config{
		BaseURL:           "http://cdr-debug.cse.buffalo.edu:8006",
		AllowInsecureHTTP: true,
		DeploymentID:      "prod-1",
		TokenID:           "root@pam!espial",
		TokenSecret:       "secret-value",
	}

	if fields := ValidateConfigFields(config); len(fields) != 0 {
		t.Fatalf("expected valid HTTP config with explicit approval, got %#v", fields)
	}
}

func TestValidateConfigFieldsRejectsApprovedTargetMismatch(t *testing.T) {
	config := Config{
		BaseURL:      "https://pve.example.test:8006",
		ApprovedHost: "other.example.test",
		ApprovedPort: 8006,
		DeploymentID: "prod-1",
		TokenID:      "root@pam!espial",
		TokenSecret:  "secret-value",
	}

	fields := ValidateConfigFields(config)
	for _, field := range fields {
		if field.Field == "approved_target" && field.Code == "mismatch" {
			return
		}
	}
	t.Fatalf("expected approved target mismatch, got %#v", fields)
}
