package hive_simulator

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/go-logr/logr"
	"github.com/openshift-online/ocm-sdk-go/logging"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	errors "github.com/zgalor/weberr"

	aaov1alpha1 "github.com/tzvatot/openshift-hive-simulator/pkg/externalapis/aws-account-operator/v1alpha1"
	gcpv1alpha1 "github.com/tzvatot/openshift-hive-simulator/pkg/externalapis/gcp-project-operator/v1alpha1"
	"github.com/tzvatot/openshift-hive-simulator/pkg/api"
	"github.com/tzvatot/openshift-hive-simulator/pkg/behavior"
	"github.com/tzvatot/openshift-hive-simulator/pkg/config"
	"github.com/tzvatot/openshift-hive-simulator/pkg/controllers"
	"github.com/tzvatot/openshift-hive-simulator/pkg/state_machine"
)

// Server is the main hive simulator server
type Server struct {
	logger         logging.Logger
	config         *config.Config
	apiPort        int
	envTest        *envtest.Environment
	k8sClient      client.Client
	mgr            manager.Manager
	behaviorEngine *behavior.Engine
	apiServer      *http.Server
	kubeconfigPath string
}

// NewServer creates a new hive simulator server
func NewServer(logger logging.Logger, cfg *config.Config, apiPort int) *Server {
	return &Server{
		logger:  logger,
		config:  cfg,
		apiPort: apiPort,
	}
}

// Start starts the simulator server
func (s *Server) Start(ctx context.Context) error {
	s.logger.Info(ctx, "Starting Hive Simulator")

	// Set up envtest
	if err := s.setupEnvtest(ctx); err != nil {
		return errors.Wrapf(err, "failed to setup envtest")
	}

	// Create Kubernetes client
	if err := s.setupK8sClient(ctx); err != nil {
		return errors.Wrapf(err, "failed to setup kubernetes client")
	}

	// Pre-populate ClusterImageSets
	if err := s.prepopulateClusterImageSets(ctx); err != nil {
		return errors.Wrapf(err, "failed to prepopulate ClusterImageSets")
	}

	// Set up behavior engine
	s.behaviorEngine = behavior.NewEngine(s.logger, s.config)

	// Set up controller manager
	if err := s.setupControllerManager(ctx); err != nil {
		return errors.Wrapf(err, "failed to setup controller manager")
	}

	// Start controller manager in background
	go func() {
		s.logger.Info(ctx, "Starting controller manager")
		if err := s.mgr.Start(ctx); err != nil {
			s.logger.Error(ctx, "Controller manager failed: %v", err)
		}
	}()

	// Wait for cache sync
	if !s.mgr.GetCache().WaitForCacheSync(ctx) {
		return errors.Errorf("failed to wait for cache sync")
	}

	// Start API server
	if err := s.startAPIServer(ctx); err != nil {
		return errors.Wrapf(err, "failed to start API server")
	}

	s.logger.Info(ctx, "Hive Simulator started successfully")
	s.logger.Info(ctx, "  Kubernetes API: Use kubeconfig at %s", s.kubeconfigPath)
	s.logger.Info(ctx, "  Configuration API: http://localhost:%d", s.apiPort)

	// Wait for context cancellation
	<-ctx.Done()

	s.logger.Info(ctx, "Shutting down Hive Simulator")
	return s.stop(context.Background())
}

// setupEnvtest sets up the envtest environment
func (s *Server) setupEnvtest(ctx context.Context) error {
	s.logger.Info(ctx, "Setting up envtest environment")

	// Set up controller-runtime logger early to avoid warnings during envtest startup
	ctrl.SetLogger(logr.Discard())

	// Create scheme with all our CRDs
	scheme := runtime.NewScheme()
	if err := hivev1.AddToScheme(scheme); err != nil {
		return errors.Wrapf(err, "failed to add Hive to scheme")
	}
	if err := aaov1alpha1.AddToScheme(scheme); err != nil {
		return errors.Wrapf(err, "failed to add AWS Account Operator to scheme")
	}
	if err := gcpv1alpha1.AddToScheme(scheme); err != nil {
		return errors.Wrapf(err, "failed to add GCP Project Operator to scheme")
	}

	// Find the CRD directory relative to the binary location
	crdPath := filepath.Join(filepath.Dir(os.Args[0]), "..", "cmd", "hive-simulator", "crds")
	if _, err := os.Stat(crdPath); os.IsNotExist(err) {
		// Fallback to relative path from working directory
		crdPath = "cmd/hive-simulator/crds"
	}
	s.logger.Info(ctx, "Loading CRDs from: %s", crdPath)

	// Note: envtest uses dynamic ports which change on each restart
	// Use restart-simulator.sh to automatically regenerate provision shard config after restart
	s.envTest = &envtest.Environment{
		Scheme: scheme,
		CRDDirectoryPaths: []string{
			crdPath,
		},
		ErrorIfCRDPathMissing:    true, // Fail if CRDs not found
		ControlPlaneStartTimeout: time.Minute,
		ControlPlaneStopTimeout:  time.Minute,
	}

	cfg, err := s.envTest.Start()
	if err != nil {
		return errors.Wrapf(err, "failed to start envtest")
	}

	s.logger.Info(ctx, "Envtest started, Kubernetes API at %s", cfg.Host)

	// Create kubeconfig file for external access
	if err := s.createKubeconfig(cfg); err != nil {
		return errors.Wrapf(err, "failed to create kubeconfig")
	}

	return nil
}

// createKubeconfig creates a kubeconfig file for external access
func (s *Server) createKubeconfig(cfg *rest.Config) error {
	// Create a kubeconfig
	kubeconfig := clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			"hive-simulator": {
				Server:                   cfg.Host,
				CertificateAuthorityData: cfg.CAData,
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"hive-simulator": {
				Cluster:  "hive-simulator",
				AuthInfo: "hive-simulator",
			},
		},
		CurrentContext: "hive-simulator",
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"hive-simulator": {
				ClientCertificateData: cfg.CertData,
				ClientKeyData:         cfg.KeyData,
			},
		},
	}

	// Write to temp file
	kubeconfigPath := filepath.Join(os.TempDir(), "hive-simulator-kubeconfig.yaml")
	if err := clientcmd.WriteToFile(kubeconfig, kubeconfigPath); err != nil {
		return errors.Wrapf(err, "failed to write kubeconfig")
	}

	s.kubeconfigPath = kubeconfigPath
	return nil
}

// setupK8sClient sets up the Kubernetes client
func (s *Server) setupK8sClient(ctx context.Context) error {
	s.logger.Info(ctx, "Setting up Kubernetes client")

	scheme := runtime.NewScheme()

	// Add Hive types
	if err := hivev1.AddToScheme(scheme); err != nil {
		return errors.Wrapf(err, "failed to add Hive to scheme")
	}

	// Add AWS Account Operator types
	if err := aaov1alpha1.AddToScheme(scheme); err != nil {
		return errors.Wrapf(err, "failed to add AWS Account Operator to scheme")
	}

	// Add GCP Project Operator types
	if err := gcpv1alpha1.AddToScheme(scheme); err != nil {
		return errors.Wrapf(err, "failed to add GCP Project Operator to scheme")
	}

	// Create client
	k8sClient, err := client.New(s.envTest.Config, client.Options{Scheme: scheme})
	if err != nil {
		return errors.Wrapf(err, "failed to create kubernetes client")
	}

	s.k8sClient = k8sClient
	return nil
}

// setupControllerManager sets up the controller manager
func (s *Server) setupControllerManager(ctx context.Context) error {
	s.logger.Info(ctx, "Setting up controller manager")

	// Set up controller-runtime logger to avoid "log.SetLogger(...) was never called" warnings
	// Use a discard logger since we do our own logging
	ctrl.SetLogger(logr.Discard())

	// Create manager with metrics disabled to avoid port conflicts
	mgr, err := ctrl.NewManager(s.envTest.Config, ctrl.Options{
		Scheme: s.k8sClient.Scheme(),
		Metrics: metricsserver.Options{
			BindAddress: "0", // Disable metrics server
		},
		HealthProbeBindAddress: "0", // Disable health probe server
	})
	if err != nil {
		return errors.Wrapf(err, "failed to create manager")
	}

	// Create state machines
	cdStateMachine := state_machine.NewClusterDeploymentStateMachine(s.logger, s.config.ClusterDeployment)
	acStateMachine := state_machine.NewAccountClaimStateMachine(s.logger, s.config.AccountClaim)
	pcStateMachine := state_machine.NewProjectClaimStateMachine(s.logger, s.config.ProjectClaim)

	// Create reconcilers
	cdReconciler := controllers.NewClusterDeploymentReconciler(
		mgr.GetClient(),
		s.logger,
		cdStateMachine,
		s.behaviorEngine,
	)

	acReconciler := controllers.NewAccountClaimReconciler(
		mgr.GetClient(),
		s.logger,
		acStateMachine,
		s.behaviorEngine,
	)

	pcReconciler := controllers.NewProjectClaimReconciler(
		mgr.GetClient(),
		s.logger,
		pcStateMachine,
		s.behaviorEngine,
	)

	// Register reconcilers with controller-runtime
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&hivev1.ClusterDeployment{}).
		Complete(cdReconciler); err != nil {
		return errors.Wrapf(err, "failed to create ClusterDeployment controller")
	}

	if err := ctrl.NewControllerManagedBy(mgr).
		For(&aaov1alpha1.AccountClaim{}).
		Complete(acReconciler); err != nil {
		return errors.Wrapf(err, "failed to create AccountClaim controller")
	}

	if err := ctrl.NewControllerManagedBy(mgr).
		For(&gcpv1alpha1.ProjectClaim{}).
		Complete(pcReconciler); err != nil {
		return errors.Wrapf(err, "failed to create ProjectClaim controller")
	}

	s.mgr = mgr
	return nil
}

// prepopulateClusterImageSets pre-populates ClusterImageSets
func (s *Server) prepopulateClusterImageSets(ctx context.Context) error {
	s.logger.Info(ctx, "Pre-populating ClusterImageSets")

	for _, cisConfig := range s.config.ClusterImageSets {
		cis := &hivev1.ClusterImageSet{}
		cis.Name = cisConfig.Name
		cis.Spec.ReleaseImage = fmt.Sprintf("quay.io/openshift-release-dev/ocp-release:%s", cisConfig.Name)

		// Add channel-group label expected by clusters-service
		channelGroup := s.extractChannelGroup(cisConfig.Name)
		if cis.Labels == nil {
			cis.Labels = make(map[string]string)
		}
		cis.Labels["api.openshift.com/channel-group"] = channelGroup

		// Add version annotation expected by clusters-service
		version := s.extractVersion(cisConfig.Name)
		if cis.Annotations == nil {
			cis.Annotations = make(map[string]string)
		}
		cis.Annotations["api.openshift.com/version"] = version

		if err := s.k8sClient.Create(ctx, cis); err != nil {
			s.logger.Warn(ctx, "Failed to create ClusterImageSet %s (may already exist): %v", cisConfig.Name, err)
			continue
		}

		s.logger.Debug(ctx, "Created ClusterImageSet: %s (channel: %s, version: %s)", cisConfig.Name, channelGroup, version)
	}

	return nil
}

// extractChannelGroup extracts the channel group from the ClusterImageSet name
func (s *Server) extractChannelGroup(name string) string {
	// Infer channel from name patterns
	// Candidate: openshift-v4.17.0-ec.0-candidate
	if strings.Contains(name, "-ec.") || strings.Contains(name, "-candidate") {
		return "candidate"
	}
	// Fast: openshift-v4.17.0-fc.0-fast
	if strings.Contains(name, "-fc.") || strings.Contains(name, "-fast") {
		return "fast"
	}
	// Nightly: openshift-v4.17.0-0.nightly-2024-08-01-120000-nightly
	if strings.Contains(name, "-nightly") {
		return "nightly"
	}
	// Default to stable: openshift-v4.17.0
	return "stable"
}

// extractVersion extracts the version string from the ClusterImageSet name
func (s *Server) extractVersion(name string) string {
	// Remove "openshift-v" prefix
	version := strings.TrimPrefix(name, "openshift-v")

	// Remove channel suffixes
	version = strings.TrimSuffix(version, "-candidate")
	version = strings.TrimSuffix(version, "-fast")
	version = strings.TrimSuffix(version, "-nightly")

	return version
}

// startAPIServer starts the REST API server
func (s *Server) startAPIServer(ctx context.Context) error {
	s.logger.Info(ctx, "Starting API server on port %d", s.apiPort)

	handlers := api.NewHandlers(s.logger, s.behaviorEngine)
	router := api.SetupRoutes(handlers)

	s.apiServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", s.apiPort),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		if err := s.apiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error(ctx, "API server failed: %v", err)
		}
	}()

	return nil
}

// stop stops the simulator
func (s *Server) stop(ctx context.Context) error {
	s.logger.Info(ctx, "Stopping Hive Simulator")

	// Stop API server
	if s.apiServer != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := s.apiServer.Shutdown(shutdownCtx); err != nil {
			s.logger.Error(ctx, "Failed to shutdown API server: %v", err)
		}
	}

	// Stop envtest
	if s.envTest != nil {
		if err := s.envTest.Stop(); err != nil {
			s.logger.Error(ctx, "Failed to stop envtest: %v", err)
		}
	}

	// Clean up kubeconfig
	if s.kubeconfigPath != "" {
		if err := os.Remove(s.kubeconfigPath); err != nil {
			s.logger.Warn(ctx, "Failed to remove kubeconfig: %v", err)
		}
	}

	s.logger.Info(ctx, "Hive Simulator stopped")
	return nil
}
