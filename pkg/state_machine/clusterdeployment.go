package state_machine

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-online/ocm-sdk-go/logging"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	errors "github.com/zgalor/weberr"

	"github.com/tzvatot/openshift-hive-simulator/pkg/config"
)

// ClusterDeploymentStateMachine manages ClusterDeployment state transitions
type ClusterDeploymentStateMachine struct {
	logger logging.Logger
	config *config.ClusterDeploymentConfig
}

// NewClusterDeploymentStateMachine creates a new ClusterDeployment state machine
func NewClusterDeploymentStateMachine(logger logging.Logger, cfg *config.ClusterDeploymentConfig) *ClusterDeploymentStateMachine {
	return &ClusterDeploymentStateMachine{
		logger: logger,
		config: cfg,
	}
}

// GetNextState determines the next state for a ClusterDeployment
func (sm *ClusterDeploymentStateMachine) GetNextState(ctx context.Context, cd *hivev1.ClusterDeployment) (string, time.Duration) {
	currentState := sm.getCurrentState(cd)
	sm.logger.Debug(ctx, "Current ClusterDeployment state for %s/%s: %s", cd.Namespace, cd.Name, currentState)

	// Find current state in config
	for i, state := range sm.config.States {
		if state.Name == currentState {
			// If this is the last state, stay here
			if i >= len(sm.config.States)-1 {
				sm.logger.Debug(ctx, "ClusterDeployment %s/%s is in final state: %s", cd.Namespace, cd.Name, currentState)
				return currentState, 0
			}

			// Return next state and its duration
			nextState := sm.config.States[i+1]
			duration := time.Duration(nextState.DurationSeconds) * time.Second
			sm.logger.Debug(ctx, "Next state for ClusterDeployment %s/%s: %s (duration: %v)", cd.Namespace, cd.Name, nextState.Name, duration)
			return nextState.Name, duration
		}
	}

	// Default to first state if current state not found
	if len(sm.config.States) > 0 {
		firstState := sm.config.States[0]
		duration := time.Duration(firstState.DurationSeconds) * time.Second
		sm.logger.Debug(ctx, "ClusterDeployment %s/%s has no current state, starting with: %s", cd.Namespace, cd.Name, firstState.Name)
		return firstState.Name, duration
	}

	return "Pending", 5 * time.Second
}

// ApplyState applies a state to the ClusterDeployment
func (sm *ClusterDeploymentStateMachine) ApplyState(ctx context.Context, cd *hivev1.ClusterDeployment, state string) error {
	sm.logger.Info(ctx, "Applying state %s to ClusterDeployment %s/%s", state, cd.Namespace, cd.Name)

	// Find state config
	var stateConfig *config.StateConfig
	for i := range sm.config.States {
		if sm.config.States[i].Name == state {
			stateConfig = &sm.config.States[i]
			break
		}
	}

	if stateConfig == nil {
		return errors.Errorf("state %s not found in configuration", state)
	}

	// Update conditions based on state
	now := metav1.Now()
	cd.Status.Conditions = sm.buildConditions(stateConfig, now)

	// Apply state-specific updates
	switch state {
	case "Provisioning":
		// Set provisioning state
		cd.Status.ProvisionRef = &corev1.LocalObjectReference{
			Name: cd.Name + "-provision",
		}

	case "Installing":
		// Set DNS ready
		cd.Status.WebConsoleURL = fmt.Sprintf("https://console-openshift-console.apps.%s.example.com", cd.Name)
		cd.Status.APIURL = fmt.Sprintf("https://api.%s.example.com:6443", cd.Name)

	case "Running":
		// Mark as installed
		cd.Spec.Installed = true
		cd.Status.InstalledTimestamp = &now
		// Set InfraID in ClusterMetadata
		if cd.Spec.ClusterMetadata == nil {
			cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{}
		}
		cd.Spec.ClusterMetadata.InfraID = fmt.Sprintf("%s-infra", cd.Name)
		cd.Status.WebConsoleURL = fmt.Sprintf("https://console-openshift-console.apps.%s.example.com", cd.Name)
		cd.Status.APIURL = fmt.Sprintf("https://api.%s.example.com:6443", cd.Name)
	}

	return nil
}

// ApplyFailure applies a failure state to the ClusterDeployment
func (sm *ClusterDeploymentStateMachine) ApplyFailure(ctx context.Context, cd *hivev1.ClusterDeployment, failure *config.FailureScenario) error {
	sm.logger.Warn(ctx, "Applying failure to ClusterDeployment %s/%s: %s - %s", cd.Namespace, cd.Name, failure.Reason, failure.Message)

	now := metav1.Now()

	// Add failure condition
	condition := hivev1.ClusterDeploymentCondition{
		Type:               hivev1.ClusterDeploymentConditionType(failure.Condition),
		Status:             corev1.ConditionTrue,
		Reason:             failure.Reason,
		Message:            failure.Message,
		LastTransitionTime: now,
		LastProbeTime:      now,
	}

	cd.Status.Conditions = append(cd.Status.Conditions, condition)

	// Mark provision as failed
	cd.Status.ProvisionRef = &corev1.LocalObjectReference{
		Name: cd.Name + "-provision-failed",
	}

	return nil
}

// ShouldWaitForDependencies checks if ClusterDeployment should wait for dependencies
func (sm *ClusterDeploymentStateMachine) ShouldWaitForDependencies() bool {
	return sm.config.DependsOnAccountClaim || sm.config.DependsOnProjectClaim
}

// getCurrentState determines the current state from the ClusterDeployment
func (sm *ClusterDeploymentStateMachine) getCurrentState(cd *hivev1.ClusterDeployment) string {
	// If installed, it's running
	if cd.Spec.Installed {
		return "Running"
	}

	// Check conditions to determine state
	for _, condition := range cd.Status.Conditions {
		switch condition.Type {
		case "ClusterDeploymentCompleted":
			if condition.Status == corev1.ConditionTrue {
				return "Running"
			}
		case "DNSNotReady":
			if condition.Status == corev1.ConditionFalse {
				return "Installing"
			}
		case "DeprovisionLaunchError":
			if condition.Status == corev1.ConditionFalse {
				return "Provisioning"
			}
		}
	}

	// If provision ref is set but no other conditions, we're provisioning
	if cd.Status.ProvisionRef != nil {
		return "Provisioning"
	}

	// Default to pending
	return "Pending"
}

// buildConditions builds conditions for a given state
func (sm *ClusterDeploymentStateMachine) buildConditions(stateConfig *config.StateConfig, now metav1.Time) []hivev1.ClusterDeploymentCondition {
	conditions := []hivev1.ClusterDeploymentCondition{}

	for _, condConfig := range stateConfig.Conditions {
		status := corev1.ConditionUnknown
		switch condConfig.Status {
		case "True":
			status = corev1.ConditionTrue
		case "False":
			status = corev1.ConditionFalse
		}

		condition := hivev1.ClusterDeploymentCondition{
			Type:               hivev1.ClusterDeploymentConditionType(condConfig.Type),
			Status:             status,
			Reason:             condConfig.Reason,
			Message:            condConfig.Message,
			LastTransitionTime: now,
			LastProbeTime:      now,
		}
		conditions = append(conditions, condition)
	}

	return conditions
}
