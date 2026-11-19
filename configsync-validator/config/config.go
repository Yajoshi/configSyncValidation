// Package config holds the run configuration: defaults, the config-file
// schema, command-line overrides and conversion into a workflow plan.
// Reading the YAML file itself is done by the kube package so that this
// package stays free of third-party dependencies.
package config

import (
	"encoding/json"
	"fmt"
	"time"

	"configsync-validator/model"
	"configsync-validator/syncer"
)

// Duration is a time.Duration that reads and writes strings like "30s".
type Duration struct{ time.Duration }

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("duration must be a string such as \"30s\" or \"10m\": %w", err)
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

// SyncConfig configures one of the two syncs. It is enabled when a manifest or
// a name is given. With a manifest the object is applied and then validated;
// with only a name, its status is checked and nothing is applied.
type SyncConfig struct {
	Manifest          string `json:"manifest,omitempty"`
	Name              string `json:"name,omitempty"`
	Namespace         string `json:"namespace,omitempty"`
	APIVersion        string `json:"apiVersion,omitempty"`
	ResourceGroupName string `json:"resourceGroupName,omitempty"`
	ExpectedCommit    string `json:"expectedCommit,omitempty"`
}

// Enabled reports whether this sync takes part in the run.
func (s SyncConfig) Enabled() bool { return s.Manifest != "" || s.Name != "" }

// Config is the full configuration.
type Config struct {
	Kubeconfig string `json:"kubeconfig,omitempty"`
	Context    string `json:"context,omitempty"`

	PollInterval   Duration `json:"pollInterval"`
	Timeout        Duration `json:"timeout"`
	ErrorThreshold int      `json:"errorThreshold"`

	FieldManager   string `json:"fieldManager"`
	ForceConflicts bool   `json:"forceConflicts"`

	SkipResourceGroupValidation bool `json:"skipResourceGroupValidation"`
	AllowEmptyResourceGroup     bool `json:"allowEmptyResourceGroup"`

	RootSync SyncConfig `json:"rootSync"`
	RepoSync SyncConfig `json:"repoSync"`
}

// Default returns the built-in defaults.
func Default() Config {
	return Config{
		PollInterval:   Duration{5 * time.Second},
		Timeout:        Duration{10 * time.Minute},
		ErrorThreshold: 3,
		FieldManager:   "configsync-validator",
	}
}

// Validate checks values that do not depend on the cluster.
func (c Config) Validate() error {
	if c.PollInterval.Duration <= 0 {
		return fmt.Errorf("pollInterval must be positive")
	}
	if c.Timeout.Duration <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	if c.ErrorThreshold < 1 {
		return fmt.Errorf("errorThreshold must be at least 1")
	}
	if !c.RootSync.Enabled() && !c.RepoSync.Enabled() {
		return fmt.Errorf("nothing to do: set rootSync and/or repoSync (a manifest, or a name to only check status)")
	}
	return nil
}

// Plan converts the configuration into a workflow plan.
func (c Config) Plan() (syncer.Plan, error) {
	if err := c.Validate(); err != nil {
		return syncer.Plan{}, err
	}
	var p syncer.Plan
	if c.RootSync.Enabled() {
		p.Root = &syncer.Step{ManifestPath: c.RootSync.Manifest, Target: targetOf(c.RootSync, model.KindRootSync)}
		if p.Root.Target.Namespace == "" {
			p.Root.Target.Namespace = model.RootSyncNamespace
		}
	}
	if c.RepoSync.Enabled() {
		p.Repo = &syncer.Step{ManifestPath: c.RepoSync.Manifest, Target: targetOf(c.RepoSync, model.KindRepoSync)}
	}
	return p, nil
}

func targetOf(s SyncConfig, kind string) model.SyncTarget {
	return model.SyncTarget{
		Kind:              kind,
		Name:              s.Name,
		Namespace:         s.Namespace,
		APIVersion:        s.APIVersion,
		ResourceGroupName: s.ResourceGroupName,
		ExpectedCommit:    s.ExpectedCommit,
	}
}
