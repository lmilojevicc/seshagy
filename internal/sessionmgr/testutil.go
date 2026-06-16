// Package sessionmgr test helpers in this file are exported for cross-package
// CLI tests (cmd/seshagy) and sessionmgr tests. They override tmux hooks, agent
// tracking time, manifest auto-update intervals, and pane-lock behavior for the
// duration of each test via t.Cleanup.
package sessionmgr

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

const fakePaneVisibilityActive = "1 1 1"

// FakeTmux is an in-memory stand-in for a tmux server's pane options.
type FakeTmux struct {
	mu   sync.Mutex
	opts map[string]map[string]string
}

// NewFakeTmux returns an empty pane-option store for tests.
func NewFakeTmux() *FakeTmux {
	return &FakeTmux{opts: map[string]map[string]string{}}
}

func (f *FakeTmux) Get(pane, opt string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if m, ok := f.opts[pane]; ok {
		return m[opt]
	}
	return ""
}

func (f *FakeTmux) Set(pane, opt, value string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.opts[pane] == nil {
		f.opts[pane] = map[string]string{}
	}
	f.opts[pane][opt] = value
}

func (f *FakeTmux) output(_ context.Context, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(args) >= 4 && args[0] == "show-option" {
		if m, ok := f.opts[args[2]]; ok {
			if value, ok := m[args[3]]; ok {
				return []byte(value), nil
			}
		}
		return nil, fmt.Errorf("tmux show-option: option %q not found", args[3])
	}
	return nil, nil
}

func (f *FakeTmux) run(_ context.Context, args ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case len(args) >= 5 && args[0] == "set-option" && args[1] == "-qpt":
		if f.opts[args[2]] == nil {
			f.opts[args[2]] = map[string]string{}
		}
		f.opts[args[2]][args[3]] = args[4]
	case len(args) >= 4 && args[0] == "set-option" && args[1] == "-qupt":
		if m, ok := f.opts[args[2]]; ok {
			delete(m, args[3])
		}
	}
	return nil
}

// agentExplainFields builds a list-panes -F line for agent tests.
func agentExplainFields(paneID string, overrides map[int]string) []string {
	fields := []string{
		paneID,
		"work",
		"1",
		"0",
		"/Users/milo/Projects/seshagy",
		"1",
		"1",
		"1",
		"0",
		"claude",
		"",
		"claude",
		"working",
		"needs ok",
		"123",
		"seshagy:claude",
		"session-123",
		"42",
		"12345",
	}
	for idx, value := range overrides {
		fields[idx] = value
	}
	return fields
}

// TmuxCall records one tmux hook invocation from a strict fake.
type TmuxCall struct {
	Output bool
	Args   []string
}

// StrictFakeTmux records tmuxOutput/tmuxRun calls and fails on unexpected ones.
type StrictFakeTmux struct {
	t              *testing.T
	base           *FakeTmux
	mu             sync.Mutex
	allowOutput    []func([]string) bool
	allowRun       []func([]string) bool
	outputHandlers []tmuxOutputHandler
	runHandlers    []tmuxRunHandler
	OutputCalls    []TmuxCall
	RunCalls       []TmuxCall
}

type tmuxOutputHandler struct {
	match func([]string) bool
	fn    func(context.Context, ...string) ([]byte, error)
}

type tmuxRunHandler struct {
	match func([]string) bool
	fn    func(context.Context, ...string) error
}

// NewStrictFakeTmux wraps base (or a new FakeTmux when nil) with call recording.
func NewStrictFakeTmux(t *testing.T, base *FakeTmux) *StrictFakeTmux {
	t.Helper()
	if base == nil {
		base = NewFakeTmux()
	}
	return &StrictFakeTmux{t: t, base: base}
}

// AllowOutput registers a matcher for tmuxOutput calls.
func (s *StrictFakeTmux) AllowOutput(match func([]string) bool) *StrictFakeTmux {
	s.allowOutput = append(s.allowOutput, match)
	return s
}

// AllowRun registers a matcher for tmuxRun calls.
func (s *StrictFakeTmux) AllowRun(match func([]string) bool) *StrictFakeTmux {
	s.allowRun = append(s.allowRun, match)
	return s
}

// HandleOutput registers a custom response for matched tmuxOutput calls.
func (s *StrictFakeTmux) HandleOutput(
	match func([]string) bool,
	fn func(context.Context, ...string) ([]byte, error),
) *StrictFakeTmux {
	s.allowOutput = append(s.allowOutput, match)
	s.outputHandlers = append(s.outputHandlers, tmuxOutputHandler{match: match, fn: fn})
	return s
}

// HandleRun registers a custom handler for matched tmuxRun calls.
func (s *StrictFakeTmux) HandleRun(
	match func([]string) bool,
	fn func(context.Context, ...string) error,
) *StrictFakeTmux {
	s.allowRun = append(s.allowRun, match)
	s.runHandlers = append(s.runHandlers, tmuxRunHandler{match: match, fn: fn})
	return s
}

// AllowPaneOptions permits show/set/unset pane option traffic handled by FakeTmux.
func (s *StrictFakeTmux) AllowPaneOptions() *StrictFakeTmux {
	return s.AllowOutput(MatchShowOption).AllowRun(MatchSetOption)
}

// Install swaps tmux hooks for the duration of the test.
func (s *StrictFakeTmux) Install(t *testing.T) *FakeTmux {
	t.Helper()
	SetTmuxHooksForTest(t, s.output, s.run)
	return s.base
}

func (s *StrictFakeTmux) output(ctx context.Context, args ...string) ([]byte, error) {
	s.t.Helper()
	s.mu.Lock()
	s.OutputCalls = append(
		s.OutputCalls,
		TmuxCall{Output: true, Args: append([]string(nil), args...)},
	)
	s.mu.Unlock()
	if !s.allowed(s.allowOutput, args) {
		s.t.Fatalf("unexpected tmux output call: %v", args)
	}
	for _, handler := range s.outputHandlers {
		if handler.match(args) {
			return handler.fn(ctx, args...)
		}
	}
	return s.base.output(ctx, args...)
}

func (s *StrictFakeTmux) run(ctx context.Context, args ...string) error {
	s.t.Helper()
	s.mu.Lock()
	s.RunCalls = append(s.RunCalls, TmuxCall{Args: append([]string(nil), args...)})
	s.mu.Unlock()
	if !s.allowed(s.allowRun, args) {
		s.t.Fatalf("unexpected tmux run call: %v", args)
	}
	for _, handler := range s.runHandlers {
		if handler.match(args) {
			return handler.fn(ctx, args...)
		}
	}
	return s.base.run(ctx, args...)
}

func (s *StrictFakeTmux) allowed(matchers []func([]string) bool, args []string) bool {
	for _, match := range matchers {
		if match(args) {
			return true
		}
	}
	return false
}

// MatchShowOption matches show-option reads against FakeTmux.
func MatchShowOption(args []string) bool {
	return len(args) >= 4 && args[0] == "show-option"
}

// MatchSetOption matches set/unset pane option writes handled by FakeTmux.
func MatchSetOption(args []string) bool {
	return len(args) >= 4 && args[0] == "set-option" &&
		(args[1] == "-qpt" || args[1] == "-qupt")
}

// MatchDisplayMessage matches display-message -p -t pane format queries.
func MatchDisplayMessage(pane string, formats ...string) func([]string) bool {
	return func(args []string) bool {
		if len(args) < 5 || args[0] != "display-message" || args[1] != "-p" ||
			args[2] != "-t" || args[3] != pane {
			return false
		}
		if len(formats) == 0 {
			return true
		}
		for _, format := range formats {
			if args[4] == format {
				return true
			}
		}
		return false
	}
}

// MatchListSessions matches list-sessions calls.
func MatchListSessions(args []string) bool {
	return len(args) >= 1 && args[0] == "list-sessions"
}

// MatchNewSession matches detached new-session creation.
func MatchNewSession(args []string) bool {
	return len(args) >= 2 && args[0] == "new-session" && args[1] == "-d"
}

// MatchListPanesAgents matches list-panes -a -F agentFormat queries.
func MatchListPanesAgents(args []string) bool {
	return len(args) >= 4 && args[0] == "list-panes" && args[1] == "-a" &&
		args[2] == "-F" && args[3] == agentFormat
}

// MatchCapturePane matches capture-pane reads for a pane target.
func MatchCapturePane(pane string) func([]string) bool {
	return func(args []string) bool {
		return len(args) >= 4 && args[0] == "capture-pane" && args[3] == pane
	}
}

// InstallListAgentsFakeTmux installs a strict fake for ListAgents tests.
func InstallListAgentsFakeTmux(
	t *testing.T,
	pane string,
	listOut []byte,
	visibility string,
	captureScreen string,
) *FakeTmux {
	t.Helper()
	if visibility == "" {
		visibility = fakePaneVisibilityActive
	}
	strict := NewStrictFakeTmux(t, nil).
		AllowPaneOptions().
		AllowOutput(MatchListPanesAgents).
		AllowOutput(MatchDisplayMessage(pane, "#{pane_active} #{window_active} #{session_attached}"))
	if captureScreen != "" {
		strict = strict.AllowOutput(MatchCapturePane(pane))
	}
	strict.
		HandleOutput(MatchListPanesAgents, func(_ context.Context, _ ...string) ([]byte, error) {
			return listOut, nil
		}).
		HandleOutput(
			MatchDisplayMessage(pane, "#{pane_active} #{window_active} #{session_attached}"),
			func(_ context.Context, _ ...string) ([]byte, error) {
				return []byte(visibility), nil
			},
		)
	if captureScreen != "" {
		strict.HandleOutput(
			MatchCapturePane(pane),
			func(_ context.Context, _ ...string) ([]byte, error) {
				return []byte(captureScreen), nil
			},
		)
	}
	return strict.Install(t)
}

// InstallExplainFakeTmux installs a strict fake for ExplainAgent tests.
func InstallExplainFakeTmux(
	t *testing.T,
	pane string,
	fields []string,
	captureScreen string,
) *FakeTmux {
	t.Helper()
	displayLine := strings.Join(fields, paneSep)
	strict := NewStrictFakeTmux(t, nil).
		AllowPaneOptions().
		AllowOutput(MatchDisplayMessage(pane, "#{pane_id}", agentFormat))
	if captureScreen != "" {
		strict = strict.AllowOutput(MatchCapturePane(pane))
	}
	strict.
		HandleOutput(MatchDisplayMessage(pane, "#{pane_id}"), func(_ context.Context, _ ...string) ([]byte, error) {
			return []byte(pane), nil
		}).
		HandleOutput(MatchDisplayMessage(pane, agentFormat), func(_ context.Context, _ ...string) ([]byte, error) {
			return []byte(displayLine), nil
		})
	if captureScreen != "" {
		strict.HandleOutput(
			MatchCapturePane(pane),
			func(_ context.Context, _ ...string) ([]byte, error) {
				return []byte(captureScreen), nil
			},
		)
	}
	return strict.Install(t)
}

// InstallAgentCLIFakeTmux installs a strict fake for cmd/seshagy agent CLI tests.
func InstallAgentCLIFakeTmux(t *testing.T, pane string, listOut []byte) *FakeTmux {
	t.Helper()
	base := NewFakeTmux()
	strict := NewStrictFakeTmux(t, base).
		AllowPaneOptions().
		AllowOutput(func(args []string) bool {
			return len(args) >= 3 && args[0] == "list-panes" && args[1] == "-a"
		}).
		AllowOutput(MatchDisplayMessage(
			pane,
			"#{pane_id}",
			"#{pane_active} #{window_active} #{session_attached}",
		))
	if listOut != nil {
		strict = strict.AllowOutput(func(args []string) bool {
			return len(args) >= 5 && args[0] == "display-message" && args[1] == "-p" &&
				args[2] == "-t" && args[3] == pane && args[4] == agentFormat
		})
	}
	strict.
		HandleOutput(func(args []string) bool {
			return len(args) >= 3 && args[0] == "list-panes" && args[1] == "-a"
		}, func(_ context.Context, _ ...string) ([]byte, error) {
			return listOut, nil
		}).
		HandleOutput(MatchDisplayMessage(pane, "#{pane_id}"), func(_ context.Context, _ ...string) ([]byte, error) {
			return []byte(pane), nil
		}).
		HandleOutput(
			MatchDisplayMessage(pane, "#{pane_active} #{window_active} #{session_attached}"),
			func(_ context.Context, _ ...string) ([]byte, error) {
				return []byte(fakePaneVisibilityActive), nil
			},
		)
	if listOut != nil {
		strict.HandleOutput(func(args []string) bool {
			return len(args) >= 5 && args[0] == "display-message" && args[1] == "-p" &&
				args[2] == "-t" && args[3] == pane && args[4] == agentFormat
		}, func(_ context.Context, _ ...string) ([]byte, error) {
			return listOut, nil
		})
	}
	strict.Install(t)
	return base
}

func InstallReportFakeTmux(t *testing.T, pane string) *FakeTmux {
	t.Helper()
	return NewStrictFakeTmux(t, nil).
		AllowPaneOptions().
		AllowOutput(MatchDisplayMessage(
			pane,
			"#{pane_id}",
			"#{pane_active} #{window_active} #{session_attached}",
		)).
		HandleOutput(MatchDisplayMessage(pane, "#{pane_id}"), func(_ context.Context, _ ...string) ([]byte, error) {
			return []byte(pane), nil
		}).
		HandleOutput(
			MatchDisplayMessage(pane, "#{pane_active} #{window_active} #{session_attached}"),
			func(_ context.Context, _ ...string) ([]byte, error) {
				return []byte(fakePaneVisibilityActive), nil
			},
		).
		Install(t)
}

// InstallTrackFakeTmux installs a strict fake for agent status tracking tests.
func InstallTrackFakeTmux(t *testing.T, f *FakeTmux) *FakeTmux {
	t.Helper()
	if f == nil {
		f = NewFakeTmux()
	}
	return NewStrictFakeTmux(t, f).AllowPaneOptions().Install(t)
}

// SetTmuxHooksForTest swaps tmuxOutput/tmuxRun for the duration of a test.
func SetTmuxHooksForTest(
	t *testing.T,
	output func(context.Context, ...string) ([]byte, error),
	run func(context.Context, ...string) error,
) {
	t.Helper()
	origOut, origRun := tmuxOutput, tmuxRun
	if output != nil {
		tmuxOutput = output
	}
	if run != nil {
		tmuxRun = run
	}
	t.Cleanup(func() {
		tmuxOutput = origOut
		tmuxRun = origRun
	})
}

// SetAgentTrackNowForTest pins agentTrackNow for cross-package CLI tests.
func SetAgentTrackNowForTest(t *testing.T, now time.Time) {
	t.Helper()
	orig := agentTrackNow
	agentTrackNow = func() time.Time { return now }
	t.Cleanup(func() { agentTrackNow = orig })
}

// SetFixedTrackTime pins agentTrackNow within sessionmgr tests.
func SetFixedTrackTime(t *testing.T, now time.Time) {
	SetAgentTrackNowForTest(t, now)
}

// SetManifestAutoUpdateIntervalForTest overrides the auto-update ticker interval.
func SetManifestAutoUpdateIntervalForTest(t *testing.T, interval time.Duration) {
	t.Helper()
	old := manifestAutoUpdateInterval.Load()
	manifestAutoUpdateInterval.Store(interval.Nanoseconds())
	t.Cleanup(func() { manifestAutoUpdateInterval.Store(old) })
}

// SetAgentPaneLockHookForTest overrides per-pane lock acquisition for tests.
func SetAgentPaneLockHookForTest(t *testing.T, hook func(pane string, fn func() error) error) {
	t.Helper()
	orig := agentPaneLockHook
	agentPaneLockHook = hook
	t.Cleanup(func() { agentPaneLockHook = orig })
}

// StopManifestAutoUpdateForTest stops any manifest auto-update goroutine during tests.
func StopManifestAutoUpdateForTest(t *testing.T) {
	t.Helper()
	t.Cleanup(StopManifestAutoUpdate)
}

func formatUnix(ts time.Time) string {
	return fmt.Sprintf("%d", ts.Unix())
}
