package controllers

import (
	"context"
	"time"

	kuberrors "k8s.io/apimachinery/pkg/api/errors"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift-online/ocm-sdk-go/logging"
	hivev1 "github.com/openshift/hive/apis/hive/v1"

	aaov1alpha1 "github.com/tzvatot/openshift-hive-simulator/pkg/externalapis/aws-account-operator/v1alpha1"
	gcpv1alpha1 "github.com/tzvatot/openshift-hive-simulator/pkg/externalapis/gcp-project-operator/v1alpha1"
	"github.com/tzvatot/openshift-hive-simulator/pkg/behavior"
	"github.com/tzvatot/openshift-hive-simulator/pkg/config"
	"github.com/tzvatot/openshift-hive-simulator/pkg/state_machine"
	"github.com/tzvatot/openshift-hive-simulator/pkg/labels"
)

// ClusterDeploymentReconciler reconciles ClusterDeployment objects
type ClusterDeploymentReconciler struct {
	client         client.Client
	logger         logging.Logger
	stateMachine   *state_machine.ClusterDeploymentStateMachine
	behaviorEngine *behavior.Engine
}

// NewClusterDeploymentReconciler creates a new ClusterDeployment reconciler
func NewClusterDeploymentReconciler(
	client client.Client,
	logger logging.Logger,
	stateMachine *state_machine.ClusterDeploymentStateMachine,
	behaviorEngine *behavior.Engine,
) *ClusterDeploymentReconciler {
	return &ClusterDeploymentReconciler{
		client:         client,
		logger:         logger,
		stateMachine:   stateMachine,
		behaviorEngine: behaviorEngine,
	}
}

// Reconcile reconciles a ClusterDeployment
func (r *ClusterDeploymentReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	r.logger.Debug(ctx, "Reconciling ClusterDeployment %s/%s", req.Namespace, req.Name)

	cd := &hivev1.ClusterDeployment{}
	if err := r.client.Get(ctx, req.NamespacedName, cd); err != nil {
		if kuberrors.IsNotFound(err) {
			r.logger.Debug(ctx, "ClusterDeployment %s/%s not found, skipping", req.Namespace, req.Name)
			return reconcile.Result{}, nil
		}
		r.logger.Error(ctx, "Failed to get ClusterDeployment %s/%s: %v", req.Namespace, req.Name, err)
		return reconcile.Result{}, err
	}

	// Skip if being deleted
	if !cd.DeletionTimestamp.IsZero() {
		r.logger.Debug(ctx, "ClusterDeployment %s/%s is being deleted, skipping", req.Namespace, req.Name)
		return reconcile.Result{}, nil
	}

	// Skip if already installed
	if cd.Spec.Installed {
		r.logger.Debug(ctx, "ClusterDeployment %s/%s is already installed, skipping", req.Namespace, req.Name)
		return reconcile.Result{}, nil
	}

	// Check for forced failure
	shouldFail, failure := r.behaviorEngine.ShouldFail(ctx, "ClusterDeployment", cd.Namespace, cd.Name)
	if shouldFail {
		return r.applyFailure(ctx, cd, failure)
	}

	// Check dependencies if configured
	if r.stateMachine.ShouldWaitForDependencies() {
		ready, requeueAfter := r.checkDependencies(ctx, cd)
		if !ready {
			r.logger.Debug(ctx, "ClusterDeployment %s/%s waiting for dependencies, requeue after %v",
				cd.Namespace, cd.Name, requeueAfter)
			return reconcile.Result{RequeueAfter: requeueAfter}, nil
		}
	}

	// Determine next state and apply it
	nextState, duration := r.stateMachine.GetNextState(ctx, cd)

	// Apply the state
	if err := r.stateMachine.ApplyState(ctx, cd, nextState); err != nil {
		r.logger.Error(ctx, "Failed to apply state %s to ClusterDeployment %s/%s: %v",
			nextState, cd.Namespace, cd.Name, err)
		return reconcile.Result{}, err
	}

	// Update the ClusterDeployment status
	if err := r.client.Status().Update(ctx, cd); err != nil {
		r.logger.Error(ctx, "Failed to update ClusterDeployment %s/%s status: %v",
			cd.Namespace, cd.Name, err)
		return reconcile.Result{}, err
	}

	// Also update spec if Installed was set
	if cd.Spec.Installed {
		if err := r.client.Update(ctx, cd); err != nil {
			r.logger.Error(ctx, "Failed to update ClusterDeployment %s/%s spec: %v",
				cd.Namespace, cd.Name, err)
			return reconcile.Result{}, err
		}
	}

	r.logger.Info(ctx, "ClusterDeployment %s/%s transitioned to state: %s", cd.Namespace, cd.Name, nextState)

	// Requeue after duration for next state transition
	if duration > 0 {
		// Check for delay override
		duration = r.behaviorEngine.GetTransitionDelay(ctx, "ClusterDeployment", cd.Namespace, cd.Name, duration)
		r.logger.Debug(ctx, "Requeuing ClusterDeployment %s/%s after %v", cd.Namespace, cd.Name, duration)
		return reconcile.Result{RequeueAfter: duration}, nil
	}

	return reconcile.Result{}, nil
}

// checkDependencies checks if AccountClaim or ProjectClaim dependencies are ready
func (r *ClusterDeploymentReconciler) checkDependencies(ctx context.Context, cd *hivev1.ClusterDeployment) (bool, time.Duration) {
	cfg := r.behaviorEngine.GetClusterDeploymentConfig()

	// Determine which dependency to check based on labels
	// Use "cloud-provider" label if it exists, otherwise assume AWS
	cloudProvider := cd.Labels["cloud-provider"]

	// Check AccountClaim for AWS clusters
	if cfg.DependsOnAccountClaim && (cloudProvider == "aws" || cloudProvider == "") {
		ready, requeue := r.checkAccountClaim(ctx, cd)
		if !ready {
			return false, requeue
		}
	}

	// Check ProjectClaim for GCP clusters
	if cfg.DependsOnProjectClaim && cloudProvider == "gcp" {
		ready, requeue := r.checkProjectClaim(ctx, cd)
		if !ready {
			return false, requeue
		}
	}

	return true, 0
}

// checkAccountClaim checks if the AccountClaim is ready
func (r *ClusterDeploymentReconciler) checkAccountClaim(ctx context.Context, cd *hivev1.ClusterDeployment) (bool, time.Duration) {
	// Find AccountClaim with matching cluster label
	clusterID, hasLabel := cd.Labels[labels.ID]
	if !hasLabel {
		r.logger.Debug(ctx, "ClusterDeployment %s/%s has no cluster ID label, assuming no AccountClaim needed",
			cd.Namespace, cd.Name)
		return true, 0
	}

	acList := &aaov1alpha1.AccountClaimList{}
	if err := r.client.List(ctx, acList, client.InNamespace(cd.Namespace)); err != nil {
		r.logger.Error(ctx, "Failed to list AccountClaims in namespace %s: %v", cd.Namespace, err)
		return false, 5 * time.Second
	}

	for i := range acList.Items {
		ac := &acList.Items[i]
		if ac.Labels[labels.ID] == clusterID {
			if ac.Status.State == aaov1alpha1.ClaimStatusReady {
				r.logger.Debug(ctx, "AccountClaim %s/%s is ready for ClusterDeployment %s/%s",
					ac.Namespace, ac.Name, cd.Namespace, cd.Name)
				return true, 0
			}
			r.logger.Debug(ctx, "AccountClaim %s/%s is not ready yet (state: %s) for ClusterDeployment %s/%s",
				ac.Namespace, ac.Name, ac.Status.State, cd.Namespace, cd.Name)
			return false, 2 * time.Second
		}
	}

	r.logger.Debug(ctx, "No AccountClaim found for ClusterDeployment %s/%s (cluster ID: %s)",
		cd.Namespace, cd.Name, clusterID)
	return false, 2 * time.Second
}

// checkProjectClaim checks if the ProjectClaim is ready
func (r *ClusterDeploymentReconciler) checkProjectClaim(ctx context.Context, cd *hivev1.ClusterDeployment) (bool, time.Duration) {
	// Find ProjectClaim with matching cluster label
	clusterID, hasLabel := cd.Labels[labels.ID]
	if !hasLabel {
		r.logger.Debug(ctx, "ClusterDeployment %s/%s has no cluster ID label, assuming no ProjectClaim needed",
			cd.Namespace, cd.Name)
		return true, 0
	}

	pcList := &gcpv1alpha1.ProjectClaimList{}
	if err := r.client.List(ctx, pcList, client.InNamespace(cd.Namespace)); err != nil {
		r.logger.Error(ctx, "Failed to list ProjectClaims in namespace %s: %v", cd.Namespace, err)
		return false, 5 * time.Second
	}

	for i := range pcList.Items {
		pc := &pcList.Items[i]
		if pc.Labels[labels.ID] == clusterID {
			if pc.Status.State == gcpv1alpha1.ClaimStatusReady {
				r.logger.Debug(ctx, "ProjectClaim %s/%s is ready for ClusterDeployment %s/%s",
					pc.Namespace, pc.Name, cd.Namespace, cd.Name)
				return true, 0
			}
			r.logger.Debug(ctx, "ProjectClaim %s/%s is not ready yet (state: %s) for ClusterDeployment %s/%s",
				pc.Namespace, pc.Name, pc.Status.State, cd.Namespace, cd.Name)
			return false, 2 * time.Second
		}
	}

	r.logger.Debug(ctx, "No ProjectClaim found for ClusterDeployment %s/%s (cluster ID: %s)",
		cd.Namespace, cd.Name, clusterID)
	return false, 2 * time.Second
}

// applyFailure applies a failure state to the ClusterDeployment
func (r *ClusterDeploymentReconciler) applyFailure(ctx context.Context, cd *hivev1.ClusterDeployment, failure *config.FailureScenario) (reconcile.Result, error) {
	if err := r.stateMachine.ApplyFailure(ctx, cd, failure); err != nil {
		r.logger.Error(ctx, "Failed to apply failure to ClusterDeployment %s/%s: %v",
			cd.Namespace, cd.Name, err)
		return reconcile.Result{}, err
	}

	if err := r.client.Status().Update(ctx, cd); err != nil {
		r.logger.Error(ctx, "Failed to update failed ClusterDeployment %s/%s status: %v",
			cd.Namespace, cd.Name, err)
		return reconcile.Result{}, err
	}

	r.logger.Info(ctx, "ClusterDeployment %s/%s failed: %s", cd.Namespace, cd.Name, failure.Message)
	return reconcile.Result{}, nil
}
