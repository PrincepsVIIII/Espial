package proxmox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultBodyLimit = 2 * 1024 * 1024

type Client struct {
	baseURL     *url.URL
	tokenID     string
	tokenSecret string
	httpClient  *http.Client
	bodyLimit   int64
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
	transport := &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{
			Timeout:   RequestTimeout(config),
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   RequestTimeout(config),
		ResponseHeaderTimeout: RequestTimeout(config),
		ExpectContinueTimeout: time.Second,
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
	return &Client{
		baseURL: baseURL, tokenID: config.TokenID, tokenSecret: config.TokenSecret,
		httpClient: httpClient, bodyLimit: bodyLimit,
	}, nil
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

// GetNodeStatus fetches CPU, memory, swap, and IO-wait counters for one node from
// /nodes/{node}/status so Espial can render host bars and IO-delay indicators.
func (client *Client) GetNodeStatus(ctx context.Context, nodeID string) (NodeStatus, error) {
	if strings.TrimSpace(nodeID) == "" || strings.ContainsAny(nodeID, "/?#") {
		return NodeStatus{}, errors.New("invalid_node")
	}
	return getJSON[NodeStatus](ctx, client, "/api2/json/nodes/"+url.PathEscape(nodeID)+"/status")
}

func getJSON[T any](ctx context.Context, client *Client, path string) (T, error) {
	var zero T
	target := client.baseURL.ResolveReference(&url.URL{Path: path})
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return zero, errors.New("invalid_request")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Espial/1 proxmox")
	request.Header.Set("Authorization", "PVEAPIToken="+client.tokenID+"="+client.tokenSecret)
	response, err := client.httpClient.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return zero, errors.New("timeout")
		}
		return zero, errors.New("temporary_upstream_failure")
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
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return zero, fmt.Errorf("upstream_status_%d", response.StatusCode)
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
