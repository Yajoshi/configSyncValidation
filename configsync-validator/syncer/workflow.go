// Package syncer orchestrates the RootSync then RepoSync workflow.
package syncer

import (
	"context"
	"errors"
	"fmt"

	"configsync-validator/model"
	"configsync-validator/monitor"
)

// Applier loads and applies sync manifests. The real implementation (server
// side apply) lives in the kube package; tests use a stub.
type Applier interface {
	// LoadManifest reads a single-document YAML or JSON manifest.
	LoadManifest(path string) (map[string]interface{}, error)
	// Apply creates or updates the object described by obj.
	Apply(ctx context.Context, obj map[string]interface{}, target model.SyncTarget) error
}

// Step describes one sync to process.
//
// With ManifestPath set, the manifest is applied and then validated; Target may
// carry overrides (ResourceGroupName, ExpectedCommit) and, if Name or Namespace
// are set, they must agree with the manifest.
//
// With ManifestPath empty, nothing is applied: Target identifies an existing
// object whose status is simply checked.
type Step struct {
	ManifestPath string
	Target       model.SyncTarget
}

// Plan lists the steps. Root always runs first and Repo only runs when Root
// succeeded. Either may be nil.
type Plan struct {
	Root *Step
	Repo *Step
}

// Workflow runs a Plan.
type Workflow struct {
	Applier Applier
	Monitor *monitor.Monitor
	Options monitor.Options
	Logf    func(format string, args ...interface{})
}

type prepared struct {
	target model.SyncTarget
	obj    map[string]interface{} // nil when status-only
	step   *Step
}

func (w *Workflow) logf(format string, args ...interface{}) {
	if w.Logf != nil {
		w.Logf(format, args...)
	}
}

// prepare validates a step without touching the cluster.
func (w *Workflow) prepare(step *Step, kind string) (*prepared, error) {
	if step.ManifestPath == "" {
		t := step.Target
		t.Kind = kind
		if t.Name == "" {
			return nil, fmt.Errorf("%s: a name is required to check status without a manifest", kind)
		}
		if kind == model.KindRootSync && t.Namespace == "" {
			t.Namespace = model.RootSyncNamespace
		}
		if t.Namespace == "" {
			return nil, fmt.Errorf("%s %s: a namespace is required to check status without a manifest", kind, t.Name)
		}
		return &prepared{target: t, step: step}, nil
	}

	obj, err := w.Applier.LoadManifest(step.ManifestPath)
	if err != nil {
		return nil, fmt.Errorf("%s manifest %s: %w", kind, step.ManifestPath, err)
	}
	t, err := TargetFromObject(obj)
	if err != nil {
		return nil, fmt.Errorf("%s manifest %s: %w", kind, step.ManifestPath, err)
	}
	if t.Kind != kind {
		return nil, fmt.Errorf("manifest %s is a %s but a %s was expected here", step.ManifestPath, t.Kind, kind)
	}
	if n := step.Target.Name; n != "" && n != t.Name {
		return nil, fmt.Errorf("manifest %s is named %q but the configuration says %q", step.ManifestPath, t.Name, n)
	}
	if ns := step.Target.Namespace; ns != "" && ns != t.Namespace {
		return nil, fmt.Errorf("manifest %s is in namespace %q but the configuration says %q", step.ManifestPath, t.Namespace, ns)
	}
	t.ResourceGroupName = step.Target.ResourceGroupName
	t.ExpectedCommit = step.Target.ExpectedCommit
	return &prepared{target: t, obj: obj, step: step}, nil
}

func failedResult(t model.SyncTarget, msg string) model.Result {
	return model.Result{Target: t, Outcome: model.Failed, Message: msg}
}

// Run executes the plan and returns the report. The returned error is only
// non-nil when the plan itself is unusable; cluster problems are reported as
// FAILED or TIMEOUT results.
func (w *Workflow) Run(ctx context.Context, plan Plan) (Report, error) {
	if plan.Root == nil && plan.Repo == nil {
		return Report{}, errors.New("nothing to do: configure a RootSync and/or a RepoSync")
	}

	// Preflight: check every step before changing anything in the cluster, so a
	// typo in the RepoSync manifest cannot leave a half-applied run behind.
	var root, repo *prepared
	var err error
	if plan.Root != nil {
		if root, err = w.prepare(plan.Root, model.KindRootSync); err != nil {
			return Report{Overall: model.Failed, Results: []model.Result{
				failedResult(model.SyncTarget{Kind: model.KindRootSync}, err.Error())}}, nil
		}
	}
	if plan.Repo != nil {
		if repo, err = w.prepare(plan.Repo, model.KindRepoSync); err != nil {
			return Report{Overall: model.Failed, Results: []model.Result{
				failedResult(model.SyncTarget{Kind: model.KindRepoSync}, err.Error())}}, nil
		}
	}

	var results []model.Result
	rootOK := true

	if root != nil {
		res := w.runOne(ctx, root)
		results = append(results, res)
		rootOK = res.Outcome == model.Success
	}
	if repo != nil {
		if rootOK {
			results = append(results, w.runOne(ctx, repo))
		} else {
			results = append(results, model.Result{
				Target:  repo.target,
				Outcome: model.Skipped,
				Message: "not started because the RootSync did not succeed",
			})
		}
	}

	return Report{Overall: overall(results), Results: results}, nil
}

func (w *Workflow) runOne(ctx context.Context, p *prepared) model.Result {
	if p.obj != nil {
		w.logf("%s: applying %s", p.target, p.step.ManifestPath)
		if err := w.Applier.Apply(ctx, p.obj, p.target); err != nil {
			return failedResult(p.target, fmt.Sprintf("apply failed: %v", err))
		}
		w.logf("%s: applied, waiting for it to sync", p.target)
	} else {
		w.logf("%s: checking status (nothing applied)", p.target)
	}

	opts := w.Options
	if opts.Logf == nil {
		opts.Logf = w.Logf
	}
	return w.Monitor.WaitAndValidate(ctx, p.target, opts)
}
