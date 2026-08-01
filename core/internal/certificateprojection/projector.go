// Package certificateprojection transactionally projects trusted webcheck evidence.
package certificateprojection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/observations"
	"github.com/PrincepsVIIII/Espial/core/internal/webcheck"
	"github.com/jackc/pgx/v5"
)

var fingerprintPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type projectedEvidence struct {
	ReasonCode        string `json:"reason_code"`
	Endpoint          string `json:"endpoint"`
	Subject           string `json:"subject"`
	SANSummary        string `json:"san_summary"`
	Issuer            string `json:"issuer"`
	SerialNumber      string `json:"serial_number"`
	FingerprintSHA256 string `json:"fingerprint_sha256"`
	NotBefore         string `json:"not_before"`
	NotAfter          string `json:"not_after"`
	HostnameValid     *bool  `json:"hostname_valid"`
	ChainValid        *bool  `json:"chain_valid"`
}

// ProjectBatch stores only the bounded, normalized certificate fields emitted by
// the trusted webcheck adapter. It runs inside the observation transaction.
func ProjectBatch(ctx context.Context, tx pgx.Tx, integrationID string, batch observations.Batch) error {
	for _, item := range batch.Observations {
		if item.CheckType != webcheck.CertificateCheckType {
			continue
		}
		encoded, err := json.Marshal(item.Metadata)
		if err != nil {
			return errors.New("encode certificate evidence")
		}
		var evidence projectedEvidence
		if json.Unmarshal(encoded, &evidence) != nil || !validEvidence(evidence) {
			return errors.New("invalid certificate evidence")
		}
		var notBefore, notAfter any
		if evidence.NotBefore != "" {
			value, err := time.Parse(time.RFC3339Nano, evidence.NotBefore)
			if err != nil {
				return errors.New("invalid certificate validity interval")
			}
			notBefore = value.UTC()
		}
		if evidence.NotAfter != "" {
			value, err := time.Parse(time.RFC3339Nano, evidence.NotAfter)
			if err != nil {
				return errors.New("invalid certificate validity interval")
			}
			notAfter = value.UTC()
		}
		var days any
		if value, ok := integerMeasurement(item.Measurements["days_remaining"]); ok {
			days = value
		}
		_, err = tx.Exec(ctx, `
			WITH selected AS (
				SELECT o.id observation_id,o.resource_id,o.integration_id,o.observed_at,
					(SELECT prior.fingerprint_sha256 FROM certificate_observations prior
					 WHERE prior.resource_id=o.resource_id ORDER BY prior.observed_at DESC,prior.observation_id DESC LIMIT 1) prior_fingerprint,
					(SELECT prior.issuer_summary FROM certificate_observations prior
					 WHERE prior.resource_id=o.resource_id ORDER BY prior.observed_at DESC,prior.observation_id DESC LIMIT 1) prior_issuer
				FROM observations o JOIN resources r ON r.id=o.resource_id
				WHERE o.integration_id=$1 AND r.external_id=$2 AND o.check_type=$3 AND o.observed_at=$4
				ORDER BY o.received_at DESC,o.id DESC LIMIT 1
			)
			INSERT INTO certificate_observations (
				observation_id,resource_id,integration_id,endpoint,subject_summary,san_summary,
				issuer_summary,serial_number,fingerprint_sha256,not_before,not_after,
				hostname_valid,chain_valid,days_remaining,certificate_state,reason_code,
				fingerprint_changed,issuer_changed,observed_at
			)
			SELECT observation_id,resource_id,integration_id,$5,NULLIF($6,''),NULLIF($7,''),
				NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),$11,$12,$13,$14,$15,$16,$17,
				prior_fingerprint IS NOT NULL AND prior_fingerprint<>NULLIF($10,''),
				prior_issuer IS NOT NULL AND prior_issuer<>NULLIF($8,''),observed_at
			FROM selected ON CONFLICT (observation_id) DO NOTHING
		`, integrationID, item.ExternalResourceID, item.CheckType, item.ObservedAt.UTC(),
			evidence.Endpoint, evidence.Subject, evidence.SANSummary, evidence.Issuer,
			evidence.SerialNumber, evidence.FingerprintSHA256, notBefore, notAfter,
			evidence.HostnameValid, evidence.ChainValid, days, item.State, evidence.ReasonCode)
		if err != nil {
			return fmt.Errorf("project certificate observation: %w", err)
		}
	}
	return nil
}

func validEvidence(value projectedEvidence) bool {
	return value.Endpoint != "" && len(value.Endpoint) <= 512 && len(value.Subject) <= 512 &&
		len(value.SANSummary) <= 1024 && len(value.Issuer) <= 512 && len(value.SerialNumber) <= 128 &&
		(value.FingerprintSHA256 == "" || fingerprintPattern.MatchString(value.FingerprintSHA256)) &&
		regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,126}$`).MatchString(value.ReasonCode) &&
		!strings.ContainsAny(value.Endpoint, "\r\n\x00")
}

func integerMeasurement(value any) (int, bool) {
	switch value := value.(type) {
	case int:
		return value, true
	case int64:
		return int(value), true
	case float64:
		if value == float64(int(value)) {
			return int(value), true
		}
	}
	return 0, false
}
