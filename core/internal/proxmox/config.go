// Package proxmox implements Espial's read-only Proxmox VE adapter.
package proxmox

import (
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	AdapterID = "org.ubnetdef.espial.proxmox"

	ClusterKind = "hypervisor_cluster"
	NodeKind    = "hypervisor_node"
	QEMUKind    = "virtual_machine"
	LXCKind     = "container"
	StorageKind = "storage"

	ClusterQuorumCheck       = "proxmox.cluster.quorum"
	NodeAvailabilityCheck    = "proxmox.node.availability"
	GuestRuntimeCheck        = "proxmox.guest.runtime"
	StorageAvailabilityCheck = "proxmox.storage.availability"
	StorageCapacityCheck     = "proxmox.storage.capacity"

	DefaultExpectedRefreshSeconds = 300
	DefaultRequestTimeoutSeconds  = 10
	DefaultStorageWarningPercent  = 80
	DefaultStorageCriticalPercent = 90
	DefaultSwapWarningPercent     = 10
	DefaultSwapCriticalPercent    = 25
	DefaultIOWaitWarningPercent   = 5
	DefaultIOWaitCriticalPercent  = 15
)

var (
	deploymentPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,62}$`)
	tokenIDPattern    = regexp.MustCompile(`^[A-Za-z0-9_.@!-]{3,160}$`)
	guestPattern      = regexp.MustCompile(`^(qemu|lxc):[0-9]{1,10}$`)
)

type Config struct {
	BaseURL                    string   `json:"base_url"`
	DeploymentID               string   `json:"deployment_id"`
	TokenID                    string   `json:"token_id"`
	TokenSecret                string   `json:"token_secret"`
	AllowInsecureHTTP          bool     `json:"allow_insecure_http,omitempty"`
	TrustSelfSignedCertificate bool     `json:"trust_self_signed_certificate,omitempty"`
	AllowUnverifiedTLS         bool     `json:"allow_unverified_tls,omitempty"`
	TrustedCertificateSHA256   string   `json:"trusted_certificate_sha256,omitempty"`
	ApprovedHost               string   `json:"approved_host,omitempty"`
	ApprovedPort               int      `json:"approved_port,omitempty"`
	ExpectedRefreshSeconds     int      `json:"expected_refresh_seconds"`
	RequestTimeoutSeconds      int      `json:"request_timeout_seconds"`
	ExpectedRunningGuests      []string `json:"expected_running_guests"`
	StorageWarningPercent      float64  `json:"storage_warning_percent"`
	StorageCriticalPercent     float64  `json:"storage_critical_percent"`
	SwapWarningPercent         float64  `json:"swap_warning_percent"`
	SwapCriticalPercent        float64  `json:"swap_critical_percent"`
	IOWaitWarningPercent       float64  `json:"io_wait_warning_percent"`
	IOWaitCriticalPercent      float64  `json:"io_wait_critical_percent"`
}

type ConfigFieldError struct {
	Field string
	Code  string
}

func DecodeConfig(payload json.RawMessage) (Config, error) {
	var wrapper struct {
		Config json.RawMessage `json:"config"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wrapper); err != nil || len(wrapper.Config) == 0 {
		return Config{}, errors.New("invalid proxmox config")
	}
	var config Config
	decoder = json.NewDecoder(strings.NewReader(string(wrapper.Config)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return Config{}, errors.New("invalid proxmox config")
	}
	config = WithDefaults(config)
	return config, ValidateConfig(config)
}

func WithDefaults(config Config) Config {
	config.BaseURL = strings.TrimSpace(config.BaseURL)
	config.DeploymentID = strings.TrimSpace(config.DeploymentID)
	config.TokenID = strings.TrimSpace(config.TokenID)
	config.ApprovedHost = strings.ToLower(strings.TrimSpace(config.ApprovedHost))
	config.TrustedCertificateSHA256 = strings.ToLower(strings.TrimSpace(config.TrustedCertificateSHA256))
	for index, guest := range config.ExpectedRunningGuests {
		config.ExpectedRunningGuests[index] = strings.TrimSpace(guest)
	}
	if config.ExpectedRefreshSeconds == 0 {
		config.ExpectedRefreshSeconds = DefaultExpectedRefreshSeconds
	}
	if config.RequestTimeoutSeconds == 0 {
		config.RequestTimeoutSeconds = DefaultRequestTimeoutSeconds
	}
	if config.StorageWarningPercent == 0 {
		config.StorageWarningPercent = DefaultStorageWarningPercent
	}
	if config.StorageCriticalPercent == 0 {
		config.StorageCriticalPercent = DefaultStorageCriticalPercent
	}
	if config.SwapWarningPercent == 0 {
		config.SwapWarningPercent = DefaultSwapWarningPercent
	}
	if config.SwapCriticalPercent == 0 {
		config.SwapCriticalPercent = DefaultSwapCriticalPercent
	}
	if config.IOWaitWarningPercent == 0 {
		config.IOWaitWarningPercent = DefaultIOWaitWarningPercent
	}
	if config.IOWaitCriticalPercent == 0 {
		config.IOWaitCriticalPercent = DefaultIOWaitCriticalPercent
	}
	return config
}

func ValidateConfig(config Config) error {
	if len(ValidateConfigFields(config)) > 0 {
		return errors.New("invalid proxmox config")
	}
	return nil
}

func ValidateConfigFields(config Config) []ConfigFieldError {
	config = WithDefaults(config)
	var fields []ConfigFieldError
	target, err := url.Parse(config.BaseURL)
	if err != nil || target.Hostname() == "" || target.User != nil || strings.Trim(target.EscapedPath(), "/") != "" || target.RawQuery != "" || target.Fragment != "" {
		fields = append(fields, ConfigFieldError{Field: "base_url", Code: "invalid"})
	} else if target.Scheme == "http" && !config.AllowInsecureHTTP {
		fields = append(fields, ConfigFieldError{Field: "base_url", Code: "requires_https"})
	} else if target.Scheme != "https" && target.Scheme != "http" {
		fields = append(fields, ConfigFieldError{Field: "base_url", Code: "invalid_scheme"})
	}
	if config.TrustSelfSignedCertificate && (err != nil || target.Scheme != "https") {
		fields = append(fields, ConfigFieldError{Field: "trust_self_signed_certificate", Code: "requires_https"})
	}
	if config.AllowUnverifiedTLS && (err != nil || target.Scheme != "https") {
		fields = append(fields, ConfigFieldError{Field: "allow_unverified_tls", Code: "requires_https"})
	}
	if config.AllowUnverifiedTLS && config.TrustSelfSignedCertificate {
		fields = append(fields, ConfigFieldError{Field: "allow_unverified_tls", Code: "conflicts_with_certificate_pin"})
	}
	if config.AllowUnverifiedTLS && config.TrustedCertificateSHA256 != "" {
		fields = append(fields, ConfigFieldError{Field: "allow_unverified_tls", Code: "conflicts_with_certificate_pin"})
	}
	if config.TrustedCertificateSHA256 != "" {
		if !config.TrustSelfSignedCertificate || !validSHA256Fingerprint(config.TrustedCertificateSHA256) {
			fields = append(fields, ConfigFieldError{Field: "trusted_certificate_sha256", Code: "invalid"})
		}
	}
	if err == nil && config.ApprovedHost != "" && !strings.EqualFold(config.ApprovedHost, target.Hostname()) {
		fields = append(fields, ConfigFieldError{Field: "approved_target", Code: "mismatch"})
	}
	if err == nil && config.ApprovedPort != 0 && config.ApprovedPort != targetPort(target) {
		fields = append(fields, ConfigFieldError{Field: "approved_target", Code: "mismatch"})
	}
	if !deploymentPattern.MatchString(config.DeploymentID) {
		fields = append(fields, ConfigFieldError{Field: "deployment_id", Code: "invalid"})
	}
	if !tokenIDPattern.MatchString(config.TokenID) {
		fields = append(fields, ConfigFieldError{Field: "token_id", Code: "invalid"})
	}
	if strings.TrimSpace(config.TokenSecret) == "" || strings.ContainsAny(config.TokenSecret, "\r\n\x00") {
		fields = append(fields, ConfigFieldError{Field: "token_secret", Code: "invalid"})
	}
	if config.ExpectedRefreshSeconds < 1 || config.ExpectedRefreshSeconds > 86400 {
		fields = append(fields, ConfigFieldError{Field: "expected_refresh_seconds", Code: "out_of_range"})
	}
	if config.RequestTimeoutSeconds < 1 || config.RequestTimeoutSeconds > 60 {
		fields = append(fields, ConfigFieldError{Field: "request_timeout_seconds", Code: "out_of_range"})
	}
	if !validThresholds(config.StorageWarningPercent, config.StorageCriticalPercent) {
		fields = append(fields, ConfigFieldError{Field: "storage_thresholds", Code: "invalid_range"})
	}
	if !validThresholds(config.SwapWarningPercent, config.SwapCriticalPercent) {
		fields = append(fields, ConfigFieldError{Field: "swap_thresholds", Code: "invalid_range"})
	}
	if !validThresholds(config.IOWaitWarningPercent, config.IOWaitCriticalPercent) {
		fields = append(fields, ConfigFieldError{Field: "io_wait_thresholds", Code: "invalid_range"})
	}
	for _, guest := range config.ExpectedRunningGuests {
		if !guestPattern.MatchString(guest) {
			fields = append(fields, ConfigFieldError{Field: "expected_running_guests", Code: "invalid"})
			break
		}
	}
	return fields
}

func RequestTimeout(config Config) time.Duration {
	config = WithDefaults(config)
	return time.Duration(config.RequestTimeoutSeconds) * time.Second
}

func validThresholds(warning, critical float64) bool {
	return warning >= 0 && warning <= 100 && critical >= 0 && critical <= 100 && warning < critical
}

func validSHA256Fingerprint(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return true
}

func targetPort(target *url.URL) int {
	if target == nil {
		return 0
	}
	port := target.Port()
	if port == "" {
		return 443
	}
	parsed, err := strconv.Atoi(port)
	if err != nil {
		return 0
	}
	return parsed
}
