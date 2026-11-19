package monitor

import (
	"fmt"
	"strings"
)

// Phase is the result of evaluating one snapshot of an object.
type Phase string

const (
	// PhasePending means the object has not converged yet; keep polling.
	PhasePending Phase = "pending"
	// PhaseSynced means the object is fully converged.
	PhaseSynced Phase = "synced"
	// PhaseError means the object reports an error. The waiter only treats
	// this as final when it persists across several polls, because Config Sync
	// retries and some errors clear on their own.
	PhaseError Phase = "error"
)

// Status is the evaluation of one snapshot.
type Status struct {
	Phase   Phase
	Message string
	Commit  string
}

func pendingf(format string, args ...interface{}) Status {
	return Status{Phase: PhasePending, Message: fmt.Sprintf(format, args...)}
}

func errorf(format string, args ...interface{}) Status {
	return Status{Phase: PhaseError, Message: fmt.Sprintf(format, args...)}
}

type condition struct {
	Type, Status, Reason, Message, Commit string
}

func readConditions(obj map[string]interface{}) map[string]condition {
	out := map[string]condition{}
	for _, it := range list(obj, "status", "conditions") {
		m := asMap(it)
		if m == nil {
			continue
		}
		c := condition{
			Type:    str(m, "type"),
			Status:  str(m, "status"),
			Reason:  str(m, "reason"),
			Message: str(m, "message"),
			Commit:  str(m, "commit"),
		}
		if c.Type != "" {
			out[c.Type] = c
		}
	}
	return out
}

// EvaluateSync decides whether a RootSync or RepoSync object (as returned by
// the API server) has fully converged. It is deliberately strict: anything it
// cannot positively confirm is reported as pending rather than synced.
//
// A sync is considered done only when all of the following hold:
//   - the controller has observed the latest spec generation
//   - no error is reported for the source, rendering or sync stage
//   - the Stalled condition is not True and Reconciling is not True
//   - the Syncing condition is present and False
//   - the source, rendering (when reported) and sync commits all agree with
//     status.lastSyncedCommit
//   - if expectedCommit is set, the synced commit matches it
func EvaluateSync(obj map[string]interface{}, expectedCommit string) Status {
	if str(obj, "metadata", "deletionTimestamp") != "" {
		return errorf("object is being deleted")
	}

	gen, _ := asInt(obj, "metadata", "generation")
	observed, _ := asInt(obj, "status", "observedGeneration")
	if observed < gen {
		return pendingf("controller has not observed generation %d yet (observed %d)", gen, observed)
	}

	conds := readConditions(obj)

	var errs []string
	for _, stage := range []string{"source", "rendering", "sync"} {
		for _, e := range list(obj, "status", stage, "errors") {
			m := asMap(e)
			if m == nil {
				continue
			}
			msg := oneLine(str(m, "errorMessage"), 300)
			if code := str(m, "code"); code != "" {
				msg = "[" + code + "] " + msg
			}
			errs = append(errs, stage+" error: "+msg)
		}
	}
	if c, ok := conds["Stalled"]; ok && c.Status == "True" {
		errs = append(errs, fmt.Sprintf("stalled (%s): %s", c.Reason, oneLine(c.Message, 300)))
	}
	if len(errs) > 0 {
		return errorf("%s", joinLimited(errs, 3))
	}

	if c, ok := conds["Reconciling"]; ok && c.Status == "True" {
		return pendingf("reconciler is being reconciled (%s)", c.Reason)
	}
	syncing, ok := conds["Syncing"]
	if !ok {
		return pendingf("Syncing condition not reported yet")
	}
	if syncing.Status != "False" {
		return pendingf("sync in progress: %s", oneLine(syncing.Message, 200))
	}

	src := str(obj, "status", "source", "commit")
	rnd := str(obj, "status", "rendering", "commit")
	snc := str(obj, "status", "sync", "commit")
	last := str(obj, "status", "lastSyncedCommit")

	if snc == "" {
		return pendingf("no commit has been synced yet")
	}
	if src != "" && src != snc {
		return pendingf("source commit %s is ahead of synced commit %s", shortCommit(src), shortCommit(snc))
	}
	if rnd != "" && rnd != snc {
		return pendingf("rendered commit %s differs from synced commit %s", shortCommit(rnd), shortCommit(snc))
	}
	if last != "" && last != snc {
		return pendingf("lastSyncedCommit %s differs from sync commit %s", shortCommit(last), shortCommit(snc))
	}
	if syncing.Commit != "" && syncing.Commit != snc {
		return pendingf("Syncing condition commit %s differs from sync commit %s", shortCommit(syncing.Commit), shortCommit(snc))
	}
	if expectedCommit != "" && !commitMatches(snc, expectedCommit) {
		return pendingf("synced commit is %s, waiting for %s", shortCommit(snc), shortCommit(expectedCommit))
	}

	return Status{Phase: PhaseSynced, Commit: snc, Message: "sync completed at commit " + shortCommit(snc)}
}

func joinLimited(items []string, limit int) string {
	if len(items) <= limit {
		return strings.Join(items, "; ")
	}
	return fmt.Sprintf("%s; and %d more", strings.Join(items[:limit], "; "), len(items)-limit)
}
