package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/openshift-online/ocm-sdk-go/logging"

	"github.com/tzvatot/openshift-hive-simulator/pkg/behavior"
	"github.com/tzvatot/openshift-hive-simulator/pkg/config"
)

// Handlers provides HTTP handlers for the simulator API
type Handlers struct {
	logger         logging.Logger
	behaviorEngine *behavior.Engine
	startTime      time.Time
}

// NewHandlers creates new API handlers
func NewHandlers(logger logging.Logger, behaviorEngine *behavior.Engine) *Handlers {
	return &Handlers{
		logger:         logger,
		behaviorEngine: behaviorEngine,
		startTime:      time.Now().UTC(),
	}
}

// GetConfig returns the current configuration
func (h *Handlers) GetConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	h.logger.Debug(ctx, "GET /api/v1/config")

	cfg := h.behaviorEngine.GetConfig()
	h.writeJSON(w, http.StatusOK, cfg)
}

// UpdateClusterDeploymentConfig updates ClusterDeployment configuration
func (h *Handlers) UpdateClusterDeploymentConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	h.logger.Debug(ctx, "POST /api/v1/config/clusterdeployment")

	var cfg config.ClusterDeploymentConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		h.writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	h.behaviorEngine.UpdateClusterDeploymentConfig(ctx, &cfg)
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// UpdateAccountClaimConfig updates AccountClaim configuration
func (h *Handlers) UpdateAccountClaimConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	h.logger.Debug(ctx, "POST /api/v1/config/accountclaim")

	var cfg config.AccountClaimConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		h.writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	h.behaviorEngine.UpdateAccountClaimConfig(ctx, &cfg)
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// UpdateProjectClaimConfig updates ProjectClaim configuration
func (h *Handlers) UpdateProjectClaimConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	h.logger.Debug(ctx, "POST /api/v1/config/projectclaim")

	var cfg config.ProjectClaimConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		h.writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	h.behaviorEngine.UpdateProjectClaimConfig(ctx, &cfg)
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// SetResourceFailure forces a failure for a specific resource
func (h *Handlers) SetResourceFailure(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	resourceType := vars["resourceType"]
	namespace := vars["namespace"]
	name := vars["name"]

	h.logger.Debug(ctx, "POST /api/v1/overrides/%s/%s/%s/failure", resourceType, namespace, name)

	var failure config.FailureScenario
	if err := json.NewDecoder(r.Body).Decode(&failure); err != nil {
		h.writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	override := &config.ResourceOverride{
		ResourceName: name,
		ForceFail:    &failure,
	}

	h.behaviorEngine.SetResourceOverride(ctx, resourceType, namespace, name, override)
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "failure set"})
}

// SetResourceDelay sets a delay override for a specific resource
func (h *Handlers) SetResourceDelay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	resourceType := vars["resourceType"]
	namespace := vars["namespace"]
	name := vars["name"]

	h.logger.Debug(ctx, "POST /api/v1/overrides/%s/%s/%s/delay", resourceType, namespace, name)

	var req struct {
		DelaySeconds int `json:"delaySeconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	override := &config.ResourceOverride{
		ResourceName: name,
		DelaySeconds: &req.DelaySeconds,
	}

	h.behaviorEngine.SetResourceOverride(ctx, resourceType, namespace, name, override)
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "delay set"})
}

// SetResourceSuccess forces success for a specific resource
func (h *Handlers) SetResourceSuccess(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	resourceType := vars["resourceType"]
	namespace := vars["namespace"]
	name := vars["name"]

	h.logger.Debug(ctx, "POST /api/v1/overrides/%s/%s/%s/success", resourceType, namespace, name)

	override := &config.ResourceOverride{
		ResourceName: name,
		ForceSuccess: true,
	}

	h.behaviorEngine.SetResourceOverride(ctx, resourceType, namespace, name, override)
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "forced success set"})
}

// ClearResourceOverride clears overrides for a specific resource
func (h *Handlers) ClearResourceOverride(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	resourceType := vars["resourceType"]
	namespace := vars["namespace"]
	name := vars["name"]

	h.logger.Debug(ctx, "DELETE /api/v1/overrides/%s/%s/%s", resourceType, namespace, name)

	h.behaviorEngine.ClearResourceOverride(ctx, resourceType, namespace, name)
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "override cleared"})
}

// Reset resets all overrides
func (h *Handlers) Reset(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	h.logger.Debug(ctx, "POST /api/v1/reset")

	h.behaviorEngine.ClearAllOverrides(ctx)
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "all overrides cleared"})
}

// GetStatus returns the simulator status
func (h *Handlers) GetStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	h.logger.Debug(ctx, "GET /api/v1/status")

	uptime := time.Since(h.startTime)
	status := map[string]interface{}{
		"healthy": true,
		"uptime":  uptime.String(),
	}

	h.writeJSON(w, http.StatusOK, status)
}

// writeJSON writes a JSON response
func (h *Handlers) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error(context.Background(), "Failed to encode JSON response: %v", err)
	}
}

// writeError writes an error response
func (h *Handlers) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, map[string]string{"error": message})
}
