// Package model holds the small value types shared by every other package.
// It has no dependencies outside the standard library.
package model

import (
	"fmt"
	"time"
)

// Outcome is the final verdict for one sync object or for the whole run.
type Outcome string

const (
	Success Outcome = "SUCCESS"
	Failed  Outcome = "FAILED"
	Timeout Outcome = "TIMEOUT"
	Skipped Outcome = "SKIPPED"
)

const (
	KindRootSync = "RootSync"
	KindRepoSync = "RepoSync"

	// ConfigSyncGroup is the API group of RootSync and RepoSync.
	ConfigSyncGroup = "configsync.gke.io"
	// DefaultAPIVersion is the version used when none is given. Older Config
	// Sync releases only serve v1alpha1.
	DefaultAPIVersion = "v1beta1"
	// RootSyncNamespace is the only namespace a RootSync may live in.
	RootSyncNamespace = "config-management-system"
)

// SyncTarget identifies one RootSync or RepoSync object.
type SyncTarget struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	// APIVersion is the version part only, for example "v1beta1".
	APIVersion string `json:"apiVersion,omitempty"`
	// ResourceGroupName overrides the ResourceGroup looked up for validation.
	// When empty the ResourceGroup is assumed to have the same name as the sync.
	ResourceGroupName string `json:"resourceGroupName,omitempty"`
	// ExpectedCommit, when set, makes the validator wait until the sync has
	// applied this commit (full hash or unambiguous prefix).
	ExpectedCommit string `json:"expectedCommit,omitempty"`
}

func (t SyncTarget) String() string {
	return fmt.Sprintf("%s %s/%s", t.Kind, t.Namespace, t.Name)
}

// Version returns the API version, falling back to the default.
func (t SyncTarget) Version() string {
	if t.APIVersion == "" {
		return DefaultAPIVersion
	}
	return t.APIVersion
}

// RGName returns the name of the ResourceGroup that tracks this sync.
func (t SyncTarget) RGName() string {
	if t.ResourceGroupName != "" {
		return t.ResourceGroupName
	}
	return t.Name
}

// Result is the verdict for one sync object.
type Result struct {
	Target  SyncTarget `json:"target"`
	Outcome Outcome    `json:"outcome"`
	Message string     `json:"message"`
	Commit  string     `json:"commit,omitempty"`
	Seconds float64    `json:"seconds"`
}

// Elapsed returns the time spent on this object.
func (r Result) Elapsed() time.Duration {
	return time.Duration(r.Seconds * float64(time.Second))
}
