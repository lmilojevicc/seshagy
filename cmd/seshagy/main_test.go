package main

import "testing"

func TestRunRoutingNoError(t *testing.T) {
	cases := [][]string{
		{"--help"},
		{"-h"},
		{"help"},
		{"--version"},
		{"version"},
		{"config", "path"},
		{"config"},
	}
	for _, args := range cases {
		if err := run(args); err != nil {
			t.Fatalf("run(%v) unexpected error: %v", args, err)
		}
	}
}

func TestRunRoutingErrors(t *testing.T) {
	cases := [][]string{
		{"config", "bogus"},
		{"config", "init", "bad"},
		{"integration", "install"},
		{"integration", "frobnicate", "x"},
		{"--delete-item"},
		{"--report-agent", "--bogus"},
		{"--release-agent", "--seq", "-1"},
	}
	for _, args := range cases {
		if err := run(args); err == nil {
			t.Fatalf("run(%v) expected error, got nil", args)
		}
	}
}

func TestParseReportArgsSessionIDAndSeq(t *testing.T) {
	report, err := parseReportArgs(
		[]string{
			"--pane",
			"%1",
			"--agent",
			"opencode",
			"--state",
			"working",
			"--source",
			"seshagy:opencode",
			"--session-id",
			"session-123",
			"--seq",
			"42",
		},
	)
	if err != nil {
		t.Fatalf("parseReportArgs() error = %v", err)
	}
	if report.Pane != "%1" || report.Name != "opencode" || report.Source != "seshagy:opencode" ||
		!report.SourceSeen {
		t.Fatalf("report parsed basic fields incorrectly: %#v", report)
	}
	if report.SessionID != "session-123" || !report.SessionIDSeen {
		t.Fatalf("report session id = %q seen=%v", report.SessionID, report.SessionIDSeen)
	}
	if report.Seq != 42 || !report.SeqSeen {
		t.Fatalf("report seq = %d seen=%v", report.Seq, report.SeqSeen)
	}
}

func TestParseReportArgsRejectsInvalidSeq(t *testing.T) {
	for _, seq := range []string{"not-an-int", "-1"} {
		if _, err := parseReportArgs([]string{"--seq", seq}); err == nil {
			t.Fatalf("parseReportArgs should reject seq %q", seq)
		}
	}
}

func TestParseReleaseArgsSeq(t *testing.T) {
	release, err := parseReleaseArgs([]string{"--pane=%2", "--source=seshagy:pi", "--seq=99"})
	if err != nil {
		t.Fatalf("parseReleaseArgs() error = %v", err)
	}
	if release.Pane != "%2" || release.Source != "seshagy:pi" || !release.SourceSeen {
		t.Fatalf("release parsed basic fields incorrectly: %#v", release)
	}
	if release.Seq != 99 || !release.SeqSeen {
		t.Fatalf("release seq = %d seen=%v", release.Seq, release.SeqSeen)
	}
}

func TestParseReleaseArgsRejectsInvalidSeq(t *testing.T) {
	for _, seq := range []string{"bad", "-1"} {
		if _, err := parseReleaseArgs([]string{"--seq", seq}); err == nil {
			t.Fatalf("parseReleaseArgs should reject seq %q", seq)
		}
	}
}
