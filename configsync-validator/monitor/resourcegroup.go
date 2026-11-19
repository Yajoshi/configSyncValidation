package monitor

import "fmt"

// EvaluateResourceGroup checks the ResourceGroup (kpt.dev/v1alpha1) that Config
// Sync maintains for each sync. Every resource the sync manages is listed in
// status.resourceStatuses with a kstatus value; all of them must be Current.
//
// The ResourceGroup schema differs a little between Config Sync versions, so
// fields that are not present (actuation, reconcile, observedGeneration) are
// simply not enforced, while the core requirement (every listed resource is
// Current) always is.
func EvaluateResourceGroup(obj map[string]interface{}, allowEmpty bool) Status {
	gen, _ := asInt(obj, "metadata", "generation")
	if observed, ok := asInt(obj, "status", "observedGeneration"); ok && observed < gen {
		return pendingf("ResourceGroup status is behind (generation %d, observed %d)", gen, observed)
	}

	conds := readConditions(obj)
	if c, ok := conds["Stalled"]; ok && c.Status == "True" {
		return errorf("ResourceGroup stalled (%s): %s", c.Reason, oneLine(c.Message, 300))
	}

	specCount := len(list(obj, "spec", "resources"))
	statuses := list(obj, "status", "resourceStatuses")

	if len(statuses) == 0 {
		if specCount == 0 {
			if allowEmpty {
				return Status{Phase: PhaseSynced, Message: "ResourceGroup tracks no resources (allowed)"}
			}
			return errorf("ResourceGroup tracks no resources (use allowEmptyResourceGroup if this is intended)")
		}
		return pendingf("ResourceGroup lists %d resources but has no status for them yet", specCount)
	}

	var failed, waiting []string
	current := 0
	for _, it := range statuses {
		m := asMap(it)
		if m == nil {
			continue
		}
		id := fmt.Sprintf("%s %s", str(m, "kind"), resourceName(m))
		status := str(m, "status")
		actuation := str(m, "actuation")
		reconcile := str(m, "reconcile")

		switch {
		case status == "Failed" || status == "NotFound":
			failed = append(failed, fmt.Sprintf("%s: status %s%s", id, status, conditionDetail(m)))
		case actuation == "Failed":
			failed = append(failed, fmt.Sprintf("%s: actuation Failed%s", id, conditionDetail(m)))
		case reconcile == "Failed" || reconcile == "Timeout":
			failed = append(failed, fmt.Sprintf("%s: reconcile %s%s", id, reconcile, conditionDetail(m)))
		case status != "Current" || actuation == "Pending" || reconcile == "Pending":
			state := status
			if state == "" {
				state = "no status"
			}
			waiting = append(waiting, fmt.Sprintf("%s (%s)", id, state))
		default:
			current++
		}
	}

	if len(failed) > 0 {
		return errorf("%s", joinLimited(failed, 5))
	}
	if len(waiting) > 0 {
		return pendingf("%d of %d resources current; waiting on %s", current, len(statuses), joinLimited(waiting, 3))
	}
	if specCount > len(statuses) {
		return pendingf("only %d of %d resources have a status yet", len(statuses), specCount)
	}
	return Status{Phase: PhaseSynced, Message: fmt.Sprintf("all %d managed resources are Current", current)}
}

func resourceName(m map[string]interface{}) string {
	ns, name := str(m, "namespace"), str(m, "name")
	if ns == "" {
		return name
	}
	return ns + "/" + name
}

func conditionDetail(m map[string]interface{}) string {
	for _, it := range list(m, "conditions") {
		c := asMap(it)
		if c == nil {
			continue
		}
		if msg := str(c, "message"); msg != "" {
			return " (" + oneLine(msg, 200) + ")"
		}
	}
	return ""
}
