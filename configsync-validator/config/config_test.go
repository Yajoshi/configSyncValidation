package config

import (
	"encoding/json"
	"flag"
	"testing"
	"time"
)

func parse(t *testing.T, base Config, args ...string) Config {
	t.Helper()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	f := RegisterFlags(fs)
	if err := fs.Parse(args); err != nil {
		t.Fatal(err)
	}
	f.Apply(&base)
	return base
}

func TestDefaultsAndValidate(t *testing.T) {
	c := Default()
	if c.PollInterval.Duration != 5*time.Second || c.Timeout.Duration != 10*time.Minute || c.ErrorThreshold != 3 {
		t.Fatalf("unexpected defaults: %+v", c)
	}
	if err := c.Validate(); err == nil {
		t.Fatal("a config with nothing to do must not validate")
	}
	c.RootSync.Name = "root-sync"
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestFlagsOnlyOverrideWhenSet(t *testing.T) {
	base := Default()
	base.Timeout = Duration{42 * time.Second}
	base.RootSync.Manifest = "from-file.yaml"

	got := parse(t, base, "-repo-manifest", "repo.yaml", "-poll-interval", "1s")
	if got.Timeout.Duration != 42*time.Second {
		t.Errorf("timeout was overridden although the flag was not passed: %v", got.Timeout)
	}
	if got.RootSync.Manifest != "from-file.yaml" {
		t.Errorf("root manifest changed: %q", got.RootSync.Manifest)
	}
	if got.RepoSync.Manifest != "repo.yaml" || got.PollInterval.Duration != time.Second {
		t.Errorf("flags not applied: %+v", got)
	}
}

func TestPlanApplyAndStatusOnly(t *testing.T) {
	c := parse(t, Default(), "-root-manifest", "root.yaml", "-repo-name", "repo-sync", "-repo-namespace", "team-a", "-repo-commit", "abc")
	p, err := c.Plan()
	if err != nil {
		t.Fatal(err)
	}
	if p.Root == nil || p.Root.ManifestPath != "root.yaml" || p.Root.Target.Namespace != "config-management-system" {
		t.Fatalf("root step = %+v", p.Root)
	}
	if p.Repo == nil || p.Repo.ManifestPath != "" || p.Repo.Target.Name != "repo-sync" ||
		p.Repo.Target.Namespace != "team-a" || p.Repo.Target.ExpectedCommit != "abc" {
		t.Fatalf("repo step = %+v", p.Repo)
	}
}

func TestPlanRejectsEmpty(t *testing.T) {
	if _, err := Default().Plan(); err == nil {
		t.Fatal("expected an error")
	}
}

func TestDurationJSON(t *testing.T) {
	var c Config
	if err := json.Unmarshal([]byte(`{"timeout":"90s","pollInterval":"2s"}`), &c); err != nil {
		t.Fatal(err)
	}
	if c.Timeout.Duration != 90*time.Second || c.PollInterval.Duration != 2*time.Second {
		t.Fatalf("%+v", c)
	}
	if err := json.Unmarshal([]byte(`{"timeout":30}`), &c); err == nil {
		t.Fatal("a bare number must be rejected, it is ambiguous")
	}
	if err := json.Unmarshal([]byte(`{"timeout":"soon"}`), &c); err == nil {
		t.Fatal("an unparsable duration must be rejected")
	}
}
