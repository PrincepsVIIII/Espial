package proxmox

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultBodyLimit = 2 * 1024 * 1024
const activeTaskLimit = "512"

type Client struct {
	baseURL                   *url.URL
	tokenID                   string
	tokenSecret               string
	httpClient                *http.Client
	bodyLimit                 int64
	expectedCertificateSHA256 string
	certificateMu             sync.Mutex
	observedCertificateSHA256 string
}

type ClientOptions struct {
	HTTPClient    *http.Client
	BodyLimit     int64
	ApprovedHosts []string
	AllowedPorts  []int
}

func NewClient(config Config, options ClientOptions) (*Client, error) {
	config = WithDefaults(config)
	if err := ValidateConfig(config); err != nil {
		return nil, err
	}
	baseURL, err := url.Parse(config.BaseURL)
	if err != nil {
		return nil, errors.New("invalid proxmox config")
	}
	if !allowsTarget(baseURL, options.ApprovedHosts, options.AllowedPorts) {
		return nil, errors.New("target_not_approved")
	}
	client := &Client{
		baseURL: baseURL, tokenID: config.TokenID, tokenSecret: config.TokenSecret,
		expectedCertificateSHA256: config.TrustedCertificateSHA256,
	}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{
			Timeout:   RequestTimeout(config),
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   RequestTimeout(config),
		ResponseHeaderTimeout: RequestTimeout(config),
		ExpectContinueTimeout: time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
	}
	if config.AllowUnverifiedTLS {
		transport.TLSClientConfig = &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true, // Explicit per-integration operator override; see Config validation.
		}
	} else if config.TrustSelfSignedCertificate {
		transport.TLSClientConfig = &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true, // Verification is replaced by the exact certificate pin below.
			VerifyConnection:   client.verifyPinnedCertificate,
		}
	}
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Transport: transport,
			Timeout:   RequestTimeout(config),
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	bodyLimit := options.BodyLimit
	if bodyLimit <= 0 || bodyLimit > defaultBodyLimit {
		bodyLimit = defaultBodyLimit
	}
	client.httpClient = httpClient
	client.bodyLimit = bodyLimit
	return client, nil
}

func (client *Client) verifyPinnedCertificate(state tls.ConnectionState) error {
	if len(state.PeerCertificates) == 0 {
		return errors.New("tls_certificate_missing")
	}
	digest := sha256.Sum256(state.PeerCertificates[0].Raw)
	fingerprint := hex.EncodeToString(digest[:])
	client.certificateMu.Lock()
	defer client.certificateMu.Unlock()
	expected := client.expectedCertificateSHA256
	if expected == "" {
		expected = client.observedCertificateSHA256
	}
	if expected != "" && fingerprint != expected {
		return errors.New("tls_certificate_changed")
	}
	client.observedCertificateSHA256 = fingerprint
	return nil
}

// CertificateFingerprint returns the SHA-256 pin observed while validating an
// explicitly trusted self-signed target. It never returns certificate contents.
func (client *Client) CertificateFingerprint() string {
	client.certificateMu.Lock()
	defer client.certificateMu.Unlock()
	return client.observedCertificateSHA256
}

func allowsTarget(target *url.URL, hosts []string, ports []int) bool {
	host := strings.ToLower(target.Hostname())
	if host == "" || len(hosts) == 0 {
		return false
	}
	port := target.Port()
	if port == "" {
		port = "443"
	}
	portAllowed := len(ports) == 0 && port == "8006"
	for _, allowed := range ports {
		if port == fmt.Sprint(allowed) {
			portAllowed = true
			break
		}
	}
	if !portAllowed {
		return false
	}
	for _, allowed := range hosts {
		if strings.EqualFold(strings.TrimSpace(allowed), host) {
			return true
		}
	}
	return false
}

// GetVersion fetches Proxmox release/build identity from /version so Espial can
// record compatibility evidence for the source that produced the collection.
func (client *Client) GetVersion(ctx context.Context) (VersionResponse, error) {
	return getJSON[VersionResponse](ctx, client, "/api2/json/version")
}

// GetClusterStatus fetches quorum and cluster-member state from /cluster/status
// so Espial can determine whether cluster membership itself is healthy.
func (client *Client) GetClusterStatus(ctx context.Context) ([]ClusterStatusItem, error) {
	return getJSON[[]ClusterStatusItem](ctx, client, "/api2/json/cluster/status")
}

// GetClusterResources fetches the bounded current node, guest, and storage list
// from /cluster/resources so Espial can map inventory and current runtime state.
func (client *Client) GetClusterResources(ctx context.Context) ([]ClusterResource, error) {
	return getJSON[[]ClusterResource](ctx, client, "/api2/json/cluster/resources")
}

// GetActiveTasks fetches the bounded active task list for one node. Proxmox
// exposes task reads below /nodes/{node}/tasks; callers with Sys.Audit on that
// node can see active console tasks regardless of which user opened them.
func (client *Client) GetActiveTasks(ctx context.Context, nodeID string) ([]Task, error) {
	if strings.TrimSpace(nodeID) == "" || strings.ContainsAny(nodeID, "/?#") {
		return nil, errors.New("invalid_node")
	}
	query := url.Values{
		"source":     []string{"active"},
		"typefilter": []string{"vncproxy"},
		"limit":      []string{activeTaskLimit},
	}
	path := "/api2/json/nodes/" + url.PathEscape(nodeID) + "/tasks?" + query.Encode()
	return getJSON[[]Task](ctx, client, path)
}

// GetGuestConfig reads one running guest's configuration only to resolve its
// bootable main disk to a configured storage name. The caller retains no raw
// configuration data after building the cluster aggregate.
func (client *Client) GetGuestConfig(ctx context.Context, nodeID, guestType string, vmid int) (GuestConfig, error) {
	if strings.TrimSpace(nodeID) == "" || strings.ContainsAny(nodeID, "/?#") {
		return nil, errors.New("invalid_node")
	}
	if guestType != "qemu" && guestType != "lxc" {
		return nil, errors.New("invalid_guest_type")
	}
	if vmid <= 0 {
		return nil, errors.New("invalid_vmid")
	}
	path := "/api2/json/nodes/" + url.PathEscape(nodeID) + "/" + guestType + "/" + strconv.Itoa(vmid) + "/config"
	return getJSON[GuestConfig](ctx, client, path)
}

// GetNodeStatus fetches CPU, memory, swap, and I/O-wait counters for one node from
// /nodes/{node}/status so Espial can render host bars and I/O-wait indicators.
func (client *Client) GetNodeStatus(ctx context.Context, nodeID string) (NodeStatus, error) {
	if strings.TrimSpace(nodeID) == "" || strings.ContainsAny(nodeID, "/?#") {
		return NodeStatus{}, errors.New("invalid_node")
	}
	return getJSON[NodeStatus](ctx, client, "/api2/json/nodes/"+url.PathEscape(nodeID)+"/status")
}

func getJSON[T any](ctx context.Context, client *Client, path string) (T, error) {
	var zero T
	relative, err := url.Parse(path)
	if err != nil {
		return zero, errors.New("invalid_request")
	}
	target := client.baseURL.ResolveReference(relative)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return zero, errors.New("invalid_request")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Espial/1 proxmox")
	request.Header.Set("Authorization", "PVEAPIToken="+client.tokenID+"="+client.tokenSecret)
	response, err := client.httpClient.Do(request)
	if err != nil {
		return zero, classifyTransportError(ctx, err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized {
		return zero, errors.New("unauthorized")
	}
	if response.StatusCode == http.StatusForbidden {
		return zero, errors.New("forbidden")
	}
	if response.StatusCode == http.StatusTooManyRequests {
		return zero, errors.New("throttled")
	}
	if response.StatusCode == http.StatusNotFound {
		return zero, errors.New("proxmox_api_not_found")
	}
	if response.StatusCode >= 500 {
		return zero, errors.New("temporary_upstream_failure")
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return zero, errors.New("upstream_request_rejected")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, client.bodyLimit+1))
	if err != nil {
		return zero, errors.New("temporary_upstream_failure")
	}
	if int64(len(body)) > client.bodyLimit {
		return zero, errors.New("oversized_response")
	}
	return decodeData[T](body)
}

func classifyTransportError(ctx context.Context, err error) error {
	if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) {
		return errors.New("timeout")
	}
	if hasErrorCode(err, "tls_certificate_changed") || hasErrorCode(err, "tls_certificate_missing") {
		if hasErrorCode(err, "tls_certificate_changed") {
			return errors.New("tls_certificate_changed")
		}
		return errors.New("tls_certificate_missing")
	}
	var unknownAuthority x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthority) {
		return errors.New("tls_certificate_untrusted")
	}
	var hostnameError x509.HostnameError
	if errors.As(err, &hostnameError) {
		return errors.New("tls_hostname_mismatch")
	}
	var certificateError x509.CertificateInvalidError
	if errors.As(err, &certificateError) {
		return errors.New("tls_certificate_invalid")
	}
	var recordError tls.RecordHeaderError
	if errors.As(err, &recordError) {
		return errors.New("tls_handshake_failed")
	}
	var dnsError *net.DNSError
	if errors.As(err, &dnsError) {
		return errors.New("dns_failed")
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return errors.New("timeout")
	}
	var operationError *net.OpError
	if errors.As(err, &operationError) {
		return errors.New("connection_failed")
	}
	return errors.New("temporary_upstream_failure")
}

func hasErrorCode(err error, code string) bool {
	for current := err; current != nil; current = errors.Unwrap(current) {
		if current.Error() == code {
			return true
		}
	}
	return false
}
