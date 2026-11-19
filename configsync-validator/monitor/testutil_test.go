package monitor

import (
	"context"
	"sync"

	"configsync-validator/model"
)

type obj = map[string]interface{}

// syncedRootSync returns a fully converged RootSync at the given commit.
func syncedRootSync(commit string) obj {
	return obj{
		"metadata": obj{"name": "root-sync", "generation": int64(2)},
		"status": obj{
			"observedGeneration": int64(2),
			"lastSyncedCommit":   commit,
			"source":             obj{"commit": commit},
			"rendering":          obj{"commit": commit},
			"sync":               obj{"commit": commit},
			"conditions": []interface{}{
				obj{"type": "Reconciling", "status": "False", "reason": "ReconcilerManagerReconciling"},
				obj{"type": "Stalled", "status": "False", "reason": "DeploymentProgressing"},
				obj{"type": "Syncing", "status": "False", "reason": "Sync", "message": "Sync Completed", "commit": commit},
			},
		},
	}
}

func setCond(o obj, typ, status, msg string) {
	status_ := o["status"].(obj)
	conds := status_["conditions"].([]interface{})
	for i, c := range conds {
		if c.(obj)["type"] == typ {
			conds[i] = obj{"type": typ, "status": status, "reason": "Test", "message": msg}
			return
		}
	}
	status_["conditions"] = append(conds, obj{"type": typ, "status": status, "reason": "Test", "message": msg})
}

func removeCond(o obj, typ string) {
	status_ := o["status"].(obj)
	conds := status_["conditions"].([]interface{})
	var out []interface{}
	for _, c := range conds {
		if c.(obj)["type"] != typ {
			out = append(out, c)
		}
	}
	status_["conditions"] = out
}

func resource(kind, ns, name, status string) obj {
	return obj{"group": "apps", "kind": kind, "namespace": ns, "name": name, "status": status,
		"actuation": "Succeeded", "reconcile": "Succeeded"}
}

func resourceGroup(statuses ...interface{}) obj {
	specRes := make([]interface{}, len(statuses))
	for i := range statuses {
		specRes[i] = obj{"kind": "x"}
	}
	return obj{
		"metadata": obj{"name": "root-sync", "generation": int64(3)},
		"spec":     obj{"resources": specRes},
		"status":   obj{"observedGeneration": int64(3), "resourceStatuses": statuses},
	}
}

// fakeGetter is an in-memory monitor.Getter whose objects can be swapped while
// a wait is running.
type fakeGetter struct {
	mu      sync.Mutex
	syncObj obj
	rgObj   obj
	syncErr error
}

func (f *fakeGetter) set(sync, rg obj) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.syncObj, f.rgObj = sync, rg
}

func (f *fakeGetter) GetSync(_ context.Context, _ model.SyncTarget) (map[string]interface{}, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.syncErr != nil {
		return nil, f.syncErr
	}
	if f.syncObj == nil {
		return nil, ErrNotFound
	}
	return f.syncObj, nil
}

func (f *fakeGetter) GetResourceGroup(_ context.Context, _ model.SyncTarget) (map[string]interface{}, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.rgObj == nil {
		return nil, ErrNotFound
	}
	return f.rgObj, nil
}
