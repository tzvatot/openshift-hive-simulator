package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/openshift-online/ocm-sdk-go/logging"

	"github.com/tzvatot/openshift-hive-simulator/pkg"
	"github.com/tzvatot/openshift-hive-simulator/pkg/config"
)

var (
	configPath = flag.String("config", "", "Path to configuration file (YAML)")
	apiPort    = flag.Int("api-port", 8080, "Port for configuration API")
	logLevel   = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
)

func main() {
	flag.Parse()

	// Setup logger
	logger, err := setupLogger(*logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup logger: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	logger.Info(ctx, "Hive Simulator starting...")
	logger.Info(ctx, "  Config file: %s", getConfigPath(*configPath))
	logger.Info(ctx, "  API port: %d", *apiPort)
	logger.Info(ctx, "  Log level: %s", *logLevel)

	// Load configuration
	cfg, err := config.LoadFromFile(*configPath)
	if err != nil {
		logger.Error(ctx, "Failed to load configuration: %v", err)
		os.Exit(1)
	}

	logger.Info(ctx, "Configuration loaded successfully")
	logger.Debug(ctx, "  ClusterDeployment delay: %ds", cfg.ClusterDeployment.DefaultDelaySeconds)
	logger.Debug(ctx, "  AccountClaim delay: %ds", cfg.AccountClaim.DefaultDelaySeconds)
	logger.Debug(ctx, "  ProjectClaim delay: %ds", cfg.ProjectClaim.DefaultDelaySeconds)
	logger.Debug(ctx, "  ClusterImageSets: %d", len(cfg.ClusterImageSets))

	// Create server
	server := hive_simulator.NewServer(logger, cfg, *apiPort)

	// Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		logger.Info(ctx, "Received signal %v, shutting down...", sig)
		cancel()
	}()

	// Start server
	if err := server.Start(ctx); err != nil {
		logger.Error(ctx, "Server failed: %v", err)
		os.Exit(1)
	}

	logger.Info(ctx, "Hive Simulator exited cleanly")
}

// setupLogger creates and configures the logger
func setupLogger(level string) (logging.Logger, error) {
	builder := logging.NewStdLoggerBuilder()

	// Set log level
	switch level {
	case "debug":
		builder.Debug(true)
	case "info":
		builder.Info(true)
	case "warn":
		builder.Warn(true)
	case "error":
		builder.Error(true)
	default:
		return nil, fmt.Errorf("invalid log level: %s", level)
	}

	logger, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build logger: %w", err)
	}

	return logger, nil
}

// getConfigPath returns the config path or a default message
func getConfigPath(path string) string {
	if path == "" {
		return "<using defaults>"
	}
	return path
}
