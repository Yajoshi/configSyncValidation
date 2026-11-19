package monitor

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"configsync-validator/model"
)

var target = model.SyncTarget{Kind: model.KindRootSync, Name: "root-sync", Namespace: model.RootSyncNamespace}

func fast(o Options) Options {
	if o.PollInterval == 0 {
		o.PollInterval = 2 * time.Millisecond
	}
	if o.Timeout == 0 {
		o.Timeout = 2 * time.Second
	}
	if o.ErrorThreshold == 0 {
		o.ErrorThreshold = 3
	}
	return o
}

func goodRG() obj { return resourceGroup(resource("Deployment", "ns", "a", "Current")) }

func TestWaitSuccess(t *testing.T) {
	g := &fakeGetter{}
	g.set(syncedRootSync("abc123"), goodRG())
	res := (&Monitor{Source: g}).WaitAndValidate(context.Background(), target, fast(Options{}))
	if res.Outcome != model.Success {
		t.Fatalf("outcome = %s (%s)", res.Outcome, res.Message)
	}
	if res.Commit != "abc123" {
		t.Fatalf("commit = %q", res.Commit)
	}
}

func TestWaitSuccessAfterConverging(t *testing.T) {
	g := &fakeGetter{}
	pending := syncedRootSync("abc123")
	setCond(pending, "Syncing", "True", "Syncing")
	g.set(pending, nil)

	go func() {
		time.Sleep(20 * time.Millisecond)
		g.set(syncedRootSync("abc123"), nil) // sync done, ResourceGroup not there yet
		time.Sleep(20 * time.Millisecond)
		g.set(syncedRootSync("abc123"), goodRG())
	}()

	res := (&Monitor{Source: g}).WaitAndValidate(context.Background(), target, fast(Options{}))
	if res.Outcome != model.Success {
		t.Fatalf("outcome = %s (%s)", res.Outcome, res.Message)
	}
}

func TestWaitWaitsForObjectToExist(t *testing.T) {
	g := &fakeGetter{}
	go func() {
		time.Sleep(20 * time.Millisecond)
		g.set(syncedRootSync("abc"), goodRG())
	}()
	res := (&Monitor{Source: g}).WaitAndValidate(context.Background(), target, fast(Options{}))
	if res.Outcome != model.Success {
		t.Fatalf("outcome = %s (%s)", res.Outcome, res.Message)
	}
}

func TestWaitFailsOnPersistentError(t *testing.T) {
	g := &fakeGetter{}
	bad := syncedRootSync("abc")
	bad["status"].(obj)["source"] = obj{"errors": []interface{}{obj{"errorMessage": "auth failed"}}}
	g.set(bad, goodRG())

	res := (&Monitor{Source: g}).WaitAndValidate(context.Background(), target, fast(Options{ErrorThreshold: 2}))
	if res.Outcome != model.Failed {
		t.Fatalf("outcome = %s (%s)", res.Outcome, res.Message)
	}
	if !strings.Contains(res.Message, "auth failed") {
		t.Fatalf("message %q should carry the error", res.Message)
	}
}

func TestWaitToleratesTransientError(t *testing.T) {
	g := &fakeGetter{}
	bad := syncedRootSync("abc")
	bad["status"].(obj)["source"] = obj{"errors": []interface{}{obj{"errorMessage": "temporary"}}}
	g.set(bad, goodRG())
	go func() {
		time.Sleep(5 * time.Millisecond) // shorter than ErrorThreshold polls below
		g.set(syncedRootSync("abc"), goodRG())
	}()

	res := (&Monitor{Source: g}).WaitAndValidate(context.Background(), target,
		fast(Options{PollInterval: 4 * time.Millisecond, ErrorThreshold: 50}))
	if res.Outcome != model.Success {
		t.Fatalf("outcome = %s (%s)", res.Outcome, res.Message)
	}
}

func TestWaitTimeout(t *testing.T) {
	g := &fakeGetter{}
	p := syncedRootSync("abc")
	setCond(p, "Syncing", "True", "still going")
	g.set(p, nil)

	res := (&Monitor{Source: g}).WaitAndValidate(context.Background(), target, fast(Options{Timeout: 60 * time.Millisecond}))
	if res.Outcome != model.Timeout {
		t.Fatalf("outcome = %s (%s)", res.Outcome, res.Message)
	}
	if !strings.Contains(res.Message, "still going") {
		t.Fatalf("message %q should include the last state", res.Message)
	}
}

func TestWaitTimeoutWhenResourceGroupNeverAppears(t *testing.T) {
	g := &fakeGetter{}
	g.set(syncedRootSync("abc"), nil)
	res := (&Monitor{Source: g}).WaitAndValidate(context.Background(), target, fast(Options{Timeout: 60 * time.Millisecond}))
	if res.Outcome != model.Timeout || !strings.Contains(res.Message, "sync is done") {
		t.Fatalf("outcome = %s (%s)", res.Outcome, res.Message)
	}
}

func TestWaitSkipResourceGroup(t *testing.T) {
	g := &fakeGetter{}
	g.set(syncedRootSync("abc"), nil)
	res := (&Monitor{Source: g}).WaitAndValidate(context.Background(), target, fast(Options{SkipResourceGroup: true}))
	if res.Outcome != model.Success || !strings.Contains(res.Message, "skipped") {
		t.Fatalf("outcome = %s (%s)", res.Outcome, res.Message)
	}
}

func TestWaitFailsWhenManagedResourceFails(t *testing.T) {
	g := &fakeGetter{}
	g.set(syncedRootSync("abc"), resourceGroup(resource("Deployment", "ns", "a", "Failed")))
	res := (&Monitor{Source: g}).WaitAndValidate(context.Background(), target, fast(Options{ErrorThreshold: 2}))
	if res.Outcome != model.Failed || !strings.Contains(res.Message, "Deployment ns/a") {
		t.Fatalf("outcome = %s (%s)", res.Outcome, res.Message)
	}
}

func TestWaitFailsOnPersistentAPIError(t *testing.T) {
	g := &fakeGetter{syncErr: errors.New("connection refused")}
	res := (&Monitor{Source: g}).WaitAndValidate(context.Background(), target, fast(Options{}))
	if res.Outcome != model.Failed || !strings.Contains(res.Message, "connection refused") {
		t.Fatalf("outcome = %s (%s)", res.Outcome, res.Message)
	}
}

func TestWaitExpectedCommit(t *testing.T) {
	g := &fakeGetter{}
	g.set(syncedRootSync("0000000000000000"), goodRG())
	tg := target
	tg.ExpectedCommit = "ffff"
	res := (&Monitor{Source: g}).WaitAndValidate(context.Background(), tg, fast(Options{Timeout: 60 * time.Millisecond}))
	if res.Outcome != model.Timeout {
		t.Fatalf("outcome = %s (%s)", res.Outcome, res.Message)
	}
}

func TestWaitInterrupted(t *testing.T) {
	g := &fakeGetter{}
	p := syncedRootSync("abc")
	setCond(p, "Syncing", "True", "busy")
	g.set(p, nil)

	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	res := (&Monitor{Source: g}).WaitAndValidate(ctx, target, fast(Options{}))
	if res.Outcome != model.Failed || !strings.Contains(res.Message, "interrupted") {
		t.Fatalf("outcome = %s (%s)", res.Outcome, res.Message)
	}
}
