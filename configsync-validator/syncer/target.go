package syncer

import (
	"fmt"
	"strings"

	"configsync-validator/model"
)

// TargetFromObject derives the sync target from a decoded RootSync or RepoSync
// manifest and rejects anything else.
func TargetFromObject(obj map[string]interface{}) (model.SyncTarget, error) {
	apiVersion, _ := obj["apiVersion"].(string)
	kind, _ := obj["kind"].(string)

	group, version := "", apiVersion
	if i := strings.Index(apiVersion, "/"); i >= 0 {
		group, version = apiVersion[:i], apiVersion[i+1:]
	}
	if group != model.ConfigSyncGroup {
		return model.SyncTarget{}, fmt.Errorf("apiVersion %q is not in group %s", apiVersion, model.ConfigSyncGroup)
	}
	if kind != model.KindRootSync && kind != model.KindRepoSync {
		return model.SyncTarget{}, fmt.Errorf("kind %q is not %s or %s", kind, model.KindRootSync, model.KindRepoSync)
	}

	meta, _ := obj["metadata"].(map[string]interface{})
	name, _ := meta["name"].(string)
	ns, _ := meta["namespace"].(string)
	if name == "" {
		return model.SyncTarget{}, fmt.Errorf("%s manifest has no metadata.name", kind)
	}

	switch kind {
	case model.KindRootSync:
		if ns == "" {
			ns = model.RootSyncNamespace
		}
		if ns != model.RootSyncNamespace {
			return model.SyncTarget{}, fmt.Errorf("a RootSync must be in namespace %s, not %s", model.RootSyncNamespace, ns)
		}
	case model.KindRepoSync:
		if ns == "" {
			return model.SyncTarget{}, fmt.Errorf("a RepoSync manifest must set metadata.namespace")
		}
	}

	return model.SyncTarget{Kind: kind, Name: name, Namespace: ns, APIVersion: version}, nil
}
