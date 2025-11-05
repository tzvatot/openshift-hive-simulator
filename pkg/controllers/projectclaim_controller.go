package controllers

import (
	"context"

	kuberrors "k8s.io/apimachinery/pkg/api/errors"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift-online/ocm-sdk-go/logging"

	gcpv1alpha1 "github.com/tzvatot/openshift-hive-simulator/pkg/externalapis/gcp-project-operator/v1alpha1"
	"github.com/tzvatot/openshift-hive-simulator/pkg/behavior"
	"github.com/tzvatot/openshift-hive-simulator/pkg/config"
	"github.com/tzvatot/openshift-hive-simulator/pkg/state_machine"
)

// ProjectClaimReconciler reconciles ProjectClaim objects
type ProjectClaimReconciler struct {
	client         client.Client
	logger         logging.Logger
	stateMachine   *state_machine.ProjectClaimStateMachine
	behaviorEngine *behavior.Engine
}

// NewProjectClaimReconciler creates a new ProjectClaim reconciler
func NewProjectClaimReconciler(
	client client.Client,
	logger logging.Logger,
	stateMachine *state_machine.ProjectClaimStateMachine,
	behaviorEngine *behavior.Engine,
) *ProjectClaimReconciler {
	return &ProjectClaimReconciler{
		client:         client,
		logger:         logger,
		stateMachine:   stateMachine,
		behaviorEngine: behaviorEngine,
	}
}

// Reconcile reconciles a ProjectClaim
func (r *ProjectClaimReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	r.logger.Debug(ctx, "Reconciling ProjectClaim %s/%s", req.Namespace, req.Name)

	pc := &gcpv1alpha1.ProjectClaim{}
	if err := r.client.Get(ctx, req.NamespacedName, pc); err != nil {
		if kuberrors.IsNotFound(err) {
			r.logger.Debug(ctx, "ProjectClaim %s/%s not found, skipping", req.Namespace, req.Name)
			return reconcile.Result{}, nil
		}
		r.logger.Error(ctx, "Failed to get ProjectClaim %s/%s: %v", req.Namespace, req.Name, err)
		return reconcile.Result{}, err
	}

	// Skip if being deleted
	if !pc.DeletionTimestamp.IsZero() {
		r.logger.Debug(ctx, "ProjectClaim %s/%s is being deleted, skipping", req.Namespace, req.Name)
		return reconcile.Result{}, nil
	}

	// Skip if already in final state
	if pc.Status.State == gcpv1alpha1.ClaimStatusReady || pc.Status.State == gcpv1alpha1.ClaimStatusError {
		r.logger.Debug(ctx, "ProjectClaim %s/%s is in final state: %s, skipping", req.Namespace, req.Name, pc.Status.State)
		return reconcile.Result{}, nil
	}

	// Check for forced failure
	shouldFail, failure := r.behaviorEngine.ShouldFail(ctx, "ProjectClaim", pc.Namespace, pc.Name)
	if shouldFail {
		return r.applyFailure(ctx, pc, failure)
	}

	// Determine next state and apply it
	nextState, duration := r.stateMachine.GetNextState(ctx, pc)

	// Apply the state
	if err := r.stateMachine.ApplyState(ctx, pc, nextState); err != nil {
		r.logger.Error(ctx, "Failed to apply state %s to ProjectClaim %s/%s: %v",
			nextState, pc.Namespace, pc.Name, err)
		return reconcile.Result{}, err
	}

	// Update the ProjectClaim
	if err := r.client.Status().Update(ctx, pc); err != nil {
		r.logger.Error(ctx, "Failed to update ProjectClaim %s/%s status: %v",
			pc.Namespace, pc.Name, err)
		return reconcile.Result{}, err
	}

	// Also update spec if fields were set
	if err := r.client.Update(ctx, pc); err != nil {
		r.logger.Error(ctx, "Failed to update ProjectClaim %s/%s spec: %v",
			pc.Namespace, pc.Name, err)
		return reconcile.Result{}, err
	}

	r.logger.Info(ctx, "ProjectClaim %s/%s transitioned to state: %s", pc.Namespace, pc.Name, nextState)

	// Requeue after duration for next state transition
	if duration > 0 {
		// Check for delay override
		duration = r.behaviorEngine.GetTransitionDelay(ctx, "ProjectClaim", pc.Namespace, pc.Name, duration)
		r.logger.Debug(ctx, "Requeuing ProjectClaim %s/%s after %v", pc.Namespace, pc.Name, duration)
		return reconcile.Result{RequeueAfter: duration}, nil
	}

	return reconcile.Result{}, nil
}

// applyFailure applies a failure state to the ProjectClaim
func (r *ProjectClaimReconciler) applyFailure(ctx context.Context, pc *gcpv1alpha1.ProjectClaim, failure *config.FailureScenario) (reconcile.Result, error) {
	if err := r.stateMachine.ApplyFailure(ctx, pc, failure); err != nil {
		r.logger.Error(ctx, "Failed to apply failure to ProjectClaim %s/%s: %v",
			pc.Namespace, pc.Name, err)
		return reconcile.Result{}, err
	}

	if err := r.client.Status().Update(ctx, pc); err != nil {
		r.logger.Error(ctx, "Failed to update failed ProjectClaim %s/%s status: %v",
			pc.Namespace, pc.Name, err)
		return reconcile.Result{}, err
	}

	r.logger.Info(ctx, "ProjectClaim %s/%s failed: %s", pc.Namespace, pc.Name, failure.Message)
	return reconcile.Result{}, nil
}
