package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/webcheck"
)

func main() {
	policy, err := webcheck.NewPolicy(webcheck.PolicyOptions{
		ApprovedHosts:  split(os.Getenv("ESPIAL_WEBCHECK_APPROVED_HOSTS")),
		ApprovedCIDRs:  split(os.Getenv("ESPIAL_WEBCHECK_APPROVED_CIDRS")),
		AllowedPorts:   integers(os.Getenv("ESPIAL_WEBCHECK_ALLOWED_PORTS")),
		ResolveTimeout: duration(os.Getenv("ESPIAL_WEBCHECK_RESOLVE_TIMEOUT"), 2*time.Second),
		BodyLimit:      integer64(os.Getenv("ESPIAL_WEBCHECK_BODY_LIMIT_BYTES"), webcheck.DefaultBodyLimit),
		HeaderLimit:    integer64(os.Getenv("ESPIAL_WEBCHECK_HEADER_LIMIT_BYTES"), webcheck.DefaultHeaderLimit),
		MaxRedirects:   integer(os.Getenv("ESPIAL_WEBCHECK_MAX_REDIRECTS"), 0),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "webcheck policy is invalid")
		os.Exit(1)
	}
	if err := webcheck.Run(os.Stdin, os.Stdout, webcheck.NewChecker(policy, webcheck.CheckerOptions{})); err != nil {
		os.Exit(1)
	}
}

func split(value string) []string {
	result := []string{}
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}
func integers(value string) []int {
	result := []int{}
	for _, item := range split(value) {
		parsed, err := strconv.Atoi(item)
		if err != nil {
			return nil
		}
		result = append(result, parsed)
	}
	return result
}
func duration(value string, fallback time.Duration) time.Duration {
	if parsed, err := time.ParseDuration(value); err == nil && parsed > 0 {
		return parsed
	}
	return fallback
}
func integer64(value string, fallback int64) int64 {
	if parsed, err := strconv.ParseInt(value, 10, 64); err == nil && parsed > 0 {
		return parsed
	}
	return fallback
}
func integer(value string, fallback int) int {
	if parsed, err := strconv.Atoi(value); err == nil && parsed >= 0 {
		return parsed
	}
	return fallback
}
