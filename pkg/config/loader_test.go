package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadFromFile_EmptyPath(t *testing.T) {
	cfg, err := LoadFromFile("")
	require.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Equal(t, DefaultConfig().ClusterDeployment.DefaultDelaySeconds, cfg.ClusterDeployment.DefaultDelaySeconds)
}

func TestLoadFromFile_ValidFile(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	configContent := `
clusterDeployment:
  defaultDelaySeconds: 10
  dependsOnAccountClaim: false
  dependsOnProjectClaim: false
  states:
    - name: Pending
      durationSeconds: 1
    - name: Running
      durationSeconds: 2

accountClaim:
  defaultDelaySeconds: 5
  states:
    - name: Pending
      durationSeconds: 1
    - name: Ready
      durationSeconds: 1

projectClaim:
  defaultDelaySeconds: 6
  states:
    - name: Pending
      durationSeconds: 1
    - name: Ready
      durationSeconds: 1

clusterImageSets:
  - name: "test-image-v1.0.0"
    visible: true
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Load config
	cfg, err := LoadFromFile(configPath)
	require.NoError(t, err)
	assert.NotNil(t, cfg)

	// Verify values
	assert.Equal(t, 10, cfg.ClusterDeployment.DefaultDelaySeconds)
	assert.False(t, cfg.ClusterDeployment.DependsOnAccountClaim)
	assert.False(t, cfg.ClusterDeployment.DependsOnProjectClaim)
	assert.Len(t, cfg.ClusterDeployment.States, 2)

	assert.Equal(t, 5, cfg.AccountClaim.DefaultDelaySeconds)
	assert.Equal(t, 6, cfg.ProjectClaim.DefaultDelaySeconds)

	assert.Len(t, cfg.ClusterImageSets, 1)
	assert.Equal(t, "test-image-v1.0.0", cfg.ClusterImageSets[0].Name)
}

func TestLoadFromFile_FileNotFound(t *testing.T) {
	cfg, err := LoadFromFile("/nonexistent/path/config.yaml")
	assert.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "failed to read config file")
}

func TestLoadFromFile_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yaml")

	err := os.WriteFile(configPath, []byte("invalid: yaml: content: ["), 0644)
	require.NoError(t, err)

	cfg, err := LoadFromFile(configPath)
	assert.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "failed to parse config file")
}

func TestValidate_NegativeDefaultDelay(t *testing.T) {
	cfg := &Config{
		ClusterDeployment: &ClusterDeploymentConfig{
			DefaultDelaySeconds: -1,
		},
		AccountClaim: &AccountClaimConfig{
			DefaultDelaySeconds: 1,
		},
		ProjectClaim: &ProjectClaimConfig{
			DefaultDelaySeconds: 1,
		},
	}

	err := validate(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ClusterDeployment defaultDelaySeconds must be >= 0")
}

func TestValidate_NegativeStateDuration(t *testing.T) {
	cfg := &Config{
		ClusterDeployment: &ClusterDeploymentConfig{
			DefaultDelaySeconds: 5,
			States: []StateConfig{
				{Name: "test", DurationSeconds: -1},
			},
		},
		AccountClaim: &AccountClaimConfig{
			DefaultDelaySeconds: 1,
		},
		ProjectClaim: &ProjectClaimConfig{
			DefaultDelaySeconds: 1,
		},
	}

	err := validate(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ClusterDeployment state test duration must be >= 0")
}

func TestValidate_InvalidFailureProbability(t *testing.T) {
	tests := []struct {
		name        string
		probability float64
		shouldError bool
	}{
		{"valid zero", 0.0, false},
		{"valid half", 0.5, false},
		{"valid one", 1.0, false},
		{"invalid negative", -0.1, true},
		{"invalid over one", 1.1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				ClusterDeployment: &ClusterDeploymentConfig{
					DefaultDelaySeconds: 5,
					FailureScenarios: []FailureScenario{
						{
							Probability: tt.probability,
							Condition:   "TestFail",
							Message:     "test",
						},
					},
				},
				AccountClaim: &AccountClaimConfig{
					DefaultDelaySeconds: 1,
				},
				ProjectClaim: &ProjectClaimConfig{
					DefaultDelaySeconds: 1,
				},
			}

			err := validate(cfg)
			if tt.shouldError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "probability must be 0.0-1.0")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidate_FillsDefaults(t *testing.T) {
	cfg := &Config{}

	err := validate(cfg)
	assert.NoError(t, err)

	// Should have filled in defaults
	assert.NotNil(t, cfg.ClusterDeployment)
	assert.NotNil(t, cfg.AccountClaim)
	assert.NotNil(t, cfg.ProjectClaim)
	assert.NotEmpty(t, cfg.ClusterImageSets)
}
