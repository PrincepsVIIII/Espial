package webcheck

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/health"
)

type Clock interface{ Now() time.Time }

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

type CheckerOptions struct {
	Clock   Clock
	RootCAs *x509.CertPool
}

type Result struct {
	State        health.State
	Summary      string
	ReasonCode   string
	ObservedAt   time.Time
	Measurements map[string]any
	Metadata     map[string]any
}

type Checker struct {
	policy  *Policy
	clock   Clock
	rootCAs *x509.CertPool
}

func NewChecker(policy *Policy, options CheckerOptions) *Checker {
	if options.Clock == nil {
		options.Clock = systemClock{}
	}
	return &Checker{policy: policy, clock: options.Clock, rootCAs: options.RootCAs}
}

type stageEvidence struct {
	dnsMS     int64
	tcpMS     int64
	tlsMS     int64
	httpMS    int64
	totalMS   int64
	status    int
	bodyBytes int64
	redirects int
	completed []string
}

func (checker *Checker) Check(ctx context.Context, config Config) Result {
	started := checker.clock.Now().UTC()
	checkContext, cancel := context.WithTimeout(ctx, time.Duration(config.TimeoutMS)*time.Millisecond)
	defer cancel()
	target, _ := url.Parse(config.URL)
	evidence := stageEvidence{completed: []string{}}
	result := checker.follow(checkContext, target, config, &evidence)
	evidence.totalMS = elapsedMS(started, checker.clock.Now())
	if result.State == health.Healthy && config.WarningLatencyMS > 0 && evidence.totalMS >= int64(config.WarningLatencyMS) {
		result = Result{State: health.Warning, ReasonCode: "latency_high", Summary: "Website responded more slowly than the configured warning threshold."}
	}
	result.ObservedAt = checker.clock.Now().UTC()
	result.Measurements = map[string]any{
		"dns_ms": evidence.dnsMS, "tcp_ms": evidence.tcpMS, "tls_ms": evidence.tlsMS,
		"http_ms": evidence.httpMS, "total_ms": evidence.totalMS, "status_code": evidence.status,
		"body_bytes": evidence.bodyBytes, "redirects": evidence.redirects,
	}
	result.Metadata = map[string]any{"reason_code": result.ReasonCode, "completed_stages": evidence.completed}
	return result
}

func (checker *Checker) follow(ctx context.Context, target *url.URL, config Config, evidence *stageEvidence) Result {
	for {
		response, body, location, result := checker.request(ctx, target, config, evidence)
		if result.ReasonCode != "" {
			return result
		}
		if response.StatusCode >= 300 && response.StatusCode < 400 {
			if !config.FollowRedirects {
				return failed("redirect_rejected", "Website returned a redirect but redirects are disabled.")
			}
			if evidence.redirects >= config.MaxRedirects {
				return failed("redirect_limit", "Website exceeded the configured redirect limit.")
			}
			next, err := target.Parse(location)
			if err != nil || next.Host == "" || next.User != nil || next.Fragment != "" || (next.Scheme != "http" && next.Scheme != "https") {
				return failed("redirect_invalid", "Website returned an invalid redirect target.")
			}
			if err := ValidateConfig(Config{URL: next.String(), AllowedStatuses: config.AllowedStatuses,
				TimeoutMS: config.TimeoutMS, WarningLatencyMS: config.WarningLatencyMS,
				FollowRedirects: config.FollowRedirects, MaxRedirects: config.MaxRedirects,
				ExpectedRefreshSeconds: config.ExpectedRefreshSeconds, HeaderNames: config.HeaderNames,
				HeaderValue1: config.HeaderValue1, HeaderValue2: config.HeaderValue2,
				HeaderValue3: config.HeaderValue3, HeaderValue4: config.HeaderValue4}); err != nil {
				return failed("redirect_invalid", "Website returned a redirect target that violates monitor policy.")
			}
			if len(config.HeaderNames) > 0 && !sameOrigin(target, next) {
				return failed("redirect_credentials_rejected", "Website redirect would forward a protected header to another origin.")
			}
			evidence.redirects++
			target = next
			continue
		}
		if !statusAllowed(response.StatusCode, config.AllowedStatuses) {
			return failed("status_unexpected", fmt.Sprintf("Website returned unexpected HTTP status %d.", response.StatusCode))
		}
		if config.ContentMatch != "" && !bytes.Contains(body, []byte(config.ContentMatch)) {
			return failed("content_mismatch", "Website response did not contain the configured exact content.")
		}
		return Result{State: health.Healthy, ReasonCode: "available", Summary: "Website completed the configured availability check."}
	}
}

func (checker *Checker) request(ctx context.Context, target *url.URL, config Config, evidence *stageEvidence) (*http.Response, []byte, string, Result) {
	port, err := targetPort(target.Scheme, target.Port())
	if err != nil {
		return nil, nil, "", failed("url_invalid", "Website URL contains an invalid port.")
	}
	dnsStarted := checker.clock.Now()
	addresses, err := checker.policy.Resolve(ctx, target.Hostname(), port)
	evidence.dnsMS += elapsedMS(dnsStarted, checker.clock.Now())
	if err != nil {
		return nil, nil, "", failed(safeReason(err), summaryForReason(safeReason(err)))
	}
	evidence.completed = appendUnique(evidence.completed, "dns")

	tcpStarted := checker.clock.Now()
	connection, err := checker.dial(ctx, addresses, port)
	evidence.tcpMS += elapsedMS(tcpStarted, checker.clock.Now())
	if err != nil {
		return nil, nil, "", failed("connect_failed", "Website TCP connection could not be established.")
	}
	defer connection.Close()
	evidence.completed = appendUnique(evidence.completed, "tcp")
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	}

	stream := connection
	if target.Scheme == "https" {
		tlsStarted := checker.clock.Now()
		secure := tls.Client(connection, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: target.Hostname(), RootCAs: checker.rootCAs})
		if err := secure.HandshakeContext(ctx); err != nil {
			evidence.tlsMS += elapsedMS(tlsStarted, checker.clock.Now())
			return nil, nil, "", failed("tls_failed", "Website TLS negotiation or certificate verification failed.")
		}
		evidence.tlsMS += elapsedMS(tlsStarted, checker.clock.Now())
		evidence.completed = appendUnique(evidence.completed, "tls")
		stream = secure
	}

	httpStarted := checker.clock.Now()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, nil, "", failed("request_invalid", "Website request could not be constructed.")
	}
	request.Header.Set("User-Agent", "Espial/1 webcheck")
	request.Header.Set("Accept", "*/*")
	request.Header.Set("Accept-Encoding", "identity")
	request.Close = true
	values := config.HeaderValues()
	for index, name := range config.HeaderNames {
		request.Header.Set(name, values[index])
	}
	if err := request.Write(stream); err != nil {
		return nil, nil, "", failed("request_write_failed", "Website request could not be sent.")
	}
	limited := &countingLimitReader{reader: stream, remaining: checker.policy.headerLimit + 1}
	buffered := bufio.NewReaderSize(limited, int(min64(checker.policy.headerLimit, 64*1024)))
	response, err := http.ReadResponse(buffered, request)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || isTimeout(err) {
			return nil, nil, "", failed("response_timeout", "Website did not return response headers before the deadline.")
		}
		if limited.exhausted {
			return nil, nil, "", failed("headers_too_large", "Website response exceeded the configured header limit.")
		}
		return nil, nil, "", failed("response_invalid", "Website returned an invalid HTTP response.")
	}
	defer response.Body.Close()
	headerBytes := limited.consumed - int64(buffered.Buffered())
	if headerBytes > checker.policy.headerLimit {
		return nil, nil, "", failed("headers_too_large", "Website response exceeded the configured header limit.")
	}
	limited.remaining = checker.policy.bodyLimit + 1
	limited.exhausted = false
	evidence.httpMS += elapsedMS(httpStarted, checker.clock.Now())
	evidence.status = response.StatusCode
	evidence.completed = appendUnique(evidence.completed, "http")
	location := response.Header.Get("Location")
	body, err := io.ReadAll(io.LimitReader(response.Body, checker.policy.bodyLimit+1))
	evidence.bodyBytes = int64(len(body))
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || isTimeout(err) {
			return nil, nil, "", failed("response_timeout", "Website response body exceeded the check deadline.")
		}
		return nil, nil, "", failed("body_read_failed", "Website response body could not be read.")
	}
	if int64(len(body)) > checker.policy.bodyLimit {
		return nil, nil, "", failed("body_too_large", "Website response exceeded the configured body limit.")
	}
	evidence.completed = appendUnique(evidence.completed, "body")
	return response, body, location, Result{}
}

func (checker *Checker) dial(ctx context.Context, addresses []netip.Addr, port int) (net.Conn, error) {
	var last error
	for _, address := range addresses {
		connection, err := checker.policy.dialer(ctx, "tcp", net.JoinHostPort(address.String(), strconv.Itoa(port)))
		if err == nil {
			return connection, nil
		}
		last = err
	}
	return nil, last
}

func failed(code, summary string) Result {
	return Result{State: health.Critical, ReasonCode: code, Summary: summary}
}

func safeReason(err error) string {
	value := err.Error()
	for _, allowed := range []string{"host_not_approved", "port_not_approved", "dns_failed", "dns_no_addresses", "address_not_approved"} {
		if value == allowed {
			return value
		}
	}
	return "network_policy_rejected"
}

func summaryForReason(code string) string {
	switch code {
	case "dns_failed", "dns_no_addresses":
		return "Website hostname could not be resolved under the configured policy."
	case "host_not_approved", "port_not_approved", "address_not_approved":
		return "Website target was rejected by the configured network policy."
	default:
		return "Website check was rejected by network policy."
	}
}

func statusAllowed(status int, allowed []int) bool {
	for _, item := range allowed {
		if item == status {
			return true
		}
	}
	return false
}
func elapsedMS(start, end time.Time) int64 {
	value := end.Sub(start).Milliseconds()
	if value < 0 {
		return 0
	}
	return value
}
func appendUnique(values []string, value string) []string {
	for _, item := range values {
		if item == value {
			return values
		}
	}
	return append(values, value)
}
func isTimeout(err error) bool { var value net.Error; return errors.As(err, &value) && value.Timeout() }
func min64(left, right int64) int64 {
	if left < right {
		return left
	}
	return right
}

func sameOrigin(left, right *url.URL) bool {
	leftPort, leftErr := targetPort(left.Scheme, left.Port())
	rightPort, rightErr := targetPort(right.Scheme, right.Port())
	return leftErr == nil && rightErr == nil && left.Scheme == right.Scheme && canonicalHost(left.Hostname()) == canonicalHost(right.Hostname()) && leftPort == rightPort
}

type countingLimitReader struct {
	reader    io.Reader
	remaining int64
	consumed  int64
	exhausted bool
}

func (reader *countingLimitReader) Read(buffer []byte) (int, error) {
	if reader.remaining <= 0 {
		reader.exhausted = true
		return 0, errors.New("response_limit_exceeded")
	}
	if int64(len(buffer)) > reader.remaining {
		buffer = buffer[:reader.remaining]
	}
	read, err := reader.reader.Read(buffer)
	reader.remaining -= int64(read)
	reader.consumed += int64(read)
	if reader.remaining <= 0 && err == nil {
		reader.exhausted = true
		err = errors.New("response_limit_exceeded")
	}
	return read, err
}
