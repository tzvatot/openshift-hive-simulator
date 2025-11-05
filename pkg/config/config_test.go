package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.NotNil(t, cfg)
	assert.NotNil(t, cfg.ClusterDeployment)
	assert.NotNil(t, cfg.AccountClaim)
	assert.NotNil(t, cfg.ProjectClaim)
	assert.NotEmpty(t, cfg.ClusterImageSets)

	// Verify ClusterDeployment defaults
	assert.Equal(t, 5, cfg.ClusterDeployment.DefaultDelaySeconds)
	assert.True(t, cfg.ClusterDeployment.DependsOnAccountClaim)
	assert.True(t, cfg.ClusterDeployment.DependsOnProjectClaim)
	assert.Len(t, cfg.ClusterDeployment.States, 4)

	// Verify state names
	assert.Equal(t, "Pending", cfg.ClusterDeployment.States[0].Name)
	assert.Equal(t, "Provisioning", cfg.ClusterDeployment.States[1].Name)
	assert.Equal(t, "Installing", cfg.ClusterDeployment.States[2].Name)
	assert.Equal(t, "Running", cfg.ClusterDeployment.States[3].Name)

	// Verify AccountClaim defaults
	assert.Equal(t, 3, cfg.AccountClaim.DefaultDelaySeconds)
	assert.Len(t, cfg.AccountClaim.States, 2)
	assert.Equal(t, "Pending", cfg.AccountClaim.States[0].Name)
	assert.Equal(t, "Ready", cfg.AccountClaim.States[1].Name)

	// Verify ProjectClaim defaults
	assert.Equal(t, 4, cfg.ProjectClaim.DefaultDelaySeconds)
	assert.Len(t, cfg.ProjectClaim.States, 3)
	assert.Equal(t, "Pending", cfg.ProjectClaim.States[0].Name)
	assert.Equal(t, "PendingProject", cfg.ProjectClaim.States[1].Name)
	assert.Equal(t, "Ready", cfg.ProjectClaim.States[2].Name)

	// Verify ClusterImageSets
	assert.Greater(t, len(cfg.ClusterImageSets), 0)
	assert.Equal(t, "openshift-v4.12.0", cfg.ClusterImageSets[0].Name)
	assert.True(t, cfg.ClusterImageSets[0].Visible)
}

func TestClusterDeploymentConfig_GetTotalDuration(t *testing.T) {
	tests := []struct {
		name     string
		config   *ClusterDeploymentConfig
		expected time.Duration
	}{
		{
			name: "uses default delay seconds",
			config: &ClusterDeploymentConfig{
				DefaultDelaySeconds: 10,
				States:              []StateConfig{{DurationSeconds: 5}, {DurationSeconds: 3}},
			},
			expected: 10 * time.Second,
		},
		{
			name: "sums state durations when no default",
			config: &ClusterDeploymentConfig{
				DefaultDelaySeconds: 0,
				States:              []StateConfig{{DurationSeconds: 5}, {DurationSeconds: 3}},
			},
			expected: 8 * time.Second,
		},
		{
			name: "handles empty states",
			config: &ClusterDeploymentConfig{
				DefaultDelaySeconds: 0,
				States:              []StateConfig{},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetTotalDuration()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAccountClaimConfig_GetTotalDuration(t *testing.T) {
	cfg := &AccountClaimConfig{
		DefaultDelaySeconds: 5,
		States:              []StateConfig{{DurationSeconds: 2}, {DurationSeconds: 3}},
	}

	assert.Equal(t, 5*time.Second, cfg.GetTotalDuration())

	cfg.DefaultDelaySeconds = 0
	assert.Equal(t, 5*time.Second, cfg.GetTotalDuration())
}

func TestProjectClaimConfig_GetTotalDuration(t *testing.T) {
	cfg := &ProjectClaimConfig{
		DefaultDelaySeconds: 8,
		States:              []StateConfig{{DurationSeconds: 1}, {DurationSeconds: 2}},
	}

	assert.Equal(t, 8*time.Second, cfg.GetTotalDuration())

	cfg.DefaultDelaySeconds = 0
	assert.Equal(t, 3*time.Second, cfg.GetTotalDuration())
}
