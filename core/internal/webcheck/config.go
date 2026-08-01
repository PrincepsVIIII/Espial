// Package webcheck implements Espial's trusted website availability adapter.
package webcheck

import (
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strings"
)

const (
	AdapterID               = "org.ubnetdef.espial.webcheck"
	CheckType               = "website.availability"
	CertificateCheckType    = "certificate.validity"
	ResourceKind            = "webpage"
	CertificateResourceKind = "certificate"
	MaxSecretHeaders        = 4
	MaxContentBytes         = 4096
	DefaultBodyLimit        = 262144
	DefaultHeaderLimit      = 32768
	DefaultWarningDays      = 30
	DefaultCriticalDays     = 14
	DefaultEscalationDays   = 7
)

type Config struct {
	URL                       string   `json:"url"`
	AllowedStatuses           []int    `json:"allowed_statuses"`
	TimeoutMS                 int      `json:"timeout_ms"`
	WarningLatencyMS          int      `json:"warning_latency_ms,omitempty"`
	ContentMatch              string   `json:"content_match,omitempty"`
	FollowRedirects           bool     `json:"follow_redirects"`
	MaxRedirects              int      `json:"max_redirects"`
	ExpectedRefreshSeconds    int      `json:"expected_refresh_seconds"`
	CertificateWarningDays    int      `json:"certificate_warning_days,omitempty"`
	CertificateCriticalDays   int      `json:"certificate_critical_days,omitempty"`
	CertificateEscalationDays int      `json:"certificate_escalation_days,omitempty"`
	HeaderNames               []string `json:"header_names,omitempty"`
	HeaderValue1              string   `json:"header_value_1,omitempty"`
	HeaderValue2              string   `json:"header_value_2,omitempty"`
	HeaderValue3              string   `json:"header_value_3,omitempty"`
	HeaderValue4              string   `json:"header_value_4,omitempty"`
}

func DecodeConfig(value json.RawMessage) (Config, error) {
	var wrapper struct {
		Config json.RawMessage `json:"config"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(value)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wrapper); err != nil || len(wrapper.Config) == 0 {
		return Config{}, errors.New("invalid config wrapper")
	}
	var result Config
	decoder = json.NewDecoder(strings.NewReader(string(wrapper.Config)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return Config{}, errors.New("invalid webcheck config")
	}
	result = WithCertificateDefaults(result)
	if err := ValidateConfig(result); err != nil {
		return Config{}, err
	}
	return result, nil
}

func ValidateConfig(config Config) error {
	config = WithCertificateDefaults(config)
	parsed, err := url.Parse(config.URL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.User != nil || parsed.Fragment != "" || len(config.URL) > 2048 {
		return errors.New("invalid website URL")
	}
	for key := range parsed.Query() {
		lower := strings.ToLower(key)
		for _, sensitive := range []string{"token", "secret", "password", "passwd", "auth", "signature", "api_key", "apikey"} {
			if strings.Contains(lower, sensitive) {
				return errors.New("website URL contains a secret-like query key")
			}
		}
	}
	if len(config.AllowedStatuses) == 0 || len(config.AllowedStatuses) > 100 ||
		config.TimeoutMS < 100 || config.TimeoutMS > 60000 ||
		config.WarningLatencyMS < 0 || config.WarningLatencyMS >= config.TimeoutMS ||
		len(config.ContentMatch) > MaxContentBytes ||
		config.ExpectedRefreshSeconds < 1 || config.ExpectedRefreshSeconds > 86400 ||
		config.MaxRedirects < 0 || config.MaxRedirects > 5 ||
		config.CertificateWarningDays < 1 || config.CertificateWarningDays > 3650 ||
		config.CertificateCriticalDays < 1 || config.CertificateCriticalDays >= config.CertificateWarningDays ||
		config.CertificateEscalationDays < 1 || config.CertificateEscalationDays >= config.CertificateCriticalDays ||
		(!config.FollowRedirects && config.MaxRedirects != 0) || len(config.HeaderNames) > MaxSecretHeaders {
		return errors.New("webcheck config is outside safe bounds")
	}
	statuses := append([]int(nil), config.AllowedStatuses...)
	sort.Ints(statuses)
	for index, status := range statuses {
		if status < 100 || status > 599 || index > 0 && status == statuses[index-1] {
			return errors.New("invalid allowed HTTP status")
		}
	}
	values := []string{config.HeaderValue1, config.HeaderValue2, config.HeaderValue3, config.HeaderValue4}
	seen := map[string]struct{}{}
	for index, name := range config.HeaderNames {
		name = strings.TrimSpace(name)
		if name == "" || len(name) > 64 || strings.ContainsAny(name, "\r\n:") || strings.EqualFold(name, "host") ||
			strings.EqualFold(name, "content-length") || strings.EqualFold(name, "connection") || values[index] == "" ||
			len(values[index]) > 4096 || strings.ContainsAny(values[index], "\r\n\x00") {
			return errors.New("invalid secret header")
		}
		canonical := strings.ToLower(name)
		if _, exists := seen[canonical]; exists {
			return errors.New("duplicate secret header")
		}
		seen[canonical] = struct{}{}
	}
	for index := len(config.HeaderNames); index < len(values); index++ {
		if values[index] != "" {
			return errors.New("secret header value has no name")
		}
	}
	return nil
}

func WithCertificateDefaults(config Config) Config {
	if config.CertificateWarningDays == 0 {
		config.CertificateWarningDays = DefaultWarningDays
	}
	if config.CertificateCriticalDays == 0 {
		config.CertificateCriticalDays = DefaultCriticalDays
	}
	if config.CertificateEscalationDays == 0 {
		config.CertificateEscalationDays = DefaultEscalationDays
	}
	return config
}

func (config Config) HeaderValues() []string {
	values := []string{config.HeaderValue1, config.HeaderValue2, config.HeaderValue3, config.HeaderValue4}
	return append([]string(nil), values[:len(config.HeaderNames)]...)
}
