package syncer

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"configsync-validator/model"
	"configsync-validator/monitor"
)

type obj = map[string]interface{}

// world is an in-memory cluster: it is both the Applier and the Getter. Objects
// only exist once they have been applied.
type world struct {
	mu        sync.Mutex
	manifests map[string]obj   // path -> manifest
	applied   map[string]bool  // "Kind ns/name"
	order     []string         // apply order
	badSource map[string]bool  // names whose source reports an error
	applyErr  map[string]error // names whose apply fails
}

func newWorld() *world {
	return &world{
		manifests: map[string]obj{},
		applied:   map[string]bool{},
		badSource: map[string]bool{},
		applyErr:  map[string]error{},
	}
}

func manifest(kind, ns, name string) obj {
	meta := obj{"name": name}
	if ns != "" {
		meta["namespace"] = ns
	}
	return obj{"apiVersion": "configsync.gke.io/v1beta1", "kind": kind, "metadata": meta}
}

func (w *world) LoadManifest(path string) (map[string]interface{}, error) {
	m, ok := w.manifests[path]
	if !ok {
		return nil, errors.New("no such file")
	}
	return m, nil
}

func (w *world) Apply(_ context.Context, o map[string]interface{}, t model.SyncTarget) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.applyErr[t.Name]; err != nil {
		return err
	}
	w.applied[t.String()] = true
	w.order = append(w.order, t.Name)
	return nil
}

func (w *world) GetSync(_ context.Context, t model.SyncTarget) (map[string]interface{}, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.applied[t.String()] {
		return nil, monitor.ErrNotFound
	}
	status := obj{
		"observedGeneration": int64(1),
		"lastSyncedCommit":   "abc",
		"source":             obj{"commit": "abc"},
		"sync":               obj{"commit": "abc"},
		"conditions": []interface{}{
			obj{"type": "Syncing", "status": "False", "reason": "Sync", "commit": "abc"},
		},
	}
	if w.badSource[t.Name] {
		status["source"] = obj{"commit": "abc", "errors": []interface{}{obj{"errorMessage": "git auth failed"}}}
	}
	return obj{"metadata": obj{"generation": int64(1)}, "status": status}, nil
}

func (w *world) GetResourceGroup(_ context.Context, t model.SyncTarget) (map[string]interface{}, error) {
	return obj{
		"metadata": obj{"generation": int64(1)},
		"spec":     obj{"resources": []interface{}{obj{}}},
		"status": obj{"resourceStatuses": []interface{}{
			obj{"kind": "ConfigMap", "namespace": "ns", "name": "c", "status": "Current"},
		}},
	}, nil
}

func (w *world) workflow() *Workflow {
	return &Workflow{
		Applier: w,
		Monitor: &monitor.Monitor{Source: w},
		Options: monitor.Options{PollInterval: 2 * time.Millisecond, Timeout: 300 * time.Millisecond, ErrorThreshold: 2},
	}
}

func fullPlan() Plan {
	return Plan{
		Root: &Step{ManifestPath: "root.yaml"},
		Repo: &Step{ManifestPath: "repo.yaml"},
	}
}

func (w *world) loadBoth() {
	w.manifests["root.yaml"] = manifest("RootSync", "", "root-sync")
	w.manifests["repo.yaml"] = manifest("RepoSync", "team-a", "repo-sync")
}

func TestRunAppliesRootThenRepo(t *testing.T) {
	w := newWorld()
	w.loadBoth()
	rep, err := w.workflow().Run(context.Background(), fullPlan())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Overall != model.Success || rep.ExitCode() != 0 {
		t.Fatalf("report = %+v", rep)
	}
	if got := strings.Join(w.order, ","); got != "root-sync,repo-sync" {
		t.Fatalf("apply order = %s", got)
	}
	if len(rep.Results) != 2 || rep.Results[0].Target.Kind != "RootSync" || rep.Results[1].Target.Kind != "RepoSync" {
		t.Fatalf("results = %+v", rep.Results)
	}
}

func TestRunDoesNotApplyRepoWhenRootFails(t *testing.T) {
	w := newWorld()
	w.loadBoth()
	w.badSource["root-sync"] = true

	rep, _ := w.workflow().Run(context.Background(), fullPlan())
	if rep.Overall != model.Failed || rep.ExitCode() != 1 {
		t.Fatalf("report = %+v", rep)
	}
	if got := strings.Join(w.order, ","); got != "root-sync" {
		t.Fatalf("only the root should have been applied, got %s", got)
	}
	if rep.Results[1].Outcome != model.Skipped {
		t.Fatalf("repo result = %+v", rep.Results[1])
	}
	if !strings.Contains(rep.Results[0].Message, "git auth failed") {
		t.Fatalf("root message = %q", rep.Results[0].Message)
	}
}

func TestRunReportsRepoFailureAfterRootSuccess(t *testing.T) {
	w := newWorld()
	w.loadBoth()
	w.badSource["repo-sync"] = true
	rep, _ := w.workflow().Run(context.Background(), fullPlan())
	if rep.Overall != model.Failed || rep.Results[0].Outcome != model.Success || rep.Results[1].Outcome != model.Failed {
		t.Fatalf("report = %+v", rep)
	}
}

func TestRunTimeoutMapsToExitCode2(t *testing.T) {
	w := newWorld()
	w.loadBoth()
	wf := w.workflow()
	wf.Options.Timeout = 30 * time.Millisecond
	// Apply "succeeds" but the object never appears.
	wf.Applier = &noopApplier{w}
	rep, _ := wf.Run(context.Background(), fullPlan())
	if rep.Overall != model.Timeout || rep.ExitCode() != 2 {
		t.Fatalf("report = %+v", rep)
	}
}

type noopApplier struct{ *world }

func (n *noopApplier) Apply(context.Context, map[string]interface{}, model.SyncTarget) error {
	return nil
}

func TestRunApplyFailure(t *testing.T) {
	w := newWorld()
	w.loadBoth()
	w.applyErr["root-sync"] = errors.New("forbidden")
	rep, _ := w.workflow().Run(context.Background(), fullPlan())
	if rep.Overall != model.Failed || !strings.Contains(rep.Results[0].Message, "apply failed: forbidden") {
		t.Fatalf("report = %+v", rep)
	}
	if rep.Results[1].Outcome != model.Skipped {
		t.Fatalf("repo should be skipped: %+v", rep.Results[1])
	}
}

func TestRunPreflightChangesNothingWhenRepoManifestIsBad(t *testing.T) {
	w := newWorld()
	w.manifests["root.yaml"] = manifest("RootSync", "", "root-sync")
	// repo.yaml missing entirely
	rep, _ := w.workflow().Run(context.Background(), fullPlan())
	if rep.Overall != model.Failed || len(w.order) != 0 {
		t.Fatalf("nothing should be applied; order=%v report=%+v", w.order, rep)
	}
}

func TestRunRejectsWrongKind(t *testing.T) {
	w := newWorld()
	w.manifests["root.yaml"] = manifest("RepoSync", "ns", "x")
	rep, _ := w.workflow().Run(context.Background(), Plan{Root: &Step{ManifestPath: "root.yaml"}})
	if rep.Overall != model.Failed || !strings.Contains(rep.Results[0].Message, "expected here") {
		t.Fatalf("report = %+v", rep)
	}
}

func TestRunRejectsNameMismatch(t *testing.T) {
	w := newWorld()
	w.manifests["root.yaml"] = manifest("RootSync", "", "root-sync")
	rep, _ := w.workflow().Run(context.Background(), Plan{Root: &Step{
		ManifestPath: "root.yaml",
		Target:       model.SyncTarget{Name: "other"},
	}})
	if rep.Overall != model.Failed || !strings.Contains(rep.Results[0].Message, "other") {
		t.Fatalf("report = %+v", rep)
	}
}

func TestRunStatusOnlyAppliesNothing(t *testing.T) {
	w := newWorld()
	w.applied["RootSync config-management-system/root-sync"] = true
	rep, err := w.workflow().Run(context.Background(), Plan{Root: &Step{Target: model.SyncTarget{Name: "root-sync"}}})
	if err != nil || rep.Overall != model.Success {
		t.Fatalf("err=%v report=%+v", err, rep)
	}
	if len(w.order) != 0 {
		t.Fatalf("status-only must not apply anything, applied %v", w.order)
	}
}

func TestRunStatusOnlyRepoNeedsNamespace(t *testing.T) {
	w := newWorld()
	rep, _ := w.workflow().Run(context.Background(), Plan{Repo: &Step{Target: model.SyncTarget{Name: "repo-sync"}}})
	if rep.Overall != model.Failed || !strings.Contains(rep.Results[0].Message, "namespace is required") {
		t.Fatalf("report = %+v", rep)
	}
}

func TestRunEmptyPlan(t *testing.T) {
	if _, err := newWorld().workflow().Run(context.Background(), Plan{}); err == nil {
		t.Fatal("expected an error for an empty plan")
	}
}

func TestTargetFromObject(t *testing.T) {
	tests := []struct {
		name    string
		in      obj
		wantErr string
		want    model.SyncTarget
	}{
		{name: "root default namespace", in: manifest("RootSync", "", "r"),
			want: model.SyncTarget{Kind: "RootSync", Name: "r", Namespace: "config-management-system", APIVersion: "v1beta1"}},
		{name: "repo", in: manifest("RepoSync", "team", "s"),
			want: model.SyncTarget{Kind: "RepoSync", Name: "s", Namespace: "team", APIVersion: "v1beta1"}},
		{name: "repo needs namespace", in: manifest("RepoSync", "", "s"), wantErr: "metadata.namespace"},
		{name: "root wrong namespace", in: manifest("RootSync", "other", "r"), wantErr: "config-management-system"},
		{name: "wrong kind", in: manifest("Deployment", "ns", "d"), wantErr: "kind"},
		{name: "wrong group", in: obj{"apiVersion": "apps/v1", "kind": "RootSync", "metadata": obj{"name": "x"}}, wantErr: "group"},
		{name: "no name", in: obj{"apiVersion": "configsync.gke.io/v1beta1", "kind": "RootSync", "metadata": obj{}}, wantErr: "name"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := TargetFromObject(tc.in)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %+v, err %v; want %+v", got, err, tc.want)
			}
		})
	}
}

func TestReportText(t *testing.T) {
	r := Report{Overall: model.Failed, Results: []model.Result{
		{Target: model.SyncTarget{Kind: "RootSync", Name: "r", Namespace: "ns"}, Outcome: model.Success, Commit: "abc", Seconds: 1.5, Message: "ok"},
		{Target: model.SyncTarget{Kind: "RepoSync", Name: "s", Namespace: "t"}, Outcome: model.Skipped, Message: "skipped"},
	}}
	txt := r.Text()
	for _, want := range []string{"RESULT: FAILED", "RootSync ns/r", "commit:  abc", "elapsed: 1.5s", "SKIPPED"} {
		if !strings.Contains(txt, want) {
			t.Errorf("text report missing %q:\n%s", want, txt)
		}
	}
	js, err := r.JSON()
	if err != nil || !strings.Contains(js, `"overall": "FAILED"`) {
		t.Fatalf("json = %s err=%v", js, err)
	}
}
