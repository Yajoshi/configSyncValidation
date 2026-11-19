package monitor

import (
	"strings"
	"testing"
)

func TestEvaluateResourceGroup(t *testing.T) {
	cur := func(name string) obj { return resource("Deployment", "ns", name, "Current") }

	tests := []struct {
		name       string
		rg         obj
		allowEmpty bool
		want       Phase
		contains   string
	}{
		{name: "all current", rg: resourceGroup(cur("a"), cur("b")), want: PhaseSynced, contains: "all 2"},
		{name: "one in progress", rg: resourceGroup(cur("a"), resource("Deployment", "ns", "b", "InProgress")),
			want: PhasePending, contains: "Deployment ns/b"},
		{name: "one failed", rg: resourceGroup(cur("a"), resource("Deployment", "ns", "b", "Failed")),
			want: PhaseError, contains: "status Failed"},
		{name: "one not found", rg: resourceGroup(resource("Deployment", "ns", "b", "NotFound")),
			want: PhaseError, contains: "NotFound"},
		{name: "actuation failed", rg: func() obj {
			r := cur("a")
			r["actuation"] = "Failed"
			return resourceGroup(r)
		}(), want: PhaseError, contains: "actuation Failed"},
		{name: "reconcile timeout", rg: func() obj {
			r := cur("a")
			r["reconcile"] = "Timeout"
			return resourceGroup(r)
		}(), want: PhaseError, contains: "reconcile Timeout"},
		{name: "reconcile pending", rg: func() obj {
			r := cur("a")
			r["reconcile"] = "Pending"
			return resourceGroup(r)
		}(), want: PhasePending},
		{name: "cluster-scoped resource name", rg: resourceGroup(resource("Namespace", "", "team-a", "Failed")),
			want: PhaseError, contains: "Namespace team-a:"},
		{name: "status missing for listed resources", rg: func() obj {
			r := resourceGroup()
			r["spec"] = obj{"resources": []interface{}{obj{"kind": "x"}}}
			return r
		}(), want: PhasePending, contains: "no status"},
		{name: "fewer statuses than resources", rg: func() obj {
			r := resourceGroup(cur("a"))
			r["spec"] = obj{"resources": []interface{}{obj{}, obj{}}}
			return r
		}(), want: PhasePending, contains: "1 of 2"},
		{name: "empty not allowed", rg: resourceGroup(), want: PhaseError, contains: "no resources"},
		{name: "empty allowed", rg: resourceGroup(), allowEmpty: true, want: PhaseSynced},
		{name: "stalled", rg: func() obj {
			r := resourceGroup(cur("a"))
			r["status"].(obj)["conditions"] = []interface{}{obj{"type": "Stalled", "status": "True", "reason": "Bad", "message": "boom"}}
			return r
		}(), want: PhaseError, contains: "stalled"},
		{name: "status behind generation", rg: func() obj {
			r := resourceGroup(cur("a"))
			r["status"].(obj)["observedGeneration"] = int64(1)
			return r
		}(), want: PhasePending, contains: "behind"},
		{name: "no observedGeneration field is tolerated", rg: func() obj {
			r := resourceGroup(cur("a"))
			delete(r["status"].(obj), "observedGeneration")
			return r
		}(), want: PhaseSynced},
		{name: "older schema without actuation fields", rg: func() obj {
			r := obj{"group": "", "kind": "ConfigMap", "namespace": "ns", "name": "c", "status": "Current"}
			return resourceGroup(r)
		}(), want: PhaseSynced},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := EvaluateResourceGroup(tc.rg, tc.allowEmpty)
			if got.Phase != tc.want {
				t.Fatalf("phase = %s (%q), want %s", got.Phase, got.Message, tc.want)
			}
			if !strings.Contains(got.Message, tc.contains) {
				t.Fatalf("message %q does not contain %q", got.Message, tc.contains)
			}
		})
	}
}
