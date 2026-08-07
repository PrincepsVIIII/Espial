package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type AdapterConfig struct {
	BaseURL string `json:"base_url"`
	DeploymentID string `json:"deployment_id"`
	TokenID string `json:"token_id"`
	TokenSecret string `json:"token_secret"`
	ExpectedRefreshSecond int `json:"expected_refresh_seconds"`
	RequestTimeoutSecond int `json:"request_timeout_seconds"`
}
const (
	ConfigFilePath = "adapters/proxmox/adapter-config.json"
	ClusterResourcesEndpoint = "/api2/json/cluster/resources"
)

func loadAdapterConfig() (AdapterConfig, error) {
	data, err := os.ReadFile(ConfigFilePath)
	if err != nil {
		return AdapterConfig{}, err
	}

	var config AdapterConfig
	err = json.Unmarshal(data, &config)
	return config, err
}

func gatherProxmoxResources() ([]byte, error) {
	data := queryProxmox("/api2/json/cluster/resources")
	return data, nil
}

func queryProxmox(endpoint string) ([]byte, error) {
	config, err := loadAdapterConfig()
	if err != nil {
		return nil, err
	}

	client := http.Client{Timeout: time.Duration(config.RequestTimeoutSecond) * time.Second}

	request, err := http.NewRequest(
		http.MethodGet,
		strings.TrimRight(config.BaseURL, "/")+"/"+strings.TrimLeft(endpoint, "/"),
		nil,
	)
	if err != nil {
		return nil, err
	}

	request.Header.Set("Authorization", "PVEAPIToken="+config.TokenID+"="+config.TokenSecret)

	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Proxmox returned HTTP %d", response.StatusCode)
	}

	stats, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	fmt.Print(string(stats))
	return stats, nil
}


