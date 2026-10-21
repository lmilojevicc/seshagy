package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

// fakeMachineMux is a herdr-flavoured Multiplexer that can target saved
// machines. Every machine id ("" is the local server) gets its own fakeMux, so a
// test can assert which server an action reached by looking at the backend it
// recorded on.
type fakeMachineMux struct {
	*fakeMux
	machines    []sessionmgr.Machine
	machinesErr error
	bound       map[string]*fakeMux
	// boundIDs records every machine a backend was requested for, in order.
	boundIDs []string
}

func newFakeMachineMux() *fakeMachineMux {
	local := newFakeMux()
	local.kind = sessionmgr.BackendHerdr
	local.terms = sessionmgr.HerdrTerms()
	return &fakeMachineMux{
		fakeMux: local,
		bound:   map[string]*fakeMux{"": local},
	}
}

func (f *fakeMachineMux) Machines(context.Context) ([]sessionmgr.Machine, error) {
	return f.machines, f.machinesErr
}

func (f *fakeMachineMux) MachineStatuses(
	context.Context,
	...string,
) ([]sessionmgr.MachineStatus, error) {
	return nil, nil
}

func (f *fakeMachineMux) WithMachine(m sessionmgr.Machine) sessionmgr.Multiplexer {
	f.boundIDs = append(f.boundIDs, m.ID)
	if backend, ok := f.bound[m.ID]; ok {
		return backend
	}
	backend := newFakeMux()
	backend.kind = sessionmgr.BackendHerdr
	backend.terms = sessionmgr.HerdrTerms()
	f.bound[m.ID] = backend
	return backend
}

// machineBackend is the fake serving one machine's commands.
func (f *fakeMachineMux) machineBackend(id string) *fakeMux {
	f.WithMachine(sessionmgr.Machine{ID: id})
	return f.bound[id]
}

// machineModel returns a herdr model whose catalog is dev + gpu, with local
// selected.
func machineModel(t *testing.T) (Model, *fakeMachineMux) {
	t.Helper()
	mux := newFakeMachineMux()
	mux.machines = []sessionmgr.Machine{
		{ID: "dev-id", Label: "devbox", Enabled: true},
		{ID: "gpu-id", Label: "gpu-box", Enabled: true},
	}
	m := newTestModelWith(t, mux)
	// The reload the catalog change schedules is exercised by the source poll;
	// these tests drive the per-machine loads themselves.
	m, _ = m.applyMachines(machinesMsg{machines: mux.machines})
	return m, mux
}

// remappedHerdrModel returns a herdr model (saved machines available) whose
// [tui.keys] overrides are the given ones.
func remappedHerdrModel(t *testing.T, overrides map[string]appconfig.KeyList) Model {
	t.Helper()
	cfg := appconfig.Default()
	cfg.TUI.Keys = overrides
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HERDR_ENV", "")
	t.Setenv("SESHAGY_AGENT_ICONS", "none")
	m := New(WithConfig(cfg), WithMultiplexer(newFakeMachineMux()))
	m.width, m.height = 120, 32
	return m
}

// deliverMachineRefresh feeds one source result through Update as the poll would,
// stamped with the machine the load ran against.
func deliverMachineRefresh(
	t *testing.T,
	m Model,
	machine string,
	items []sessionmgr.Item,
	err error,
) Model {
	t.Helper()
	source := sessionmgr.ModeAgents
	if m.inflightRefresh == nil {
		m.inflightRefresh = make(map[sessionmgr.SourceMode]uint64)
	}
	if m.refreshGen == nil {
		m.refreshGen = make(map[sessionmgr.SourceMode]uint64)
	}
	m.refreshGen[source]++
	gen := m.refreshGen[source]
	m.inflightRefresh[source] = gen
	next, _ := m.handleRefreshMsg(refreshMsg{
		source:  source,
		gen:     gen,
		machine: machine,
		items:   items,
		err:     err,
	})
	return next
}

func TestAltMCyclesTheSavedMachines(t *testing.T) {
	m, _ := machineModel(t)
	if m.machine != "" {
		t.Fatalf("machine = %q, want the local server", m.machine)
	}
	var order []string
	for range 3 {
		m, _ = press(t, m, "alt+m")
		order = append(order, m.machine)
	}
	want := []string{"dev-id", "gpu-id", ""}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("alt+m cycled %v, want %v", order, want)
	}
	if got := latestNotificationText(m); got != "machine @local" {
		t.Fatalf("toast = %q, want the machine that is now selected", got)
	}
}

func TestAltMListsTheSelectedMachinesItems(t *testing.T) {
	m, _ := machineModel(t)
	devAgent := agentItem("pi", "w1:p1", "frontend")
	devAgent.Machine = "dev-id"
	m.machine = "dev-id"
	m = deliverMachineRefresh(t, m, "dev-id", []sessionmgr.Item{devAgent}, nil)

	if len(m.items) != 1 {
		t.Fatalf("items = %+v, want the dev machine's agent", m.items)
	}
	if m.items[0].Machine != "dev-id" {
		t.Fatalf("item machine = %q, want dev-id", m.items[0].Machine)
	}
}

func TestDisabledMachinesAreNotOffered(t *testing.T) {
	mux := newFakeMachineMux()
	m := newTestModelWith(t, mux)
	next, _ := m.applyMachines(machinesMsg{machines: []sessionmgr.Machine{
		{ID: "dev-id", Label: "devbox", Enabled: true},
		{ID: "off-id", Label: "gpu-box"},
	}})
	if len(next.machines) != 2 {
		t.Fatalf("cycle = %+v, want local plus the one enabled profile", next.machines)
	}
	if next.machineIndex("off-id") >= 0 {
		t.Fatal("a disabled profile is in the alt+m cycle")
	}
}

func TestMachineSwitchDropsAStaleRefresh(t *testing.T) {
	m, _ := machineModel(t)
	m.machine = "dev-id"
	m = deliverMachineRefresh(t, m, "dev-id", []sessionmgr.Item{
		agentItem("pi", "w1:p1", "frontend"),
	}, nil)

	// A refresh armed against the old machine and delivered after the switch
	// must not replace the list.
	stale, _ := m.handleRefreshMsg(refreshMsg{
		source:  sessionmgr.ModeAgents,
		gen:     m.inflightRefresh[sessionmgr.ModeAgents],
		machine: "",
		items:   []sessionmgr.Item{agentItem("codex", "w2:p2", "backend")},
	})
	if len(stale.items) != 1 || stale.items[0].AgentName != "pi" {
		t.Fatalf("a stale refresh replaced the list: %+v", stale.items)
	}

	// The same machine with an old generation is dropped too.
	inFlight := m.inflightRefresh[sessionmgr.ModeAgents]
	staleGen, _ := m.handleRefreshMsg(refreshMsg{
		source:  sessionmgr.ModeAgents,
		gen:     inFlight - 1,
		machine: "dev-id",
		items:   []sessionmgr.Item{agentItem("codex", "w2:p2", "backend")},
	})
	if len(staleGen.items) != 1 || staleGen.items[0].AgentName != "pi" {
		t.Fatalf("a stale generation replaced the list: %+v", staleGen.items)
	}
}

func TestMachineSwitchClearsPreviewAndAgentStarts(t *testing.T) {
	m, _ := machineModel(t)
	m.preview, m.previewKey, m.previewPendingKey, m.previewError = "text", "k", "p", "boom"
	m.agentStarted["w1:p1"] = agentStart{pid: 1}
	m.loadError = "unreachable"
	m.machine = "dev-id"

	m = m.switchMachine("gpu-id")
	switch {
	case m.preview != "" || m.previewKey != "" || m.previewPendingKey != "" ||
		m.previewError != "":
		t.Fatalf("preview survived the switch: %+v", m)
	case len(m.agentStarted) != 0:
		t.Fatalf("agent starts survived the switch: %+v", m.agentStarted)
	case m.loadError != "":
		t.Fatalf("loadError = %q, want it cleared for the new machine", m.loadError)
	case m.loadedOnce:
		t.Fatal("loadedOnce survived the switch")
	case len(m.cache) != 0:
		t.Fatalf("cache survived the switch: %+v", m.cache)
	}
}

func TestMachineSwitchFallsBackWhenTheProfileVanishes(t *testing.T) {
	m, _ := machineModel(t)
	m.machine = "dev-id"
	next, _ := m.applyMachines(machinesMsg{machines: []sessionmgr.Machine{
		{ID: "gpu-id", Label: "gpu-box", Enabled: true},
	}})
	if next.machine != "" {
		t.Fatalf("machine = %q, want the local server", next.machine)
	}
	if got := latestNotificationText(next); got != "machine removed — back to local" {
		t.Fatalf("toast = %q, want the fallback notice", got)
	}
}

func TestMachineCatalogFailureKeepsThePickerLocal(t *testing.T) {
	mux := newFakeMachineMux()
	mux.machinesErr = errors.New("herdr machine list: unknown command")
	m := newTestModelWith(t, mux)
	next, cmd := m.applyMachines(machinesMsg{err: mux.machinesErr})
	if cmd != nil {
		t.Fatal("a failed catalog read scheduled a reload")
	}
	if len(next.machines) != 0 || next.machineErr == nil {
		t.Fatalf("machines = %+v err = %v", next.machines, next.machineErr)
	}
	if got := latestNotificationText(next); got != "cannot list herdr machines" {
		t.Fatalf("toast = %q", got)
	}
	if title := next.filterTitle(); title != "all" {
		t.Fatalf("title = %q, want the plain title", title)
	}
	// A second failure does not toast again.
	again, _ := next.applyMachines(machinesMsg{err: mux.machinesErr})
	if len(again.notifications) != len(next.notifications) {
		t.Fatal("a repeated catalog failure toasted again")
	}
}

func TestMachineCatalogFailureKeepsTheSelectedMachine(t *testing.T) {
	m, _ := machineModel(t)
	m = m.switchMachine("dev-id")
	m = deliverMachineRefresh(t, m, "dev-id", []sessionmgr.Item{
		agentItem("pi", "w1:p1", "frontend"),
	}, nil)
	before := m.filterTitle()

	next, cmd := m.applyMachines(machinesMsg{err: errors.New("herdr machine list: bad json")})
	if cmd != nil {
		t.Fatal("a failed catalog re-read scheduled a reload")
	}
	if next.machine != "dev-id" {
		t.Fatalf("machine = %q, want the selection to stay on dev-id", next.machine)
	}
	if len(next.machines) != 3 {
		t.Fatalf("machines = %+v, want the catalog kept", next.machines)
	}
	if next.machineErr == nil {
		t.Fatal("machineErr was not recorded")
	}
	if got := next.filterTitle(); got != before || !strings.Contains(got, "@devbox") {
		t.Fatalf("title = %q, want the machine suffix kept at %q", got, before)
	}
	for _, hint := range next.footerHints() {
		if hint.label == "machine" {
			return
		}
	}
	t.Fatal("the footer dropped the machine hint after a failed catalog read")
}

func TestMachineSwitchDropsThePreviousServersRows(t *testing.T) {
	m, _ := machineModel(t)
	m = m.switchMachine("dev-id")
	devAgent := agentItem("pi", "w1:p1", "frontend")
	devAgent.Machine = "dev-id"
	m = deliverMachineRefresh(t, m, "dev-id", []sessionmgr.Item{
		devAgent,
		{Kind: sessionmgr.KindSession, Name: "api", Target: "w9", Machine: "dev-id"},
	}, nil)
	if len(m.items) != 2 {
		t.Fatalf("items = %+v, want the dev machine's rows", m.items)
	}

	m = m.switchMachine("gpu-id")
	for _, item := range m.items {
		if item.Machine != "gpu-id" {
			t.Fatalf("row %+v survived the switch, want none from dev-id", item)
		}
	}
	if m.cursorKey != "" || m.cursor != 0 {
		t.Fatalf("cursor = %d/%q, want it reset on the new machine", m.cursor, m.cursorKey)
	}
}

func TestFilterTitleNamesTheSelectedMachine(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(t *testing.T) Model
		want    string
	}{
		{
			name: "no selector",
			prepare: func(t *testing.T) Model {
				t.Helper()
				return newTestModel(t)
			},
			want: "all",
		},
		{
			name: "selector without a catalog yet",
			prepare: func(t *testing.T) Model {
				t.Helper()
				m, _ := machineModel(t)
				m.machines = nil
				return m
			},
			want: "all",
		},
		{
			name: "only the local server saved",
			prepare: func(t *testing.T) Model {
				t.Helper()
				m, _ := machineModel(t)
				m.machines = []sessionmgr.Machine{sessionmgr.LocalMachine()}
				return m
			},
			want: "all",
		},
		{
			name: "local selected beside a remote",
			prepare: func(t *testing.T) Model {
				t.Helper()
				m, _ := machineModel(t)
				return m
			},
			want: "all · @local",
		},
		{
			name: "remote selected with the filters",
			prepare: func(t *testing.T) Model {
				t.Helper()
				m, _ := machineModel(t)
				m.machine = "dev-id"
				m.scope = scopeAgents
				m.stateFilter = stateFilterBlocked
				return m
			},
			want: "agents · blocked · @devbox",
		},
		{
			name: "a label with control characters",
			prepare: func(t *testing.T) Model {
				t.Helper()
				m, _ := machineModel(t)
				m.machines = []sessionmgr.Machine{
					sessionmgr.LocalMachine(),
					{ID: "dev-id", Label: "dev\x1b[31mbox\x07", Enabled: true},
				}
				m.machine = "dev-id"
				return m
			},
			want: "all · @devbox",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.prepare(t).filterTitle(); got != tt.want {
				t.Fatalf("filterTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPromptWindowTitleShowsTheMachine(t *testing.T) {
	m, _ := machineModel(t)
	m.machine = "dev-id"
	m.scope = scopeAgents
	m.stateFilter = stateFilterBlocked
	m.width, m.height = 120, 32
	m.loadedOnce = true
	frame := sessionmgr.StripANSI(m.View())
	if !strings.Contains(frame, " seshagy · agents · blocked · @devbox ") {
		t.Fatalf("the prompt title does not name the machine:\n%s", frame)
	}
}

func TestMachineBindingIsRemappable(t *testing.T) {
	m := remappedHerdrModel(t, keys("machine", "alt+n"))
	if binding, ok := m.keys.bindingFor("alt+n"); !ok || binding.id != "machine" {
		t.Fatalf("alt+n dispatches to %+v, want the machine filter", binding)
	}
	if _, ok := m.keys.bindingFor("alt+m"); ok {
		t.Fatal("the default alt+m still dispatches after the remap")
	}
	hints := m.footerHints()
	if hints[len(hints)-3].key != "alt+n" || hints[len(hints)-3].label != "machine" {
		t.Fatalf("footer hints = %+v, want the remapped machine hint", hints)
	}
	if row := helpRowFor(t, openHelp(t, m), "alt+n"); row.label != "machine" {
		t.Fatalf("the help row = %+v, want the machine filter", row)
	}

	// A printable key would type into the query, so it is rejected and the
	// default binding survives.
	printable := remappedHerdrModel(t, keys("machine", "n"))
	issues := printable.keys.Issues()
	if len(issues) == 0 || !strings.Contains(issues[0], "would type into the query") {
		t.Fatalf("issues = %v, want the printable-key complaint", issues)
	}
	if binding, ok := printable.keys.bindingFor("alt+m"); !ok || binding.id != "machine" {
		t.Fatal("a rejected remap unbound the machine filter")
	}
}

func TestMachineBindingIsInertWithoutHerdrMachines(t *testing.T) {
	for _, tt := range []struct {
		name string
		mux  func() sessionmgr.Multiplexer
		want string
	}{
		{"tmux", func() sessionmgr.Multiplexer { return newFakeMux() }, "saved machines need herdr"},
		{
			"herdr without saved machines",
			func() sessionmgr.Multiplexer {
				mux := newFakeMachineMux()
				return mux
			},
			"no saved herdr machines (herdr machine add)",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModelWith(t, tt.mux())
			m = withItems(m, []sessionmgr.Item{
				{Kind: sessionmgr.KindSession, Name: "demo", Target: "demo"},
			})
			m.scope = scopeAgents
			before := m.filterTitle()
			m, _ = press(t, m, "alt+m")
			if m.machine != "" {
				t.Fatalf("machine = %q, want the local server", m.machine)
			}
			if got := m.filterTitle(); got != before {
				t.Fatalf("title = %q, want it unchanged at %q", got, before)
			}
			if got := latestNotificationText(m); got != tt.want {
				t.Fatalf("toast = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestActionsRouteToTheItemsMachine(t *testing.T) {
	m, mux := machineModel(t)
	dev := mux.machineBackend("dev-id")
	dev.preview = "dev pane output"
	m.machine = "dev-id"

	remoteSession := sessionmgr.Item{
		Kind:    sessionmgr.KindSession,
		Name:    "api",
		Target:  "w9",
		Machine: "dev-id",
	}
	m = withItems(m, []sessionmgr.Item{remoteSession})
	local := mux.machineBackend("")
	local.preview = "local pane output"

	// Preview captures from the item's server.
	next, cmd := m.requestPreview()
	if cmd == nil {
		t.Fatal("requestPreview scheduled nothing")
	}
	msg, ok := cmd().(previewMsg)
	if !ok || msg.preview != "dev pane output" {
		t.Fatalf("preview = %#v, want the dev machine's capture", msg)
	}
	m = next

	// Kill targets the item's server.
	m, _ = press(t, m, "ctrl+x")
	m, cmd = press(t, m, "enter")
	if cmd == nil {
		t.Fatal("the kill confirmation scheduled nothing")
	}
	_ = cmd()
	if len(dev.killed) != 1 || dev.killed[0] != "w9" {
		t.Fatalf("dev killed = %v, want [w9]", dev.killed)
	}
	if len(local.killed) != 0 {
		t.Fatalf("the local server was asked to kill %v", local.killed)
	}

	// Rename targets the item's server, even after the selection moved.
	m.killInFlight = false
	m = withItems(m, []sessionmgr.Item{remoteSession})
	m, _ = press(t, m, "ctrl+r")
	m.renameText = "renamed"
	m.machine = ""
	_, cmd = m.applyRename()
	if cmd == nil {
		t.Fatal("applyRename scheduled nothing")
	}
	_ = cmd()
	if len(dev.renamed) != 1 || dev.renamed[0] != "w9=renamed" {
		t.Fatalf("dev renamed = %v, want [w9=renamed]", dev.renamed)
	}
	if len(local.renamed) != 0 {
		t.Fatalf("the local server was asked to rename %v", local.renamed)
	}
}

func TestRemoteFocusDoesNotHandOverTheTerminal(t *testing.T) {
	m, _ := machineModel(t)
	m.machine = "dev-id"
	remote := sessionmgr.Item{
		Kind:    sessionmgr.KindSession,
		Name:    "api",
		Target:  "w9",
		Machine: "dev-id",
	}
	remoteMsg, ok := attachCmd(m, remote)().(attachDoneMsg)
	if !ok {
		t.Fatal("a remote attach did not report back to the picker")
	}
	if remoteMsg.machine != "dev-id" || remoteMsg.name != "api" || remoteMsg.err != nil {
		t.Fatalf("remote attach = %#v", remoteMsg)
	}

	// The local item still hands the terminal to the multiplexer: its command
	// produces bubbletea's own exec message rather than an attachDoneMsg.
	local := sessionmgr.Item{Kind: sessionmgr.KindSession, Name: "demo", Target: "demo"}
	if _, ok := attachCmd(m, local)().(attachDoneMsg); ok {
		t.Fatal("a local attach reported back instead of handing over the terminal")
	}

	// The agent focus follows the same split.
	agent := agentItem("pi", "w1:p1", "frontend")
	agent.Machine = "dev-id"
	if _, ok := focusAgentCmd(m, agent)().(attachDoneMsg); !ok {
		t.Fatal("a remote agent focus did not report back to the picker")
	}
	if _, ok := focusAgentCmd(m, agentItem("pi", "w1:p1", "x"))().(attachDoneMsg); ok {
		t.Fatal("a local agent focus reported back instead of handing over the terminal")
	}
}

func TestRemoteFocusToastNamesTheMachine(t *testing.T) {
	m, _ := machineModel(t)
	m.machine = "dev-id"
	next, _ := m.Update(attachDoneMsg{machine: "dev-id", name: "api"})
	if got := latestNotificationText(next.(Model)); got != "focused api on @devbox" {
		t.Fatalf("toast = %q, want the machine-qualified focus notice", got)
	}
}

func TestRemoteLoadFailureRendersTheEmptyState(t *testing.T) {
	m, _ := machineModel(t)
	m.machine = "dev-id"
	m.loadedOnce = true
	m.loadError = "herdr workspace list: ssh: connection refused"

	m = withItems(m, nil)
	frame := sessionmgr.StripANSI(m.View())
	for _, want := range []string{
		"machine @devbox unavailable",
		"alt+m switch machine",
		"^l retry",
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("the empty state does not draw %q:\n%s", want, frame)
		}
	}

	// A failing local source keeps the toast-only behaviour: the window still
	// says there is nothing to jump to.
	local := newTestModel(t)
	local.loadedOnce = true
	local = deliverMachineRefresh(t, local, "", nil, errors.New("herdr agent list: boom"))
	if local.loadError != "" {
		t.Fatalf("loadError = %q, want none for the local machine", local.loadError)
	}
	if frame := sessionmgr.StripANSI(local.View()); !strings.Contains(frame, "nothing to jump to") {
		t.Fatalf("the local empty state changed:\n%s", frame)
	}
}

func TestRemoteLoadFailureToastsOncePerMachine(t *testing.T) {
	m, _ := machineModel(t)
	m.machine = "dev-id"
	m = deliverMachineRefresh(t, m, "dev-id", nil, errors.New("ssh: refused"))
	count := len(m.notifications)
	m = deliverMachineRefresh(t, m, "dev-id", nil, errors.New("ssh: refused"))
	if len(m.notifications) != count {
		t.Fatal("a repeated remote failure toasted again")
	}
	if m.loadError != "ssh: refused" {
		t.Fatalf("loadError = %q", m.loadError)
	}
	// The same failure on the local server is a different message stream.
	m = m.switchMachine("")
	m = deliverMachineRefresh(t, m, "", nil, errors.New("ssh: refused"))
	if len(m.notifications) == count {
		t.Fatal("the local server's failure was suppressed by the remote one")
	}
	if m.loadError != "" {
		t.Fatalf("loadError = %q, want none after switching to local", m.loadError)
	}
}

func TestAgentUptimeSkipsRemoteMachines(t *testing.T) {
	m, _ := machineModel(t)
	remote := agentItem("pi", "w1:p1", "frontend")
	remote.Machine = "dev-id"
	local := agentItem("codex", "w2:p2", "backend")
	m = withItems(m, []sessionmgr.Item{remote, local})

	pending := m.pendingAgentStarts()
	if len(pending) != 1 || pending[0].AgentName != "codex" {
		t.Fatalf("pending = %+v, want only the local pane", pending)
	}

	// A resolved local start stamps only the local row.
	m = m.applyAgentStarts(agentStartedMsg{starts: map[string]agentStart{
		agentStartKey(local): {pid: 42, started: time.Now().Add(-time.Minute)},
	}})
	for _, item := range m.items {
		switch item.AgentName {
		case "codex":
			if item.StartedAt.IsZero() {
				t.Fatal("the local agent's age was not stamped")
			}
		case "pi":
			if !item.StartedAt.IsZero() {
				t.Fatal("a remote agent inherited a local process age")
			}
		}
	}

	// Two servers can both have pane w1:p1: their cache keys differ, and so do
	// their item keys.
	if agentStartKey(remote) == agentStartKey(local) {
		t.Fatal("pane keys collide across machines")
	}
	remoteSamePane := local
	remoteSamePane.Machine = "dev-id"
	if remoteSamePane.Key() == local.Key() {
		t.Fatal("item keys collide across machines")
	}
}

func TestMachinesCmdReadsTheCatalogOncePerCall(t *testing.T) {
	mux := newFakeMachineMux()
	mux.machines = []sessionmgr.Machine{{ID: "dev-id", Label: "devbox", Enabled: true}}
	msg, ok := machinesCmd(mux)().(machinesMsg)
	if !ok || msg.err != nil || len(msg.machines) != 1 {
		t.Fatalf("machinesCmd() = %#v", msg)
	}
	if msg, ok := machinesCmd(sessionmgr.NewTmuxBackend())().(machinesMsg); !ok || msg.err != nil {
		t.Fatalf("a backend without machines returned %#v, want an empty catalog", msg)
	}
}

func TestMachineBindingIsRemappableAltM(t *testing.T) {
	table := ResolveKeys(map[string]appconfig.KeyList{})
	mapping, ok := mappingFor(table, "machine")
	if !ok {
		t.Fatal("the machine action is not in the key table")
	}
	if strings.Join(mapping.Keys, ",") != "alt+m" {
		t.Fatalf("machine keys = %v, want alt+m", mapping.Keys)
	}
	if _, ok := table.bindingFor("ctrl+m"); ok {
		t.Fatal("ctrl+m is bound: bubbletea delivers it as enter")
	}
}

// mappingFor returns one action's mapping from the key table.
func mappingFor(table KeyTable, id string) (KeyMapping, bool) {
	for _, mapping := range table.Mappings() {
		if mapping.ID == id {
			return mapping, true
		}
	}
	return KeyMapping{}, false
}

func TestUnboundMachineBindingDrawsNoRowHintOrTitle(t *testing.T) {
	m := remappedHerdrModel(t, map[string]appconfig.KeyList{"machine": {}})
	if _, ok := m.keys.bindingFor("alt+m"); ok {
		t.Fatal("an unbound machine filter still dispatches alt+m")
	}
	for _, hint := range m.footerHints() {
		if hint.label == "machine" {
			t.Fatalf("the footer still draws the unbound machine hint: %+v", hint)
		}
	}
	for _, block := range m.helpBlocks() {
		for _, row := range block.rows {
			if row.label == "machine" {
				t.Fatalf("the help overlay still draws the unbound machine row: %+v", row)
			}
		}
	}
}
