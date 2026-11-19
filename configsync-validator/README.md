# configsync-validator

Applies a Config Sync RootSync, waits for it to reconcile, validates the
resources it manages, and only then does the same for a RepoSync. It finishes
with one clear verdict: SUCCESS, FAILED or TIMEOUT.

It can also be used purely to answer "did my RootSync sync successfully?"
without applying anything (status-only mode).

## Build

You need Go 1.23 or newer and network access to fetch modules.

    go mod tidy        # downloads dependencies and writes go.sum
    go build -o configsync-validator .
    go test ./...

## Usage

Apply RootSync, wait, validate, then apply RepoSync, wait, validate:

    configsync-validator -root-manifest examples/rootsync.yaml \
                         -repo-manifest examples/reposync.yaml

Or from a config file (flags override file values):

    configsync-validator -config examples/config.yaml

Only check the status of an existing RootSync (nothing is applied):

    configsync-validator -root-name root-sync -timeout 30s

Check an existing RepoSync too:

    configsync-validator -root-name root-sync \
                         -repo-name repo-sync -repo-namespace team-a

Useful options:

    -kubeconfig, -context      which cluster to talk to
    -timeout 10m               per sync object
    -poll-interval 5s
    -error-threshold 3         consecutive polls with an error before FAILED
    -root-commit / -repo-commit   wait until this commit has been synced
    -root-resourcegroup / -repo-resourcegroup   ResourceGroup name override
    -skip-resourcegroup        skip ResourceGroup validation
    -allow-empty-resourcegroup accept a sync that manages zero resources
    -output json               machine-readable report

Progress is logged to stderr; the final report goes to stdout.

## Workflow

1. Preflight: load and check both manifests. Nothing is applied if either is
   invalid (wrong kind, missing namespace, and so on).
2. Apply the RootSync with server-side apply.
3. Poll until the RootSync has converged and its ResourceGroup shows every
   managed resource as Current.
4. If, and only if, the RootSync succeeded: apply the RepoSync and repeat.
   Otherwise the RepoSync is reported as SKIPPED.
5. Print the report.

## What "synced" means

A RootSync or RepoSync counts as converged only when all of these hold:

- the controller has observed the latest spec generation
- no errors are reported under status.source, status.rendering or status.sync
- the Stalled condition is not True and Reconciling is not True
- the Syncing condition is present and False
- source, rendering (if reported) and sync commits agree with
  status.lastSyncedCommit
- if an expected commit was given, that commit is the synced one

Then the ResourceGroup (kpt.dev/v1alpha1) with the same name in the same
namespace must list every managed resource with status Current, with no failed
actuation or reconcile.

Anything that cannot be positively confirmed is treated as "not yet", never as
success. Errors only turn into FAILED when they persist for -error-threshold
consecutive polls, because Config Sync retries and many errors clear by
themselves.

## Exit codes

    0  SUCCESS
    1  FAILED
    2  TIMEOUT
    3  usage, configuration or connection problem

## Layout

    main.go        command line entry point
    config/        configuration, flags, plan building
    model/         shared types
    monitor/       status evaluation and waiting (no Kubernetes imports)
    syncer/        the RootSync then RepoSync workflow and report
    kube/          everything that talks to Kubernetes or reads YAML
    examples/      sample manifests and configuration

Only kube/ and main.go import Kubernetes or YAML libraries. The evaluation
and workflow logic is plain Go and is covered by unit tests that use an
in-memory fake cluster.

## Things to verify in your environment

- This code was unit tested for its evaluation and workflow logic, but the
  kube/ package (server-side apply, reading objects, ResourceGroup lookup) has
  not been run against a real cluster. Try status-only mode first.
- The ResourceGroup is looked up by the same name as the sync. If your Config
  Sync version names it differently, the error message lists the ResourceGroups
  found in that namespace; set resourceGroupName accordingly.
- Older Config Sync releases only serve configsync.gke.io/v1alpha1. Use
  apiVersion v1alpha1 in your manifests, or -api-version v1alpha1 for
  status-only checks.
- The user running this needs permission to get, create and patch rootsyncs
  and reposyncs, and to get and list resourcegroups.
- Applying a manifest whose fields are owned by another manager can produce a
  server-side apply conflict. Use -force-conflicts only if you intend to take
  ownership.

## Equivalent manual checks

    nomos status
    kubectl get rootsync root-sync -n config-management-system -o yaml
    kubectl get resourcegroup root-sync -n config-management-system -o yaml
