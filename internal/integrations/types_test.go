package integrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanUnavailableAgentSetsReason(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())

	rec := findRec(t, Scan(), TargetClaude)
	if rec.AgentAvailable || rec.Installable || rec.Reason == "" {
		t.Fatalf("unavailable claude scan = %#v", rec)
	}
}

func TestParseTargetAcceptsKnownTargets(t *testing.T) {
	for _, spec := range specs() {
		got, err := ParseTarget(string(spec.target))
		if err != nil || got != spec.target {
			t.Fatalf(
				"ParseTarget(%q) = (%q, %v), want (%q, nil)",
				spec.target,
				got,
				err,
				spec.target,
			)
		}
		trimmed, err := ParseTarget("  " + string(spec.target) + "  ")
		if err != nil || trimmed != spec.target {
			t.Fatalf("ParseTarget(trimmed %q) = (%q, %v)", spec.target, trimmed, err)
		}
	}
}

func TestParseTargetRejectsUnknown(t *testing.T) {
	if _, err := ParseTarget("not-a-target"); err == nil {
		t.Fatal("ParseTarget() expected error for unknown target")
	} else if !strings.Contains(err.Error(), `unknown integration target "not-a-target"`) {
		t.Fatalf("ParseTarget() error = %v", err)
	}
}

func TestInstallAndUninstallWrappers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".pi", "agent"), 0o755); err != nil {
		t.Fatal(err)
	}

	messages, err := Install(TargetPi)
	if err != nil {
		t.Fatalf("Install(pi) error = %v", err)
	}
	if len(messages) == 0 {
		t.Fatal("Install(pi) returned no messages")
	}

	messages, err = Uninstall(TargetPi)
	if err != nil {
		t.Fatalf("Uninstall(pi) error = %v", err)
	}
	if len(messages) == 0 {
		t.Fatal("Uninstall(pi) returned no messages")
	}
	extPath := filepath.Join(home, ".pi", "agent", "extensions", "seshagy-agent-state.ts")
	if _, err := os.Stat(extPath); !os.IsNotExist(err) {
		t.Fatalf("pi extension should be removed at %s, stat err=%v", extPath, err)
	}
}

func TestInstallUninstallUnknownTarget(t *testing.T) {
	if _, err := Install(Target("bogus")); err == nil {
		t.Fatal("Install(bogus) expected error")
	} else if !strings.Contains(err.Error(), `unknown integration target "bogus"`) {
		t.Fatalf("Install() error = %v", err)
	}
	if _, err := Uninstall(Target("bogus")); err == nil {
		t.Fatal("Uninstall(bogus) expected error")
	} else if !strings.Contains(err.Error(), `unknown integration target "bogus"`) {
		t.Fatalf("Uninstall() error = %v", err)
	}
}

func TestTargetLabelUnknown(t *testing.T) {
	if got := TargetLabel(Target("missing")); got != "missing" {
		t.Fatalf("TargetLabel(missing) = %q, want missing", got)
	}
}

func TestInstallViaPublicWrapper(t *testing.T) {
	cases := []struct {
		name          string
		target        Target
		setup         func(t *testing.T, home string)
		wantMessages  bool
		assertCurrent bool
		uninstall     bool
	}{
		{
			name:   "pi",
			target: TargetPi,
			setup: func(t *testing.T, home string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(home, ".pi", "agent"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:   "claude",
			target: TargetClaude,
			setup: func(t *testing.T, home string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			uninstall: true,
		},
		{
			name:   "qodercli",
			target: TargetQodercli,
			setup: func(t *testing.T, home string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(home, ".qoder"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:   "kimi",
			target: TargetKimi,
			setup: func(t *testing.T, home string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(home, ".kimi"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:   "opencode",
			target: TargetOpencode,
			setup: func(t *testing.T, home string) {
				t.Helper()
				t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
				if err := os.MkdirAll(
					filepath.Join(home, ".config", "opencode"),
					0o755,
				); err != nil {
					t.Fatal(err)
				}
			},
			wantMessages: true,
		},
		{
			name:   "kilo",
			target: TargetKilo,
			setup: func(t *testing.T, home string) {
				t.Helper()
				t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
				if err := os.MkdirAll(filepath.Join(home, ".config", "kilo"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			wantMessages: true,
		},
		{
			name:   "codex",
			target: TargetCodex,
			setup: func(t *testing.T, home string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			wantMessages: true,
		},
		{
			name:   "droid",
			target: TargetDroid,
			setup: func(t *testing.T, home string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(home, ".factory"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			wantMessages: true,
		},
		{
			name:   "cursor",
			target: TargetCursor,
			setup: func(t *testing.T, home string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(home, ".cursor"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			wantMessages: true,
		},
		{
			name:   "copilot",
			target: TargetCopilot,
			setup: func(t *testing.T, home string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(home, ".copilot"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			wantMessages:  true,
			assertCurrent: true,
		},
		{
			name:   "grok",
			target: TargetGrok,
			setup: func(t *testing.T, home string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(home, ".grok"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			wantMessages:  true,
			assertCurrent: true,
		},
		{
			name:   "hermes",
			target: TargetHermes,
			setup: func(t *testing.T, home string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(home, ".hermes"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(
					filepath.Join(home, ".hermes", "config.yaml"),
					[]byte("plugins:\n  enabled:\n    - platforms/discord\n"),
					0o644,
				); err != nil {
					t.Fatal(err)
				}
			},
			uninstall: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			tc.setup(t, home)

			messages, err := Install(tc.target)
			if err != nil {
				t.Fatalf("Install(%s) error = %v", tc.target, err)
			}
			if tc.wantMessages && len(messages) == 0 {
				t.Fatalf("Install(%s) returned no messages", tc.target)
			}
			if tc.assertCurrent {
				rec := findRec(t, Scan(), tc.target)
				if rec.State != StatusCurrent {
					t.Fatalf("%s status after install = %#v", tc.target, rec)
				}
			}
			rec := findRec(t, Scan(), tc.target)
			if rec.InstallPath == "" {
				t.Fatalf("%s install path missing after install", tc.target)
			}
			if _, err := os.Stat(rec.InstallPath); err != nil {
				t.Fatalf(
					"%s install artifact missing at %s: %v",
					tc.target,
					rec.InstallPath,
					err,
				)
			}

			if tc.uninstall {
				if _, err := Uninstall(tc.target); err != nil {
					t.Fatalf("Uninstall(%s) error = %v", tc.target, err)
				}
				assertInstallPathRemoved(t, rec.InstallPath)
				if tc.target == TargetPi {
					extPath := filepath.Join(
						home,
						".pi",
						"agent",
						"extensions",
						"seshagy-agent-state.ts",
					)
					assertInstallPathRemoved(t, extPath)
				}
			}
		})
	}
}

func assertInstallPathRemoved(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("install artifact should be removed at %s, stat err=%v", path, err)
	}
}

func TestAuthorityUnknownTarget(t *testing.T) {
	if got := Authority(Target("unknown")); got != SessionOnly {
		t.Fatalf("Authority(unknown) = %q, want %q", got, SessionOnly)
	}
}

func TestRecommendedForPromptFiltersInstallableOutdated(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".pi", "agent"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(t.TempDir(), "pi"))
	t.Setenv("PATH", t.TempDir())

	recs := RecommendedForPrompt()
	found := false
	for _, rec := range recs {
		if rec.Target == TargetPi {
			found = true
			if rec.State == StatusCurrent {
				t.Fatalf("pi should not be current before install: %#v", rec)
			}
		}
	}
	if !found {
		t.Fatalf("RecommendedForPrompt() missing pi: %#v", recs)
	}

	if _, err := installPi("/bin/seshagy"); err != nil {
		t.Fatal(err)
	}
	recs = RecommendedForPrompt()
	for _, rec := range recs {
		if rec.Target == TargetPi {
			t.Fatalf("current pi integration should not be recommended: %#v", rec)
		}
	}
}
