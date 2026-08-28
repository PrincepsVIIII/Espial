package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/hypervisor"
	"github.com/PrincepsVIIII/Espial/core/internal/monitoring"
	"github.com/PrincepsVIIII/Espial/core/internal/proxmox"
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
	var body proxmoxClusterRequest
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
	if !body.ApproveTarget {
		server.validationError(w, r, []APIFieldError{{Field: "approve_target", Code: "required"}})
		return
	}
	if fields := proxmoxConfigFields(body); len(fields) > 0 {
		server.validationError(w, r, fields)
		return
	}
	receipt, err := server.dependencies.Hypervisor.CreateProxmoxCluster(r.Context(), body.definition(session.User.ID, sourceAddress(r), requestID(r), hasPermission(session.User.Permissions, "audit:read")))
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

type proxmoxClusterRequest struct {
	DisplayName                string   `json:"display_name"`
	BaseURL                    string   `json:"base_url"`
	ApproveTarget              bool     `json:"approve_target"`
	AllowInsecureHTTP          bool     `json:"allow_insecure_http"`
	TrustSelfSignedCertificate bool     `json:"trust_self_signed_certificate"`
	AllowUnverifiedTLS         bool     `json:"allow_unverified_tls"`
	DeploymentID               string   `json:"deployment_id"`
	TokenID                    string   `json:"token_id"`
	TokenSecret                string   `json:"token_secret"`
	IntervalSeconds            int      `json:"interval_seconds"`
	ExpectedRefreshSeconds     int      `json:"expected_refresh_seconds"`
	RequestTimeoutSeconds      int      `json:"request_timeout_seconds"`
	StorageWarningPercent      float64  `json:"storage_warning_percent"`
	StorageCriticalPercent     float64  `json:"storage_critical_percent"`
	SwapWarningPercent         float64  `json:"swap_warning_percent"`
	SwapCriticalPercent        float64  `json:"swap_critical_percent"`
	IOWaitWarningPercent       float64  `json:"io_wait_warning_percent"`
	IOWaitCriticalPercent      float64  `json:"io_wait_critical_percent"`
	ExpectedRunningGuests      []string `json:"expected_running_guests"`
}

func (body proxmoxClusterRequest) definition(actorID, address, correlationID string, canReadAudit bool) hypervisor.ProxmoxClusterDefinition {
	return hypervisor.ProxmoxClusterDefinition{
		DisplayName: body.DisplayName, BaseURL: body.BaseURL, ApproveTarget: body.ApproveTarget, AllowInsecureHTTP: body.AllowInsecureHTTP,
		TrustSelfSignedCertificate: body.TrustSelfSignedCertificate, AllowUnverifiedTLS: body.AllowUnverifiedTLS,
		DeploymentID: body.DeploymentID,
		TokenID:      body.TokenID, TokenSecret: body.TokenSecret, IntervalSeconds: body.IntervalSeconds,
		ExpectedRefreshSeconds: body.ExpectedRefreshSeconds, RequestTimeoutSeconds: body.RequestTimeoutSeconds,
		StorageWarningPercent: body.StorageWarningPercent, StorageCriticalPercent: body.StorageCriticalPercent,
		SwapWarningPercent: body.SwapWarningPercent, SwapCriticalPercent: body.SwapCriticalPercent,
		IOWaitWarningPercent: body.IOWaitWarningPercent, IOWaitCriticalPercent: body.IOWaitCriticalPercent,
		ExpectedRunningGuests: body.ExpectedRunningGuests, ActorUserID: actorID, SourceAddress: address,
		CorrelationID: correlationID, CanReadAudit: canReadAudit,
	}
}

func (server *server) proxmoxClusterConfiguration(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.authorize(w, r, "integrations:manage", "hypervisor.proxmox.cluster.configuration.read"); !ok {
		return
	}
	if server.dependencies.Hypervisor == nil {
		server.error(w, r, http.StatusServiceUnavailable, "unavailable", "Hypervisor management is unavailable.")
		return
	}
	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		server.error(w, r, http.StatusNotFound, "not_found", "The requested cluster was not found.")
		return
	}
	configuration, err := server.dependencies.Hypervisor.ProxmoxClusterConfiguration(r.Context(), id)
	if err != nil {
		writeProxmoxConfigurationError(server, w, r, err)
		return
	}
	w.Header().Set("ETag", integrationETag(configuration.UpdatedAt))
	writeJSON(w, http.StatusOK, configuration)
}

func (server *server) updateProxmoxCluster(w http.ResponseWriter, r *http.Request) {
	session, ok := server.mutationSession(w, r, "integrations:manage", "hypervisor.proxmox.cluster.configuration.update")
	if !ok {
		return
	}
	if server.dependencies.Hypervisor == nil {
		server.error(w, r, http.StatusServiceUnavailable, "unavailable", "Hypervisor management is unavailable.")
		return
	}
	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		server.error(w, r, http.StatusNotFound, "not_found", "The requested cluster was not found.")
		return
	}
	ifMatch := strings.TrimSpace(r.Header.Get("If-Match"))
	if ifMatch == "" {
		server.error(w, r, http.StatusPreconditionRequired, "precondition_required", "If-Match is required.")
		return
	}
	expected, err := parseIntegrationETag(ifMatch)
	if err != nil {
		server.errorFields(w, r, http.StatusBadRequest, "validation_failed", "Request validation failed.", []APIFieldError{{Field: "If-Match", Code: "invalid"}})
		return
	}
	var body proxmoxClusterRequest
	if err := decodeJSONLimit(w, r, &body, 64*1024); err != nil {
		server.decodeError(w, r, err)
		return
	}
	if strings.TrimSpace(body.DisplayName) == "" || strings.TrimSpace(body.BaseURL) == "" ||
		strings.TrimSpace(body.DeploymentID) == "" || strings.TrimSpace(body.TokenID) == "" {
		server.validationError(w, r, []APIFieldError{{Field: "body", Code: "required_fields"}})
		return
	}
	if !body.ApproveTarget {
		server.validationError(w, r, []APIFieldError{{Field: "approve_target", Code: "required"}})
		return
	}
	validationBody := body
	if strings.TrimSpace(validationBody.TokenSecret) == "" {
		validationBody.TokenSecret = "existing-secret"
	}
	if fields := proxmoxConfigFields(validationBody); len(fields) > 0 {
		server.validationError(w, r, fields)
		return
	}
	receipt, err := server.dependencies.Hypervisor.UpdateProxmoxCluster(r.Context(), hypervisor.ProxmoxClusterUpdateDefinition{
		ProxmoxClusterDefinition: body.definition(session.User.ID, sourceAddress(r), requestID(r), hasPermission(session.User.Permissions, "audit:read")),
		ID:                       id, ExpectedUpdatedAt: expected,
	})
	if err != nil {
		writeProxmoxConfigurationError(server, w, r, err)
		return
	}
	w.Header().Set("ETag", integrationETag(time.UnixMicro(receipt.Version)))
	writeJSON(w, http.StatusOK, receipt)
}

func (server *server) reorderProxmoxCluster(w http.ResponseWriter, r *http.Request) {
	session, ok := server.mutationSession(w, r, "integrations:manage", "hypervisor.proxmox.cluster.order.update")
	if !ok {
		return
	}
	if server.dependencies.Hypervisor == nil {
		server.error(w, r, http.StatusServiceUnavailable, "unavailable", "Hypervisor management is unavailable.")
		return
	}
	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		server.error(w, r, http.StatusNotFound, "not_found", "The requested cluster was not found.")
		return
	}
	ifMatch := strings.TrimSpace(r.Header.Get("If-Match"))
	if ifMatch == "" {
		server.error(w, r, http.StatusPreconditionRequired, "precondition_required", "If-Match is required.")
		return
	}
	expected, err := parseIntegrationETag(ifMatch)
	if err != nil {
		server.errorFields(w, r, http.StatusBadRequest, "validation_failed", "Request validation failed.", []APIFieldError{{Field: "If-Match", Code: "invalid"}})
		return
	}
	var body struct {
		Direction string `json:"direction"`
	}
	if err := decodeJSONLimit(w, r, &body, 1024); err != nil {
		server.decodeError(w, r, err)
		return
	}
	if body.Direction != "up" && body.Direction != "down" {
		server.validationError(w, r, []APIFieldError{{Field: "direction", Code: "invalid"}})
		return
	}
	receipt, err := server.dependencies.Hypervisor.ReorderProxmoxCluster(r.Context(), hypervisor.ProxmoxClusterOrderDefinition{
		ID: id, Direction: body.Direction, ExpectedUpdatedAt: expected,
		ActorUserID: session.User.ID, SourceAddress: sourceAddress(r), CorrelationID: requestID(r),
		CanReadAudit: hasPermission(session.User.Permissions, "audit:read"),
	})
	if err != nil {
		writeProxmoxConfigurationError(server, w, r, err)
		return
	}
	w.Header().Set("ETag", integrationETag(time.UnixMicro(receipt.Version)))
	writeJSON(w, http.StatusOK, receipt)
}

func writeProxmoxConfigurationError(server *server, w http.ResponseWriter, r *http.Request, err error) {
	var operational *monitoring.Error
	if errors.As(err, &operational) {
		switch operational.Code {
		case "integration_not_found":
			server.error(w, r, http.StatusNotFound, "not_found", "The requested cluster was not found.")
		case "integration_config_conflict":
			server.error(w, r, http.StatusPreconditionFailed, "precondition_failed", "The cluster configuration changed; reopen it and retry.")
		default:
			server.validationError(w, r, []APIFieldError{{Field: "proxmox", Code: operational.Code}})
		}
		return
	}
	server.internalError(w, r, "proxmox cluster configuration failed", err)
}

func proxmoxConfigFields(body proxmoxClusterRequest) []APIFieldError {
	configFields := proxmox.ValidateConfigFields(proxmox.Config{
		BaseURL: body.BaseURL, DeploymentID: body.DeploymentID,
		TokenID: body.TokenID, TokenSecret: body.TokenSecret, AllowInsecureHTTP: body.AllowInsecureHTTP,
		TrustSelfSignedCertificate: body.TrustSelfSignedCertificate,
		AllowUnverifiedTLS:         body.AllowUnverifiedTLS,
		ExpectedRefreshSeconds:     body.ExpectedRefreshSeconds,
		RequestTimeoutSeconds:      body.RequestTimeoutSeconds,
		StorageWarningPercent:      body.StorageWarningPercent,
		StorageCriticalPercent:     body.StorageCriticalPercent,
		SwapWarningPercent:         body.SwapWarningPercent,
		SwapCriticalPercent:        body.SwapCriticalPercent,
		IOWaitWarningPercent:       body.IOWaitWarningPercent,
		IOWaitCriticalPercent:      body.IOWaitCriticalPercent,
		ExpectedRunningGuests:      body.ExpectedRunningGuests,
	})
	fields := make([]APIFieldError, 0, len(configFields))
	for _, field := range configFields {
		fields = append(fields, APIFieldError{Field: field.Field, Code: field.Code})
	}
	return fields
}
