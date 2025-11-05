package behavior

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/openshift-online/ocm-sdk-go/logging"

	"github.com/tzvatot/openshift-hive-simulator/pkg/config"
)

// Engine manages behavior configuration and per-resource overrides
type Engine struct {
	logger    logging.Logger
	config    *config.Config
	overrides map[string]*config.ResourceOverride
	mu        sync.RWMutex
	rng       *rand.Rand
}

// NewEngine creates a new behavior engine
func NewEngine(logger logging.Logger, cfg *config.Config) *Engine {
	return &Engine{
		logger:    logger,
		config:    cfg,
		overrides: make(map[string]*config.ResourceOverride),
		rng:       rand.New(rand.NewSource(time.Now().UTC().UnixNano())),
	}
}

// GetConfig returns the current configuration (thread-safe copy)
func (e *Engine) GetConfig() *config.Config {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Return a copy to prevent external modifications
	return e.config
}

// UpdateClusterDeploymentConfig updates ClusterDeployment configuration
func (e *Engine) UpdateClusterDeploymentConfig(ctx context.Context, cfg *config.ClusterDeploymentConfig) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.logger.Info(ctx, "Updating ClusterDeployment configuration: defaultDelay=%ds", cfg.DefaultDelaySeconds)
	e.config.ClusterDeployment = cfg
}

// UpdateAccountClaimConfig updates AccountClaim configuration
func (e *Engine) UpdateAccountClaimConfig(ctx context.Context, cfg *config.AccountClaimConfig) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.logger.Info(ctx, "Updating AccountClaim configuration: defaultDelay=%ds", cfg.DefaultDelaySeconds)
	e.config.AccountClaim = cfg
}

// UpdateProjectClaimConfig updates ProjectClaim configuration
func (e *Engine) UpdateProjectClaimConfig(ctx context.Context, cfg *config.ProjectClaimConfig) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.logger.Info(ctx, "Updating ProjectClaim configuration: defaultDelay=%ds", cfg.DefaultDelaySeconds)
	e.config.ProjectClaim = cfg
}

// SetResourceOverride sets an override for a specific resource
func (e *Engine) SetResourceOverride(ctx context.Context, resourceType, namespace, name string, override *config.ResourceOverride) {
	e.mu.Lock()
	defer e.mu.Unlock()

	key := e.makeKey(resourceType, namespace, name)
	e.logger.Info(ctx, "Setting override for %s: %s", resourceType, key)
	e.overrides[key] = override
}

// ClearResourceOverride clears an override for a specific resource
func (e *Engine) ClearResourceOverride(ctx context.Context, resourceType, namespace, name string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	key := e.makeKey(resourceType, namespace, name)
	e.logger.Info(ctx, "Clearing override for %s: %s", resourceType, key)
	delete(e.overrides, key)
}

// ClearAllOverrides clears all resource overrides
func (e *Engine) ClearAllOverrides(ctx context.Context) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.logger.Info(ctx, "Clearing all resource overrides (%d total)", len(e.overrides))
	e.overrides = make(map[string]*config.ResourceOverride)
}

// ShouldFail determines if a resource should fail based on configuration and overrides
func (e *Engine) ShouldFail(ctx context.Context, resourceType, namespace, name string) (bool, *config.FailureScenario) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	key := e.makeKey(resourceType, namespace, name)

	// Check for resource-specific override
	if override, exists := e.overrides[key]; exists {
		// If ForceSuccess is set, never fail
		if override.ForceSuccess {
			e.logger.Debug(ctx, "Resource %s has ForceSuccess=true, skipping failure", key)
			return false, nil
		}

		// If ForceFail is set, always fail
		if override.ForceFail != nil {
			e.logger.Info(ctx, "Resource %s has forced failure: %s", key, override.ForceFail.Message)
			return true, override.ForceFail
		}
	}

	// Check probabilistic failures from configuration
	var scenarios []config.FailureScenario
	switch resourceType {
	case "ClusterDeployment":
		if e.config.ClusterDeployment != nil {
			scenarios = e.config.ClusterDeployment.FailureScenarios
		}
	case "AccountClaim":
		if e.config.AccountClaim != nil {
			scenarios = e.config.AccountClaim.FailureScenarios
		}
	case "ProjectClaim":
		if e.config.ProjectClaim != nil {
			scenarios = e.config.ProjectClaim.FailureScenarios
		}
	}

	for i := range scenarios {
		scenario := &scenarios[i]
		if scenario.Probability > 0 {
			roll := e.rng.Float64()
			if roll < scenario.Probability {
				e.logger.Info(ctx, "Resource %s failed probabilistic check (%.2f < %.2f): %s",
					key, roll, scenario.Probability, scenario.Message)
				return true, scenario
			}
		}
	}

	return false, nil
}

// GetTransitionDelay gets the transition delay for a resource
func (e *Engine) GetTransitionDelay(ctx context.Context, resourceType, namespace, name string, defaultDuration time.Duration) time.Duration {
	e.mu.RLock()
	defer e.mu.RUnlock()

	key := e.makeKey(resourceType, namespace, name)

	// Check for resource-specific override
	if override, exists := e.overrides[key]; exists {
		if override.DelaySeconds != nil {
			duration := time.Duration(*override.DelaySeconds) * time.Second
			e.logger.Debug(ctx, "Resource %s has delay override: %v", key, duration)
			return duration
		}
	}

	return defaultDuration
}

// GetClusterDeploymentConfig returns the ClusterDeployment configuration
func (e *Engine) GetClusterDeploymentConfig() *config.ClusterDeploymentConfig {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.config.ClusterDeployment
}

// GetAccountClaimConfig returns the AccountClaim configuration
func (e *Engine) GetAccountClaimConfig() *config.AccountClaimConfig {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.config.AccountClaim
}

// GetProjectClaimConfig returns the ProjectClaim configuration
func (e *Engine) GetProjectClaimConfig() *config.ProjectClaimConfig {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.config.ProjectClaim
}

// GetClusterImageSetsConfig returns the ClusterImageSets configuration
func (e *Engine) GetClusterImageSetsConfig() []config.ClusterImageSetConfig {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.config.ClusterImageSets
}

// makeKey creates a unique key for a resource
func (e *Engine) makeKey(resourceType, namespace, name string) string {
	return fmt.Sprintf("%s/%s/%s", resourceType, namespace, name)
}
