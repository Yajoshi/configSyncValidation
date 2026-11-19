// Command configsync-validator applies a Config Sync RootSync and then a
// RepoSync, waits for each to reconcile, validates the resources they manage,
// and reports SUCCESS, FAILED or TIMEOUT.
//
// Exit codes: 0 success, 1 failed, 2 timeout, 3 usage or setup error.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"configsync-validator/config"
	"configsync-validator/kube"
	"configsync-validator/monitor"
	"configsync-validator/syncer"
)

func main() {
	os.Exit(run())
}

func run() int {
	fs := flag.NewFlagSet("configsync-validator", flag.ContinueOnError)
	flags := config.RegisterFlags(fs)
	if err := fs.Parse(os.Args[1:]); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 3
	}

	cfg := config.Default()
	if flags.ConfigPath != "" {
		loaded, err := kube.LoadConfigFile(flags.ConfigPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 3
		}
		cfg = loaded
	}
	flags.Apply(&cfg)

	plan, err := cfg.Plan()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		fs.Usage()
		return 3
	}
	if flags.Output != "text" && flags.Output != "json" {
		fmt.Fprintf(os.Stderr, "error: -output must be text or json, not %q\n", flags.Output)
		return 3
	}

	client, err := kube.NewDynamicClient(cfg.Kubeconfig, cfg.Context)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 3
	}

	logf := func(format string, args ...interface{}) {
		fmt.Fprintf(os.Stderr, "%s  %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
	}

	wf := &syncer.Workflow{
		Applier: &kube.Applier{Client: client, FieldManager: cfg.FieldManager, Force: cfg.ForceConflicts},
		Monitor: &monitor.Monitor{Source: &kube.Getter{Client: client}},
		Options: monitor.Options{
			PollInterval:            cfg.PollInterval.Duration,
			Timeout:                 cfg.Timeout.Duration,
			ErrorThreshold:          cfg.ErrorThreshold,
			SkipResourceGroup:       cfg.SkipResourceGroupValidation,
			AllowEmptyResourceGroup: cfg.AllowEmptyResourceGroup,
		},
		Logf: logf,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	report, err := wf.Run(ctx, plan)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 3
	}

	if flags.Output == "json" {
		out, err := report.JSON()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 3
		}
		fmt.Print(out)
	} else {
		fmt.Print(report.Text())
	}
	return report.ExitCode()
}
