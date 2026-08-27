package proxmox

import (
	"encoding/json"
	"errors"
)

type VersionResponse struct {
	Version string `json:"version"`
	Release string `json:"release"`
	RepoID  string `json:"repoid"`
}

type ClusterStatusItem struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Quorate *int   `json:"quorate,omitempty"`
}

type ClusterResource struct {
	ID       string  `json:"id"`
	Type     string  `json:"type"`
	Node     string  `json:"node"`
	Name     string  `json:"name"`
	Status   string  `json:"status"`
	Storage  string  `json:"storage"`
	Shared   *int    `json:"shared,omitempty"`
	VMID     int     `json:"vmid,omitempty"`
	CPU      float64 `json:"cpu,omitempty"`
	MaxCPU   float64 `json:"maxcpu,omitempty"`
	Mem      float64 `json:"mem,omitempty"`
	MaxMem   float64 `json:"maxmem,omitempty"`
	Disk     float64 `json:"disk,omitempty"`
	MaxDisk  float64 `json:"maxdisk,omitempty"`
	Uptime   float64 `json:"uptime,omitempty"`
	Template *int    `json:"template,omitempty"`
}

type NodeStatus struct {
	CPU     float64  `json:"cpu"`
	Wait    float64  `json:"wait"`
	Uptime  float64  `json:"uptime"`
	Memory  Usage    `json:"memory"`
	Swap    Usage    `json:"swap"`
	RootFS  Usage    `json:"rootfs"`
	LoadAvg []string `json:"loadavg"`
}

// Task is the bounded task-list evidence used to identify currently open
// Proxmox console proxies. It intentionally excludes task-log content and user
// identity because neither is required for the aggregate load comparison.
type Task struct {
	ID      string `json:"id"`
	Node    string `json:"node"`
	Type    string `json:"type"`
	EndTime *int64 `json:"endtime,omitempty"`
}

type Usage struct {
	Used  float64 `json:"used"`
	Total float64 `json:"total"`
	Free  float64 `json:"free"`
}

func decodeData[T any](body []byte) (T, error) {
	var envelope struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		var zero T
		return zero, errors.New("malformed_data")
	}
	return envelope.Data, nil
}
