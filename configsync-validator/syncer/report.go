package syncer

import (
	"encoding/json"
	"fmt"
	"strings"

	"configsync-validator/model"
)

// Report is the outcome of a whole run.
type Report struct {
	Overall model.Outcome  `json:"overall"`
	Results []model.Result `json:"results"`
}

// ExitCode maps the overall outcome to a process exit code:
// 0 success, 1 failed, 2 timeout.
func (r Report) ExitCode() int {
	switch r.Overall {
	case model.Success:
		return 0
	case model.Timeout:
		return 2
	default:
		return 1
	}
}

// Text renders the report as plain text.
func (r Report) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "RESULT: %s\n", r.Overall)
	for _, res := range r.Results {
		fmt.Fprintf(&b, "\n%s\n", res.Target)
		fmt.Fprintf(&b, "  outcome: %s\n", res.Outcome)
		if res.Commit != "" {
			fmt.Fprintf(&b, "  commit:  %s\n", res.Commit)
		}
		if res.Outcome != model.Skipped {
			fmt.Fprintf(&b, "  elapsed: %.1fs\n", res.Seconds)
		}
		fmt.Fprintf(&b, "  detail:  %s\n", res.Message)
	}
	return b.String()
}

// JSON renders the report as indented JSON.
func (r Report) JSON() (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data) + "\n", nil
}

func overall(results []model.Result) model.Outcome {
	out := model.Success
	for _, r := range results {
		switch r.Outcome {
		case model.Failed:
			return model.Failed
		case model.Timeout:
			out = model.Timeout
		}
	}
	return out
}
