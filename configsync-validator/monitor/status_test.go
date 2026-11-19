package monitor

import (
	"strings"
	"testing"
)

func TestEvaluateSync(t *testing.T) {
	const c1 = "0123456789abcdef0123456789abcdef01234567"
	const c2 = "fedcba9876543210fedcba9876543210fedcba98"

	tests := []struct {
		name     string
		mutate   func(o obj)
		expected string
		want     Phase
		contains string
	}{
		{name: "converged", want: PhaseSynced, contains: "0123456789ab"},
		{name: "rendering skipped", want: PhaseSynced,
			mutate: func(o obj) { o["status"].(obj)["rendering"] = obj{} }},
		{name: "spec generation not observed yet", want: PhasePending, contains: "not observed",
			mutate: func(o obj) { o["metadata"].(obj)["generation"] = int64(3) }},
		{name: "source error", want: PhaseError, contains: "source error",
			mutate: func(o obj) {
				o["status"].(obj)["source"] = obj{"commit": c1, "errors": []interface{}{
					obj{"code": "2004", "errorMessage": "git fetch failed:\n authentication required"}}}
			}},
		{name: "rendering error", want: PhaseError, contains: "rendering error",
			mutate: func(o obj) {
				o["status"].(obj)["rendering"] = obj{"errors": []interface{}{obj{"errorMessage": "kustomize build failed"}}}
			}},
		{name: "sync error", want: PhaseError, contains: "sync error",
			mutate: func(o obj) {
				o["status"].(obj)["sync"] = obj{"commit": c1, "errors": []interface{}{obj{"errorMessage": "apply failed"}}}
			}},
		{name: "stalled", want: PhaseError, contains: "stalled",
			mutate: func(o obj) { setCond(o, "Stalled", "True", "deployment failed") }},
		{name: "reconciler still reconciling", want: PhasePending, contains: "reconciler",
			mutate: func(o obj) { setCond(o, "Reconciling", "True", "") }},
		{name: "syncing in progress", want: PhasePending, contains: "in progress",
			mutate: func(o obj) { setCond(o, "Syncing", "True", "Syncing") }},
		{name: "no syncing condition", want: PhasePending, contains: "Syncing condition",
			mutate: func(o obj) { removeCond(o, "Syncing") }},
		{name: "no sync commit", want: PhasePending, contains: "no commit",
			mutate: func(o obj) { o["status"].(obj)["sync"] = obj{} }},
		{name: "source ahead of sync", want: PhasePending, contains: "ahead",
			mutate: func(o obj) { o["status"].(obj)["source"] = obj{"commit": c2} }},
		{name: "rendered differs", want: PhasePending, contains: "rendered",
			mutate: func(o obj) { o["status"].(obj)["rendering"] = obj{"commit": c2} }},
		{name: "last synced differs", want: PhasePending, contains: "lastSyncedCommit",
			mutate: func(o obj) { o["status"].(obj)["lastSyncedCommit"] = c2 }},
		{name: "expected commit reached (prefix)", want: PhaseSynced, expected: c1[:8]},
		{name: "expected commit not reached", want: PhasePending, contains: "waiting for", expected: c2},
		{name: "being deleted", want: PhaseError, contains: "deleted",
			mutate: func(o obj) { o["metadata"].(obj)["deletionTimestamp"] = "2026-11-18T00:00:00Z" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o := syncedRootSync(c1)
			if tc.mutate != nil {
				tc.mutate(o)
			}
			got := EvaluateSync(o, tc.expected)
			if got.Phase != tc.want {
				t.Fatalf("phase = %s (%q), want %s", got.Phase, got.Message, tc.want)
			}
			if !strings.Contains(got.Message, tc.contains) {
				t.Fatalf("message %q does not contain %q", got.Message, tc.contains)
			}
			if got.Phase == PhaseSynced && got.Commit != c1 {
				t.Fatalf("commit = %q, want %q", got.Commit, c1)
			}
		})
	}
}

func TestEvaluateSyncMessageIsSingleLine(t *testing.T) {
	o := syncedRootSync("abc")
	o["status"].(obj)["source"] = obj{"errors": []interface{}{obj{"errorMessage": "line one\nline two\n\n  line three"}}}
	got := EvaluateSync(o, "")
	if strings.Contains(got.Message, "\n") {
		t.Fatalf("message should be one line, got %q", got.Message)
	}
}
