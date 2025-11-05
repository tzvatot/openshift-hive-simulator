package config

import (
	"os"

	"gopkg.in/yaml.v3"

	errors "github.com/zgalor/weberr"
)

// LoadFromFile loads configuration from a YAML file
func LoadFromFile(path string) (*Config, error) {
	// If no path provided, return default config
	if path == "" {
		return DefaultConfig(), nil
	}

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read config file %s", path)
	}

	// Parse YAML
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, errors.Wrapf(err, "failed to parse config file %s", path)
	}

	// Validate configuration
	if err := validate(&cfg); err != nil {
		return nil, errors.Wrapf(err, "invalid configuration")
	}

	return &cfg, nil
}

// validate validates the configuration
func validate(cfg *Config) error {
	// Ensure we have ClusterDeployment config
	if cfg.ClusterDeployment == nil {
		cfg.ClusterDeployment = DefaultConfig().ClusterDeployment
	}

	// Ensure we have AccountClaim config
	if cfg.AccountClaim == nil {
		cfg.AccountClaim = DefaultConfig().AccountClaim
	}

	// Ensure we have ProjectClaim config
	if cfg.ProjectClaim == nil {
		cfg.ProjectClaim = DefaultConfig().ProjectClaim
	}

	// Ensure we have ClusterImageSets
	if len(cfg.ClusterImageSets) == 0 {
		cfg.ClusterImageSets = DefaultConfig().ClusterImageSets
	}

	// Validate delay values are positive
	if cfg.ClusterDeployment.DefaultDelaySeconds < 0 {
		return errors.Errorf("ClusterDeployment defaultDelaySeconds must be >= 0")
	}
	if cfg.AccountClaim.DefaultDelaySeconds < 0 {
		return errors.Errorf("AccountClaim defaultDelaySeconds must be >= 0")
	}
	if cfg.ProjectClaim.DefaultDelaySeconds < 0 {
		return errors.Errorf("ProjectClaim defaultDelaySeconds must be >= 0")
	}

	// Validate state durations
	for _, state := range cfg.ClusterDeployment.States {
		if state.DurationSeconds < 0 {
			return errors.Errorf("ClusterDeployment state %s duration must be >= 0", state.Name)
		}
	}
	for _, state := range cfg.AccountClaim.States {
		if state.DurationSeconds < 0 {
			return errors.Errorf("AccountClaim state %s duration must be >= 0", state.Name)
		}
	}
	for _, state := range cfg.ProjectClaim.States {
		if state.DurationSeconds < 0 {
			return errors.Errorf("ProjectClaim state %s duration must be >= 0", state.Name)
		}
	}

	// Validate failure probabilities
	for i, scenario := range cfg.ClusterDeployment.FailureScenarios {
		if scenario.Probability < 0.0 || scenario.Probability > 1.0 {
			return errors.Errorf("ClusterDeployment failure scenario %d probability must be 0.0-1.0", i)
		}
	}
	for i, scenario := range cfg.AccountClaim.FailureScenarios {
		if scenario.Probability < 0.0 || scenario.Probability > 1.0 {
			return errors.Errorf("AccountClaim failure scenario %d probability must be 0.0-1.0", i)
		}
	}
	for i, scenario := range cfg.ProjectClaim.FailureScenarios {
		if scenario.Probability < 0.0 || scenario.Probability > 1.0 {
			return errors.Errorf("ProjectClaim failure scenario %d probability must be 0.0-1.0", i)
		}
	}

	return nil
}
