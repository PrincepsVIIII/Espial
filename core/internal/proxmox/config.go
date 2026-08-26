// Package proxmox implements Espial's read-only Proxmox VE adapter.
package proxmox

import (
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
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
	BaseURL                string   `json:"base_url"`
	DeploymentID           string   `json:"deployment_id"`
	TokenID                string   `json:"token_id"`
	TokenSecret            string   `json:"token_secret"`
	ExpectedRefreshSeconds int      `json:"expected_refresh_seconds"`
	RequestTimeoutSeconds  int      `json:"request_timeout_seconds"`
	ExpectedRunningGuests  []string `json:"expected_running_guests"`
	StorageWarningPercent  float64  `json:"storage_warning_percent"`
	StorageCriticalPercent float64  `json:"storage_critical_percent"`
	SwapWarningPercent     float64  `json:"swap_warning_percent"`
	SwapCriticalPercent    float64  `json:"swap_critical_percent"`
	IOWaitWarningPercent   float64  `json:"io_wait_warning_percent"`
	IOWaitCriticalPercent  float64  `json:"io_wait_critical_percent"`
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
	config = WithDefaults(config)
	target, err := url.Parse(config.BaseURL)
	if err != nil || target.Scheme != "https" || target.Hostname() == "" || target.User != nil || target.RawQuery != "" || target.Fragment != "" {
		return errors.New("invalid proxmox config")
	}
	if !deploymentPattern.MatchString(config.DeploymentID) || !tokenIDPattern.MatchString(config.TokenID) ||
		strings.TrimSpace(config.TokenSecret) == "" || strings.ContainsAny(config.TokenSecret, "\r\n\x00") {
		return errors.New("invalid proxmox config")
	}
	if config.ExpectedRefreshSeconds < 1 || config.ExpectedRefreshSeconds > 86400 ||
		config.RequestTimeoutSeconds < 1 || config.RequestTimeoutSeconds > 60 {
		return errors.New("invalid proxmox config")
	}
	if !validThresholds(config.StorageWarningPercent, config.StorageCriticalPercent) ||
		!validThresholds(config.SwapWarningPercent, config.SwapCriticalPercent) ||
		!validThresholds(config.IOWaitWarningPercent, config.IOWaitCriticalPercent) {
		return errors.New("invalid proxmox config")
	}
	for _, guest := range config.ExpectedRunningGuests {
		if !guestPattern.MatchString(guest) {
			return errors.New("invalid proxmox config")
		}
	}
	return nil
}

func RequestTimeout(config Config) time.Duration {
	config = WithDefaults(config)
	return time.Duration(config.RequestTimeoutSeconds) * time.Second
}

func validThresholds(warning, critical float64) bool {
	return warning >= 0 && warning <= 100 && critical >= 0 && critical <= 100 && warning < critical
}
