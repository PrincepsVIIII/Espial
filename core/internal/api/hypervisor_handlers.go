package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/PrincepsVIIII/Espial/core/internal/hypervisor"
	"github.com/PrincepsVIIII/Espial/core/internal/monitoring"
)

func (server *server) proxmoxClusterList(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.authorize(w, r, "resources:read", "hypervisor.proxmox.clusters.read"); !ok {
		return
	}
	if server.dependencies.Hypervisor == nil {
		server.error(w, r, http.StatusServiceUnavailable, "unavailable", "Hypervisor reads are unavailable.")
		return
	}
	result, err := server.dependencies.Hypervisor.ProxmoxClusters(r.Context())
	if err != nil {
		server.internalError(w, r, "proxmox cluster read failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (server *server) createProxmoxCluster(w http.ResponseWriter, r *http.Request) {
	session, ok := server.mutationSession(w, r, "integrations:manage", "hypervisor.proxmox.cluster.create")
	if !ok {
		return
	}
	if server.dependencies.Hypervisor == nil {
		server.error(w, r, http.StatusServiceUnavailable, "unavailable", "Hypervisor management is unavailable.")
		return
	}
	var body struct {
		DisplayName            string   `json:"display_name"`
		BaseURL                string   `json:"base_url"`
		DeploymentID           string   `json:"deployment_id"`
		TokenID                string   `json:"token_id"`
		TokenSecret            string   `json:"token_secret"`
		IntervalSeconds        int      `json:"interval_seconds"`
		ExpectedRefreshSeconds int      `json:"expected_refresh_seconds"`
		RequestTimeoutSeconds  int      `json:"request_timeout_seconds"`
		StorageWarningPercent  float64  `json:"storage_warning_percent"`
		StorageCriticalPercent float64  `json:"storage_critical_percent"`
		SwapWarningPercent     float64  `json:"swap_warning_percent"`
		SwapCriticalPercent    float64  `json:"swap_critical_percent"`
		IOWaitWarningPercent   float64  `json:"io_wait_warning_percent"`
		IOWaitCriticalPercent  float64  `json:"io_wait_critical_percent"`
		ExpectedRunningGuests  []string `json:"expected_running_guests"`
	}
	if err := decodeJSONLimit(w, r, &body, 64*1024); err != nil {
		server.decodeError(w, r, err)
		return
	}
	if strings.TrimSpace(body.DisplayName) == "" || strings.TrimSpace(body.BaseURL) == "" ||
		strings.TrimSpace(body.DeploymentID) == "" || strings.TrimSpace(body.TokenID) == "" ||
		strings.TrimSpace(body.TokenSecret) == "" {
		server.validationError(w, r, []APIFieldError{{Field: "body", Code: "required_fields"}})
		return
	}
	receipt, err := server.dependencies.Hypervisor.CreateProxmoxCluster(r.Context(), hypervisor.ProxmoxClusterDefinition{
		DisplayName: body.DisplayName, BaseURL: body.BaseURL, DeploymentID: body.DeploymentID,
		TokenID: body.TokenID, TokenSecret: body.TokenSecret,
		IntervalSeconds: body.IntervalSeconds, ExpectedRefreshSeconds: body.ExpectedRefreshSeconds,
		RequestTimeoutSeconds: body.RequestTimeoutSeconds, StorageWarningPercent: body.StorageWarningPercent,
		StorageCriticalPercent: body.StorageCriticalPercent, SwapWarningPercent: body.SwapWarningPercent,
		SwapCriticalPercent: body.SwapCriticalPercent, IOWaitWarningPercent: body.IOWaitWarningPercent,
		IOWaitCriticalPercent: body.IOWaitCriticalPercent, ExpectedRunningGuests: body.ExpectedRunningGuests,
		ActorUserID: session.User.ID, SourceAddress: sourceAddress(r), CorrelationID: requestID(r),
		CanReadAudit: hasPermission(session.User.Permissions, "audit:read"),
	})
	if err != nil {
		var operational *monitoring.Error
		if errors.As(err, &operational) {
			server.validationError(w, r, []APIFieldError{{Field: "proxmox", Code: operational.Code}})
			return
		}
		server.internalError(w, r, "proxmox cluster creation failed", err)
		return
	}
	w.Header().Set("Location", "/api/v1/integrations/"+receipt.ID)
	writeJSON(w, http.StatusCreated, receipt)
}
