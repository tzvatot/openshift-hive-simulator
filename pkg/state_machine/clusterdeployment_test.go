package state_machine

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-online/ocm-sdk-go/logging"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
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

func createTestClusterDeploymentConfig() *config.ClusterDeploymentConfig {
	return &config.ClusterDeploymentConfig{
		DefaultDelaySeconds:   5,
		DependsOnAccountClaim: true,
		DependsOnProjectClaim: true,
		States: []config.StateConfig{
			{Name: "Pending", DurationSeconds: 1},
			{Name: "Provisioning", DurationSeconds: 2},
			{Name: "Installing", DurationSeconds: 1},
			{Name: "Running", DurationSeconds: 1},
		},
	}
}

func TestNewClusterDeploymentStateMachine(t *testing.T) {
	logger := createTestLogger()
	cfg := createTestClusterDeploymentConfig()

	sm := NewClusterDeploymentStateMachine(logger, cfg)

	assert.NotNil(t, sm)
	assert.NotNil(t, sm.logger)
	assert.NotNil(t, sm.config)
}

func TestClusterDeploymentStateMachine_GetNextState(t *testing.T) {
	logger := createTestLogger()
	cfg := createTestClusterDeploymentConfig()
	sm := NewClusterDeploymentStateMachine(logger, cfg)
	ctx := context.Background()

	tests := []struct {
		name              string
		clusterDeployment *hivev1.ClusterDeployment
		expectedState     string
		expectDuration    bool
	}{
		{
			name: "new ClusterDeployment transitions to Provisioning",
			clusterDeployment: &hivev1.ClusterDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "default",
				},
			},
			expectedState:  "Provisioning",
			expectDuration: true,
		},
		{
			name: "installed ClusterDeployment stays in Running",
			clusterDeployment: &hivev1.ClusterDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "default",
				},
				Spec: hivev1.ClusterDeploymentSpec{
					Installed: true,
				},
			},
			expectedState:  "Running",
			expectDuration: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nextState, duration := sm.GetNextState(ctx, tt.clusterDeployment)

			assert.Equal(t, tt.expectedState, nextState)
			if tt.expectDuration {
				assert.Greater(t, duration.Seconds(), 0.0)
			} else {
				assert.Equal(t, 0.0, duration.Seconds())
			}
		})
	}
}

func TestClusterDeploymentStateMachine_ApplyState(t *testing.T) {
	logger := createTestLogger()
	cfg := createTestClusterDeploymentConfig()
	// Add conditions to states
	cfg.States[1].Conditions = []config.ConditionConfig{
		{
			Type:    "DeprovisionLaunchError",
			Status:  "False",
			Reason:  "Provisioning",
			Message: "Cluster is provisioning",
		},
	}
	sm := NewClusterDeploymentStateMachine(logger, cfg)
	ctx := context.Background()

	cd := &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
	}

	// Test applying Provisioning state
	err := sm.ApplyState(ctx, cd, "Provisioning")
	require.NoError(t, err)

	assert.NotNil(t, cd.Status.ProvisionRef)
	assert.Len(t, cd.Status.Conditions, 1)
	assert.Equal(t, hivev1.ClusterDeploymentConditionType("DeprovisionLaunchError"), cd.Status.Conditions[0].Type)
	assert.Equal(t, corev1.ConditionFalse, cd.Status.Conditions[0].Status)

	// Test applying Running state
	err = sm.ApplyState(ctx, cd, "Running")
	require.NoError(t, err)

	assert.True(t, cd.Spec.Installed)
	assert.NotNil(t, cd.Status.InstalledTimestamp)
	assert.NotNil(t, cd.Spec.ClusterMetadata)
	assert.NotEmpty(t, cd.Spec.ClusterMetadata.InfraID)
	assert.NotEmpty(t, cd.Status.WebConsoleURL)
	assert.NotEmpty(t, cd.Status.APIURL)
}

func TestClusterDeploymentStateMachine_ApplyState_InvalidState(t *testing.T) {
	logger := createTestLogger()
	cfg := createTestClusterDeploymentConfig()
	sm := NewClusterDeploymentStateMachine(logger, cfg)
	ctx := context.Background()

	cd := &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
	}

	err := sm.ApplyState(ctx, cd, "InvalidState")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "state InvalidState not found in configuration")
}

func TestClusterDeploymentStateMachine_ApplyFailure(t *testing.T) {
	logger := createTestLogger()
	cfg := createTestClusterDeploymentConfig()
	sm := NewClusterDeploymentStateMachine(logger, cfg)
	ctx := context.Background()

	cd := &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
	}

	failure := &config.FailureScenario{
		Condition: "ProvisionFailed",
		Message:   "Test failure message",
		Reason:    "TestReason",
	}

	err := sm.ApplyFailure(ctx, cd, failure)
	require.NoError(t, err)

	assert.NotNil(t, cd.Status.ProvisionRef)
	assert.Len(t, cd.Status.Conditions, 1)
	assert.Equal(t, hivev1.ClusterDeploymentConditionType("ProvisionFailed"), cd.Status.Conditions[0].Type)
	assert.Equal(t, corev1.ConditionTrue, cd.Status.Conditions[0].Status)
	assert.Equal(t, "TestReason", cd.Status.Conditions[0].Reason)
	assert.Equal(t, "Test failure message", cd.Status.Conditions[0].Message)
}

func TestClusterDeploymentStateMachine_ShouldWaitForDependencies(t *testing.T) {
	logger := createTestLogger()

	tests := []struct {
		name           string
		config         *config.ClusterDeploymentConfig
		expectedResult bool
	}{
		{
			name: "both dependencies enabled",
			config: &config.ClusterDeploymentConfig{
				DependsOnAccountClaim: true,
				DependsOnProjectClaim: true,
			},
			expectedResult: true,
		},
		{
			name: "only AccountClaim dependency",
			config: &config.ClusterDeploymentConfig{
				DependsOnAccountClaim: true,
				DependsOnProjectClaim: false,
			},
			expectedResult: true,
		},
		{
			name: "no dependencies",
			config: &config.ClusterDeploymentConfig{
				DependsOnAccountClaim: false,
				DependsOnProjectClaim: false,
			},
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := NewClusterDeploymentStateMachine(logger, tt.config)
			result := sm.ShouldWaitForDependencies()
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}
