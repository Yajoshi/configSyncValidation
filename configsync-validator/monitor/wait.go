package monitor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"configsync-validator/model"
)

// ErrNotFound must be wrapped (errors.Is) by a Getter when the requested object
// does not exist.
var ErrNotFound = errors.New("not found")

// Getter reads objects from the cluster. The real implementation lives in the
// kube package; tests use an in-memory fake.
type Getter interface {
	// GetSync returns the RootSync or RepoSync object.
	GetSync(ctx context.Context, t model.SyncTarget) (map[string]interface{}, error)
	// GetResourceGroup returns the ResourceGroup that tracks the sync.
	GetResourceGroup(ctx context.Context, t model.SyncTarget) (map[string]interface{}, error)
}

// Monitor waits for a sync to converge and validates what it manages.
type Monitor struct {
	Source Getter
}

// Options tunes WaitAndValidate.
type Options struct {
	PollInterval time.Duration
	// Timeout bounds the whole wait (sync convergence plus ResourceGroup
	// validation) for one object.
	Timeout time.Duration
	// ErrorThreshold is how many consecutive polls must report an error before
	// the object is declared FAILED.
	ErrorThreshold          int
	SkipResourceGroup       bool
	AllowEmptyResourceGroup bool
	Logf                    func(format string, args ...interface{})
}

func (o Options) withDefaults() Options {
	if o.PollInterval <= 0 {
		o.PollInterval = 5 * time.Second
	}
	if o.Timeout <= 0 {
		o.Timeout = 10 * time.Minute
	}
	if o.ErrorThreshold <= 0 {
		o.ErrorThreshold = 3
	}
	return o
}

const maxConsecutiveAPIErrors = 5

type state struct {
	target                    model.SyncTarget
	opts                      Options
	syncErrs, rgErrs, apiErrs int
	last, logged, commit      string
}

func (s *state) note(format string, args ...interface{}) {
	s.last = fmt.Sprintf(format, args...)
	if s.last != s.logged {
		s.logged = s.last
		if s.opts.Logf != nil {
			s.opts.Logf("%s: %s", s.target, s.last)
		}
	}
}

// WaitAndValidate polls until the sync has converged and (unless skipped) its
// ResourceGroup reports every managed resource as Current. It returns SUCCESS,
// FAILED (persistent error, or the context was cancelled) or TIMEOUT.
func (m *Monitor) WaitAndValidate(ctx context.Context, target model.SyncTarget, opts Options) model.Result {
	opts = opts.withDefaults()
	start := time.Now()
	st := &state{target: target, opts: opts}

	finish := func(o model.Outcome, msg string) model.Result {
		return model.Result{
			Target:  target,
			Outcome: o,
			Message: msg,
			Commit:  st.commit,
			Seconds: time.Since(start).Seconds(),
		}
	}

	tctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	ticker := time.NewTicker(opts.PollInterval)
	defer ticker.Stop()

	for {
		if done, outcome, msg := m.poll(tctx, st); done {
			return finish(outcome, msg)
		}
		select {
		case <-tctx.Done():
			if ctx.Err() != nil {
				return finish(model.Failed, "interrupted: "+ctx.Err().Error()+"; last state: "+st.last)
			}
			return finish(model.Timeout, fmt.Sprintf("timed out after %s; last state: %s", opts.Timeout, st.last))
		case <-ticker.C:
		}
	}
}

// poll performs one evaluation. It returns done=true when a final verdict is
// known.
func (m *Monitor) poll(ctx context.Context, st *state) (done bool, outcome model.Outcome, msg string) {
	t, opts := st.target, st.opts

	obj, err := m.Source.GetSync(ctx, t)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			st.apiErrs = 0
			st.note("%s not found yet", t)
			return false, "", ""
		}
		st.apiErrs++
		st.note("error reading %s: %v", t, err)
		if st.apiErrs >= maxConsecutiveAPIErrors && ctx.Err() == nil {
			return true, model.Failed, fmt.Sprintf("cannot read %s: %v", t, err)
		}
		return false, "", ""
	}
	st.apiErrs = 0

	s := EvaluateSync(obj, t.ExpectedCommit)
	switch s.Phase {
	case PhaseError:
		st.syncErrs++
		st.note("%s", s.Message)
		if st.syncErrs >= opts.ErrorThreshold {
			return true, model.Failed, s.Message
		}
		return false, "", ""
	case PhasePending:
		st.syncErrs = 0
		st.note("%s", s.Message)
		return false, "", ""
	}
	st.syncErrs = 0
	st.commit = s.Commit

	if opts.SkipResourceGroup {
		return true, model.Success, s.Message + " (ResourceGroup validation skipped)"
	}

	rg, err := m.Source.GetResourceGroup(ctx, t)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			st.rgErrs = 0
			st.note("sync is done but %v", err)
			return false, "", ""
		}
		st.apiErrs++
		st.note("error reading ResourceGroup: %v", err)
		if st.apiErrs >= maxConsecutiveAPIErrors && ctx.Err() == nil {
			return true, model.Failed, fmt.Sprintf("cannot read ResourceGroup: %v", err)
		}
		return false, "", ""
	}

	r := EvaluateResourceGroup(rg, opts.AllowEmptyResourceGroup)
	switch r.Phase {
	case PhaseError:
		st.rgErrs++
		st.note("%s", r.Message)
		if st.rgErrs >= opts.ErrorThreshold {
			return true, model.Failed, r.Message
		}
		return false, "", ""
	case PhasePending:
		st.rgErrs = 0
		st.note("sync is done; %s", r.Message)
		return false, "", ""
	}
	return true, model.Success, s.Message + "; " + r.Message
}
