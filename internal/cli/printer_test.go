package cli

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"
)

// ansiRe matches SGR escape sequences so styled output can be reduced to its
// plain text for byte-exact comparison.
var ansiRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripAnsi(s string) string { return ansiRe.ReplaceAllString(s, "") }

// newTestPrinter returns a Printer whose stdout/stderr are captured in the
// returned buffers, with color explicitly on or off for determinism.
func newTestPrinter(color bool) (p *Printer, out, errw *bytes.Buffer) {
	out = &bytes.Buffer{}
	errw = &bytes.Buffer{}
	return NewWith(out, errw, color), out, errw
}

func TestSeverityRoutingAndPlainText(t *testing.T) {
	cases := []struct {
		name   string
		call   func(p *Printer)
		stream string // "out" or "err"
		plain  string
	}{
		{"error", func(p *Printer) { p.Error("boom") }, "err", "error: boom\n"},
		{"errorf", func(p *Printer) { p.Errorf("code %d", 7) }, "err", "error: code 7\n"},
		{"warn", func(p *Printer) { p.Warn("careful") }, "err", "warning: careful\n"},
		{"warnf", func(p *Printer) { p.Warnf("seq %d", 3) }, "err", "warning: seq 3\n"},
		{"note", func(p *Printer) { p.Note("fyi") }, "err", "note: fyi\n"},
		{
			"success",
			func(p *Printer) { p.Success("installed herdr keybind") },
			"out",
			"installed herdr keybind\n",
		},
		{
			"successf",
			func(p *Printer) { p.Successf("reported %s on %s", "pi", "%5") },
			"out",
			"reported pi on %5\n",
		},
		{
			"info",
			func(p *Printer) { p.Info("reload with: herdr server reload-config") },
			"out",
			"reload with: herdr server reload-config\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name+"/plain", func(t *testing.T) {
			p, out, errw := newTestPrinter(false) // NO_COLOR / non-TTY
			tc.call(p)
			got := out.String()
			if tc.stream == "err" {
				got = errw.String()
			}
			if got != tc.plain {
				t.Fatalf("plain output = %q, want %q", got, tc.plain)
			}
			if strings.Contains(got, "\x1b[") {
				t.Fatalf("plain output must contain no ANSI escapes: %q", got)
			}
			// The other stream must be empty (routing is exclusive).
			if tc.stream == "err" && out.String() != "" {
				t.Fatalf("error leaked to stdout: %q", out.String())
			}
			if tc.stream == "out" && errw.String() != "" {
				t.Fatalf("success/info leaked to stderr: %q", errw.String())
			}
		})
		t.Run(tc.name+"/styled", func(t *testing.T) {
			p, out, errw := newTestPrinter(true) // color forced on
			tc.call(p)
			got := out.String()
			if tc.stream == "err" {
				got = errw.String()
			}
			if got == tc.plain {
				t.Fatalf("styled output equals plain; color was not applied: %q", got)
			}
			if !strings.Contains(got, "\x1b[") {
				t.Fatalf("styled output contains no ANSI escape: %q", got)
			}
			if stripAnsi(got) != tc.plain {
				t.Fatalf("stripped styled = %q, want %q", stripAnsi(got), tc.plain)
			}
		})
	}
}

func TestSuccessColorsOnlyLeadingVerb(t *testing.T) {
	p, _, _ := newTestPrinter(true)
	p.Success("installed herdr keybind")
	got := p.out.(*bytes.Buffer).String()
	stripped := stripAnsi(got)
	if stripped != "installed herdr keybind\n" {
		t.Fatalf("stripped = %q", stripped)
	}
	// Exactly one ANSI run opens before "installed" and closes right after it,
	// so the verb is styled and the remainder is plain.
	if !strings.HasPrefix(got, "\x1b[") {
		t.Fatalf("output must start with the verb's style: %q", got)
	}
	// After the verb's closing reset, " herdr keybind\n" must be plain (no
	// further styling on the message body). The reset itself is expected.
	idx := strings.Index(got, "installed")
	tail := strings.TrimPrefix(got[idx+len("installed"):], "\x1b[0m")
	if strings.Contains(tail, "\x1b[") {
		t.Fatalf("remainder after the verb must be plain: %q", tail)
	}
}

func TestSuccessNoColorLeavesMessageUnchanged(t *testing.T) {
	p, out, _ := newTestPrinter(false)
	p.Success("installed")
	if got := out.String(); got != "installed\n" {
		t.Fatalf("single-word success (no color) = %q, want %q", got, "installed\n")
	}
}

func TestVerbatimPrintIsByteExact(t *testing.T) {
	p, out, _ := newTestPrinter(true) // color on — must NOT color verbatim output
	p.Println(`{"ok":true}`)
	p.Print("raw")
	p.Printf("%d-%d", 1, 2)
	got := out.String()
	want := `{"ok":true}` + "\n" + "raw" + "1-2"
	if got != want {
		t.Fatalf("verbatim output = %q, want %q", got, want)
	}
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("verbatim output must never be colored: %q", got)
	}
}

func TestColorEnabledEnvPolicy(t *testing.T) {
	stdout := os.Stdout
	t.Run("NO_COLOR disables", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")
		t.Setenv("CLICOLOR_FORCE", "")
		if colorEnabled(stdout) {
			t.Fatal("NO_COLOR set must disable color")
		}
	})
	t.Run("CLICOLOR=0 disables", func(t *testing.T) {
		t.Setenv("NO_COLOR", "")
		t.Setenv("CLICOLOR", "0")
		t.Setenv("CLICOLOR_FORCE", "")
		if colorEnabled(stdout) {
			t.Fatal("CLICOLOR=0 must disable color")
		}
	})
	t.Run("CLICOLOR_FORCE enables even off-TTY", func(t *testing.T) {
		t.Setenv("NO_COLOR", "")
		t.Setenv("CLICOLOR", "")
		t.Setenv("CLICOLOR_FORCE", "1")
		if !colorEnabled(stdout) {
			t.Fatal("CLICOLOR_FORCE=1 must enable color regardless of TTY")
		}
	})
	t.Run("CLICOLOR_FORCE=0 does not force", func(t *testing.T) {
		t.Setenv("CLICOLOR_FORCE", "0")
		t.Setenv("NO_COLOR", "")
		// Result then depends on the real stdout TTY status; just assert no panic.
		_ = colorEnabled(stdout)
	})
	t.Run("CLICOLOR_FORCE wins over NO_COLOR", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")
		t.Setenv("CLICOLOR_FORCE", "1")
		if !colorEnabled(stdout) {
			t.Fatal("CLICOLOR_FORCE=1 must win over a simultaneous NO_COLOR=1")
		}
	})
}

func TestHelpColoring(t *testing.T) {
	text := "seshagy — minimal terminal session manager\n\n" +
		"Usage:\n" +
		"  seshagy --get-all [--json]      print everything\n" +
		"  seshagy config show [--json]    print config\n" +
		"<not-a-flag>\n"

	t.Run("no color is byte exact", func(t *testing.T) {
		p, _, _ := newTestPrinter(false)
		var b bytes.Buffer
		p.Help(&b, text)
		if b.String() != text {
			t.Fatalf("no-color help differs from input:\n got: %q\nwant: %q", b.String(), text)
		}
	})

	t.Run("headers cyan, flags bold, metavars dim", func(t *testing.T) {
		p, _, _ := newTestPrinter(true)
		var b bytes.Buffer
		p.Help(&b, text)
		got := b.String()
		// Byte-exact once ANSI is stripped.
		if stripAnsi(got) != text {
			t.Fatalf("stripped help != input:\n got: %q", stripAnsi(got))
		}
		// "Usage:" header is bold+underline+CYAN (SGR 36). lipgloss emits styles
		// per-rune, so assert the cyan foreground code is present on that line.
		usageHeader := styledLineContaining(got, "Usage:")
		if usageHeader == "" || !strings.Contains(usageHeader, "\x1b[") {
			t.Fatalf("Usage header must be styled: %q", usageHeader)
		}
		if !strings.Contains(usageHeader, "36") {
			t.Fatalf("Usage header must be cyan (SGR 36): %q", usageHeader)
		}
		// A usage line with a long flag is styled (bold) and keeps the flag token.
		flagLine := styledLineContaining(got, "get-all")
		if flagLine == "" || !strings.Contains(flagLine, "\x1b[") {
			t.Fatalf("flag line must be styled: %q", flagLine)
		}
		// "<not-a-flag>" metavar is now DIM (faint, SGR 2) — it must contain ANSI.
		metaLine := styledLineContaining(got, "<not-a-flag>")
		if metaLine == "" || !strings.Contains(metaLine, "\x1b[") {
			t.Fatalf("metavar line must be dim (contain ANSI): %q", metaLine)
		}
	})
}

// styledLineContaining returns the first line of s whose ANSI-stripped form
// contains sub. Works around lipgloss per-rune styling (which scatters escapes
// inside a token so a raw substring match would miss).
func styledLineContaining(s, sub string) string {
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(stripAnsi(l), sub) {
			return l
		}
	}
	return ""
}

func TestDefaultDelegatesToWorkingPrinter(t *testing.T) {
	saved := Default
	t.Cleanup(func() { Default = saved })
	out := &bytes.Buffer{}
	errw := &bytes.Buffer{}
	Default = NewWith(out, errw, true) // color on

	Error("boom")
	Warn("careful")
	Note("fyi")
	Success("installed keybind")
	Info("reload")
	Println(`{"ok":true}`)

	// Severity helpers route to the right buffer and carry ANSI (color on).
	if stripAnsi(errw.String()) != "error: boom\nwarning: careful\nnote: fyi\n" {
		t.Fatalf("stderr (stripped) = %q", errw.String())
	}
	if !strings.Contains(errw.String(), "\x1b[") {
		t.Fatalf("stderr severity output must be styled: %q", errw.String())
	}
	wantOut := "installed keybind\n" + "reload\n" + `{"ok":true}` + "\n"
	if stripAnsi(out.String()) != wantOut {
		t.Fatalf("stdout (stripped) = %q, want %q", stripAnsi(out.String()), wantOut)
	}
	if !strings.Contains(out.String(), "\x1b[") {
		t.Fatalf("stdout success/info must be styled: %q", out.String())
	}
}

func TestPerStreamColorIndependence(t *testing.T) {
	// Emulate stdout=TTY (color) + stderr redirected to a file/pipe (plain):
	// the stdout style set is colored, the stderr style set is plain. This is
	// the regression guard for per-stream color — without it, redirecting stderr
	// (`seshagy … 2>errors.log`) would leak ANSI escapes into the file.
	out := &bytes.Buffer{}
	errw := &bytes.Buffer{}
	p := &Printer{
		out:   out,
		err:   errw,
		outSt: newStyles(profileFor(true)),  // stdout color on
		errSt: newStyles(profileFor(false)), // stderr color off
	}
	p.Success("installed keybind") // -> stdout
	p.Error("boom")                // -> stderr

	if !strings.Contains(out.String(), "\x1b[") {
		t.Fatalf("stdout (TTY) must be colored: %q", out.String())
	}
	if strings.Contains(errw.String(), "\x1b[") {
		t.Fatalf("stderr (redirected) must stay plain, got ANSI: %q", errw.String())
	}
	if stripAnsi(out.String()) != "installed keybind\n" {
		t.Fatalf("stdout text = %q", out.String())
	}
	if stripAnsi(errw.String()) != "error: boom\n" {
		t.Fatalf("stderr text = %q", errw.String())
	}
}
