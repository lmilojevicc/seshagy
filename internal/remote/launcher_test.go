//go:build !windows

package remote

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// clientRecorder records the argv of every client the launcher starts and
// decides what those clients do.
type clientRecorder struct {
	mu     sync.Mutex
	args   [][]string
	client func() *exec.Cmd
}

func (r *clientRecorder) start(args ...string) *exec.Cmd {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.args = append(r.args, append([]string(nil), args...))
	if r.client != nil {
		return r.client()
	}
	// A real client blocks until it detaches on SIGTERM, which is what sleep
	// does.
	return exec.Command("sleep", "60")
}

func (r *clientRecorder) started() [][]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([][]string(nil), r.args...)
}

func (r *clientRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.args)
}

// exitAfter makes every client exit on its own (as if the user quit herdr).
func (r *clientRecorder) exitAfter(delay time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.client = func() *exec.Cmd {
		return exec.Command("sleep", strconv.FormatFloat(delay.Seconds(), 'f', -1, 64))
	}
}

// launcherTestEnv points the launcher state dir and ssh config at throwaway
// dirs and stubs the herdr client and `herdr session list` so no real server is
// needed. The temp dir is deliberately short: the launcher control socket has
// to fit in the unix sun_path limit (104 bytes on macOS).
func launcherTestEnv(t *testing.T, socket string) *clientRecorder {
	t.Helper()
	base, err := os.MkdirTemp("", "s")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	home := filepath.Join(base, "h")
	for _, dir := range []string{
		filepath.Join(base, "s"),
		filepath.Join(home, ".config"),
	} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(base, "s"))
	t.Setenv("HERDR_ENV", "")

	recorder := &clientRecorder{}
	start := startClientProcess
	startClientProcess = recorder.start
	list := herdrSessionList
	herdrSessionList = func(context.Context) ([]byte, error) {
		if socket == "" {
			return []byte(`{"sessions":[]}`), nil
		}
		entry := func(name string) map[string]any {
			return map[string]any{
				"name": name, "running": true, "socket_path": socket,
			}
		}
		return json.Marshal(map[string]any{"sessions": []map[string]any{
			entry("default"), entry("scratch"),
		}})
	}
	t.Cleanup(func() {
		startClientProcess = start
		herdrSessionList = list
	})
	return recorder
}

func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func launcherRecords(t *testing.T) []launcherRecord {
	t.Helper()
	entries, err := os.ReadDir(launcherDir())
	if err != nil {
		return nil
	}
	var records []launcherRecord
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(launcherDir(), entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var record launcherRecord
		if err := json.Unmarshal(data, &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	return records
}

func TestClientArgs(t *testing.T) {
	tests := []struct {
		target Target
		want   string
	}{
		{Target{}, ""},
		{Target{Session: "work"}, "--session work"},
		{Target{Alias: "seshagy-homelab"}, "--remote seshagy-homelab"},
		{
			Target{Alias: "seshagy-homelab", Session: "work"},
			"--remote seshagy-homelab --session work",
		},
		{
			Target{Alias: "seshagy-homelab", Keybindings: KeybindingsServer},
			"--remote seshagy-homelab --remote-keybindings server",
		},
		{
			Target{Alias: "seshagy-homelab", Session: "work", Keybindings: KeybindingsLocal},
			"--remote seshagy-homelab --session work --remote-keybindings local",
		},
		// The local server takes no remote flags, even with a mode set.
		{Target{Session: "work", Keybindings: KeybindingsServer}, "--session work"},
	}
	for _, test := range tests {
		got := strings.Join(clientArgs(test.target), " ")
		if got != test.want {
			t.Errorf("clientArgs(%+v) = %q, want %q", test.target, got, test.want)
		}
	}
}

func TestKillClientReapsTheProcessItKills(t *testing.T) {
	client := exec.Command("sleep", "60")
	if err := client.Start(); err != nil {
		t.Fatal(err)
	}
	killClient(client)
	if client.ProcessState == nil {
		t.Fatal("killClient left the process unreaped")
	}
}

func TestRunLauncherRefusesInsideHerdrPane(t *testing.T) {
	launcherTestEnv(t, "")
	t.Setenv("HERDR_ENV", "1")
	err := RunLauncher(context.Background(), LaunchOptions{})
	if err == nil || !strings.Contains(err.Error(), "herdr pane") {
		t.Fatalf("RunLauncher() error = %v, want a nested-session refusal", err)
	}
}

func TestRunLauncherSwitchesBetweenTargets(t *testing.T) {
	socket := "/tmp/seshagy-launcher-test/default/herdr.sock"
	recorder := launcherTestEnv(t, socket)
	hosts := []Host{
		{Name: "homelab", Target: "milo@192.168.1.10", Transport: TransportSSH},
		{Name: "host.tailnet.ts.net", Target: "host.tailnet.ts.net", FromSSHConfig: true},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- RunLauncher(ctx, LaunchOptions{
			Initial:      Target{Alias: "seshagy-homelab", Session: "work"},
			LocalSession: "scratch",
			Hosts:        hosts,
		})
	}()

	waitFor(t, "the launcher record", func() bool { return len(launcherRecords(t)) == 1 })
	record := launcherRecords(t)[0]
	if record.HerdrSocket != socket || record.PID != os.Getpid() {
		t.Fatalf("launcher record = %+v, want socket %q and pid %d", record, socket, os.Getpid())
	}
	if _, err := os.Stat(launcherSocketPath(os.Getpid())); err != nil {
		t.Fatalf("launcher socket missing: %v", err)
	}
	waitFor(t, "the remote client", func() bool { return recorder.count() == 1 })
	if got := strings.Join(
		recorder.started()[0],
		" ",
	); got != "--remote seshagy-homelab --session work" {
		t.Fatalf("initial client args = %q", got)
	}

	// A discovered host switches by its own pattern.
	if err := RequestSwitch(ctx, SwitchRequest{Host: "host.tailnet.ts.net"}, socket); err != nil {
		t.Fatalf("RequestSwitch(tailnet) error = %v", err)
	}
	waitFor(t, "the tailnet client", func() bool { return recorder.count() == 2 })
	if got := strings.Join(recorder.started()[1], " "); got != "--remote host.tailnet.ts.net" {
		t.Fatalf("tailnet client args = %q", got)
	}

	// An unknown host is rejected and the running client keeps its pane.
	if err := RequestSwitch(ctx, SwitchRequest{Host: "nope"}, socket); err == nil ||
		!strings.Contains(err.Error(), "unknown host") {
		t.Fatalf("RequestSwitch(nope) error = %v, want unknown host", err)
	}
	if recorder.count() != 2 {
		t.Fatalf("rejected switch started another client: %v", recorder.started())
	}

	// "local" goes back to the local server on the launcher's local session.
	if err := RequestSwitch(ctx, SwitchRequest{Host: "local"}, socket); err != nil {
		t.Fatalf("RequestSwitch(local) error = %v", err)
	}
	waitFor(t, "the local client", func() bool { return recorder.count() == 3 })
	if got := strings.Join(recorder.started()[2], " "); got != "--session scratch" {
		t.Fatalf("local client args = %q", got)
	}

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("RunLauncher() error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunLauncher did not stop after cancellation")
	}
	if entries, err := os.ReadDir(launcherDir()); err != nil || len(entries) != 0 {
		t.Fatalf("launcher state dir = %v, %v; want empty", entries, err)
	}
	if err := RequestSwitch(ctx, SwitchRequest{}, socket); !errors.Is(err, ErrNoLauncher) {
		t.Fatalf("RequestSwitch() after exit = %v, want ErrNoLauncher", err)
	}
}

func TestRunLauncherFallsBackToLocalWhenRemoteClientExits(t *testing.T) {
	socket := "/tmp/seshagy-launcher-test/fallback/herdr.sock"
	recorder := launcherTestEnv(t, socket)
	recorder.exitAfter(200 * time.Millisecond)
	done := make(chan error, 1)
	go func() {
		done <- RunLauncher(context.Background(), LaunchOptions{
			Initial: Target{Alias: "seshagy-homelab"},
			Hosts:   []Host{{Name: "homelab", Target: "milo@192.168.1.10"}},
		})
	}()

	waitFor(t, "the local client after the remote one exited", func() bool {
		return recorder.count() == 2
	})
	if got := strings.Join(recorder.started()[0], " "); got != "--remote seshagy-homelab" {
		t.Fatalf("initial client args = %q", got)
	}
	if got := strings.Join(recorder.started()[1], " "); got != "" {
		t.Fatalf("fallback client args = %q, want the default local server", got)
	}
	// Quitting the local client ends the launcher.
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunLauncher() error = %v, want nil", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("RunLauncher did not stop after the local client exited")
	}
	if entries, err := os.ReadDir(launcherDir()); err != nil || len(entries) != 0 {
		t.Fatalf("launcher state dir = %v, %v; want empty", entries, err)
	}
}

func TestRunLauncherRegeneratesSSHConfigAndWarnsWithoutInclude(t *testing.T) {
	recorder := launcherTestEnv(t, "/tmp/seshagy-launcher-test/setup/herdr.sock")
	done := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		done <- RunLauncher(ctx, LaunchOptions{
			Initial: Target{Alias: "seshagy-homelab"},
			Hosts:   []Host{{Name: "homelab", Target: "milo@192.168.1.10"}},
		})
	}()
	waitFor(t, "the remote client", func() bool { return recorder.count() == 1 })
	data, err := os.ReadFile(SSHConfigPath())
	if err != nil {
		t.Fatalf("generated ssh config missing: %v", err)
	}
	if !strings.Contains(string(data), "Host seshagy-homelab") {
		t.Fatalf("generated ssh config does not define the alias:\n%s", data)
	}
	if setup := IncludeIsSetup(); setup {
		t.Fatal("IncludeIsSetup() = true without ~/.ssh/config")
	}
	cancel()
	<-done
}

func TestRequestSwitchWithoutLauncher(t *testing.T) {
	launcherTestEnv(t, "")
	if err := RequestSwitch(context.Background(), SwitchRequest{}, "socket"); !errors.Is(
		err,
		ErrNoLauncher,
	) {
		t.Fatalf("RequestSwitch() = %v, want ErrNoLauncher", err)
	}
	err := RequestSwitch(context.Background(), SwitchRequest{}, "")
	if !errors.Is(err, ErrNoLauncher) || !strings.Contains(err.Error(), "HERDR_SOCKET_PATH") {
		t.Fatalf("RequestSwitch(empty socket) = %v, want a HERDR_SOCKET_PATH hint", err)
	}
}

func TestRequestSwitchCleansStaleLauncherRecords(t *testing.T) {
	launcherTestEnv(t, "")
	dir := launcherDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	stale := launcherRecord{
		PID:         1 << 30, // cannot be running
		HerdrSocket: "socket",
		StartedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.Marshal(stale)
	if err != nil {
		t.Fatal(err)
	}
	recordPath := filepath.Join(dir, strconv.Itoa(stale.PID)+".json")
	if err := os.WriteFile(recordPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	socketPath := launcherSocketPath(stale.PID)
	if err := os.WriteFile(socketPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RequestSwitch(context.Background(), SwitchRequest{}, "socket"); !errors.Is(
		err,
		ErrNoLauncher,
	) {
		t.Fatalf("RequestSwitch() = %v, want ErrNoLauncher", err)
	}
	for _, path := range []string{recordPath, socketPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("stale launcher artifact %s not cleaned: %v", path, err)
		}
	}
}

// serveFakeLauncher starts a live child process with a launcher record and a
// control socket that answers requests through answer, returning the requests
// it receives.
func serveFakeLauncher(
	t *testing.T,
	socket, startedAt string,
	answer func(SwitchRequest) switchReply,
) <-chan SwitchRequest {
	t.Helper()
	dir := launcherDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	child := exec.Command("sleep", "60")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Process.Kill() })
	listener, err := net.Listen("unix", launcherSocketPath(child.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	data, err := json.Marshal(launcherRecord{
		PID: child.Process.Pid, HerdrSocket: socket, StartedAt: startedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, strconv.Itoa(child.Process.Pid)+".json"), data, 0o600,
	); err != nil {
		t.Fatal(err)
	}
	received := make(chan SwitchRequest, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		var request SwitchRequest
		if json.NewDecoder(conn).Decode(&request) != nil {
			return
		}
		received <- request
		_ = json.NewEncoder(conn).Encode(answer(request))
	}()
	return received
}

func fakeLauncher(t *testing.T, socket, startedAt string) <-chan SwitchRequest {
	t.Helper()
	return serveFakeLauncher(t, socket, startedAt, func(SwitchRequest) switchReply {
		return switchReply{OK: true}
	})
}

func TestRequestSwitchPicksTheMostRecentLauncher(t *testing.T) {
	launcherTestEnv(t, "")
	fakeLauncher(t, "socket", time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano))
	newest := fakeLauncher(t, "socket", time.Now().UTC().Format(time.RFC3339Nano))

	if err := RequestSwitch(
		context.Background(),
		SwitchRequest{Host: "homelab"},
		"socket",
	); err != nil {
		t.Fatalf("RequestSwitch() error = %v", err)
	}
	select {
	case request := <-newest:
		if request.Host != "homelab" {
			t.Fatalf("request = %+v", request)
		}
	case <-time.After(time.Second):
		t.Fatal("the most recently started launcher did not receive the request")
	}
}

func TestRequestSwitchReportsLauncherErrors(t *testing.T) {
	launcherTestEnv(t, "")
	serveFakeLauncher(t, "socket", time.Now().UTC().Format(time.RFC3339Nano),
		func(SwitchRequest) switchReply {
			return switchReply{Error: "unknown host: nope"}
		})
	err := RequestSwitch(context.Background(), SwitchRequest{Host: "nope"}, "socket")
	if err == nil || err.Error() != "unknown host: nope" {
		t.Fatalf("RequestSwitch() error = %v, want the launcher's message", err)
	}
}

func TestLookupHerdrSocket(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		session string
		want    string
	}{
		{
			"default session",
			`{"sessions":[{"name":"default","default":true,"running":true,"socket_path":"/s/default.sock"}]}`,
			"",
			"/s/default.sock",
		},
		{
			"named session",
			`{"sessions":[{"name":"default","default":true,"running":true,"socket_path":"/s/default.sock"},{"name":"work","running":true,"socket_path":"/s/work.sock"}]}`,
			"work",
			"/s/work.sock",
		},
		{
			"running session wins",
			`{"sessions":[{"name":"work","running":false,"socket_path":"/s/stale.sock"},{"name":"work","running":true,"socket_path":"/s/work.sock"}]}`,
			"work",
			"/s/work.sock",
		},
		{
			"stopped session still has a socket path",
			`{"sessions":[{"name":"work","running":false,"socket_path":"/s/stale.sock"}]}`,
			"work",
			"/s/stale.sock",
		},
		{
			"missing session",
			`{"sessions":[]}`,
			"work",
			"",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			launcherTestEnv(t, "")
			herdrSessionList = func(context.Context) ([]byte, error) {
				return []byte(test.payload), nil
			}
			got, err := lookupHerdrSocket(context.Background(), test.session)
			if err != nil || got != test.want {
				t.Fatalf(
					"lookupHerdrSocket(%q) = %q, %v; want %q",
					test.session,
					got,
					err,
					test.want,
				)
			}
		})
	}
}

func TestHostLaunchTargetCarriesSessionAndKeybindings(t *testing.T) {
	host := Host{
		Name: "homelab", Target: "milo@192.168.1.10", Session: "work",
		Keybindings: KeybindingsServer,
	}
	tests := []struct {
		session string
		want    Target
	}{
		{"", Target{Alias: "seshagy-homelab", Session: "work", Keybindings: "server"}},
		{"other", Target{Alias: "seshagy-homelab", Session: "other", Keybindings: "server"}},
	}
	for _, test := range tests {
		if got := host.LaunchTarget(test.session); got != test.want {
			t.Errorf("LaunchTarget(%q) = %+v, want %+v", test.session, got, test.want)
		}
	}
}
