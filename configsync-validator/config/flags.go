package config

import (
	"flag"
	"time"
)

// Flags are the command-line options. Only flags the user actually passed
// override the configuration file or the defaults.
type Flags struct {
	ConfigPath string
	Output     string

	fs *flag.FlagSet

	kubeconfig, kubeContext, fieldManager, apiVersion string
	pollInterval, timeout                             time.Duration
	errorThreshold                                    int
	force, skipRG, allowEmpty                         bool

	rootManifest, rootName, rootRG, rootCommit             string
	repoManifest, repoName, repoNamespace, repoRG, repoCom string
}

// RegisterFlags defines every option on fs.
func RegisterFlags(fs *flag.FlagSet) *Flags {
	f := &Flags{fs: fs}
	fs.StringVar(&f.ConfigPath, "config", "", "path to a YAML configuration file")
	fs.StringVar(&f.Output, "output", "text", "final report format: text or json")

	fs.StringVar(&f.kubeconfig, "kubeconfig", "", "path to kubeconfig (default: KUBECONFIG or ~/.kube/config, then in-cluster)")
	fs.StringVar(&f.kubeContext, "context", "", "kubeconfig context to use")
	fs.DurationVar(&f.pollInterval, "poll-interval", 0, "how often to poll status (default 5s)")
	fs.DurationVar(&f.timeout, "timeout", 0, "maximum wait per sync object (default 10m)")
	fs.IntVar(&f.errorThreshold, "error-threshold", 0, "consecutive polls with an error before declaring FAILED (default 3)")
	fs.StringVar(&f.fieldManager, "field-manager", "", "server-side apply field manager (default configsync-validator)")
	fs.BoolVar(&f.force, "force-conflicts", false, "force server-side apply conflicts")
	fs.BoolVar(&f.skipRG, "skip-resourcegroup", false, "do not validate ResourceGroup status")
	fs.BoolVar(&f.allowEmpty, "allow-empty-resourcegroup", false, "accept a ResourceGroup that tracks no resources")
	fs.StringVar(&f.apiVersion, "api-version", "", "configsync.gke.io API version for status-only checks (default v1beta1)")

	fs.StringVar(&f.rootManifest, "root-manifest", "", "RootSync manifest to apply, then validate")
	fs.StringVar(&f.rootName, "root-name", "", "RootSync name (status-only check if no manifest is given)")
	fs.StringVar(&f.rootRG, "root-resourcegroup", "", "ResourceGroup name for the RootSync (default: same as the RootSync)")
	fs.StringVar(&f.rootCommit, "root-commit", "", "wait until the RootSync has synced this commit")

	fs.StringVar(&f.repoManifest, "repo-manifest", "", "RepoSync manifest to apply after the RootSync succeeds")
	fs.StringVar(&f.repoName, "repo-name", "", "RepoSync name (status-only check if no manifest is given)")
	fs.StringVar(&f.repoNamespace, "repo-namespace", "", "RepoSync namespace (status-only check)")
	fs.StringVar(&f.repoRG, "repo-resourcegroup", "", "ResourceGroup name for the RepoSync (default: same as the RepoSync)")
	fs.StringVar(&f.repoCom, "repo-commit", "", "wait until the RepoSync has synced this commit")
	return f
}

// Apply copies every flag that was explicitly set onto cfg.
func (f *Flags) Apply(cfg *Config) {
	f.fs.Visit(func(fl *flag.Flag) {
		switch fl.Name {
		case "kubeconfig":
			cfg.Kubeconfig = f.kubeconfig
		case "context":
			cfg.Context = f.kubeContext
		case "poll-interval":
			cfg.PollInterval = Duration{f.pollInterval}
		case "timeout":
			cfg.Timeout = Duration{f.timeout}
		case "error-threshold":
			cfg.ErrorThreshold = f.errorThreshold
		case "field-manager":
			cfg.FieldManager = f.fieldManager
		case "force-conflicts":
			cfg.ForceConflicts = f.force
		case "skip-resourcegroup":
			cfg.SkipResourceGroupValidation = f.skipRG
		case "allow-empty-resourcegroup":
			cfg.AllowEmptyResourceGroup = f.allowEmpty
		case "api-version":
			cfg.RootSync.APIVersion = f.apiVersion
			cfg.RepoSync.APIVersion = f.apiVersion
		case "root-manifest":
			cfg.RootSync.Manifest = f.rootManifest
		case "root-name":
			cfg.RootSync.Name = f.rootName
		case "root-resourcegroup":
			cfg.RootSync.ResourceGroupName = f.rootRG
		case "root-commit":
			cfg.RootSync.ExpectedCommit = f.rootCommit
		case "repo-manifest":
			cfg.RepoSync.Manifest = f.repoManifest
		case "repo-name":
			cfg.RepoSync.Name = f.repoName
		case "repo-namespace":
			cfg.RepoSync.Namespace = f.repoNamespace
		case "repo-resourcegroup":
			cfg.RepoSync.ResourceGroupName = f.repoRG
		case "repo-commit":
			cfg.RepoSync.ExpectedCommit = f.repoCom
		}
	})
}
