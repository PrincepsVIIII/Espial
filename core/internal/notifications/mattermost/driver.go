// Package mattermost implements the bounded Mattermost incoming-webhook driver.
package mattermost

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/PrincepsVIIII/Espial/core/internal/notifications"
)

var safePathPrefix = regexp.MustCompile(`^/[A-Za-z0-9._~/-]{0,255}$`)

const (
	defaultRequestTimeout = 10 * time.Second
	defaultResolveTimeout = 2 * time.Second
	defaultResponseLimit  = 4096
	maximumRetryAfter     = 5 * time.Minute
)

type Resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

type Options struct {
	ApprovedHosts     []string
	ApprovedCIDRs     []string
	AllowedPorts      []int
	RequestTimeout    time.Duration
	ResolveTimeout    time.Duration
	ResponseBodyLimit int64
	Resolver          Resolver
	RootCAs           *x509.CertPool
}

type Driver struct {
	hosts          map[string]struct{}
	networks       []netip.Prefix
	ports          map[int]struct{}
	requestTimeout time.Duration
	resolveTimeout time.Duration
	responseLimit  int64
	resolver       Resolver
	rootCAs        *x509.CertPool
}

func New(options Options) (*Driver, error) {
	driver := &Driver{
		hosts:    make(map[string]struct{}, len(options.ApprovedHosts)),
		networks: []netip.Prefix{}, ports: make(map[int]struct{}, len(options.AllowedPorts)),
		requestTimeout: options.RequestTimeout, resolveTimeout: options.ResolveTimeout,
		responseLimit: options.ResponseBodyLimit, resolver: options.Resolver, rootCAs: options.RootCAs,
	}
	if driver.requestTimeout <= 0 {
		driver.requestTimeout = defaultRequestTimeout
	}
	if driver.resolveTimeout <= 0 {
		driver.resolveTimeout = defaultResolveTimeout
	}
	if driver.responseLimit <= 0 {
		driver.responseLimit = defaultResponseLimit
	}
	if driver.resolver == nil {
		driver.resolver = net.DefaultResolver
	}
	for _, host := range options.ApprovedHosts {
		host = canonicalHost(host)
		if host == "" || strings.ContainsAny(host, "/?#@") {
			return nil, errors.New("invalid approved Mattermost host")
		}
		driver.hosts[host] = struct{}{}
	}
	for _, value := range options.ApprovedCIDRs {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("invalid approved Mattermost CIDR: %w", err)
		}
		driver.networks = append(driver.networks, prefix.Masked())
	}
	for _, port := range options.AllowedPorts {
		if port < 1 || port > 65535 {
			return nil, errors.New("invalid approved Mattermost port")
		}
		driver.ports[port] = struct{}{}
	}
	return driver, nil
}

func (driver *Driver) Validate(ctx context.Context, target notifications.Target) error {
	_, err := driver.resolveTarget(ctx, target)
	return err
}

func (driver *Driver) Deliver(ctx context.Context, request notifications.DeliveryRequest) notifications.DeliveryResult {
	addresses, err := driver.resolveTarget(ctx, request.Target)
	if err != nil {
		return notifications.DeliveryResult{ErrorCode: "network_policy_rejected"}
	}
	if strings.TrimSpace(request.WebhookToken) == "" || len(request.WebhookToken) > 4096 ||
		strings.ContainsAny(request.WebhookToken, "\r\n\x00") {
		return notifications.DeliveryResult{ErrorCode: "secret_invalid"}
	}
	payload, err := json.Marshal(map[string]string{"text": formatMessage(request.Message)})
	if err != nil || len(payload) > 16*1024 {
		return notifications.DeliveryResult{ErrorCode: "payload_invalid"}
	}
	host := canonicalHost(request.Target.Host)
	hostPort := net.JoinHostPort(host, strconv.Itoa(request.Target.Port))
	if request.Target.Port == 443 {
		hostPort = host
		if strings.Contains(host, ":") {
			hostPort = "[" + host + "]"
		}
	}
	pathPrefix := strings.TrimSuffix(request.Target.PathPrefix, "/")
	path := pathPrefix + "/" + request.WebhookToken
	rawPath := pathPrefix + "/" + url.PathEscape(request.WebhookToken)
	endpoint := (&url.URL{Scheme: "https", Host: hostPort, Path: path, RawPath: rawPath}).String()
	requestContext, cancel := context.WithTimeout(ctx, driver.requestTimeout)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(requestContext, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return notifications.DeliveryResult{ErrorCode: "request_invalid"}
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("User-Agent", "Espial/1 notification-delivery")
	transport := &http.Transport{
		Proxy: nil, DisableCompression: true, ForceAttemptHTTP2: false,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host, RootCAs: driver.rootCAs},
		TLSHandshakeTimeout: driver.requestTimeout, ResponseHeaderTimeout: driver.requestTimeout,
		MaxResponseHeaderBytes: 8192,
	}
	transport.DialContext = pinnedDialer(addresses, request.Target.Port, driver.requestTimeout)
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: driver.requestTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(httpRequest)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(requestContext.Err(), context.DeadlineExceeded) {
			return notifications.DeliveryResult{Retryable: true, ErrorCode: "timeout"}
		}
		return notifications.DeliveryResult{Retryable: true, ErrorCode: "connection_failed"}
	}
	defer response.Body.Close()
	read, readErr := io.ReadAll(io.LimitReader(response.Body, driver.responseLimit+1))
	if readErr != nil {
		return notifications.DeliveryResult{Retryable: true, HTTPStatus: response.StatusCode, ErrorCode: "response_read_failed"}
	}
	providerID := safeProviderID(response.Header.Get("X-Request-ID"))
	if int64(len(read)) > driver.responseLimit {
		return notifications.DeliveryResult{HTTPStatus: response.StatusCode, ErrorCode: "response_too_large", ProviderRequestID: providerID}
	}
	result := notifications.DeliveryResult{HTTPStatus: response.StatusCode, ProviderRequestID: providerID}
	switch {
	case response.StatusCode >= 200 && response.StatusCode < 300:
		result.Delivered = true
	case response.StatusCode == http.StatusTooManyRequests:
		result.Retryable, result.ErrorCode = true, "rate_limited"
		result.RetryAfter = parseRetryAfter(response.Header.Get("Retry-After"), time.Now().UTC())
	case response.StatusCode >= 500:
		result.Retryable, result.ErrorCode = true, "provider_unavailable"
	case response.StatusCode >= 300 && response.StatusCode < 400:
		result.ErrorCode = "redirect_rejected"
	default:
		result.ErrorCode = "provider_rejected"
	}
	return result
}

func (driver *Driver) resolveTarget(ctx context.Context, target notifications.Target) ([]netip.Addr, error) {
	host := canonicalHost(target.Host)
	if _, exists := driver.hosts[host]; !exists {
		return nil, notifications.ErrNetworkPolicy
	}
	if _, exists := driver.ports[target.Port]; !exists {
		return nil, notifications.ErrNetworkPolicy
	}
	if !safePathPrefix.MatchString(target.PathPrefix) || strings.Contains(target.PathPrefix, "..") {
		return nil, notifications.ErrNetworkPolicy
	}
	addresses := []netip.Addr{}
	if literal, err := netip.ParseAddr(host); err == nil {
		addresses = append(addresses, literal.Unmap())
	} else {
		resolveContext, cancel := context.WithTimeout(ctx, driver.resolveTimeout)
		resolved, err := driver.resolver.LookupNetIP(resolveContext, "ip", host)
		cancel()
		if err != nil {
			return nil, notifications.ErrNetworkPolicy
		}
		for _, item := range resolved {
			if !item.IsValid() {
				return nil, notifications.ErrNetworkPolicy
			}
			addresses = append(addresses, item.Unmap())
		}
	}
	if len(addresses) == 0 {
		return nil, notifications.ErrNetworkPolicy
	}
	for _, address := range addresses {
		allowed := false
		for _, prefix := range driver.networks {
			if prefix.Contains(address) {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, notifications.ErrNetworkPolicy
		}
	}
	return addresses, nil
}

func pinnedDialer(addresses []netip.Addr, port int, timeout time.Duration) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, _ string) (net.Conn, error) {
		var last error
		dialer := net.Dialer{Timeout: timeout}
		for _, address := range addresses {
			connection, err := dialer.DialContext(ctx, network, net.JoinHostPort(address.String(), strconv.Itoa(port)))
			if err == nil {
				return connection, nil
			}
			last = err
		}
		return nil, last
	}
}

func formatMessage(message notifications.Message) string {
	if message.Test {
		return "**Espial test notification**\n\nThis is an explicitly labeled test delivery.\n\nEvent ID: `" + escapeText(message.EventID) + "`"
	}
	lines := []string{
		"**Espial incident " + escapeText(message.Kind) + "**",
		"", "Title: " + escapeText(message.Title),
		"Severity: " + escapeText(message.Severity),
		"Status: " + escapeText(message.Status),
		"Summary: " + escapeText(message.Summary),
		"Event ID: `" + escapeText(message.EventID) + "`",
	}
	if message.IncidentURL != "" {
		lines = append(lines, "Incident: "+message.IncidentURL)
	}
	return strings.Join(lines, "\n")
}

func escapeText(value string) string {
	value = strings.Map(func(current rune) rune {
		if unicode.IsControl(current) {
			return ' '
		}
		return current
	}, value)
	replacer := strings.NewReplacer(
		"\\", "\\\\", "*", "\\*", "_", "\\_", "~", "\\~",
		"`", "\\`", "[", "\\[", "]", "\\]", "<", "\\<", ">", "\\>",
		"@", "@\u200b",
	)
	return replacer.Replace(value)
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		duration := time.Duration(seconds) * time.Second
		if duration > maximumRetryAfter {
			return maximumRetryAfter
		}
		return duration
	}
	if parsed, err := http.ParseTime(value); err == nil && parsed.After(now) {
		duration := parsed.Sub(now)
		if duration > maximumRetryAfter {
			return maximumRetryAfter
		}
		return duration
	}
	return 0
}

func safeProviderID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 || strings.ContainsFunc(value, unicode.IsControl) {
		return ""
	}
	return value
}

func canonicalHost(value string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
}

var _ notifications.Driver = (*Driver)(nil)
var _ notifications.DestinationValidator = (*Driver)(nil)
