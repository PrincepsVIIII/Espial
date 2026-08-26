package proxmox

import (
	"os"
	"strconv"
	"strings"
)

type Policy struct {
	ApprovedHosts []string
	AllowedPorts  []int
}

func PolicyFromEnvironment() Policy {
	return Policy{
		ApprovedHosts: split(os.Getenv("ESPIAL_PROXMOX_APPROVED_HOSTS")),
		AllowedPorts:  ports(os.Getenv("ESPIAL_PROXMOX_ALLOWED_PORTS")),
	}
}

func split(value string) []string {
	items := strings.Split(value, ",")
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

func ports(value string) []int {
	items := split(value)
	if len(items) == 0 {
		return []int{8006}
	}
	result := make([]int, 0, len(items))
	for _, item := range items {
		port, err := strconv.Atoi(item)
		if err == nil && port > 0 && port <= 65535 {
			result = append(result, port)
		}
	}
	return result
}
