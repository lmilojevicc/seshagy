package completion

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestScriptsMatchGoldenAndRejectUnsafeConstructs(t *testing.T) {
	for _, shell := range []Shell{Bash, Zsh, Fish} {
		script, err := Script(shell)
		if err != nil {
			t.Fatal(err)
		}
		golden, err := os.ReadFile(filepath.Join("testdata", string(shell)+".golden"))
		if err != nil {
			t.Fatal(err)
		}
		if script != string(golden) {
			t.Fatalf("%s generated script differs from golden", shell)
		}
		if strings.Contains(script, "eval") {
			t.Fatalf("%s script contains eval", shell)
		}
	}
	for _, forbidden := range []string{"declare -A", "mapfile", "readarray", "compopt", "${value,,}"} {
		if strings.Contains(bashScript, forbidden) {
			t.Fatalf("bash script uses Bash 4-only construct %q", forbidden)
		}
	}
}

func TestGeneratedScriptsKeepSeparatedDirectoryFallback(t *testing.T) {
	checks := map[Shell]string{
		Bash: `compgen -d -- "$prefix"`,
		Zsh:  `_files -/`,
		Fish: `__fish_complete_directories "$current"`,
	}
	for shell, want := range checks {
		script, err := Script(shell)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(script, want) {
			t.Errorf("%s script missing separated directory fallback %q", shell, want)
		}
	}
}

func TestGeneratedScriptSyntax(t *testing.T) {
	tests := []struct {
		shell Shell
		args  []string
	}{
		{Bash, []string{"-n"}},
		{Zsh, []string{"-n"}},
		{Fish, []string{"--no-execute"}},
	}
	for _, test := range tests {
		t.Run(string(test.shell), func(t *testing.T) {
			binary, err := exec.LookPath(string(test.shell))
			if err != nil {
				t.Skipf("%s unavailable", test.shell)
			}
			script, _ := Script(test.shell)
			command := exec.Command(binary, test.args...)
			command.Stdin = strings.NewReader(script)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("syntax check: %v\n%s", err, output)
			}
		})
	}
}

func TestBashAdapterKeepsHostileCandidatesLiteral(t *testing.T) {
	binary, err := bashHarnessBinary()
	if err != nil {
		t.Skip("bash unavailable")
	}
	candidate := `space '$() backtick` + "`" + ` ; * ? [x] -leading`
	protocol := Encode(
		Result{
			Directive:  NoFiles,
			Candidates: []Candidate{{Value: candidate, Description: "hostile"}},
		},
	)
	binDir := t.TempDir()
	fake := filepath.Join(binDir, "seshagy")
	if err := os.WriteFile(
		fake,
		[]byte("#!/bin/sh\ncat <<'SESHAGY_PROTOCOL'\n"+protocol+"SESHAGY_PROTOCOL\n"),
		0o755,
	); err != nil {
		t.Fatal(err)
	}
	harness := bashScript + "\nCOMP_WORDS=(seshagy '')\nCOMP_CWORD=1\nCOMP_LINE='seshagy '\nCOMP_POINT=${#COMP_LINE}\n_seshagy_complete\nprintf '<%s>\\n' \"${COMPREPLY[@]}\"\n"
	command := exec.Command(binary)
	command.Env = append(
		os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	command.Stdin = strings.NewReader(harness)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("bash behavior harness: %v\n%s", err, output)
	}
	if string(output) != "<"+candidate+">\n" {
		t.Fatalf("candidate was reinterpreted: %q", output)
	}
}

func TestZshAdapterKeepsHostileCandidatesLiteral(t *testing.T) {
	binary, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh unavailable")
	}
	candidate := `space '$() backtick` + "`" + ` ; * ? [x] -leading`
	description := `description '$() backtick` + "`" + ` ; * ? [x]`
	protocol := Encode(
		Result{
			Directive:  NoFiles,
			Candidates: []Candidate{{Value: candidate, Description: description}},
		},
	)
	binDir := t.TempDir()
	fake := filepath.Join(binDir, "seshagy")
	if err := os.WriteFile(
		fake,
		[]byte("#!/bin/sh\ncat <<'SESHAGY_PROTOCOL'\n"+protocol+"SESHAGY_PROTOCOL\n"),
		0o755,
	); err != nil {
		t.Fatal(err)
	}
	harness := "compdef() { :; }\n_files() { :; }\ncompadd() { local seen=0 arg; print -r -- \"D<${descriptions[1]}>\"; for arg in \"$@\"; do if (( seen )); then print -r -- \"<$arg>\"; elif [[ \"$arg\" == -- ]]; then seen=1; fi; done; }\n" +
		zshScript + "\nwords=(seshagy '')\nCURRENT=2\nPREFIX=''\n_seshagy_complete\n"
	command := exec.Command(binary)
	command.Env = append(
		os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	command.Stdin = strings.NewReader(harness)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("zsh behavior harness: %v\n%s", err, output)
	}
	if string(output) != "D<"+description+">\n<"+candidate+">\n" {
		t.Fatalf("candidate or description was reinterpreted: %q", output)
	}
}

func TestFishAdapterKeepsHostileCandidatesLiteral(t *testing.T) {
	binary, err := exec.LookPath("fish")
	if err != nil {
		t.Skip("fish unavailable")
	}
	candidate := `space '$() backtick` + "`" + ` ; * ? [x] -leading`
	description := `description '$() backtick` + "`" + ` ; * ? [x]`
	protocol := Encode(
		Result{
			Directive:  NoFiles,
			Candidates: []Candidate{{Value: candidate, Description: description}},
		},
	)
	binDir := t.TempDir()
	fake := filepath.Join(binDir, "seshagy")
	if err := os.WriteFile(
		fake,
		[]byte("#!/bin/sh\ncat <<'SESHAGY_PROTOCOL'\n"+protocol+"SESHAGY_PROTOCOL\n"),
		0o755,
	); err != nil {
		t.Fatal(err)
	}
	harness := fishScript + "\ncomplete -C 'seshagy ' | string collect\n"
	command := exec.Command(binary)
	command.Env = append(
		os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	command.Stdin = strings.NewReader(harness)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("fish behavior harness: %v\n%s", err, output)
	}
	line := strings.TrimSuffix(string(output), "\n")
	value, gotDescription, ok := strings.Cut(line, "\t")
	if !ok || value != candidate || gotDescription != description {
		t.Fatalf("candidate or description was reinterpreted: %q", output)
	}
}

func TestAdaptersRejectMalformedProtocolRecords(t *testing.T) {
	protocols := []string{
		"v1\t\tnofiles\n",
		"v1\tnofiles\t\n",
		"v1\tnofiles\nc\tgood\tdescription\t\n",
		"v1\tnofiles\nc\tgood\n",
	}
	for _, shell := range []Shell{Bash, Zsh, Fish} {
		for i, protocol := range protocols {
			t.Run(string(shell)+"/case-"+string(rune('1'+i)), func(t *testing.T) {
				output := runAdapterHarness(t, shell, protocol)
				if strings.TrimSpace(output) != "" {
					t.Fatalf("malformed protocol produced candidates: %q", output)
				}
			})
		}
	}
}

func TestBashDirectoryFallbackUsesCursorPrefixAndContinuesDirectories(t *testing.T) {
	binary, err := bashHarnessBinary()
	if err != nil {
		t.Skip("bash unavailable")
	}
	root := t.TempDir()
	for _, dir := range []string{"alpha space", filepath.Join("parent space", "child space")} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	binDir := t.TempDir()
	record := filepath.Join(t.TempDir(), "argv")
	writeFakeSeshagy(
		t,
		binDir,
		"v1\tdirs\n",
		"printf '<%s>\\n' \"$@\" >>\"$SESHAGY_ARGV_RECORD\"\n",
	)
	harness := bashScript + `
cd "$SESHAGY_FS_ROOT"
COMP_WORDS=(seshagy --report-agent --cwd 'alphaZZ')
COMP_CWORD=3
COMP_LINE='seshagy --report-agent --cwd alphaZZ'
COMP_POINT=$((${#COMP_LINE}-2))
_seshagy_complete
printf 'MID<%s>\n' "${COMPREPLY[@]}"
COMP_WORDS=(seshagy --report-agent --cwd 'parent space/')
COMP_CWORD=3
COMP_LINE='seshagy --report-agent --cwd parent space/'
COMP_POINT=${#COMP_LINE}
_seshagy_complete
printf 'NESTED<%s>\n' "${COMPREPLY[@]}"
`
	command := exec.Command(binary)
	command.Env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"SESHAGY_FS_ROOT="+root,
		"SESHAGY_ARGV_RECORD="+record,
	)
	command.Stdin = strings.NewReader(harness)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("bash directory harness: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "MID<alpha space/>\n") ||
		!strings.Contains(string(output), "NESTED<parent space/child space/>\n") {
		t.Fatalf("directory continuation output = %q", output)
	}
	argv, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(argv), "<alpha>") ||
		!strings.Contains(string(argv), "<parent space/>") {
		t.Fatalf("cursor prefixes not passed literally: %q", argv)
	}
}

func TestBashFilesFallbackMarksDirectoriesForNestedTraversal(t *testing.T) {
	binary, err := bashHarnessBinary()
	if err != nil {
		t.Skip("bash unavailable")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "parent space", "child space"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "parent file"), []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	writeFakeSeshagy(t, binDir, "v1\tfiles\n", "")
	harness := bashScript + `
cd "$SESHAGY_FS_ROOT"
COMP_WORDS=(seshagy --report-agent --message 'parentZZ')
COMP_CWORD=3
COMP_LINE='seshagy --report-agent --message parentZZ'
COMP_POINT=$((${#COMP_LINE}-2))
_seshagy_complete
printf 'MID<%s>\n' "${COMPREPLY[@]}"
COMP_WORDS=(seshagy --report-agent --message 'parent space/')
COMP_CWORD=3
COMP_LINE='seshagy --report-agent --message parent space/'
COMP_POINT=${#COMP_LINE}
_seshagy_complete
printf 'NESTED<%s>\n' "${COMPREPLY[@]}"
`
	command := exec.Command(binary)
	command.Env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"SESHAGY_FS_ROOT="+root,
	)
	command.Stdin = strings.NewReader(harness)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("bash files harness: %v\n%s", err, output)
	}
	text := string(output)
	if !strings.Contains(text, "MID<parent space/>\n") ||
		!strings.Contains(text, "MID<parent file>\n") ||
		!strings.Contains(text, "NESTED<parent space/child space/>\n") {
		t.Fatalf("files fallback output = %q", output)
	}
	if strings.Contains(text, "MID<parent file/>\n") {
		t.Fatalf("regular file incorrectly marked as directory: %q", output)
	}
}

func TestFishAdapterPreservesTrailingEmptyToken(t *testing.T) {
	binary, err := exec.LookPath("fish")
	if err != nil {
		t.Skip("fish unavailable")
	}
	binDir := t.TempDir()
	record := filepath.Join(t.TempDir(), "argv")
	writeFakeSeshagy(
		t,
		binDir,
		"v1\tnofiles\nc\tconfig\tInspect configuration\n",
		"printf '<%s>\\n' \"$@\" >\"$SESHAGY_ARGV_RECORD\"\n",
	)
	command := exec.Command(binary)
	command.Env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"SESHAGY_ARGV_RECORD="+record,
	)
	command.Stdin = strings.NewReader(fishScript + "\ncomplete -C 'seshagy ' | string collect\n")
	output, err := command.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "config") {
		t.Fatalf("fish empty-token harness: %v\n%s", err, output)
	}
	argv, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(argv), "<__complete>\n<-->\n<seshagy>\n<>\n"; got != want {
		t.Fatalf("completion endpoint argv = %q, want %q", got, want)
	}
}

func runAdapterHarness(t *testing.T, shell Shell, protocol string) string {
	t.Helper()
	binary, err := exec.LookPath(string(shell))
	if err != nil {
		t.Skipf("%s unavailable", shell)
	}
	binDir := t.TempDir()
	writeFakeSeshagy(t, binDir, protocol, "")
	var harness string
	switch shell {
	case Bash:
		harness = bashScript + "\nCOMP_WORDS=(seshagy '')\nCOMP_CWORD=1\nCOMP_LINE='seshagy '\nCOMP_POINT=${#COMP_LINE}\n_seshagy_complete\nfor reply in \"${COMPREPLY[@]}\"; do printf '<%s>\\n' \"$reply\"; done\n"
	case Zsh:
		harness = "compdef() { :; }\n_files() { :; }\ncompadd() { local seen=0 arg; for arg in \"$@\"; do if (( seen )); then print -r -- \"<$arg>\"; elif [[ \"$arg\" == -- ]]; then seen=1; fi; done; }\n" +
			zshScript + "\nwords=(seshagy '')\nCURRENT=2\nPREFIX=''\n_seshagy_complete\n"
	case Fish:
		harness = fishScript + "\ncomplete -C 'seshagy ' | string collect\n"
	}
	command := exec.Command(binary)
	command.Env = append(
		os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	command.Stdin = strings.NewReader(harness)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s behavior harness: %v\n%s", shell, err, output)
	}
	return string(output)
}

func writeFakeSeshagy(t *testing.T, binDir, protocol, prelude string) {
	t.Helper()
	script := "#!/bin/sh\n" + prelude + "cat <<'SESHAGY_PROTOCOL'\n" + protocol + "SESHAGY_PROTOCOL\n"
	if err := os.WriteFile(filepath.Join(binDir, "seshagy"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func bashHarnessBinary() (string, error) {
	const systemBash = "/bin/bash"
	if output, err := exec.Command(systemBash, "--version").Output(); err == nil &&
		strings.Contains(string(output), "version 3.2") {
		return systemBash, nil
	}
	return exec.LookPath("bash")
}

func TestScriptUnknownShell(t *testing.T) {
	if _, err := Script("powershell"); err == nil {
		t.Fatal("unknown shell accepted")
	}
}
