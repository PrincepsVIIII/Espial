// Package certificates owns the bounded certificate observation projection and read models.
package certificates

import (
	"errors"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/health"
)

var (
	ErrNotFound      = errors.New("certificate not found")
	ErrInvalidCursor = errors.New("certificate cursor is invalid")
	ErrInvalidFilter = errors.New("certificate filter is invalid")
)

type Filter struct {
	Limit         int
	Cursor        string
	States        []health.State
	HostnameValid *bool
	ExpiryDays    *int
}

type Summary struct {
	ID               string       `json:"id"`
	MonitorID        string       `json:"monitor_id"`
	Endpoint         string       `json:"endpoint"`
	State            health.State `json:"state"`
	RawState         health.State `json:"raw_state"`
	CertificateState health.State `json:"certificate_state"`
	Reason           string       `json:"reason"`
	ReasonCode       string       `json:"reason_code"`
	NotAfter         *time.Time   `json:"not_after,omitempty"`
	DaysRemaining    *int         `json:"days_remaining,omitempty"`
	Issuer           string       `json:"issuer,omitempty"`
	HostnameValid    *bool        `json:"hostname_valid,omitempty"`
	ChainValid       *bool        `json:"chain_valid,omitempty"`
	ObservedAt       *time.Time   `json:"observed_at,omitempty"`
	UpdatedAt        time.Time    `json:"updated_at"`
	Source           string       `json:"source"`
	Freshness        string       `json:"freshness"`
	ActiveIncidentID string       `json:"active_incident_id,omitempty"`
}

type Detail struct {
	Summary
	Subject            string     `json:"subject,omitempty"`
	SANSummary         string     `json:"san_summary,omitempty"`
	SerialNumber       string     `json:"serial_number,omitempty"`
	FingerprintSHA256  string     `json:"fingerprint_sha256,omitempty"`
	NotBefore          *time.Time `json:"not_before,omitempty"`
	FingerprintChanged bool       `json:"fingerprint_changed"`
	IssuerChanged      bool       `json:"issuer_changed"`
	FirstSeenAt        time.Time  `json:"first_seen_at"`
	LastSeenAt         time.Time  `json:"last_seen_at"`
}

type List struct {
	Items      []Summary `json:"items"`
	NextCursor string    `json:"next_cursor,omitempty"`
}

type WarningSummary struct {
	Warning  int64 `json:"warning"`
	Critical int64 `json:"critical"`
	Unknown  int64 `json:"unknown"`
}
