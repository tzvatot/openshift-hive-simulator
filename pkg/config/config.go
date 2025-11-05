package config

import (
	"time"
)

// Config is the main configuration for the hive simulator
type Config struct {
	ClusterDeployment *ClusterDeploymentConfig `yaml:"clusterDeployment" json:"clusterDeployment"`
	AccountClaim      *AccountClaimConfig      `yaml:"accountClaim" json:"accountClaim"`
	ProjectClaim      *ProjectClaimConfig      `yaml:"projectClaim" json:"projectClaim"`
	ClusterImageSets  []ClusterImageSetConfig  `yaml:"clusterImageSets" json:"clusterImageSets"`
}

// ClusterDeploymentConfig configures ClusterDeployment simulation behavior
type ClusterDeploymentConfig struct {
	// DefaultDelaySeconds is the total time from creation to ready state
	DefaultDelaySeconds int `yaml:"defaultDelaySeconds" json:"defaultDelaySeconds"`

	// States defines the progression and timing for each state
	States []StateConfig `yaml:"states" json:"states"`

	// FailureScenarios defines potential failure modes
	FailureScenarios []FailureScenario `yaml:"failureScenarios" json:"failureScenarios"`

	// DependsOnAccountClaim if true, waits for AccountClaim to be Ready before progressing
	DependsOnAccountClaim bool `yaml:"dependsOnAccountClaim" json:"dependsOnAccountClaim"`

	// DependsOnProjectClaim if true, waits for ProjectClaim to be Ready before progressing
	DependsOnProjectClaim bool `yaml:"dependsOnProjectClaim" json:"dependsOnProjectClaim"`
}

// AccountClaimConfig configures AccountClaim simulation behavior
type AccountClaimConfig struct {
	// DefaultDelaySeconds is the total time from creation to ready state
	DefaultDelaySeconds int `yaml:"defaultDelaySeconds" json:"defaultDelaySeconds"`

	// States defines the progression and timing for each state
	States []StateConfig `yaml:"states" json:"states"`

	// FailureScenarios defines potential failure modes
	FailureScenarios []FailureScenario `yaml:"failureScenarios" json:"failureScenarios"`
}

// ProjectClaimConfig configures ProjectClaim simulation behavior
type ProjectClaimConfig struct {
	// DefaultDelaySeconds is the total time from creation to ready state
	DefaultDelaySeconds int `yaml:"defaultDelaySeconds" json:"defaultDelaySeconds"`

	// States defines the progression and timing for each state
	States []StateConfig `yaml:"states" json:"states"`

	// FailureScenarios defines potential failure modes
	FailureScenarios []FailureScenario `yaml:"failureScenarios" json:"failureScenarios"`
}

// StateConfig defines a state and its duration
type StateConfig struct {
	// Name is the state name (e.g., "Pending", "Installing", "Running")
	Name string `yaml:"name" json:"name"`

	// DurationSeconds is how long to stay in this state
	DurationSeconds int `yaml:"durationSeconds" json:"durationSeconds"`

	// Conditions are additional conditions to set for this state
	Conditions []ConditionConfig `yaml:"conditions,omitempty" json:"conditions,omitempty"`
}

// ConditionConfig defines a condition to set on a resource
type ConditionConfig struct {
	Type    string `yaml:"type" json:"type"`
	Status  string `yaml:"status" json:"status"`
	Reason  string `yaml:"reason,omitempty" json:"reason,omitempty"`
	Message string `yaml:"message,omitempty" json:"message,omitempty"`
}

// FailureScenario defines a potential failure mode
type FailureScenario struct {
	// Probability is the chance of this failure occurring (0.0-1.0)
	Probability float64 `yaml:"probability" json:"probability"`

	// Condition is the failure condition type
	Condition string `yaml:"condition" json:"condition"`

	// Message is the failure message
	Message string `yaml:"message" json:"message"`

	// Reason is the failure reason
	Reason string `yaml:"reason,omitempty" json:"reason,omitempty"`
}

// ClusterImageSetConfig defines a ClusterImageSet to pre-populate
type ClusterImageSetConfig struct {
	Name    string `yaml:"name" json:"name"`
	Visible bool   `yaml:"visible" json:"visible"`
}

// ResourceOverride allows per-resource behavior overrides
type ResourceOverride struct {
	// ResourceName is the name of the specific resource
	ResourceName string `json:"resourceName"`

	// DelaySeconds overrides the default delay
	DelaySeconds *int `json:"delaySeconds,omitempty"`

	// ForceFail forces this resource to fail
	ForceFail *FailureScenario `json:"forceFail,omitempty"`

	// ForceSuccess forces this resource to succeed (overrides probability-based failures)
	ForceSuccess bool `json:"forceSuccess,omitempty"`
}

// GetTotalDuration returns the total duration for all states
func (c *ClusterDeploymentConfig) GetTotalDuration() time.Duration {
	if c.DefaultDelaySeconds > 0 {
		return time.Duration(c.DefaultDelaySeconds) * time.Second
	}
	total := 0
	for _, state := range c.States {
		total += state.DurationSeconds
	}
	return time.Duration(total) * time.Second
}

// GetTotalDuration returns the total duration for all states
func (c *AccountClaimConfig) GetTotalDuration() time.Duration {
	if c.DefaultDelaySeconds > 0 {
		return time.Duration(c.DefaultDelaySeconds) * time.Second
	}
	total := 0
	for _, state := range c.States {
		total += state.DurationSeconds
	}
	return time.Duration(total) * time.Second
}

// GetTotalDuration returns the total duration for all states
func (c *ProjectClaimConfig) GetTotalDuration() time.Duration {
	if c.DefaultDelaySeconds > 0 {
		return time.Duration(c.DefaultDelaySeconds) * time.Second
	}
	total := 0
	for _, state := range c.States {
		total += state.DurationSeconds
	}
	return time.Duration(total) * time.Second
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		ClusterDeployment: &ClusterDeploymentConfig{
			DefaultDelaySeconds:   5,
			DependsOnAccountClaim: true,
			DependsOnProjectClaim: true,
			States: []StateConfig{
				{
					Name:            "Pending",
					DurationSeconds: 1,
				},
				{
					Name:            "Provisioning",
					DurationSeconds: 2,
					Conditions: []ConditionConfig{
						{
							Type:    "DeprovisionLaunchError",
							Status:  "False",
							Reason:  "Provisioning",
							Message: "Cluster is provisioning",
						},
					},
				},
				{
					Name:            "Installing",
					DurationSeconds: 1,
					Conditions: []ConditionConfig{
						{
							Type:    "DNSNotReady",
							Status:  "False",
							Reason:  "DNSReady",
							Message: "DNS is ready",
						},
					},
				},
				{
					Name:            "Running",
					DurationSeconds: 1,
					Conditions: []ConditionConfig{
						{
							Type:    "ClusterDeploymentCompleted",
							Status:  "True",
							Reason:  "ClusterDeploymentCompleted",
							Message: "Cluster deployment is complete",
						},
					},
				},
			},
		},
		AccountClaim: &AccountClaimConfig{
			DefaultDelaySeconds: 3,
			States: []StateConfig{
				{
					Name:            "Pending",
					DurationSeconds: 2,
				},
				{
					Name:            "Ready",
					DurationSeconds: 1,
				},
			},
		},
		ProjectClaim: &ProjectClaimConfig{
			DefaultDelaySeconds: 4,
			States: []StateConfig{
				{
					Name:            "Pending",
					DurationSeconds: 1,
				},
				{
					Name:            "PendingProject",
					DurationSeconds: 2,
				},
				{
					Name:            "Ready",
					DurationSeconds: 1,
				},
			},
		},
		ClusterImageSets: []ClusterImageSetConfig{
			{Name: "openshift-v4.12.0", Visible: true},
			{Name: "openshift-v4.13.0", Visible: true},
			{Name: "openshift-v4.14.0", Visible: true},
			{Name: "openshift-v4.15.0", Visible: true},
		},
	}
}
