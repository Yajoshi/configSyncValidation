// Package kube is the only package that talks to Kubernetes or parses YAML.
// Everything else in the project is plain Go with no third-party imports.
package kube

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"configsync-validator/model"
)

// ResourceGroupGVR is the kpt ResourceGroup resource that Config Sync keeps up
// to date for every RootSync and RepoSync.
var ResourceGroupGVR = schema.GroupVersionResource{
	Group:    "kpt.dev",
	Version:  "v1alpha1",
	Resource: "resourcegroups",
}

// SyncGVR returns the resource for a RootSync or RepoSync target.
func SyncGVR(t model.SyncTarget) schema.GroupVersionResource {
	resource := "rootsyncs"
	if t.Kind == model.KindRepoSync {
		resource = "reposyncs"
	}
	return schema.GroupVersionResource{
		Group:    model.ConfigSyncGroup,
		Version:  t.Version(),
		Resource: resource,
	}
}

// NewDynamicClient builds a dynamic client from a kubeconfig. When no
// kubeconfig is named and none can be loaded, it falls back to in-cluster
// configuration.
func NewDynamicClient(kubeconfig, kubeContext string) (dynamic.Interface, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		rules.ExplicitPath = kubeconfig
	}
	overrides := &clientcmd.ConfigOverrides{}
	if kubeContext != "" {
		overrides.CurrentContext = kubeContext
	}

	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
	if err != nil {
		if kubeconfig == "" {
			if inCluster, inErr := rest.InClusterConfig(); inErr == nil {
				cfg, err = inCluster, nil
			}
		}
		if err != nil {
			return nil, fmt.Errorf("load kubeconfig: %w", err)
		}
	}

	client, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes client: %w", err)
	}
	return client, nil
}
