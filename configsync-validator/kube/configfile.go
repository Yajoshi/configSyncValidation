package kube

import (
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"

	"configsync-validator/config"
)

// LoadConfigFile reads a YAML configuration file on top of the defaults.
// Unknown keys are rejected so typos do not silently change behaviour, and
// relative manifest paths are resolved against the file's own directory.
func LoadConfigFile(path string) (config.Config, error) {
	cfg := config.Default()

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := yaml.UnmarshalStrict(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}

	dir := filepath.Dir(path)
	cfg.RootSync.Manifest = resolve(dir, cfg.RootSync.Manifest)
	cfg.RepoSync.Manifest = resolve(dir, cfg.RepoSync.Manifest)
	if cfg.Kubeconfig != "" {
		cfg.Kubeconfig = resolve(dir, cfg.Kubeconfig)
	}
	return cfg, nil
}

func resolve(dir, p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(dir, p)
}
