package state_machine

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-online/ocm-sdk-go/logging"

	gcpv1alpha1 "github.com/tzvatot/openshift-hive-simulator/pkg/externalapis/gcp-project-operator/v1alpha1"
	"github.com/tzvatot/openshift-hive-simulator/pkg/config"
)

// ProjectClaimStateMachine manages ProjectClaim state transitions
type ProjectClaimStateMachine struct {
	logger logging.Logger
	config *config.ProjectClaimConfig
}

// NewProjectClaimStateMachine creates a new ProjectClaim state machine
func NewProjectClaimStateMachine(logger logging.Logger, cfg *config.ProjectClaimConfig) *ProjectClaimStateMachine {
	return &ProjectClaimStateMachine{
		logger: logger,
		config: cfg,
	}
}

// GetNextState determines the next state for a ProjectClaim
func (sm *ProjectClaimStateMachine) GetNextState(ctx context.Context, pc *gcpv1alpha1.ProjectClaim) (gcpv1alpha1.ClaimStatus, time.Duration) {
	currentState := pc.Status.State
	sm.logger.Debug(ctx, "Current ProjectClaim state for %s/%s: %s", pc.Namespace, pc.Name, currentState)

	// Find current state in config
	for i, state := range sm.config.States {
		if string(currentState) == state.Name || (currentState == "" && state.Name == "Pending") {
			// If this is the last state, stay here
			if i >= len(sm.config.States)-1 {
				sm.logger.Debug(ctx, "ProjectClaim %s/%s is in final state: %s", pc.Namespace, pc.Name, state.Name)
				return gcpv1alpha1.ClaimStatus(state.Name), 0
			}

			// Return next state and its duration
			nextState := sm.config.States[i+1]
			duration := time.Duration(nextState.DurationSeconds) * time.Second
			sm.logger.Debug(ctx, "Next state for ProjectClaim %s/%s: %s (duration: %v)", pc.Namespace, pc.Name, nextState.Name, duration)
			return gcpv1alpha1.ClaimStatus(nextState.Name), duration
		}
	}

	// Default to first state
	if len(sm.config.States) > 0 {
		firstState := sm.config.States[0]
		duration := time.Duration(firstState.DurationSeconds) * time.Second
		sm.logger.Debug(ctx, "ProjectClaim %s/%s has no current state, starting with: %s", pc.Namespace, pc.Name, firstState.Name)
		return gcpv1alpha1.ClaimStatus(firstState.Name), duration
	}

	return gcpv1alpha1.ClaimStatusPending, 4 * time.Second
}

// ApplyState applies a state to the ProjectClaim
func (sm *ProjectClaimStateMachine) ApplyState(ctx context.Context, pc *gcpv1alpha1.ProjectClaim, state gcpv1alpha1.ClaimStatus) error {
	sm.logger.Info(ctx, "Applying state %s to ProjectClaim %s/%s", state, pc.Namespace, pc.Name)

	pc.Status.State = state

	now := metav1.Now()

	// Update conditions based on state
	switch state {
	case gcpv1alpha1.ClaimStatusPending:
		pc.Status.Conditions = []gcpv1alpha1.Condition{
			{
				Type:               gcpv1alpha1.ConditionType("Pending"),
				Status:             corev1.ConditionTrue,
				Reason:             "ProjectPending",
				Message:            "Project claim is pending",
				LastTransitionTime: now,
				LastProbeTime:      now,
			},
		}

	case gcpv1alpha1.ClaimStatusPendingProject:
		pc.Status.Conditions = []gcpv1alpha1.Condition{
			{
				Type:               gcpv1alpha1.ConditionType("PendingProject"),
				Status:             corev1.ConditionTrue,
				Reason:             "ProjectCreating",
				Message:            "GCP project is being created",
				LastTransitionTime: now,
				LastProbeTime:      now,
			},
		}
		// Simulate GCP project ID
		if pc.Spec.GCPProjectID == "" {
			pc.Spec.GCPProjectID = fmt.Sprintf("project-%s-%d", pc.Name, time.Now().UTC().Unix()%10000)
		}

	case gcpv1alpha1.ClaimStatusReady:
		pc.Status.Conditions = []gcpv1alpha1.Condition{
			{
				Type:               gcpv1alpha1.ConditionType("Ready"),
				Status:             corev1.ConditionTrue,
				Reason:             "ProjectReady",
				Message:            "GCP project is ready",
				LastTransitionTime: now,
				LastProbeTime:      now,
			},
		}
		// Ensure GCP project ID is set
		if pc.Spec.GCPProjectID == "" {
			pc.Spec.GCPProjectID = fmt.Sprintf("project-%s-%d", pc.Name, time.Now().UTC().Unix()%10000)
		}

	case gcpv1alpha1.ClaimStatusError:
		pc.Status.Conditions = []gcpv1alpha1.Condition{
			{
				Type:               gcpv1alpha1.ConditionType("Error"),
				Status:             corev1.ConditionTrue,
				Reason:             "ClaimFailed",
				Message:            "Project claim failed",
				LastTransitionTime: now,
				LastProbeTime:      now,
			},
		}
	}

	return nil
}

// ApplyFailure applies a failure state to the ProjectClaim
func (sm *ProjectClaimStateMachine) ApplyFailure(ctx context.Context, pc *gcpv1alpha1.ProjectClaim, failure *config.FailureScenario) error {
	sm.logger.Warn(ctx, "Applying failure to ProjectClaim %s/%s: %s - %s", pc.Namespace, pc.Name, failure.Reason, failure.Message)

	pc.Status.State = gcpv1alpha1.ClaimStatusError

	now := metav1.Now()
	condition := gcpv1alpha1.Condition{
		Type:               gcpv1alpha1.ConditionType(failure.Condition),
		Status:             corev1.ConditionTrue,
		Reason:             failure.Reason,
		Message:            failure.Message,
		LastTransitionTime: now,
		LastProbeTime:      now,
	}

	pc.Status.Conditions = append(pc.Status.Conditions, condition)

	return nil
}
