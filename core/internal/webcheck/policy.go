package webcheck

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

type Resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

type PolicyOptions struct {
	ApprovedHosts  []string
	ApprovedCIDRs  []string
	AllowedPorts   []int
	ResolveTimeout time.Duration
	BodyLimit      int64
	HeaderLimit    int64
	MaxRedirects   int
	Resolver       Resolver
	Dialer         func(context.Context, string, string) (net.Conn, error)
}

type Policy struct {
	hosts          map[string]struct{}
	networks       []netip.Prefix
	ports          map[int]struct{}
	resolveTimeout time.Duration
	bodyLimit      int64
	headerLimit    int64
	maxRedirects   int
	resolver       Resolver
	dialer         func(context.Context, string, string) (net.Conn, error)
}

func NewPolicy(options PolicyOptions) (*Policy, error) {
	policy := &Policy{hosts: map[string]struct{}{}, networks: []netip.Prefix{}, ports: map[int]struct{}{},
		resolveTimeout: options.ResolveTimeout, bodyLimit: options.BodyLimit, headerLimit: options.HeaderLimit,
		maxRedirects: options.MaxRedirects, resolver: options.Resolver, dialer: options.Dialer}
	if policy.resolveTimeout <= 0 {
		policy.resolveTimeout = 2 * time.Second
	}
	if policy.bodyLimit <= 0 {
		policy.bodyLimit = DefaultBodyLimit
	}
	if policy.headerLimit <= 0 {
		policy.headerLimit = DefaultHeaderLimit
	}
	if policy.maxRedirects < 0 || policy.maxRedirects > 5 {
		return nil, errors.New("website redirect policy is invalid")
	}
	if policy.resolver == nil {
		policy.resolver = net.DefaultResolver
	}
	if policy.dialer == nil {
		dialer := &net.Dialer{}
		policy.dialer = dialer.DialContext
	}
	for _, host := range options.ApprovedHosts {
		host = canonicalHost(host)
		if host == "" || strings.ContainsAny(host, "/?#@") {
			return nil, errors.New("invalid approved website host")
		}
		policy.hosts[host] = struct{}{}
	}
	for _, item := range options.ApprovedCIDRs {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(item))
		if err != nil {
			return nil, fmt.Errorf("invalid approved website CIDR: %w", err)
		}
		policy.networks = append(policy.networks, prefix.Masked())
	}
	for _, port := range options.AllowedPorts {
		if port < 1 || port > 65535 {
			return nil, errors.New("invalid approved website port")
		}
		policy.ports[port] = struct{}{}
	}
	if policy.bodyLimit < 1 || policy.bodyLimit > 4*1024*1024 || policy.headerLimit < 1024 || policy.headerLimit > 128*1024 {
		return nil, errors.New("website response bounds are invalid")
	}
	return policy, nil
}

func (policy *Policy) Resolve(ctx context.Context, host string, port int) ([]netip.Addr, error) {
	host = canonicalHost(host)
	if _, exists := policy.hosts[host]; !exists {
		return nil, errors.New("host_not_approved")
	}
	if _, exists := policy.ports[port]; !exists {
		return nil, errors.New("port_not_approved")
	}
	addresses := []netip.Addr{}
	if literal, err := netip.ParseAddr(host); err == nil {
		addresses = append(addresses, literal.Unmap())
	} else {
		resolveContext, cancel := context.WithTimeout(ctx, policy.resolveTimeout)
		resolved, err := policy.resolver.LookupNetIP(resolveContext, "ip", host)
		cancel()
		if err != nil {
			return nil, errors.New("dns_failed")
		}
		for _, address := range resolved {
			addresses = append(addresses, address.Unmap())
		}
	}
	if len(addresses) == 0 {
		return nil, errors.New("dns_no_addresses")
	}
	seen := map[netip.Addr]struct{}{}
	approved := make([]netip.Addr, 0, len(addresses))
	for _, address := range addresses {
		if !address.IsValid() {
			return nil, errors.New("address_not_approved")
		}
		allowed := false
		for _, prefix := range policy.networks {
			if prefix.Contains(address) {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, errors.New("address_not_approved")
		}
		if _, exists := seen[address]; !exists {
			seen[address] = struct{}{}
			approved = append(approved, address)
		}
	}
	return approved, nil
}

func (policy *Policy) AllowsTarget(host string, port int) bool {
	_, hostAllowed := policy.hosts[canonicalHost(host)]
	_, portAllowed := policy.ports[port]
	return hostAllowed && portAllowed
}

func (policy *Policy) AllowsRedirects(count int) bool {
	return count >= 0 && count <= policy.maxRedirects
}

func canonicalHost(value string) string {
	value = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".")
	return strings.Trim(value, "[]")
}

func targetPort(scheme, explicit string) (int, error) {
	if explicit != "" {
		port, err := strconv.Atoi(explicit)
		if err != nil || port < 1 || port > 65535 {
			return 0, errors.New("invalid_port")
		}
		return port, nil
	}
	if scheme == "https" {
		return 443, nil
	}
	return 80, nil
}
