package remote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// remoteTestEnv points HOME and XDG_CONFIG_HOME at throwaway dirs, with the
// config under HOME (the layout that lets Include paths contract to ~).
func remoteTestEnv(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(filepath.Join(home, ".config"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	return home
}

func TestHostAliasUsesGeneratedAliasExceptForSSHConfigHosts(t *testing.T) {
	configured := Host{Name: "homelab", Target: "user@192.168.1.1"}
	if got := configured.Alias(); got != "seshagy-homelab" {
		t.Fatalf("configured alias = %q, want seshagy-homelab", got)
	}
	discovered := Host{Name: "homelab-lan", FromSSHConfig: true}
	if got := discovered.Alias(); got != "homelab-lan" {
		t.Fatalf("discovered alias = %q, want the host pattern itself", got)
	}
}

func TestFindMatchesNameAndAlias(t *testing.T) {
	hosts := []Host{
		{Name: "homelab", Target: "user@192.168.1.1"},
		{Name: "edge", FromSSHConfig: true},
	}
	for _, name := range []string{"homelab", "seshagy-homelab", "edge"} {
		if _, ok := Find(hosts, name); !ok {
			t.Errorf("Find(%q) = false, want true", name)
		}
	}
	if _, ok := Find(hosts, "seshagy-edge"); ok {
		t.Error("Find(seshagy-edge) = true; discovered hosts keep their own alias")
	}
}

func TestRenderSSHConfigRendersConfiguredHostsInOrder(t *testing.T) {
	hosts := []Host{
		{Name: "homelab", Target: "milo@192.168.1.10:2222", Transport: TransportSSH},
		{Name: "edge", Target: "ssh://root@edge.example.com", Transport: TransportSSH},
		{
			Name:      "irohbox",
			Target:    "8f14e45fceea167a5a36dedd4bea2543a1b2c3d4e5f60718293a4b5c6d7e8f90",
			Transport: TransportIroh,
		},
		{
			Name:         "proxy",
			Target:       "jump.internal",
			Transport:    TransportSSH,
			ProxyCommand: "iroh-ssh proxy %h",
		},
		{Name: "tailnet", Target: "host.tailnet.ts.net", FromSSHConfig: true},
	}
	got, err := RenderSSHConfig(hosts)
	if err != nil {
		t.Fatalf("RenderSSHConfig() error = %v", err)
	}
	want := sshConfigHeader + `

Host seshagy-homelab
  HostName 192.168.1.10
  User milo
  Port 2222

Host seshagy-edge
  HostName edge.example.com
  User root

Host seshagy-irohbox
  HostName 8f14e45fceea167a5a36dedd4bea2543a1b2c3d4e5f60718293a4b5c6d7e8f90
  ProxyCommand iroh-ssh proxy %h

Host seshagy-proxy
  HostName jump.internal
  ProxyCommand iroh-ssh proxy %h
`
	if got != want {
		t.Fatalf("RenderSSHConfig() =\n%s\nwant\n%s", got, want)
	}
	if strings.Contains(got, "tailnet") {
		t.Fatal("discovered hosts must not be rendered: their own config defines them")
	}
}

func TestRenderSSHConfigRejectsUnsafeValues(t *testing.T) {
	tests := []struct {
		name string
		host Host
	}{
		{"empty name", Host{Name: "", Target: "host"}},
		{"name with space", Host{Name: "two words", Target: "host"}},
		{"name leading dash", Host{Name: "-x", Target: "host"}},
		{"empty target", Host{Name: "h", Target: ""}},
		{"target leading dash", Host{Name: "h", Target: "-oProxyCommand=evil"}},
		{"target with space", Host{Name: "h", Target: "user@ho st"}},
		{"target with newline", Host{Name: "h", Target: "host\nHost *"}},
		{"target with control char", Host{Name: "h", Target: "host\x01"}},
		{"target user with space", Host{Name: "h", Target: "us er@host"}},
		{"target non-numeric port", Host{Name: "h", Target: "host:ssh"}},
		{"target port out of range", Host{Name: "h", Target: "host:70000"}},
		{"target url path", Host{Name: "h", Target: "ssh://user@host:22/something"}},
		{"proxy command with newline", Host{Name: "h", Target: "host", ProxyCommand: "a\nb"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := RenderSSHConfig([]Host{test.host}); err == nil {
				t.Fatalf("RenderSSHConfig(%+v) = nil error, want rejection", test.host)
			}
		})
	}
}

func TestParseAddressAcceptedForms(t *testing.T) {
	tests := []struct {
		in   string
		want Address
	}{
		{"host", Address{Host: "host"}},
		{"user@host", Address{User: "user", Host: "host"}},
		{"user@host:2222", Address{User: "user", Host: "host", Port: "2222"}},
		{"host:2222", Address{Host: "host", Port: "2222"}},
		{"ssh://user@host:2222", Address{User: "user", Host: "host", Port: "2222"}},
		{"ssh://host", Address{Host: "host"}},
		{"host.tailnet.ts.net", Address{Host: "host.tailnet.ts.net"}},
		{"[::1]:22", Address{Host: "::1", Port: "22"}},
	}
	for _, test := range tests {
		got, err := ParseAddress(test.in)
		if err != nil || got != test.want {
			t.Errorf("ParseAddress(%q) = %+v, %v; want %+v", test.in, got, err, test.want)
		}
	}
}

func TestWriteSSHConfigWritesOnlyWhenChanged(t *testing.T) {
	remoteTestEnv(t)
	hosts := []Host{{Name: "homelab", Target: "user@192.168.1.1", Transport: TransportSSH}}
	changed, err := WriteSSHConfig(hosts)
	if err != nil || !changed {
		t.Fatalf("WriteSSHConfig() = %v, %v; want changed", changed, err)
	}
	info, err := os.Stat(SSHConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("generated ssh config mode = %v, want 0600", info.Mode().Perm())
	}
	dirInfo, err := os.Stat(filepath.Dir(SSHConfigPath()))
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("generated ssh config dir mode = %v, want 0700", dirInfo.Mode().Perm())
	}
	changed, err = WriteSSHConfig(hosts)
	if err != nil || changed {
		t.Fatalf("second WriteSSHConfig() = %v, %v; want unchanged", changed, err)
	}
	changed, err = WriteSSHConfig([]Host{{Name: "edge", Target: "edge.example.com"}})
	if err != nil || !changed {
		t.Fatalf("third WriteSSHConfig() = %v, %v; want changed", changed, err)
	}
	data, err := os.ReadFile(SSHConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Host seshagy-edge") ||
		strings.Contains(string(data), "seshagy-homelab") {
		t.Fatalf("generated ssh config was not rewritten:\n%s", data)
	}
}

func TestIncludeLineUsesContractedHome(t *testing.T) {
	home := remoteTestEnv(t)
	got := IncludeLine()
	want := "Include ~/.config/seshagy/ssh_config"
	if got != want {
		t.Fatalf("IncludeLine() = %q, want %q", got, want)
	}
	if strings.Contains(got, home) {
		t.Fatalf("IncludeLine() = %q, want ~ contracted", got)
	}
}

func TestIncludeLineKeepsAbsolutePathOutsideHome(t *testing.T) {
	remoteTestEnv(t)
	t.Setenv("XDG_CONFIG_HOME", "/etc/xdg-outside")
	got := IncludeLine()
	if got != "Include /etc/xdg-outside/seshagy/ssh_config" {
		t.Fatalf("IncludeLine() = %q, want the real path when it is not under HOME", got)
	}
}

func TestUserSSHConfigHasInclude(t *testing.T) {
	home := remoteTestEnv(t)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sshDir, "config")

	missing, err := UserSSHConfigHasInclude()
	if err != nil || missing {
		t.Fatalf("missing ssh config = %v, %v; want false, nil", missing, err)
	}

	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"tilde include", "Include ~/.config/seshagy/ssh_config\n", true},
		{"absolute include", "Include " + SSHConfigPath() + "\n", true},
		{
			"include later in file",
			"Host *\n  ServerAliveInterval 30\n" + IncludeLine() + "\n",
			true,
		},
		{"keyword=value form", "Include=~/.config/seshagy/ssh_config\n", true},
		{"case-insensitive keyword", "include ~/.config/seshagy/ssh_config\n", true},
		{"commented out", "# " + IncludeLine() + "\n", false},
		{"different file", "Include ~/.ssh/other_config\n", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(test.content), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := UserSSHConfigHasInclude()
			if err != nil || got != test.want {
				t.Fatalf("UserSSHConfigHasInclude() = %v, %v; want %v", got, err, test.want)
			}
		})
	}
}

func TestEnsureUserSSHConfigIncludeCreatesBacksUpAndIsIdempotent(t *testing.T) {
	home := remoteTestEnv(t)
	path := filepath.Join(home, ".ssh", "config")

	added, err := EnsureUserSSHConfigInclude()
	if err != nil || !added {
		t.Fatalf("EnsureUserSSHConfigInclude() = %v, %v; want added on a missing file", added, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("created ssh config mode = %v, want 0600", info.Mode().Perm())
	}
	if _, err := os.Stat(path + ".seshagy.bak"); !os.IsNotExist(err) {
		t.Fatalf("backup created for a missing file: %v", err)
	}

	original := "Host homelab-lan\n  HostName 192.168.1.10\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	added, err = EnsureUserSSHConfigInclude()
	if err != nil || !added {
		t.Fatalf("EnsureUserSSHConfigInclude() = %v, %v; want added", added, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.HasPrefix(text, IncludeLine()+"\n") {
		t.Fatalf("include line is not first:\n%s", text)
	}
	if !strings.Contains(text, original) {
		t.Fatalf("existing user content was not preserved:\n%s", text)
	}
	backup, err := os.ReadFile(path + ".seshagy.bak")
	if err != nil || string(backup) != original {
		t.Fatalf("backup = %q, %v; want the original content", backup, err)
	}

	// Whatever happens next, the backup is written once and never replaced.
	added, err = EnsureUserSSHConfigInclude()
	if err != nil || added {
		t.Fatalf("repeated EnsureUserSSHConfigInclude() = %v, %v; want no change", added, err)
	}
	if err := os.WriteFile(path, []byte("changed\n"+original), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureUserSSHConfigInclude(); err != nil {
		t.Fatalf("EnsureUserSSHConfigInclude() error = %v", err)
	}
	backup, err = os.ReadFile(path + ".seshagy.bak")
	if err != nil || string(backup) != original {
		t.Fatalf("backup was replaced: %q, %v", backup, err)
	}
}

func TestDiscoverSSHConfigHostsSkipsWildcardsAndGeneratedAliases(t *testing.T) {
	remoteTestEnv(t)
	dir := t.TempDir()
	confd := filepath.Join(dir, "conf.d")
	if err := os.MkdirAll(confd, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(confd, "work.conf"), []byte(
		"Host work-jump\n  HostName jump.internal\n\nHost *\n  User someone\n",
	), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config")
	content := "# my ssh config\n" +
		"Host homelab-lan\n  HostName 192.168.1.10\n\n" +
		"Host srv-1 srv-2\n\n" +
		"Host *.wild\n\n" +
		"Host !negated\n\n" +
		"Host seshagy-homelab\n  HostName 10.0.0.1\n\n" +
		"Host homelab-lan\n  HostName 10.0.0.2\n\n" +
		"Host\tTABBED\n\n" +
		"Include " + confd + "/*.conf\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	hosts, err := DiscoverSSHConfigHosts(path)
	if err != nil {
		t.Fatalf("DiscoverSSHConfigHosts() error = %v", err)
	}
	var names []string
	for _, host := range hosts {
		names = append(names, host.Name)
		if !host.FromSSHConfig || host.Alias() != host.Name || host.Transport != TransportSSH {
			t.Fatalf("discovered host = %+v, want a from-ssh-config host", host)
		}
	}
	want := "homelab-lan,srv-1,srv-2,TABBED,work-jump"
	if strings.Join(names, ",") != want {
		t.Fatalf("discovered hosts = %v, want %s", names, want)
	}
}

func TestDiscoverSSHConfigHostsSkipsOptionLikeAndControlNames(t *testing.T) {
	remoteTestEnv(t)
	path := filepath.Join(t.TempDir(), "config")
	content := "Host homelab-lan\n  HostName 192.168.1.10\n\n" +
		"Host -oProxyCommand\n\n" +
		"Host bad\x01name\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	hosts, err := DiscoverSSHConfigHosts(path)
	if err != nil {
		t.Fatalf("DiscoverSSHConfigHosts() error = %v", err)
	}
	var names []string
	for _, host := range hosts {
		names = append(names, host.Name)
	}
	if want := "homelab-lan"; strings.Join(names, ",") != want {
		t.Fatalf("discovered hosts = %v, want %s", names, want)
	}
}

func TestDiscoverSSHConfigHostsMissingFile(t *testing.T) {
	remoteTestEnv(t)
	hosts, err := DiscoverSSHConfigHosts(filepath.Join(t.TempDir(), "absent"))
	if err != nil || len(hosts) != 0 {
		t.Fatalf("DiscoverSSHConfigHosts(absent) = %v, %v; want none, nil", hosts, err)
	}
}

func TestHostsMergesConfiguredThenDiscovered(t *testing.T) {
	home := remoteTestEnv(t)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte(
		"Host homelab-lan\nHost seshagy-dupe\n",
	), 0o600); err != nil {
		t.Fatal(err)
	}
	configured := []Host{{Name: "edge", Target: "edge.example.com", Transport: TransportSSH}}

	got := Hosts(configured, true)
	var names []string
	for _, host := range got {
		names = append(names, host.Name)
	}
	if strings.Join(names, ",") != "edge,homelab-lan" {
		t.Fatalf("Hosts(configured, true) = %v, want edge,homelab-lan", names)
	}
	if host, ok := Find(got, "seshagy-dupe"); ok {
		t.Fatalf("Find(seshagy-dupe) = %+v; generated aliases must not be listed twice", host)
	}

	got = Hosts(configured, false)
	if len(got) != 1 || got[0].Name != "edge" {
		t.Fatalf("Hosts(configured, false) = %+v, want only the configured host", got)
	}
}
