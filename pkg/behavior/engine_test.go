package behavior

import (
	"context"
	"testing"
	"time"

	"github.com/openshift-online/ocm-sdk-go/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tzvatot/openshift-hive-simulator/pkg/config"
)

func createTestLogger() logging.Logger {
	builder := logging.NewStdLoggerBuilder()
	builder.Info(true)
	logger, _ := builder.Build()
	return logger
}

func createTestConfig() *config.Config {
	return &config.Config{
		ClusterDeployment: &config.ClusterDeploymentConfig{
			DefaultDelaySeconds: 5,
			FailureScenarios: []config.FailureScenario{
				{
					Probability: 0.5,
					Condition:   "TestFailure",
					Message:     "Test failure message",
					Reason:      "TestReason",
				},
			},
		},
		AccountClaim: &config.AccountClaimConfig{
			DefaultDelaySeconds: 3,
		},
		ProjectClaim: &config.ProjectClaimConfig{
			DefaultDelaySeconds: 4,
		},
	}
}

func TestNewEngine(t *testing.T) {
	logger := createTestLogger()
	cfg := createTestConfig()

	engine := NewEngine(logger, cfg)

	assert.NotNil(t, engine)
	assert.NotNil(t, engine.logger)
	assert.NotNil(t, engine.config)
	assert.NotNil(t, engine.overrides)
	assert.NotNil(t, engine.rng)
}

func TestEngine_GetConfig(t *testing.T) {
	logger := createTestLogger()
	cfg := createTestConfig()
	engine := NewEngine(logger, cfg)

	retrievedConfig := engine.GetConfig()

	assert.NotNil(t, retrievedConfig)
	assert.Equal(t, cfg.ClusterDeployment.DefaultDelaySeconds, retrievedConfig.ClusterDeployment.DefaultDelaySeconds)
}

func TestEngine_UpdateConfigs(t *testing.T) {
	logger := createTestLogger()
	cfg := createTestConfig()
	engine := NewEngine(logger, cfg)
	ctx := context.Background()

	// Update ClusterDeployment config
	newCdConfig := &config.ClusterDeploymentConfig{
		DefaultDelaySeconds: 10,
	}
	engine.UpdateClusterDeploymentConfig(ctx, newCdConfig)
	assert.Equal(t, 10, engine.GetClusterDeploymentConfig().DefaultDelaySeconds)

	// Update AccountClaim config
	newAcConfig := &config.AccountClaimConfig{
		DefaultDelaySeconds: 7,
	}
	engine.UpdateAccountClaimConfig(ctx, newAcConfig)
	assert.Equal(t, 7, engine.GetAccountClaimConfig().DefaultDelaySeconds)

	// Update ProjectClaim config
	newPcConfig := &config.ProjectClaimConfig{
		DefaultDelaySeconds: 9,
	}
	engine.UpdateProjectClaimConfig(ctx, newPcConfig)
	assert.Equal(t, 9, engine.GetProjectClaimConfig().DefaultDelaySeconds)
}

func TestEngine_ResourceOverrides(t *testing.T) {
	logger := createTestLogger()
	cfg := createTestConfig()
	engine := NewEngine(logger, cfg)
	ctx := context.Background()

	resourceType := "ClusterDeployment"
	namespace := "default"
	name := "test-cluster"

	// Set override
	override := &config.ResourceOverride{
		ResourceName: name,
		DelaySeconds: intPtr(30),
	}
	engine.SetResourceOverride(ctx, resourceType, namespace, name, override)

	// Verify override exists
	delay := engine.GetTransitionDelay(ctx, resourceType, namespace, name, 5*time.Second)
	assert.Equal(t, 30*time.Second, delay)

	// Clear override
	engine.ClearResourceOverride(ctx, resourceType, namespace, name)

	// Verify override cleared
	delay = engine.GetTransitionDelay(ctx, resourceType, namespace, name, 5*time.Second)
	assert.Equal(t, 5*time.Second, delay)
}

func TestEngine_ShouldFail_ForceSuccess(t *testing.T) {
	logger := createTestLogger()
	cfg := createTestConfig()
	engine := NewEngine(logger, cfg)
	ctx := context.Background()

	resourceType := "ClusterDeployment"
	namespace := "default"
	name := "test-cluster"

	// Set ForceSuccess override
	override := &config.ResourceOverride{
		ResourceName: name,
		ForceSuccess: true,
	}
	engine.SetResourceOverride(ctx, resourceType, namespace, name, override)

	// Should never fail
	shouldFail, failure := engine.ShouldFail(ctx, resourceType, namespace, name)
	assert.False(t, shouldFail)
	assert.Nil(t, failure)
}

func TestEngine_ShouldFail_ForceFail(t *testing.T) {
	logger := createTestLogger()
	cfg := createTestConfig()
	engine := NewEngine(logger, cfg)
	ctx := context.Background()

	resourceType := "ClusterDeployment"
	namespace := "default"
	name := "test-cluster"

	// Set ForceFail override
	failureScenario := &config.FailureScenario{
		Condition: "ForcedFailure",
		Message:   "This is a forced failure",
		Reason:    "TestReason",
	}
	override := &config.ResourceOverride{
		ResourceName: name,
		ForceFail:    failureScenario,
	}
	engine.SetResourceOverride(ctx, resourceType, namespace, name, override)

	// Should always fail
	shouldFail, failure := engine.ShouldFail(ctx, resourceType, namespace, name)
	assert.True(t, shouldFail)
	require.NotNil(t, failure)
	assert.Equal(t, "ForcedFailure", failure.Condition)
	assert.Equal(t, "This is a forced failure", failure.Message)
}

func TestEngine_GetTransitionDelay_WithOverride(t *testing.T) {
	logger := createTestLogger()
	cfg := createTestConfig()
	engine := NewEngine(logger, cfg)
	ctx := context.Background()

	resourceType := "ClusterDeployment"
	namespace := "default"
	name := "test-cluster"

	// Without override
	delay := engine.GetTransitionDelay(ctx, resourceType, namespace, name, 5*time.Second)
	assert.Equal(t, 5*time.Second, delay)

	// With override
	override := &config.ResourceOverride{
		ResourceName: name,
		DelaySeconds: intPtr(20),
	}
	engine.SetResourceOverride(ctx, resourceType, namespace, name, override)

	delay = engine.GetTransitionDelay(ctx, resourceType, namespace, name, 5*time.Second)
	assert.Equal(t, 20*time.Second, delay)
}

func TestEngine_ClearAllOverrides(t *testing.T) {
	logger := createTestLogger()
	cfg := createTestConfig()
	engine := NewEngine(logger, cfg)
	ctx := context.Background()

	// Add multiple overrides
	engine.SetResourceOverride(ctx, "ClusterDeployment", "ns1", "cluster1", &config.ResourceOverride{
		ResourceName: "cluster1",
		DelaySeconds: intPtr(10),
	})
	engine.SetResourceOverride(ctx, "AccountClaim", "ns2", "account1", &config.ResourceOverride{
		ResourceName: "account1",
		DelaySeconds: intPtr(15),
	})

	// Verify overrides exist
	delay1 := engine.GetTransitionDelay(ctx, "ClusterDeployment", "ns1", "cluster1", 5*time.Second)
	assert.Equal(t, 10*time.Second, delay1)

	// Clear all
	engine.ClearAllOverrides(ctx)

	// Verify all cleared
	delay1 = engine.GetTransitionDelay(ctx, "ClusterDeployment", "ns1", "cluster1", 5*time.Second)
	delay2 := engine.GetTransitionDelay(ctx, "AccountClaim", "ns2", "account1", 5*time.Second)
	assert.Equal(t, 5*time.Second, delay1)
	assert.Equal(t, 5*time.Second, delay2)
}

func TestEngine_GetClusterImageSetsConfig(t *testing.T) {
	logger := createTestLogger()
	cfg := createTestConfig()
	cfg.ClusterImageSets = []config.ClusterImageSetConfig{
		{Name: "test-image-1", Visible: true},
		{Name: "test-image-2", Visible: false},
	}
	engine := NewEngine(logger, cfg)

	imageSets := engine.GetClusterImageSetsConfig()

	assert.Len(t, imageSets, 2)
	assert.Equal(t, "test-image-1", imageSets[0].Name)
	assert.True(t, imageSets[0].Visible)
	assert.Equal(t, "test-image-2", imageSets[1].Name)
	assert.False(t, imageSets[1].Visible)
}

// Helper function
func intPtr(i int) *int {
	return &i
}
