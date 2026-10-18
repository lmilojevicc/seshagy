//go:build !windows

package remote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/lmilojevicc/seshagy/internal/cli"
	"github.com/lmilojevicc/seshagy/internal/xdg"
)

// ErrNoLauncher reports that no running `seshagy run` launcher is attached to
// the herdr server the caller asked about. Only a launcher can switch the
// user's terminal to another server: a herdr pane runs as a child of the herdr
// server, so it cannot detach the client itself.
var ErrNoLauncher = errors.New("no running `seshagy run` launcher")

// SwitchRequest asks a running launcher to switch its herdr client to another
// target. Host is a host name as listed by `seshagy hosts` ("" or "local" for
// the local server); Session optionally overrides the host's session.
type SwitchRequest struct {
	Host    string `json:"host"`
	Session string `json:"session,omitempty"`
}

// Target identifies the herdr server a launcher is attached to. An empty Alias
// means the local server.
type Target struct {
	Alias   string
	Session string
	// Keybindings is passed to herdr as --remote-keybindings for a remote
	// target; empty leaves herdr's default (local).
	Keybindings string
}

// LaunchOptions configures RunLauncher.
type LaunchOptions struct {
	// Initial is the server to attach to first.
	Initial Target
	// LocalSession is the local herdr session used by the initial target and
	// whenever a remote client exits back to the local server.
	LocalSession string
	// Hosts is the full host list (configured + discovered) used to resolve
	// switch requests and to regenerate the ssh config before a remote client.
	Hosts []Host
}

type switchReply struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// launcherRecord is the on-disk descriptor of a running launcher: it maps the
// herdr socket of the attached local server to the launcher's control socket.
type launcherRecord struct {
	PID         int    `json:"pid"`
	HerdrSocket string `json:"herdr_socket"`
	StartedAt   string `json:"started_at"`
}

type sessionEntry struct {
	Name       string `json:"name"`
	Default    bool   `json:"default"`
	Running    bool   `json:"running"`
	SocketPath string `json:"socket_path"`
}

type sessionList struct {
	Sessions []sessionEntry `json:"sessions"`
}

// startClientProcess is the seam that starts the attached herdr client. Tests
// replace it so no real herdr server is needed.
var startClientProcess = func(args ...string) *exec.Cmd {
	cmd := exec.Command("herdr", args...)
	cmd.Env = clientEnv()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd
}

// herdrSessionList is the seam that reads `herdr session list --json`.
var herdrSessionList = func(ctx context.Context) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "herdr", "session", "list", "--json")
	cmd.Env = clientEnv()
	return cmd.Output()
}

// clientEnv drops every HERDR_* variable so a new client never inherits this
// pane's server identity (herdr reads HERDR_ENV / HERDR_SOCKET_PATH as nesting).
func clientEnv() []string {
	env := os.Environ()
	out := make([]string, 0, len(env))
	for _, entry := range env {
		if strings.HasPrefix(entry, "HERDR_") {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// launcherDir is the state directory holding launcher records and sockets.
func launcherDir() string {
	return filepath.Join(xdg.StateHome(), "seshagy", "launchers")
}

// Launcher attaches a herdr client to a target and re-attaches it whenever a
// pane asks for another one.
type Launcher struct {
	opts        LaunchOptions
	socketPath  string
	recordPath  string
	herdrSocket string
	signals     chan os.Signal
}

// RunLauncher runs the `seshagy run` launcher until the user quits the last
// local client. It refuses to start inside a herdr pane: a popup/plugin pane
// cannot switch the terminal that hosts it.
func RunLauncher(ctx context.Context, opts LaunchOptions) error {
	if os.Getenv("HERDR_ENV") == "1" {
		return errors.New(
			"refusing to run inside a herdr pane: `seshagy run` must own the terminal " +
				"(run it from a plain terminal or a keybind outside herdr)",
		)
	}
	dir := launcherDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create launcher state dir: %w", err)
	}
	pid := strconv.Itoa(os.Getpid())
	launcher := &Launcher{
		opts:       opts,
		socketPath: filepath.Join(dir, pid+".sock"),
		recordPath: filepath.Join(dir, pid+".json"),
		signals:    make(chan os.Signal, 4),
	}
	signal.Notify(launcher.signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(launcher.signals)
	return launcher.loop(ctx)
}

func (l *Launcher) loop(ctx context.Context) error {
	current := l.opts.Initial
	for {
		if err := l.prepare(current); err != nil {
			return err
		}
		result, err := l.runClient(ctx, current)
		if err != nil {
			return err
		}
		switch {
		case result.stop:
			return result.stopErr
		case result.next != nil:
			current = *result.next
		case current.Alias != "":
			// The remote client exited: fall back to the local server.
			current = Target{Session: l.opts.LocalSession}
		default:
			return result.err
		}
	}
}

// prepare validates the target and, for a remote target, regenerates the ssh
// config the generated alias lives in.
func (l *Launcher) prepare(target Target) error {
	if target.Alias == "" {
		return nil
	}
	host, ok := Find(l.opts.Hosts, target.Alias)
	if !ok {
		return fmt.Errorf("unknown host: %s", target.Alias)
	}
	if _, err := WriteSSHConfig(l.opts.Hosts); err != nil {
		return fmt.Errorf("write generated ssh config: %w", err)
	}
	if !host.FromSSHConfig && !IncludeIsSetup() {
		cli.Warn("~/.ssh/config does not include the generated seshagy ssh config; " +
			"run `seshagy hosts setup` to add it")
	}
	return nil
}

// clientResult reports why a client stopped.
type clientResult struct {
	next    *Target // pending switch request, if any
	stop    bool    // a signal or context cancellation ended the launcher
	stopErr error   // context error when stop is set
	err     error   // client exit status when stop is unset
}

func (l *Launcher) runClient(ctx context.Context, target Target) (clientResult, error) {
	client := startClientProcess(clientArgs(target)...)
	if err := client.Start(); err != nil {
		return clientResult{}, fmt.Errorf("start herdr: %w", err)
	}
	l.herdrSocket = l.resolveHerdrSocket(ctx, target)
	listener, err := l.listen()
	if err != nil {
		killClient(client)
		return clientResult{}, err
	}
	if err := l.writeRecord(); err != nil {
		_ = listener.Close()
		killClient(client)
		return clientResult{}, err
	}
	switches := make(chan Target, 1)
	go l.serve(listener, switches)
	result := l.wait(ctx, client, switches)
	// A request that arrived while the client was already exiting is still
	// queued: honor it instead of falling back to the local server.
	if !result.stop {
		select {
		case target := <-switches:
			result.next = &target
		default:
		}
	}
	_ = listener.Close()
	l.removeRecord()
	return result, nil
}

// killClient kills a client the launcher could not attach to and reaps it, so
// no zombie is left behind.
func killClient(client *exec.Cmd) {
	_ = client.Process.Kill()
	_ = client.Wait()
}

func clientArgs(target Target) []string {
	switch {
	case target.Alias == "" && target.Session == "":
		return nil
	case target.Alias == "":
		return []string{"--session", target.Session}
	default:
		args := []string{"--remote", target.Alias}
		if target.Session != "" {
			args = append(args, "--session", target.Session)
		}
		if target.Keybindings != "" {
			args = append(args, "--remote-keybindings", target.Keybindings)
		}
		return args
	}
}

func (l *Launcher) wait(
	ctx context.Context,
	client *exec.Cmd,
	switches <-chan Target,
) clientResult {
	done := make(chan error, 1)
	go func() { done <- client.Wait() }()
	var pending *Target
	for {
		select {
		case target := <-switches:
			pending = &target
			// SIGTERM on a herdr client is a clean detach: the server and its
			// panes keep running, and the launcher gets another turn.
			_ = client.Process.Signal(syscall.SIGTERM)
		case sig := <-l.signals:
			signal.Stop(l.signals)
			_ = client.Process.Signal(sig)
			<-done
			return clientResult{stop: true}
		case <-ctx.Done():
			_ = client.Process.Signal(syscall.SIGTERM)
			<-done
			return clientResult{stop: true, stopErr: ctx.Err()}
		case err := <-done:
			return clientResult{next: pending, err: exitError(err)}
		}
	}
}

// exitError turns a client exit status into a launcher error, leaving a clean
// exit (status 0) as nil.
func exitError(err error) error {
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 0 {
		return nil
	}
	return fmt.Errorf("herdr exited: %w", err)
}

func (l *Launcher) serve(listener net.Listener, out chan Target) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go l.handle(conn, out)
	}
}

// handle answers one switch request. The reply is written before the pending
// target is pushed so the caller learns the outcome before its client detaches.
func (l *Launcher) handle(conn net.Conn, out chan Target) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(switchDeadline))
	var request SwitchRequest
	if err := json.NewDecoder(io.LimitReader(conn, switchMaxRequestBytes)).
		Decode(&request); err != nil {
		writeReply(conn, switchReply{Error: "malformed switch request"})
		return
	}
	target, err := l.resolve(request)
	if err != nil {
		writeReply(conn, switchReply{Error: err.Error()})
		return
	}
	writeReply(conn, switchReply{OK: true})
	select {
	case out <- target:
	default:
		// Keep only the newest pending target.
		select {
		case <-out:
		default:
		}
		select {
		case out <- target:
		default:
		}
	}
}

func (l *Launcher) resolve(request SwitchRequest) (Target, error) {
	if request.Host == "" || request.Host == "local" {
		session := request.Session
		if session == "" {
			session = l.opts.LocalSession
		}
		return Target{Session: session}, nil
	}
	host, ok := Find(l.opts.Hosts, request.Host)
	if !ok {
		return Target{}, fmt.Errorf("unknown host: %s", request.Host)
	}
	return host.LaunchTarget(request.Session), nil
}

func writeReply(conn net.Conn, reply switchReply) {
	_ = json.NewEncoder(conn).Encode(reply)
}

// resolveHerdrSocket records which local herdr server this launcher is attached
// to, so panes of that server can find the launcher through
// $HERDR_SOCKET_PATH. Remote sessions keep the local socket: a socket path on
// the remote machine cannot address this launcher.
func (l *Launcher) resolveHerdrSocket(ctx context.Context, target Target) string {
	session := l.opts.LocalSession
	if target.Alias == "" {
		session = target.Session
	}
	deadline := time.Now().Add(socketDiscoverTimeout)
	for {
		socket, err := lookupHerdrSocket(ctx, session)
		if err == nil && socket != "" {
			return socket
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return ""
		}
		select {
		case <-ctx.Done():
			return ""
		case <-time.After(socketDiscoverInterval):
		}
	}
}

// lookupHerdrSocket resolves the socket path of a local herdr session ("" is
// the default session), preferring a running session.
func lookupHerdrSocket(ctx context.Context, session string) (string, error) {
	out, err := herdrSessionList(ctx)
	if err != nil {
		return "", err
	}
	var list sessionList
	if err := json.Unmarshal(out, &list); err != nil {
		return "", fmt.Errorf("parse herdr session list: %w", err)
	}
	want := session
	if want == "" {
		want = "default"
	}
	fallback := ""
	for _, entry := range list.Sessions {
		name := entry.Name
		if name == "" && entry.Default {
			name = "default"
		}
		if name != want || entry.SocketPath == "" {
			continue
		}
		if entry.Running {
			return entry.SocketPath, nil
		}
		if fallback == "" {
			fallback = entry.SocketPath
		}
	}
	return fallback, nil
}

func (l *Launcher) listen() (net.Listener, error) {
	_ = os.Remove(l.socketPath)
	listener, err := net.Listen("unix", l.socketPath)
	if err != nil {
		return nil, fmt.Errorf("listen on launcher socket: %w", err)
	}
	return listener, nil
}

func (l *Launcher) writeRecord() error {
	record := launcherRecord{
		PID:         os.Getpid(),
		HerdrSocket: l.herdrSocket,
		StartedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode launcher record: %w", err)
	}
	if err := os.WriteFile(l.recordPath, data, 0o600); err != nil {
		return fmt.Errorf("write launcher record: %w", err)
	}
	return nil
}

func (l *Launcher) removeRecord() {
	_ = os.Remove(l.socketPath)
	_ = os.Remove(l.recordPath)
}

// RequestSwitch asks the launcher attached to herdrSocket to switch to request.
// It is called from inside a herdr pane, where HERDR_SOCKET_PATH identifies the
// server the pane belongs to.
func RequestSwitch(ctx context.Context, request SwitchRequest, herdrSocket string) error {
	record, err := findLauncher(herdrSocket)
	if err != nil {
		return err
	}
	dialer := &net.Dialer{Timeout: switchDeadline}
	conn, err := dialer.DialContext(ctx, "unix", launcherSocketPath(record.PID))
	if err != nil {
		return fmt.Errorf("contact launcher: %w", err)
	}
	defer func() { _ = conn.Close() }()
	deadline := time.Now().Add(switchDeadline)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = conn.SetDeadline(deadline)
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return fmt.Errorf("send switch request: %w", err)
	}
	var reply switchReply
	if err := json.NewDecoder(io.LimitReader(conn, switchMaxReplyBytes)).
		Decode(&reply); err != nil {
		return fmt.Errorf("read launcher reply: %w", err)
	}
	if !reply.OK {
		if reply.Error == "" {
			return errors.New("launcher rejected the switch request")
		}
		return errors.New(reply.Error)
	}
	return nil
}

func launcherSocketPath(pid int) string {
	return filepath.Join(launcherDir(), strconv.Itoa(pid)+".sock")
}

// findLauncher returns the most recently started live launcher attached to
// herdrSocket, cleaning up records whose process is gone.
func findLauncher(herdrSocket string) (launcherRecord, error) {
	if herdrSocket == "" {
		return launcherRecord{}, fmt.Errorf(
			"%w (HERDR_SOCKET_PATH is not set; run this from a herdr pane)",
			ErrNoLauncher,
		)
	}
	var best launcherRecord
	var bestStarted time.Time
	for _, record := range liveLaunchers() {
		if record.HerdrSocket != herdrSocket {
			continue
		}
		started, err := time.Parse(time.RFC3339Nano, record.StartedAt)
		if err != nil {
			continue
		}
		if best.PID == 0 || started.After(bestStarted) {
			best, bestStarted = record, started
		}
	}
	if best.PID == 0 {
		return launcherRecord{}, fmt.Errorf(
			"%w for this herdr server; start one with `seshagy run`", ErrNoLauncher,
		)
	}
	return best, nil
}

// liveLaunchers reads the launcher records, dropping (and cleaning up) entries
// whose process is no longer alive.
func liveLaunchers() []launcherRecord {
	dir := launcherDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var records []launcherRecord
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var record launcherRecord
		if err := json.Unmarshal(data, &record); err != nil || record.PID <= 0 {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
			continue
		}
		if !processAlive(record.PID) {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
			_ = os.Remove(launcherSocketPath(record.PID))
			continue
		}
		records = append(records, record)
	}
	return records
}

func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

const (
	switchDeadline         = 5 * time.Second
	switchMaxRequestBytes  = 4 << 10
	switchMaxReplyBytes    = 4 << 10
	socketDiscoverTimeout  = 3 * time.Second
	socketDiscoverInterval = 100 * time.Millisecond
)
