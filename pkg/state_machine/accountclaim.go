package state_machine

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-online/ocm-sdk-go/logging"

	aaov1alpha1 "github.com/tzvatot/openshift-hive-simulator/pkg/externalapis/aws-account-operator/v1alpha1"
	"github.com/tzvatot/openshift-hive-simulator/pkg/config"
)

// AccountClaimStateMachine manages AccountClaim state transitions
type AccountClaimStateMachine struct {
	logger logging.Logger
	config *config.AccountClaimConfig
}

// NewAccountClaimStateMachine creates a new AccountClaim state machine
func NewAccountClaimStateMachine(logger logging.Logger, cfg *config.AccountClaimConfig) *AccountClaimStateMachine {
	return &AccountClaimStateMachine{
		logger: logger,
		config: cfg,
	}
}

// GetNextState determines the next state for an AccountClaim
func (sm *AccountClaimStateMachine) GetNextState(ctx context.Context, ac *aaov1alpha1.AccountClaim) (aaov1alpha1.ClaimStatus, time.Duration) {
	currentState := ac.Status.State
	sm.logger.Debug(ctx, "Current AccountClaim state for %s/%s: %s", ac.Namespace, ac.Name, currentState)

	// Find current state in config
	for i, state := range sm.config.States {
		if string(currentState) == state.Name || (currentState == "" && state.Name == "Pending") {
			// If this is the last state, stay here
			if i >= len(sm.config.States)-1 {
				sm.logger.Debug(ctx, "AccountClaim %s/%s is in final state: %s", ac.Namespace, ac.Name, state.Name)
				return aaov1alpha1.ClaimStatus(state.Name), 0
			}

			// Return next state and its duration
			nextState := sm.config.States[i+1]
			duration := time.Duration(nextState.DurationSeconds) * time.Second
			sm.logger.Debug(ctx, "Next state for AccountClaim %s/%s: %s (duration: %v)", ac.Namespace, ac.Name, nextState.Name, duration)
			return aaov1alpha1.ClaimStatus(nextState.Name), duration
		}
	}

	// Default to first state
	if len(sm.config.States) > 0 {
		firstState := sm.config.States[0]
		duration := time.Duration(firstState.DurationSeconds) * time.Second
		sm.logger.Debug(ctx, "AccountClaim %s/%s has no current state, starting with: %s", ac.Namespace, ac.Name, firstState.Name)
		return aaov1alpha1.ClaimStatus(firstState.Name), duration
	}

	return aaov1alpha1.ClaimStatusPending, 3 * time.Second
}

// ApplyState applies a state to the AccountClaim
func (sm *AccountClaimStateMachine) ApplyState(ctx context.Context, ac *aaov1alpha1.AccountClaim, state aaov1alpha1.ClaimStatus) error {
	sm.logger.Info(ctx, "Applying state %s to AccountClaim %s/%s", state, ac.Namespace, ac.Name)

	ac.Status.State = state

	now := metav1.Now()

	// Update conditions based on state
	switch state {
	case aaov1alpha1.ClaimStatusPending:
		ac.Status.Conditions = []aaov1alpha1.AccountClaimCondition{
			{
				Type:               aaov1alpha1.AccountUnclaimed,
				Status:             corev1.ConditionTrue,
				Reason:             "AccountPending",
				Message:            "Account claim is pending",
				LastTransitionTime: now,
				LastProbeTime:      now,
			},
		}

	case aaov1alpha1.ClaimStatusReady:
		ac.Status.Conditions = []aaov1alpha1.AccountClaimCondition{
			{
				Type:               aaov1alpha1.AccountClaimed,
				Status:             corev1.ConditionTrue,
				Reason:             "AccountClaimed",
				Message:            "Account has been claimed",
				LastTransitionTime: now,
				LastProbeTime:      now,
			},
		}
		// Simulate AWS account ID
		if ac.Spec.BYOCAWSAccountID == "" {
			ac.Spec.BYOCAWSAccountID = fmt.Sprintf("123456789%03d", time.Now().UTC().Unix()%1000)
		}

	case aaov1alpha1.ClaimStatusError:
		ac.Status.Conditions = []aaov1alpha1.AccountClaimCondition{
			{
				Type:               aaov1alpha1.AccountClaimFailed,
				Status:             corev1.ConditionTrue,
				Reason:             "ClaimFailed",
				Message:            "Account claim failed",
				LastTransitionTime: now,
				LastProbeTime:      now,
			},
		}
	}

	return nil
}

// ApplyFailure applies a failure state to the AccountClaim
func (sm *AccountClaimStateMachine) ApplyFailure(ctx context.Context, ac *aaov1alpha1.AccountClaim, failure *config.FailureScenario) error {
	sm.logger.Warn(ctx, "Applying failure to AccountClaim %s/%s: %s - %s", ac.Namespace, ac.Name, failure.Reason, failure.Message)

	ac.Status.State = aaov1alpha1.ClaimStatusError

	now := metav1.Now()
	condition := aaov1alpha1.AccountClaimCondition{
		Type:               aaov1alpha1.AccountClaimConditionType(failure.Condition),
		Status:             corev1.ConditionTrue,
		Reason:             failure.Reason,
		Message:            failure.Message,
		LastTransitionTime: now,
		LastProbeTime:      now,
	}

	ac.Status.Conditions = append(ac.Status.Conditions, condition)

	return nil
}
