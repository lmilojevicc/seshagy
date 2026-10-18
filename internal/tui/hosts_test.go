package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/remote"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

// stubHostSource replaces the host source seam for one test.
func stubHostSource(t *testing.T, hosts []remote.Host) {
	t.Helper()
	previous := loadRemoteHosts
	loadRemoteHosts = func(appconfig.Config) []remote.Host { return hosts }
	t.Cleanup(func() { loadRemoteHosts = previous })
}

func testHosts() []remote.Host {
	return []remote.Host{{
		Name:      "homelab",
		Target:    "user@192.168.1.1",
		Transport: remote.TransportSSH,
		Session:   "work",
	}}
}

// withHosts loads hosts through the message the picker's own host source
// delivers, so the item list is rebuilt exactly as it is at startup.
func withHosts(m Model, hosts []remote.Host) Model {
	model, _ := m.Update(hostsMsg{hosts: hosts})
	return model.(Model)
}

func TestHostsCmdReadsTheHostSource(t *testing.T) {
	stubHostSource(t, testHosts())
	msg, ok := hostsCmd(appconfig.Default())().(hostsMsg)
	if !ok {
		t.Fatal("hostsCmd() did not return a hostsMsg")
	}
	if len(msg.hosts) != 1 || msg.hosts[0].Name != "homelab" {
		t.Fatalf("hosts = %#v", msg.hosts)
	}
}

// configWithHosts points HOME at a temp dir holding a ~/.ssh/config host and
// returns a config with one [[hosts]] entry of its own.
func configWithHosts(t *testing.T) appconfig.Config {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	userConfig := "Host bastion\n  HostName bastion.example.com\n"
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte(userConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := appconfig.Default()
	cfg.Hosts = []appconfig.HostConfig{{
		Name: "homelab", Target: "milo@192.168.1.10", Session: "work",
	}}
	return cfg
}

func TestLoadRemoteHostsMergesConfigAndSSHConfig(t *testing.T) {
	cfg := configWithHosts(t)
	hosts := loadRemoteHosts(cfg)
	if len(hosts) != 2 {
		t.Fatalf("hosts = %#v, want the configured host and the ssh_config host", hosts)
	}
	if hosts[0].Name != "homelab" || hosts[0].Alias() != "seshagy-homelab" ||
		hosts[0].Session != "work" || hosts[0].FromSSHConfig {
		t.Fatalf("configured host = %#v", hosts[0])
	}
	if hosts[1].Name != "bastion" || hosts[1].Alias() != "bastion" ||
		!hosts[1].FromSSHConfig {
		t.Fatalf("discovered host = %#v", hosts[1])
	}

	disabled := false
	cfg.Remote.SSHConfigHosts = &disabled
	if hosts := loadRemoteHosts(cfg); len(hosts) != 1 {
		t.Fatalf("ssh_config_hosts = false listed %#v", hosts)
	}
}

func TestPickerShowsHostsFromConfig(t *testing.T) {
	cfg := configWithHosts(t)
	m := New(WithConfig(cfg), WithMultiplexer(newFakeMux()))
	m.width, m.height = 120, 32

	model, _ := m.Update(hostsCmd(cfg)())
	m = model.(Model)
	m.loadedOnce = true // render rows instead of the loading state
	out := sessionmgr.StripANSI(m.View())
	// The row names the host and marks it as a remote ssh host with the alias
	// the launcher is handed; the configured target itself is in its preview.
	for _, want := range []string{"homelab", "seshagy-homelab", "bastion"} {
		if !strings.Contains(out, want) {
			t.Fatalf("picker view missing %q:\n%s", want, out)
		}
	}
	// The hosts scope is reached with ctrl+e and appears in the prompt window
	// title once it is active; it is no longer spelled out as a sigil hint.
	m, _ = press(t, m, "ctrl+e")
	if m.scope != scopeHosts {
		t.Fatalf("ctrl+e: scope = %v, want the hosts scope", m.scope)
	}
	if !strings.Contains(sessionmgr.StripANSI(m.View()), "seshagy · remotes") {
		t.Fatalf(
			"hosts scope is not named in the prompt title:\n%s",
			sessionmgr.StripANSI(m.View()),
		)
	}
}

func TestHostsMsgFilesHostRowsBeforeDirs(t *testing.T) {
	m := newTestModel(t)
	m = withItems(m, []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "demo", Target: "demo"},
		{Kind: sessionmgr.KindFD, Path: "/tmp/demo", Name: "/tmp/demo"},
	})
	m = withHosts(m, testHosts())
	if len(m.hosts) != 1 {
		t.Fatalf("hosts = %#v", m.hosts)
	}

	rows := m.rows()
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	item := rows[1].item
	if item.Kind != sessionmgr.KindHost {
		t.Fatalf("second row kind = %q, want host", item.Kind)
	}
	if item.Name != "homelab" || item.Location != remote.TransportSSH ||
		item.Target != "seshagy-homelab" || item.Path != "user@192.168.1.1" {
		t.Fatalf("host item = %#v", item)
	}

	m.scope = scopeHosts
	hostRows := m.rows()
	if len(hostRows) != 1 || hostRows[0].item.Key() != item.Key() {
		t.Fatalf("hosts scope rows = %#v", hostRows)
	}
}

func TestHostsMsgRefreshesThePreview(t *testing.T) {
	m := newTestModel(t)
	if _, cmd := m.Update(hostsMsg{hosts: testHosts()}); cmd == nil {
		t.Fatal("hostsMsg requested no preview for the newly selected host")
	}
}

func TestActivateHostByBackend(t *testing.T) {
	t.Run("herdr asks the launcher to switch", func(t *testing.T) {
		t.Setenv("HERDR_SOCKET_PATH", "/tmp/herdr.sock")
		var got remote.SwitchRequest
		var gotSocket string
		previous := requestSwitch
		requestSwitch = func(_ context.Context, req remote.SwitchRequest, socket string) error {
			got, gotSocket = req, socket
			return nil
		}
		t.Cleanup(func() { requestSwitch = previous })

		mux := newFakeMux()
		mux.kind = sessionmgr.BackendHerdr
		m := withHosts(newTestModelWith(t, mux), testHosts())

		model, cmd := m.activateSelected()
		m = model.(Model)
		if cmd == nil {
			t.Fatal("host activation returned no command")
		}
		msg := cmd()
		if got.Host != "homelab" || got.Session != "work" {
			t.Fatalf("switch request = %#v", got)
		}
		if gotSocket != "/tmp/herdr.sock" {
			t.Fatalf("switch socket = %q", gotSocket)
		}

		_, quit := m.Update(msg)
		if quit == nil {
			t.Fatal("a successful switch did not quit the picker")
		}
		if _, ok := quit().(tea.QuitMsg); !ok {
			t.Fatalf("switch quit command = %#v", quit())
		}
	})

	t.Run("herdr without a launcher explains how to start one", func(t *testing.T) {
		previous := requestSwitch
		requestSwitch = func(context.Context, remote.SwitchRequest, string) error {
			return fmt.Errorf("switch: %w", remote.ErrNoLauncher)
		}
		t.Cleanup(func() { requestSwitch = previous })

		mux := newFakeMux()
		mux.kind = sessionmgr.BackendHerdr
		m := withHosts(newTestModelWith(t, mux), testHosts())

		model, cmd := m.activateSelected()
		m = model.(Model)
		if cmd == nil {
			t.Fatal("host activation returned no command")
		}
		model, quit := m.Update(cmd())
		m = model.(Model)
		if quit != nil {
			t.Fatalf("failed switch quit the picker: %v", quit)
		}
		if got := latestNotificationText(
			m,
		); got != "start herdr with `seshagy run` to switch servers" {
			t.Fatalf("notification = %q", got)
		}
		if sev := latestNotificationSeverity(m); sev != sevError {
			t.Fatalf("notification severity = %v, want error", sev)
		}
	})

	t.Run("tmux opens an ssh window", func(t *testing.T) {
		var gotName, gotAlias string
		previous := runSSHWindow
		runSSHWindow = func(name, alias string) error {
			gotName, gotAlias = name, alias
			return nil
		}
		t.Cleanup(func() { runSSHWindow = previous })

		m := withHosts(newTestModel(t), testHosts())

		_, cmd := m.activateSelected()
		if cmd == nil {
			t.Fatal("host activation returned no command")
		}
		if msg := cmd().(hostWindowMsg); msg.err != nil {
			t.Fatalf("openSSHWindowCmd = %#v", msg)
		}
		if gotName != "homelab" || gotAlias != "seshagy-homelab" {
			t.Fatalf("ssh window = %q %q", gotName, gotAlias)
		}
	})

	t.Run("tmux window failure is toasted", func(t *testing.T) {
		previous := runSSHWindow
		runSSHWindow = func(string, string) error { return errors.New("no server") }
		t.Cleanup(func() { runSSHWindow = previous })

		m := withHosts(newTestModel(t), testHosts())
		_, cmd := m.activateSelected()
		model, _ := m.Update(cmd())
		m = model.(Model)
		if got := latestNotificationText(m); got != "no server" {
			t.Fatalf("notification = %q", got)
		}
	})

	t.Run("no multiplexer refuses", func(t *testing.T) {
		mux := newFakeMux()
		mux.kind = sessionmgr.BackendNone
		m := withHosts(newTestModelWith(t, mux), testHosts())

		model, cmd := m.activateSelected()
		if cmd != nil {
			t.Fatalf("host activation returned a command: %v", cmd)
		}
		if got := latestNotificationText(
			model.(Model),
		); got != "cannot open remote hosts without tmux or herdr" {
			t.Fatalf("notification = %q", got)
		}
	})

	t.Run("an unknown host row warns", func(t *testing.T) {
		m := withItems(newTestModel(t), []sessionmgr.Item{
			{Kind: sessionmgr.KindHost, Name: "ghost", Target: "ghost"},
		})
		model, cmd := m.activateSelected()
		if cmd != nil {
			t.Fatalf("host activation returned a command: %v", cmd)
		}
		if got := latestNotificationText(model.(Model)); got != "unknown host ghost" {
			t.Fatalf("notification = %q", got)
		}
	})
}

func TestHostPreviewRendersConfigWithoutNetwork(t *testing.T) {
	t.Run("configured host shows its generated block", func(t *testing.T) {
		m := newTestModel(t)
		_, cmd := m.Update(hostsMsg{hosts: testHosts()})
		if cmd == nil {
			t.Fatal("loading hosts requested no preview")
		}
		msg := cmd().(previewMsg)
		if msg.err != nil {
			t.Fatalf("host preview error = %v", msg.err)
		}
		if msg.key != "host:homelab" {
			t.Fatalf("preview key = %q", msg.key)
		}
		for _, want := range []string{
			"transport  ssh",
			"target     user@192.168.1.1",
			"session    work",
			"alias      seshagy-homelab",
			"Host seshagy-homelab",
			"  HostName 192.168.1.1",
			"  User user",
		} {
			if !strings.Contains(msg.preview, want) {
				t.Fatalf("preview missing %q:\n%s", want, msg.preview)
			}
		}
	})

	t.Run("discovered host points at the user's ssh config", func(t *testing.T) {
		m := newTestModel(t)
		_, cmd := m.Update(hostsMsg{hosts: []remote.Host{{
			Name: "gitbox", Target: "gitbox", Transport: remote.TransportSSH, FromSSHConfig: true,
		}}})
		if cmd == nil {
			t.Fatal("loading hosts requested no preview")
		}
		msg := cmd().(previewMsg)
		if msg.key != "host:gitbox" {
			t.Fatalf("preview key = %q", msg.key)
		}
		if !strings.Contains(msg.preview, "# defined in ~/.ssh/config") {
			t.Fatalf("preview = %q", msg.preview)
		}
		if strings.Contains(msg.preview, "Host gitbox") {
			t.Fatalf("discovered host rendered a generated block:\n%s", msg.preview)
		}
	})
}

// TestHostRowShowsTheTransportAndAlias pins the host item: the bright name and,
// two spaces after it, the transport and the alias the launcher is handed. A host
// whose alias is its own name (an ssh_config entry) shows its transport alone, and
// either host is a single line.
func TestHostRowShowsTheTransportAndAlias(t *testing.T) {
	m := withHosts(newTestModel(t), testHosts())
	m.loadedOnce = true // render rows instead of the loading state
	out := sessionmgr.StripANSI(m.View())
	if !strings.Contains(out, "homelab  ssh · seshagy-homelab") {
		t.Fatalf("host row missing its transport and alias:\n%s", out)
	}
	if strings.Contains(out, "  ssh\n") {
		t.Fatalf("host row spills onto a second line:\n%s", out)
	}

	// A host discovered in ~/.ssh/config is used under its own name, so the alias
	// is dropped and the transport is all the row has left to say.
	m = withHosts(newTestModel(t), []remote.Host{{
		Name:          "gitbox",
		Target:        "gitbox",
		Transport:     remote.TransportSSH,
		FromSSHConfig: true,
	}})
	m.loadedOnce = true
	out = sessionmgr.StripANSI(m.View())
	row := resultRowLine(t, out, "gitbox")
	if !strings.HasSuffix(row, "gitbox  ssh") {
		t.Fatalf("host row without a distinct alias = %q, want its transport", row)
	}
	if strings.Contains(row, "·") || strings.Contains(row, "seshagy-gitbox") {
		t.Fatalf("host row repeats an alias it does not have: %q", row)
	}
}
