package certificates

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/health"
	"github.com/PrincepsVIIII/Espial/core/internal/monitoring"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool      *pgxpool.Pool
	resources *monitoring.ReadService
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, resources: monitoring.NewReadService(pool)}
}

func (service *Service) Certificates(ctx context.Context, filter Filter) (List, error) {
	if filter.Limit == 0 {
		filter.Limit = defaultPageLimit
	}
	if filter.Limit < 1 || filter.Limit > maximumPageLimit || filter.ExpiryDays != nil && (*filter.ExpiryDays < 0 || *filter.ExpiryDays > 3650) {
		return List{}, ErrInvalidFilter
	}
	states := make([]string, len(filter.States))
	for index, state := range filter.States {
		if state != health.Healthy && state != health.Warning && state != health.Critical && state != health.Unknown {
			return List{}, ErrInvalidFilter
		}
		states[index] = string(state)
	}
	cursor, err := decodeCursor(filter.Cursor)
	if err != nil {
		return List{}, err
	}
	var orderedAt, cursorID any
	if filter.Cursor != "" {
		orderedAt, cursorID = cursor.OrderedAt, cursor.ID
	}
	var hostname, expiry any
	if filter.HostnameValid != nil {
		hostname = *filter.HostnameValid
	}
	if filter.ExpiryDays != nil {
		expiry = *filter.ExpiryDays
	}
	rows, err := service.pool.Query(ctx, `
		WITH latest AS (
			SELECT DISTINCT ON (resource_id) resource_id,observed_at,certificate_state,hostname_valid,not_after
			FROM certificate_observations ORDER BY resource_id,observed_at DESC,observation_id DESC
		)
		SELECT resource_id::text,observed_at FROM latest
		WHERE (cardinality($1::text[])=0 OR certificate_state=ANY($1))
		 AND ($2::boolean IS NULL OR hostname_valid=$2)
		 AND ($3::integer IS NULL OR not_after IS NOT NULL AND not_after<=now()+make_interval(days=>$3))
		 AND ($4::timestamptz IS NULL OR (observed_at,resource_id)<($4,$5::uuid))
		ORDER BY observed_at DESC,resource_id DESC LIMIT $6
	`, states, hostname, expiry, orderedAt, cursorID, filter.Limit+1)
	if err != nil {
		return List{}, fmt.Errorf("list certificates: %w", err)
	}
	type orderedID struct {
		id string
		at time.Time
	}
	ids := []orderedID{}
	for rows.Next() {
		var item orderedID
		if err := rows.Scan(&item.id, &item.at); err != nil {
			rows.Close()
			return List{}, err
		}
		ids = append(ids, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return List{}, err
	}
	rows.Close()
	result := List{Items: []Summary{}}
	count := len(ids)
	if count > filter.Limit {
		count = filter.Limit
	}
	for _, ordered := range ids[:count] {
		detail, err := service.Certificate(ctx, ordered.id)
		if err != nil {
			return List{}, err
		}
		result.Items = append(result.Items, detail.Summary)
	}
	if len(ids) > filter.Limit {
		last := ids[filter.Limit-1]
		result.NextCursor, err = encodeCursor(pageCursor{Kind: "certificates", OrderedAt: last.at, ID: last.id})
	}
	return result, err
}

func (service *Service) Certificate(ctx context.Context, id string) (Detail, error) {
	resource, err := service.resources.Resource(ctx, id)
	if err != nil || resource.Kind != "certificate" {
		return Detail{}, ErrNotFound
	}
	var result Detail
	var subject, sans, issuer, serial, fingerprint pgtype.Text
	var notBefore, notAfter pgtype.Timestamptz
	var hostname, chain pgtype.Bool
	var days pgtype.Int4
	var certificateState string
	err = service.pool.QueryRow(ctx, `
		SELECT endpoint,subject_summary,san_summary,issuer_summary,serial_number,fingerprint_sha256,
			not_before,not_after,hostname_valid,chain_valid,days_remaining,certificate_state,reason_code,
			fingerprint_changed,issuer_changed
		FROM certificate_observations WHERE resource_id=$1
		ORDER BY observed_at DESC,observation_id DESC LIMIT 1
	`, id).Scan(&result.Endpoint, &subject, &sans, &issuer, &serial, &fingerprint, &notBefore, &notAfter, &hostname, &chain, &days, &certificateState, &result.ReasonCode, &result.FingerprintChanged, &result.IssuerChanged)
	if errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, ErrNotFound
	}
	if err != nil {
		return Detail{}, err
	}
	result.ID = id
	result.MonitorID = resource.IntegrationID
	result.State = resource.Health.State
	result.RawState = resource.Health.RawState
	result.CertificateState = health.State(certificateState)
	result.Reason = resource.Health.Reason
	result.ObservedAt = resource.Health.ObservedAt
	result.UpdatedAt = resource.Health.UpdatedAt
	result.Source = "webcheck"
	result.Freshness = "fresh"
	if result.State == health.Stale {
		result.Freshness = "stale"
	}
	if result.State == health.Unknown {
		result.Freshness = "unknown"
	}
	if result.State == health.Maintenance {
		result.Freshness = "maintenance"
	}
	result.Subject = subject.String
	result.SANSummary = sans.String
	result.Issuer = issuer.String
	result.SerialNumber = serial.String
	result.FingerprintSHA256 = fingerprint.String
	if notBefore.Valid {
		value := notBefore.Time.UTC()
		result.NotBefore = &value
	}
	if notAfter.Valid {
		value := notAfter.Time.UTC()
		result.NotAfter = &value
	}
	if hostname.Valid {
		value := hostname.Bool
		result.HostnameValid = &value
	}
	if chain.Valid {
		value := chain.Bool
		result.ChainValid = &value
	}
	if days.Valid {
		value := int(days.Int32)
		result.DaysRemaining = &value
	}
	result.FirstSeenAt = resource.FirstSeenAt
	result.LastSeenAt = resource.LastSeenAt
	_ = service.pool.QueryRow(ctx, `SELECT id::text FROM incidents WHERE resource_id=$1 AND status<>'resolved' ORDER BY updated_at DESC,id DESC LIMIT 1`, id).Scan(&result.ActiveIncidentID)
	return result, nil
}
