package api

import (
	"github.com/gorilla/mux"
)

// SetupRoutes sets up the API routes
func SetupRoutes(handlers *Handlers) *mux.Router {
	router := mux.NewRouter()

	// Configuration endpoints
	router.HandleFunc("/api/v1/config", handlers.GetConfig).Methods("GET")
	router.HandleFunc("/api/v1/config/clusterdeployment", handlers.UpdateClusterDeploymentConfig).Methods("POST")
	router.HandleFunc("/api/v1/config/accountclaim", handlers.UpdateAccountClaimConfig).Methods("POST")
	router.HandleFunc("/api/v1/config/projectclaim", handlers.UpdateProjectClaimConfig).Methods("POST")

	// Per-resource override endpoints
	router.HandleFunc("/api/v1/overrides/{resourceType}/{namespace}/{name}/failure", handlers.SetResourceFailure).Methods("POST")
	router.HandleFunc("/api/v1/overrides/{resourceType}/{namespace}/{name}/delay", handlers.SetResourceDelay).Methods("POST")
	router.HandleFunc("/api/v1/overrides/{resourceType}/{namespace}/{name}/success", handlers.SetResourceSuccess).Methods("POST")
	router.HandleFunc("/api/v1/overrides/{resourceType}/{namespace}/{name}", handlers.ClearResourceOverride).Methods("DELETE")

	// State management endpoints
	router.HandleFunc("/api/v1/reset", handlers.Reset).Methods("POST")
	router.HandleFunc("/api/v1/status", handlers.GetStatus).Methods("GET")

	return router
}
