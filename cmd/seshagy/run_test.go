package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/remote"
)

// writeHostConfig writes a config.toml with the given [[hosts]] entries.
func writeHostConfig(t *testing.T, hosts string) {
	t.Helper()
	path := appconfig.Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(hosts), 0o600); err != nil {
		t.Fatal(err)
	}
}

// writeUserSSHConfig writes ~/.ssh/config with the given content.
func writeUserSSHConfig(t *testing.T, content string) {
	t.Helper()
	path := remote.UserSSHConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestParseRunCommand(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    runCommand
		wantErr bool
	}{
		{name: "bare", args: nil, want: runCommand{}},
		{name: "host", args: []string{"homelab"}, want: runCommand{host: "homelab"}},
		{
			name: "session",
			args: []string{"--session", "work"},
			want: runCommand{session: "work"},
		},
		{
			name: "host then flag",
			args: []string{"homelab", "--session", "work"},
			want: runCommand{host: "homelab", session: "work"},
		},
		{
			name: "flag then host",
			args: []string{"--session", "work", "homelab"},
			want: runCommand{host: "homelab", session: "work"},
		},
		{name: "missing session value", args: []string{"--session"}, wantErr: true},
		{name: "unknown flag", args: []string{"--bogus"}, wantErr: true},
		{name: "two hosts", args: []string{"a", "b"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseRunCommand(test.args)
			if test.wantErr {
				if err == nil {
					t.Fatalf("parseRunCommand(%q) = %+v, want error", test.args, got)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("parseRunCommand(%q) = %+v, %v; want %+v", test.args, got, err, test.want)
			}
		})
	}
}

func TestRunHostsListsConfiguredAndDiscoveredHosts(t *testing.T) {
	manifestTestDirs(t)
	writeHostConfig(t, `
[[hosts]]
name = "homelab"
target = "milo@192.168.1.10:2222"
session = "work"

[[hosts]]
name = "irohbox"
target = "8f14e45fceea167a5a36dedd4bea2543a1b2c3d4e5f60718293a4b5c6d7e8f90"
transport = "iroh"
`)
	writeUserSSHConfig(t, "Host homelab-lan\n  HostName 192.168.1.10\n\nHost *.wild\n")

	out, err := captureStdout(t, func() error { return run([]string{"hosts"}) })
	if err != nil {
		t.Fatalf("run(hosts) error = %v", err)
	}
	if !strings.Contains(out, "homelab  ssh  milo@192.168.1.10:2222  config  session=work") {
		t.Fatalf("hosts output missing the configured host:\n%s", out)
	}
	if !strings.Contains(out, "irohbox  iroh") {
		t.Fatalf("hosts output missing the iroh host:\n%s", out)
	}
	if !strings.Contains(out, "homelab-lan  ssh  homelab-lan  ssh_config") {
		t.Fatalf("hosts output missing the discovered host:\n%s", out)
	}
	if strings.Contains(out, "wild") {
		t.Fatalf("hosts output listed a wildcard pattern:\n%s", out)
	}

	jsonOut, err := captureStdout(t, func() error { return run([]string{"hosts", "--json"}) })
	if err != nil {
		t.Fatalf("run(hosts --json) error = %v", err)
	}
	var payload struct {
		Ok    bool `json:"ok"`
		Hosts []struct {
			Name, Target, Transport, Session, Alias, Source string
		} `json:"hosts"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(jsonOut)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, jsonOut)
	}
	if !payload.Ok || len(payload.Hosts) != 3 {
		t.Fatalf("hosts payload = %+v", payload)
	}
	first := payload.Hosts[0]
	if first.Name != "homelab" || first.Alias != "seshagy-homelab" || first.Source != "config" {
		t.Fatalf("first host = %+v", first)
	}
	if last := payload.Hosts[2]; last.Name != "homelab-lan" || last.Source != "ssh_config" ||
		last.Alias != "homelab-lan" {
		t.Fatalf("last host = %+v", last)
	}
}

func TestRunHostsWarnsAboutInvalidEntries(t *testing.T) {
	manifestTestDirs(t)
	writeHostConfig(t, `
[[hosts]]
name = "bad name"
target = "host"

[[hosts]]
name = "dup"
target = "a"

[[hosts]]
name = "dup"
target = "b"

[[hosts]]
name = "short-iroh"
target = "not-an-endpoint-id"
transport = "iroh"
`)
	stderr, err := captureStderr(t, func() error { return run([]string{"hosts"}) })
	if err != nil {
		t.Fatalf("run(hosts) error = %v", err)
	}
	for _, want := range []string{
		`hosts "bad name": name may only contain`,
		`hosts "dup": duplicate host name`,
		`hosts "short-iroh": iroh target host does not look like`,
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, stderr)
		}
	}
	// A malformed entry must not hide the valid one behind it.
	out, err := captureStdout(t, func() error { return run([]string{"hosts"}) })
	if err != nil {
		t.Fatalf("run(hosts) error = %v", err)
	}
	if !strings.Contains(out, "dup  ssh  a") || strings.Contains(out, "dup  ssh  b") {
		t.Fatalf("duplicate host resolution wrong:\n%s", out)
	}
}

func TestRunHostsSetupWritesGeneratedConfigAndInclude(t *testing.T) {
	manifestTestDirs(t)
	writeHostConfig(t, `
[[hosts]]
name = "homelab"
target = "milo@192.168.1.10"
`)
	writeUserSSHConfig(t, "Host homelab-lan\n  HostName 192.168.1.10\n")

	out, err := captureStdout(t, func() error { return run([]string{"hosts", "setup"}) })
	if err != nil {
		t.Fatalf("run(hosts setup) error = %v", err)
	}
	path := remote.SSHConfigPath()
	if !strings.Contains(out, path) {
		t.Fatalf("setup output missing %q:\n%s", path, out)
	}
	generated, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated ssh config: %v", err)
	}
	if !strings.Contains(string(generated), "Host seshagy-homelab") {
		t.Fatalf("generated ssh config missing the alias:\n%s", generated)
	}
	user, err := os.ReadFile(remote.UserSSHConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(user), remote.IncludeLine()+"\n") {
		t.Fatalf("~/.ssh/config does not include the generated config:\n%s", user)
	}
	if !strings.Contains(string(user), "Host homelab-lan") {
		t.Fatalf("~/.ssh/config content was lost:\n%s", user)
	}

	jsonOut, err := captureStdout(t, func() error {
		return run([]string{"hosts", "setup", "--json"})
	})
	if err != nil {
		t.Fatalf("run(hosts setup --json) error = %v", err)
	}
	var payload struct {
		Ok           bool   `json:"ok"`
		SSHConfig    string `json:"ssh_config"`
		UserConfig   string `json:"user_config"`
		Changed      bool   `json:"changed"`
		IncludeAdded bool   `json:"include_added"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(jsonOut)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, jsonOut)
	}
	if !payload.Ok || payload.Changed || payload.IncludeAdded ||
		payload.SSHConfig != path || payload.UserConfig != remote.UserSSHConfigPath() {
		t.Fatalf("repeated setup payload = %+v", payload)
	}
}

func TestRunHostsUsageErrors(t *testing.T) {
	manifestTestDirs(t)
	for _, args := range [][]string{
		{"hosts", "setup", "extra"},
		{"hosts", "switch"},
		{"hosts", "switch", "a", "b"},
		{"hosts", "bogus"},
		{"run", "--bogus"},
		{"run", "a", "b"},
	} {
		if err := run(args); err == nil {
			t.Fatalf("run(%q) expected an error", args)
		}
	}
}

func TestRunHostsSwitchRequiresLauncher(t *testing.T) {
	manifestTestDirs(t)
	t.Setenv("HERDR_SOCKET_PATH", "")
	// Not inside herdr: there is no HERDR_SOCKET_PATH to match a launcher.
	err := run([]string{"hosts", "switch", "homelab"})
	if err == nil || !strings.Contains(err.Error(), "HERDR_SOCKET_PATH") {
		t.Fatalf("run(hosts switch) error = %v, want a HERDR_SOCKET_PATH hint", err)
	}
	t.Setenv("HERDR_SOCKET_PATH", "/tmp/nowhere/herdr.sock")
	err = run([]string{"hosts", "switch", "local"})
	if err == nil || !strings.Contains(err.Error(), "seshagy run") {
		t.Fatalf("run(hosts switch local) error = %v, want a `seshagy run` hint", err)
	}
}

func TestRunRefusesInsideHerdrPane(t *testing.T) {
	manifestTestDirs(t)
	t.Setenv("HERDR_ENV", "1")
	err := run([]string{"run"})
	if err == nil || !strings.Contains(err.Error(), "herdr pane") {
		t.Fatalf("run(run) error = %v, want a nested-session refusal", err)
	}
}

func TestRunUnknownHost(t *testing.T) {
	manifestTestDirs(t)
	writeHostConfig(t, `
[[hosts]]
name = "homelab"
target = "milo@192.168.1.10"
`)
	err := run([]string{"run", "nope"})
	if err == nil || !strings.Contains(err.Error(), "unknown host: nope") {
		t.Fatalf("run(run nope) error = %v, want unknown host", err)
	}
}
