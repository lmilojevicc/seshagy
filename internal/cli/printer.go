// Package cli is the single sanctioned writer for user-facing stdout/stderr
// output in seshagy. Severity styling follows cargo/rustc conventions: only the
// prefix word is colored (bold bright-red "error", bold yellow "warning", bold
// bright-green "note"/success-verb, dim info) while the colon and message stay
// in the default terminal color. Color is auto-disabled for non-TTY streams and
// when NO_COLOR is set; CLICOLOR_FORCE overrides (mirroring cargo/clap/uv).
//
// Every other package must emit CLI messages through this package — enforced by
// the golangci-lint forbidigo rule (.golangci.yml) and the pi-lens ast-grep rule
// (rules/ast-grep-rules/rules/no-raw-cli-output.yml); see AGENTS.md.
package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"
	"github.com/muesli/termenv"
)

// Printer renders user-facing CLI messages with severity styling. Construct
// with New or NewWith; the zero value is not usable. Default writes lazily to
// os.Stdout/os.Stderr so test harnesses that swap those globals still capture.
type Printer struct {
	out   io.Writer // nil => os.Stdout, resolved per write
	err   io.Writer // nil => os.Stderr, resolved per write
	outSt styles    // color verdict from os.Stdout (Success/Info/help)
	errSt styles    // color verdict from os.Stderr (Error/Warn/Note)
}

type styles struct {
	err, warn, note, success, info lipgloss.Style
	header, literal, metavar       lipgloss.Style
}

// New returns a Printer that writes lazily to os.Stdout/os.Stderr, with color
// auto-detected PER STREAM from the environment and each stream's own TTY
// status — so redirecting one stream (e.g. `seshagy … 2>errors.log`) disables
// color on that stream while leaving the other colored. Mirrors the per-stream
// anstream policy used by cargo/clap/uv.
func New() *Printer {
	return &Printer{
		outSt: newStyles(profileFor(colorEnabled(os.Stdout))),
		errSt: newStyles(profileFor(colorEnabled(os.Stderr))),
	}
}

// NewWith returns a Printer writing to out/err (nil => lazy os.Stdout/os.Stderr)
// with color explicitly enabled or disabled on BOTH streams. Intended for
// tests, which pass non-TTY buffers and want deterministic color.
func NewWith(out, err io.Writer, color bool) *Printer {
	st := newStyles(profileFor(color))
	return &Printer{out: out, err: err, outSt: st, errSt: st}
}

// newStyles builds the severity style set for a color profile. With the Ascii
// profile every Render call is a no-op, so output stays plain.
func newStyles(p termenv.Profile) styles {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(p)
	return styles{
		err: r.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("9")),
		// bold bright red
		warn: r.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("3")),
		// bold yellow
		note: r.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("10")),
		// bold bright green
		success: r.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("10")),
		// bold bright green
		info: r.NewStyle().Faint(true), // dim
		header: r.NewStyle().
			Bold(true).
			Underline(true).
			Foreground(lipgloss.Color("6")),
		// two-tone: bold+underline+cyan header
		literal: r.NewStyle().
			Bold(true),
		// bold literal (flags, binary)
		metavar: r.NewStyle().
			Faint(true),
		// two-tone: dim metavar (<name>, <key>, …)
	}
}

func profileFor(color bool) termenv.Profile {
	if color {
		return termenv.ANSI
	}
	return termenv.Ascii
}

// colorEnabled reports whether color should be emitted for f, honoring
// CLICOLOR_FORCE (force on), NO_COLOR (off), CLICOLOR=0 (off), and the stream's
// TTY status. Mirrors the anstream policy used by cargo/clap/uv.
func colorEnabled(f *os.File) bool {
	if v, ok := os.LookupEnv("CLICOLOR_FORCE"); ok && v != "" && v != "0" {
		return true
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("CLICOLOR") == "0" {
		return false
	}
	return term.IsTerminal(f.Fd())
}

// Default is the package-level printer backing the top-level helpers.
var Default = New()

func (p *Printer) outStream() io.Writer {
	if p.out != nil {
		return p.out
	}
	return os.Stdout
}

func (p *Printer) errStream() io.Writer {
	if p.err != nil {
		return p.err
	}
	return os.Stderr
}

// StderrWriter returns the stream Error/Warn/Note write to, so other stderr
// writers (e.g. flag.FlagSet.SetOutput) can route through the printer.
func (p *Printer) StderrWriter() io.Writer { return p.errStream() }

// StdoutWriter returns the stream Success/Info/Print write to.
func (p *Printer) StdoutWriter() io.Writer { return p.outStream() }

// Error prints "error: <msg>" to stderr with the prefix word bold bright-red.
func (p *Printer) Error(msg string) {
	write(p.errStream(), p.errSt.err.Render("error")+": "+msg+"\n")
}

// Errorf is the formatted variant of Error.
func (p *Printer) Errorf(format string, args ...any) { p.Error(fmt.Sprintf(format, args...)) }

// Warn prints "warning: <msg>" to stderr with the prefix word bold yellow.
func (p *Printer) Warn(msg string) {
	write(p.errStream(), p.errSt.warn.Render("warning")+": "+msg+"\n")
}

// Warnf is the formatted variant of Warn.
func (p *Printer) Warnf(format string, args ...any) { p.Warn(fmt.Sprintf(format, args...)) }

// Note prints "note: <msg>" to stderr with the prefix word bold bright-green.
// Reserved for future use; wired for extendability.
func (p *Printer) Note(msg string) {
	write(p.errStream(), p.errSt.note.Render("note")+": "+msg+"\n")
}

// Notef is the formatted variant of Note.
func (p *Printer) Notef(format string, args ...any) { p.Note(fmt.Sprintf(format, args...)) }

// Success colors the leading verb of msg bold bright-green and leaves the rest
// of the message in the default color; written to stdout. Mirrors cargo's
// status-verb styling (there is no "success:" token upstream).
func (p *Printer) Success(msg string) {
	write(p.outStream(), successLine(p.outSt.success, msg))
}

// Successf is the formatted variant of Success.
func (p *Printer) Successf(format string, args ...any) { p.Success(fmt.Sprintf(format, args...)) }

// Info writes msg dimmed to stdout.
func (p *Printer) Info(msg string) { write(p.outStream(), p.outSt.info.Render(msg)+"\n") }

// Infof is the formatted variant of Info.
func (p *Printer) Infof(format string, args ...any) { p.Info(fmt.Sprintf(format, args...)) }

// Help writes text to w with the two-tone accent theme: section headers are
// bold+underlined+cyan, long flags and the leading binary name are bold,
// metavars (<name>, <key>, …) are dim, and descriptions stay plain. When color
// is disabled the text is emitted byte-for-byte unchanged.
func (p *Printer) Help(w io.Writer, text string) {
	write(w, renderHelp(p.outSt.header, p.outSt.literal, p.outSt.metavar, text))
}

// Print, Println, Printf emit verbatim to stdout — no severity, no color. This
// is the sanctioned path for machine-readable output (--json, --version, config
// paths, TOML dumps, rendered list lines).
func (p *Printer) Print(args ...any) { write(p.outStream(), fmt.Sprint(args...)) }

func (p *Printer) Println(args ...any) { write(p.outStream(), fmt.Sprintln(args...)) }

func (p *Printer) Printf(format string, args ...any) {
	write(p.outStream(), fmt.Sprintf(format, args...))
}

func write(w io.Writer, s string) {
	_, _ = io.WriteString(w, s)
}

func successLine(verb lipgloss.Style, msg string) string {
	idx := strings.IndexAny(msg, " \t")
	if idx < 0 {
		return verb.Render(msg) + "\n"
	}
	return verb.Render(msg[:idx]) + msg[idx:] + "\n"
}

// --- package-level helpers (delegate to Default) ---

// Error prints "error: <msg>" to stderr via Default.
func Error(msg string) { Default.Error(msg) }

// Errorf is the formatted variant of Error.
func Errorf(format string, args ...any) { Default.Errorf(format, args...) }

// Warn prints "warning: <msg>" to stderr via Default.
func Warn(msg string) { Default.Warn(msg) }

// Warnf is the formatted variant of Warn.
func Warnf(format string, args ...any) { Default.Warnf(format, args...) }

// Note prints "note: <msg>" to stderr via Default.
func Note(msg string) { Default.Note(msg) }

// Notef is the formatted variant of Note.
func Notef(format string, args ...any) { Default.Notef(format, args...) }

// Success colors the leading verb of msg via Default.
func Success(msg string) { Default.Success(msg) }

// Successf is the formatted variant of Success.
func Successf(format string, args ...any) { Default.Successf(format, args...) }

// Info writes msg dimmed to stdout via Default.
func Info(msg string) { Default.Info(msg) }

// Infof is the formatted variant of Info.
func Infof(format string, args ...any) { Default.Infof(format, args...) }

// Help writes clap-v4-colored help to w via Default.
func Help(w io.Writer, text string) { Default.Help(w, text) }

// Print emits verbatim to stdout via Default.
func Print(args ...any) { Default.Print(args...) }

// Println emits verbatim to stdout via Default.
func Println(args ...any) { Default.Println(args...) }

// Printf emits verbatim to stdout via Default.
func Printf(format string, args ...any) { Default.Printf(format, args...) }
