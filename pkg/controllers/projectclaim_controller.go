package controllers

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	kuberrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

	// Create GCP credentials secret when transitioning to Ready
	if nextState == gcpv1alpha1.ClaimStatusReady && pc.Spec.GCPCredentialSecret.Name != "" {
		if err := r.createGCPCredentialsSecret(ctx, pc); err != nil {
			r.logger.Error(ctx, "Failed to create GCP credentials secret for ProjectClaim %s/%s: %v",
				pc.Namespace, pc.Name, err)
			return reconcile.Result{}, err
		}
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

// createGCPCredentialsSecret creates the GCP credentials secret for the ProjectClaim
func (r *ProjectClaimReconciler) createGCPCredentialsSecret(ctx context.Context, pc *gcpv1alpha1.ProjectClaim) error {
	// Check if secret already exists
	secret := &corev1.Secret{}
	secretName := client.ObjectKey{
		Namespace: pc.Spec.GCPCredentialSecret.Namespace,
		Name:      pc.Spec.GCPCredentialSecret.Name,
	}

	err := r.client.Get(ctx, secretName, secret)
	if err == nil {
		// Secret already exists, nothing to do
		r.logger.Debug(ctx, "GCP credentials secret %s/%s already exists",
			secretName.Namespace, secretName.Name)
		return nil
	}

	if !kuberrors.IsNotFound(err) {
		// Some other error occurred
		return err
	}

	// Secret doesn't exist, create it with simulated GCP service account JSON
	simulatedServiceAccount := `{
  "type": "service_account",
  "project_id": "simulated-project-id",
  "private_key_id": "simulated-key-id",
  "private_key": "-----BEGIN PRIVATE KEY-----\nSimulatedPrivateKey\n-----END PRIVATE KEY-----\n",
  "client_email": "simulated@simulated-project-id.iam.gserviceaccount.com",
  "client_id": "123456789012345678901",
  "auth_uri": "https://accounts.google.com/o/oauth2/auth",
  "token_uri": "https://oauth2.googleapis.com/token",
  "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
  "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/simulated%40simulated-project-id.iam.gserviceaccount.com"
}`

	secret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName.Name,
			Namespace: secretName.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"osServiceAccount.json": []byte(simulatedServiceAccount),
		},
	}

	if err := r.client.Create(ctx, secret); err != nil {
		return err
	}

	r.logger.Info(ctx, "Created GCP credentials secret %s/%s for ProjectClaim %s/%s",
		secretName.Namespace, secretName.Name, pc.Namespace, pc.Name)

	return nil
}
