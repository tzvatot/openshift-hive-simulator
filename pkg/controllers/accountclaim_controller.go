package controllers

import (
	"context"

	kuberrors "k8s.io/apimachinery/pkg/api/errors"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift-online/ocm-sdk-go/logging"

	aaov1alpha1 "github.com/tzvatot/openshift-hive-simulator/pkg/externalapis/aws-account-operator/v1alpha1"
	"github.com/tzvatot/openshift-hive-simulator/pkg/behavior"
	"github.com/tzvatot/openshift-hive-simulator/pkg/config"
	"github.com/tzvatot/openshift-hive-simulator/pkg/state_machine"
)

// AccountClaimReconciler reconciles AccountClaim objects
type AccountClaimReconciler struct {
	client         client.Client
	logger         logging.Logger
	stateMachine   *state_machine.AccountClaimStateMachine
	behaviorEngine *behavior.Engine
}

// NewAccountClaimReconciler creates a new AccountClaim reconciler
func NewAccountClaimReconciler(
	client client.Client,
	logger logging.Logger,
	stateMachine *state_machine.AccountClaimStateMachine,
	behaviorEngine *behavior.Engine,
) *AccountClaimReconciler {
	return &AccountClaimReconciler{
		client:         client,
		logger:         logger,
		stateMachine:   stateMachine,
		behaviorEngine: behaviorEngine,
	}
}

// Reconcile reconciles an AccountClaim
func (r *AccountClaimReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	r.logger.Debug(ctx, "Reconciling AccountClaim %s/%s", req.Namespace, req.Name)

	ac := &aaov1alpha1.AccountClaim{}
	if err := r.client.Get(ctx, req.NamespacedName, ac); err != nil {
		if kuberrors.IsNotFound(err) {
			r.logger.Debug(ctx, "AccountClaim %s/%s not found, skipping", req.Namespace, req.Name)
			return reconcile.Result{}, nil
		}
		r.logger.Error(ctx, "Failed to get AccountClaim %s/%s: %v", req.Namespace, req.Name, err)
		return reconcile.Result{}, err
	}

	// Skip if being deleted
	if !ac.DeletionTimestamp.IsZero() {
		r.logger.Debug(ctx, "AccountClaim %s/%s is being deleted, skipping", req.Namespace, req.Name)
		return reconcile.Result{}, nil
	}

	// Skip if already in final state
	if ac.Status.State == aaov1alpha1.ClaimStatusReady || ac.Status.State == aaov1alpha1.ClaimStatusError {
		r.logger.Debug(ctx, "AccountClaim %s/%s is in final state: %s, skipping", req.Namespace, req.Name, ac.Status.State)
		return reconcile.Result{}, nil
	}

	// Check for forced failure
	shouldFail, failure := r.behaviorEngine.ShouldFail(ctx, "AccountClaim", ac.Namespace, ac.Name)
	if shouldFail {
		return r.applyFailure(ctx, ac, failure)
	}

	// Determine next state and apply it
	nextState, duration := r.stateMachine.GetNextState(ctx, ac)

	// Apply the state
	if err := r.stateMachine.ApplyState(ctx, ac, nextState); err != nil {
		r.logger.Error(ctx, "Failed to apply state %s to AccountClaim %s/%s: %v",
			nextState, ac.Namespace, ac.Name, err)
		return reconcile.Result{}, err
	}

	// Update the AccountClaim
	if err := r.client.Status().Update(ctx, ac); err != nil {
		r.logger.Error(ctx, "Failed to update AccountClaim %s/%s status: %v",
			ac.Namespace, ac.Name, err)
		return reconcile.Result{}, err
	}

	// Also update spec if fields were set
	if err := r.client.Update(ctx, ac); err != nil {
		r.logger.Error(ctx, "Failed to update AccountClaim %s/%s spec: %v",
			ac.Namespace, ac.Name, err)
		return reconcile.Result{}, err
	}

	r.logger.Info(ctx, "AccountClaim %s/%s transitioned to state: %s", ac.Namespace, ac.Name, nextState)

	// Requeue after duration for next state transition
	if duration > 0 {
		// Check for delay override
		duration = r.behaviorEngine.GetTransitionDelay(ctx, "AccountClaim", ac.Namespace, ac.Name, duration)
		r.logger.Debug(ctx, "Requeuing AccountClaim %s/%s after %v", ac.Namespace, ac.Name, duration)
		return reconcile.Result{RequeueAfter: duration}, nil
	}

	return reconcile.Result{}, nil
}

// applyFailure applies a failure state to the AccountClaim
func (r *AccountClaimReconciler) applyFailure(ctx context.Context, ac *aaov1alpha1.AccountClaim, failure *config.FailureScenario) (reconcile.Result, error) {
	if err := r.stateMachine.ApplyFailure(ctx, ac, failure); err != nil {
		r.logger.Error(ctx, "Failed to apply failure to AccountClaim %s/%s: %v",
			ac.Namespace, ac.Name, err)
		return reconcile.Result{}, err
	}

	if err := r.client.Status().Update(ctx, ac); err != nil {
		r.logger.Error(ctx, "Failed to update failed AccountClaim %s/%s status: %v",
			ac.Namespace, ac.Name, err)
		return reconcile.Result{}, err
	}

	r.logger.Info(ctx, "AccountClaim %s/%s failed: %s", ac.Namespace, ac.Name, failure.Message)
	return reconcile.Result{}, nil
}
