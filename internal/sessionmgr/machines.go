package sessionmgr

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/lmilojevicc/seshagy/internal/logging"
)

// Machine is one saved herdr SSH profile (`herdr machine list`). The local
// herdr server is the implicit machine with an empty ID: it always exists, it
// is never listed by herdr, and every command without `--machine` targets it.
//
// ID, Label, Target and Session are all user-controlled strings; treat them as
// opaque. Never log them (see logging rules in AGENTS.md).
type Machine struct {
	ID       string // opaque profile id; "" means the local server
	Label    string // user-controlled display label
	Target   string // ssh target, display only
	Session  string // the profile's remote herdr session
	Enabled  bool   // a disabled profile rejects every forwarded command
	Selected bool   // herdr's saved client selection; informational only
}

// LocalMachine returns the implicit local machine. Its ID is empty, which is
// what the herdr backend uses to decide "no --machine prefix".
func LocalMachine() Machine { return Machine{Label: "local"} }

// IsLocal reports whether the machine is the local herdr server.
func (m Machine) IsLocal() bool { return m.ID == "" }

// DisplayLabel returns the label to show in the UI, falling back to "local"
// for the implicit machine and to the opaque id when a profile has no label.
func (m Machine) DisplayLabel() string {
	if m.IsLocal() {
		return "local"
	}
	if m.Label != "" {
		return m.Label
	}
	return m.ID
}

// MachineStatus is one `herdr machine status --json` row. Status is an opaque
// herdr value ("reachable", "auth required", "error", "disabled", ...): herdr
// does not document the complete set, so the classification helpers below are
// deliberately tolerant, and Error (when present) is the more reliable signal.
type MachineStatus struct {
	ID     string
	Label  string
	Status string
	// Error is herdr's diagnostic text for a failed probe. It embeds
	// user-controlled labels/hostnames, so it is display-only and must never be
	// logged.
	Error string
}

// reachableStatuses are the herdr status values that mean "usable now". The
// list is intentionally an allowlist: an unrecognized value must not be
// reported as reachable.
var reachableStatuses = map[string]struct{}{
	"reachable": {},
	"ok":        {},
	"online":    {},
	"ready":     {},
	"running":   {},
}

// unusableStatuses are the status values that mean "do not route commands
// here". Anything outside both tables is "unknown but selectable" — herdr may
// add values, and the caller still has a real error field to consider.
var unusableStatuses = map[string]struct{}{
	"disabled":      {},
	"unreachable":   {},
	"error":         {},
	"auth required": {},
	"not ready":     {},
	"not_ready":     {},
	"incompatible":  {},
}

// normalizedStatus lowercases and trims the herdr status value.
func (s MachineStatus) normalizedStatus() string {
	return strings.ToLower(strings.TrimSpace(s.Status))
}

// Reachable reports whether the machine answered and is usable now.
func (s MachineStatus) Reachable() bool {
	if s.Unusable() {
		return false
	}
	_, ok := reachableStatuses[s.normalizedStatus()]
	return ok
}

// Unusable reports whether herdr says the machine cannot serve commands right
// now. A status value outside both tables is "unknown but selectable": the
// caller may still try, which is why Unusable is a separate, narrower question
// than Reachable.
func (s MachineStatus) Unusable() bool {
	if s.Error != "" {
		return true
	}
	_, ok := unusableStatuses[s.normalizedStatus()]
	return ok
}

// Disabled reports whether herdr has the profile disabled (it rejects every
// forwarded command with exit code 2).
func (s MachineStatus) Disabled() bool {
	return s.normalizedStatus() == "disabled"
}

// NeedsAuth reports whether the probe failed because SSH authentication is
// required, which only the user can resolve (`herdr machine reconnect`).
func (s MachineStatus) NeedsAuth() bool {
	return s.normalizedStatus() == "auth required"
}

// Machines lists the saved herdr SSH profiles. Machine management is a local
// herdr command: it must never carry a `--machine` prefix, which is why this
// uses the package-level seam rather than the backend's scoped one.
// The caller owns the timeout: the command only reads a local catalog file, so
// a small budget is plenty.
func (herdrBackend) Machines(ctx context.Context) ([]Machine, error) {
	return listHerdrMachines(ctx)
}

func listHerdrMachines(ctx context.Context) ([]Machine, error) {
	out, err := herdrOutput(ctx, "machine", "list", "--json")
	if err != nil && len(bytes.TrimSpace(out)) == 0 {
		return nil, fmt.Errorf("herdr machine list: %w", err)
	}
	machines, parseErr := parseHerdrMachines(out)
	if parseErr != nil {
		return nil, fmt.Errorf("herdr machine list: %w", parseErr)
	}
	return machines, nil
}

// MachineStatuses probes saved profiles with `herdr machine status --json`.
//
// herdr exits 1 when any probed row failed even though stdout still carries the
// full array, so stdout is parsed before the exit status is considered. A
// selector that herdr rejects (disabled or unknown: exit 2, empty stdout) falls
// back to probing every profile so the caller still gets data.
func (herdrBackend) MachineStatuses(
	ctx context.Context,
	selectors ...string,
) ([]MachineStatus, error) {
	out, _ := herdrOutput(ctx, machineStatusArgs(selectors)...)
	if len(bytes.TrimSpace(out)) == 0 && len(selectors) > 0 {
		out, _ = herdrOutput(ctx, machineStatusArgs(nil)...)
	}
	statuses, err := parseHerdrMachineStatuses(out)
	if err != nil {
		return nil, fmt.Errorf("herdr machine status: %w", err)
	}
	return statuses, nil
}

func machineStatusArgs(selectors []string) []string {
	args := make([]string, 0, len(selectors)+3)
	args = append(args, "machine", "status")
	args = append(args, selectors...)
	return append(args, "--json")
}

// WithMachine binds the backend to a saved profile. The zero Machine is the
// local server, so WithMachine(LocalMachine()) == the local backend.
func (herdrBackend) WithMachine(m Machine) Multiplexer { return herdrBackend{machine: m.ID} }

// forItem returns the backend bound to the server the item lives on. Actions
// that carry an Item derive their target from the item, never from the picker's
// current selection: an action armed before a machine switch must still reach
// the server the item came from. String-only actions (KillSession, KillPane,
// RenameSession, CaptureSession, CaptureAgentPane, ResolvePaneByCwd) have no
// item to read, so callers must fetch them from the item's bound backend
// (MachineSelector.WithMachine) instead.
func (b herdrBackend) forItem(item Item) herdrBackend {
	if item.Machine == "" {
		return b
	}
	return herdrBackend{machine: item.Machine}
}

// scoped prefixes args with the herdr global `--machine <id>` routing option,
// which must precede the subcommand (`herdr --machine devbox workspace list`).
func (b herdrBackend) scoped(args []string) []string {
	if b.machine == "" {
		return args
	}
	scoped := make([]string, 0, len(args)+2)
	scoped = append(scoped, "--machine", b.machine)
	return append(scoped, args...)
}

// output runs a herdr API command against the backend's machine.
func (b herdrBackend) output(ctx context.Context, args ...string) ([]byte, error) {
	if b.machine == "" {
		return herdrOutput(ctx, args...)
	}
	started := time.Now()
	out, err := herdrOutput(ctx, b.scoped(args)...)
	b.logCommandFailure(ctx, started, err)
	return out, err
}

// run runs a herdr API command against the backend's machine, discarding stdout.
func (b herdrBackend) run(ctx context.Context, args ...string) error {
	if b.machine == "" {
		return herdrRun(ctx, args...)
	}
	started := time.Now()
	err := herdrRun(ctx, b.scoped(args)...)
	b.logCommandFailure(ctx, started, err)
	return err
}

// logCommandFailure is the only machine-aware logging call site. The profile id
// is user-controlled, so it is logged as a debug-only raw id; herdr's stderr
// (labels, hostnames, ssh text) is never read or logged.
func (b herdrBackend) logCommandFailure(ctx context.Context, started time.Time, err error) {
	if err == nil || b.machine == "" {
		return
	}
	logger := logging.Default()
	if !logger.Enabled(ctx, slog.LevelDebug) {
		return
	}
	logging.LogAttrs(ctx, logger, slog.LevelDebug,
		logging.EventMachineCommandFailed, logging.ComponentSession,
		slog.String("machine_id", b.machine),
		slog.String("error_class", logging.ClassifyError(err)),
		slog.Int64("duration_ms", time.Since(started).Milliseconds()))
}
