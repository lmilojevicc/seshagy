package integrations

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const (
	installVersion = 2
	markerPrefix   = "SESHAGY_INTEGRATION_"
)

type Target string

const (
	TargetPi       Target = "pi"
	TargetClaude   Target = "claude"
	TargetCodex    Target = "codex"
	TargetCopilot  Target = "copilot"
	TargetDroid    Target = "droid"
	TargetOpencode Target = "opencode"
	TargetQodercli Target = "qodercli"
	TargetCursor   Target = "cursor"
)

type StatusKind string

const (
	StatusNotInstalled StatusKind = "not-installed"
	StatusCurrent      StatusKind = "current"
	StatusOutdated     StatusKind = "outdated"
)

type Recommendation struct {
	Target         Target
	Label          string
	Commands       []string
	ConfigDir      string
	InstallPath    string
	AgentAvailable bool
	State          StatusKind
	Version        int
	Installable    bool
	Reason         string
}

type spec struct {
	target      Target
	label       string
	commands    []string
	configDir   func() string
	installPath func() string
	install     func(binaryPath string) ([]string, error)
	uninstall   func() ([]string, error)
}

func Targets() []Target {
	out := make([]Target, 0, len(specs()))
	for _, spec := range specs() {
		out = append(out, spec.target)
	}
	return out
}

func ParseTarget(raw string) (Target, error) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	for _, spec := range specs() {
		if raw == string(spec.target) {
			return spec.target, nil
		}
	}
	return "", fmt.Errorf("unknown integration target %q", raw)
}

func TargetLabel(target Target) string {
	if spec, ok := specFor(target); ok {
		return spec.label
	}
	return string(target)
}

func Scan() []Recommendation {
	out := make([]Recommendation, 0, len(specs()))
	for _, spec := range specs() {
		out = append(out, statusFor(spec))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out
}

func RecommendedForPrompt() []Recommendation {
	var out []Recommendation
	for _, rec := range Scan() {
		if rec.AgentAvailable && rec.Installable && rec.State != StatusCurrent {
			out = append(out, rec)
		}
	}
	return out
}

func Install(target Target) ([]string, error) {
	spec, ok := specFor(target)
	if !ok {
		return nil, fmt.Errorf("unknown integration target %q", target)
	}
	binary, err := os.Executable()
	if err != nil || binary == "" {
		binary = "seshagy"
	}
	return spec.install(binary)
}

func Uninstall(target Target) ([]string, error) {
	spec, ok := specFor(target)
	if !ok {
		return nil, fmt.Errorf("unknown integration target %q", target)
	}
	return spec.uninstall()
}

func specFor(target Target) (spec, bool) {
	for _, spec := range specs() {
		if spec.target == target {
			return spec, true
		}
	}
	return spec{}, false
}

func statusFor(spec spec) Recommendation {
	configDir := spec.configDir()
	installPath := spec.installPath()
	agentAvailable := configDirExists(configDir) || commandAvailable(spec.commands)
	state, version := installedState(installPath, spec.target)
	rec := Recommendation{
		Target:         spec.target,
		Label:          spec.label,
		Commands:       append([]string(nil), spec.commands...),
		ConfigDir:      configDir,
		InstallPath:    installPath,
		AgentAvailable: agentAvailable,
		State:          state,
		Version:        version,
		Installable:    configDirExists(configDir),
	}
	if !rec.AgentAvailable {
		rec.Reason = "agent command/config not found"
	} else if !rec.Installable {
		rec.Reason = "config directory not found"
	}
	return rec
}

func installedState(path string, target Target) (StatusKind, int) {
	if path == "" {
		return StatusNotInstalled, 0
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return StatusNotInstalled, 0
	}
	content := string(data)
	if !strings.Contains(content, markerPrefix+"ID="+string(target)) {
		return StatusOutdated, 0
	}
	version := parseMarkerVersion(content)
	if version >= installVersion {
		return StatusCurrent, version
	}
	return StatusOutdated, version
}

func parseMarkerVersion(content string) int {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(line, "/# "))
		value, ok := strings.CutPrefix(line, markerPrefix+"VERSION=")
		if !ok {
			continue
		}
		var n int
		_, _ = fmt.Sscanf(value, "%d", &n)
		return n
	}
	return 0
}

func commandAvailable(commands []string) bool {
	for _, command := range commands {
		if _, err := exec.LookPath(command); err == nil {
			return true
		}
	}
	return false
}

func configDirExists(dir string) bool {
	if dir == "" {
		return false
	}
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

func specs() []spec {
	return []spec{
		{TargetPi, "Pi", []string{"pi"}, piDir, func() string { return filepath.Join(piDir(), "extensions", "seshagy-agent-state.ts") }, installPi, uninstallPi},
		{TargetClaude, "Claude Code", []string{"claude", "claude-code"}, claudeDir, func() string { return filepath.Join(claudeDir(), "hooks", shellHookName) }, installClaude, uninstallClaude},
		{TargetCodex, "Codex", []string{"codex"}, codexDir, func() string { return filepath.Join(codexDir(), shellHookName) }, installCodex, uninstallCodex},
		{TargetCopilot, "GitHub Copilot CLI", []string{"copilot", "github-copilot", "ghcs"}, copilotDir, func() string { return filepath.Join(copilotDir(), "hooks", shellHookName) }, installCopilot, uninstallCopilot},
		{TargetDroid, "Factory Droid", []string{"droid"}, droidDir, func() string { return filepath.Join(droidDir(), "hooks", shellHookName) }, installDroid, uninstallDroid},
		{TargetOpencode, "OpenCode", []string{"opencode", "open-code"}, opencodeDir, func() string { return filepath.Join(opencodeDir(), "plugins", "seshagy-agent-state.js") }, installOpencode, uninstallOpencode},
		{TargetQodercli, "Qoder CLI", []string{"qodercli", "qoder", "qoderclicn", "qodercn"}, qoderDir, func() string { return filepath.Join(qoderDir(), "hooks", shellHookName) }, installQodercli, uninstallQodercli},
		{TargetCursor, "Cursor Agent", []string{"cursor-agent"}, cursorDir, func() string { return filepath.Join(cursorDir(), shellHookName) }, installCursor, uninstallCursor},
	}
}
