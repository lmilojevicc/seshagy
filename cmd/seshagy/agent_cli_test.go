package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

const agentPaneSep = "\x1f"

func agentCLIExplainFields(paneID string, overrides map[int]string) []string {
	fields := []string{
		paneID,
		"work",
		"1",
		"0",
		"/Users/milo/Projects/seshagy",
		"1",
		"1",
		"1",
		"0",
		"claude",
		"",
		"claude",
		"working",
		"needs ok",
		"123",
		"seshagy:claude",
		"session-123",
		"42",
		"12345",
	}
	for idx, value := range overrides {
		fields[idx] = value
	}
	return fields
}

func runWithCLIJSONHandling(args []string) error {
	if err := run(args); err != nil {
		if hasJSONFlag(args) {
			return encodeJSONError(err)
		}
		return err
	}
	return nil
}

func TestRunReportAgentRequiresName(t *testing.T) {
	cliTestEnv(t)
	const pane = "%27"
	sessionmgr.InstallAgentCLIFakeTmux(t, pane, nil)

	err := run([]string{
		"--report-agent",
		"--pane", pane,
		"--state", "working",
		"--source", "hook",
		"--json",
	})
	if err == nil {
		t.Fatal("run(--report-agent) expected name required error")
	}
	if !strings.Contains(err.Error(), "--agent/--name is required") {
		t.Fatalf("run() error = %v", err)
	}
}

func TestRunReleaseAgentWithoutSourceJSON(t *testing.T) {
	cliTestEnv(t)
	const pane = "%28"
	f := sessionmgr.InstallAgentCLIFakeTmux(t, pane, nil)

	_, err := captureStdout(t, func() error {
		return run([]string{
			"--report-agent",
			"--pane", pane,
			"--agent", "claude",
			"--state", "working",
			"--source", "hook-a",
			"--seq", "70",
			"--json",
		})
	})
	if err != nil {
		t.Fatalf("setup report error = %v", err)
	}
	if got := f.Get(pane, "@agent_source"); got != "hook-a" {
		t.Fatalf("@agent_source = %q, want hook-a", got)
	}

	releaseOut, err := captureStdout(t, func() error {
		return run([]string{
			"--release-agent",
			"--pane", pane,
			"--seq", "71",
			"--json",
		})
	})
	if err != nil {
		t.Fatalf("release without source error = %v", err)
	}
	var releasePayload struct {
		Ok       bool `json:"ok"`
		Released bool `json:"released"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(releaseOut)), &releasePayload); err != nil {
		t.Fatalf("release json.Unmarshal() error = %v, out=%q", err, releaseOut)
	}
	if !releasePayload.Ok || !releasePayload.Released {
		t.Fatalf("release payload = %#v", releasePayload)
	}
	for _, opt := range []string{"@agent_name", "@agent_state", "@agent_source"} {
		if got := f.Get(pane, opt); got != "" {
			t.Fatalf("release without source left %s = %q, want cleared", opt, got)
		}
	}
	if got := f.Get(pane, "@agent_seq"); got != "71" {
		t.Fatalf("@agent_seq = %q, want tombstone 71", got)
	}
}

func TestRunAgentListJSON(t *testing.T) {
	cliTestEnv(t)
	const pane = "%4"
	fields := agentCLIExplainFields(pane, map[int]string{
		11: "claude",
		12: "working",
		15: "seshagy:claude",
	})
	listOut := []byte(strings.Join(fields, agentPaneSep) + "\n")
	sessionmgr.InstallAgentCLIFakeTmux(t, pane, listOut)

	out, err := captureStdout(t, func() error {
		return run([]string{"agent", "list", "--json"})
	})
	if err != nil {
		t.Fatalf("run(agent list --json) error = %v", err)
	}

	var payload struct {
		SchemaVersion int    `json:"schema_version"`
		Ok            bool   `json:"ok"`
		Mode          string `json:"mode"`
		Items         []struct {
			Kind      string `json:"kind"`
			AgentName string `json:"agent_name"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, out)
	}
	if payload.SchemaVersion != 1 || !payload.Ok || payload.Mode != "agents" {
		t.Fatalf("envelope = %#v", payload)
	}
	if len(payload.Items) != 1 || payload.Items[0].Kind != "agent" ||
		payload.Items[0].AgentName != "claude" {
		t.Fatalf("items = %#v", payload.Items)
	}
}

func TestRunAgentUsageErrors(t *testing.T) {
	cliTestEnv(t)
	cases := [][]string{
		{"explain"},
		{"explain", "%1", "extra"},
		{"track"},
		{"seen", "%1", "--json", "extra"},
		{"frobnicate"},
	}
	for _, args := range cases {
		if err := runAgent(context.Background(), args); err == nil {
			t.Fatalf("runAgent(%v) expected error", args)
		}
	}
}

func TestRunAgentTrackSeen(t *testing.T) {
	cliTestEnv(t)
	const pane = "%5"
	now := time.Unix(1_700_000_000, 0)
	sessionmgr.SetAgentTrackNowForTest(t, now)

	fields := agentCLIExplainFields(pane, map[int]string{
		5:  "0",
		6:  "0",
		7:  "1",
		11: "claude",
		12: "idle",
		15: "seshagy:claude",
	})
	listOut := []byte(strings.Join(fields, agentPaneSep) + "\n")
	f := sessionmgr.InstallAgentCLIFakeTmux(t, pane, listOut)
	f.Set(pane, "@agent_name", "claude")
	f.Set(pane, "@agent_last_state", string(sessionmgr.AgentWorking))
	f.Set(pane, "@agent_last_status", string(sessionmgr.AgentWorking))
	f.Set(pane, "@agent_startup_grace", fmt.Sprintf("%d", now.Add(-3*time.Second).Unix()))
	f.Set(
		pane,
		"@agent_pending_idle_since",
		fmt.Sprintf("%d", now.Add(-700*time.Millisecond).Unix()),
	)
	f.Set(pane, "@agent_pending_idle_count", "2")

	trackOut, err := captureStdout(t, func() error {
		return run([]string{"agent", "track", pane, "--json"})
	})
	if err != nil {
		t.Fatalf("run(agent track) error = %v", err)
	}
	var trackPayload struct {
		Ok    bool   `json:"ok"`
		Pane  string `json:"pane"`
		State string `json:"state"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(trackOut)), &trackPayload); err != nil {
		t.Fatalf("track json.Unmarshal() error = %v, out=%q", err, trackOut)
	}
	if !trackPayload.Ok || trackPayload.Pane != pane ||
		trackPayload.State != string(sessionmgr.AgentDone) {
		t.Fatalf("track payload = %#v", trackPayload)
	}

	seenOut, err := captureStdout(t, func() error {
		return run([]string{"agent", "seen", pane, "--json"})
	})
	if err != nil {
		t.Fatalf("run(agent seen) error = %v", err)
	}
	var seenPayload struct {
		Ok   bool   `json:"ok"`
		Pane string `json:"pane"`
		Seen bool   `json:"seen"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(seenOut)), &seenPayload); err != nil {
		t.Fatalf("seen json.Unmarshal() error = %v, out=%q", err, seenOut)
	}
	if !seenPayload.Ok || seenPayload.Pane != pane || !seenPayload.Seen {
		t.Fatalf("seen payload = %#v", seenPayload)
	}
	wantSeen := fmt.Sprintf("%d", now.Unix())
	if got := f.Get(pane, "@agent_last_seen"); got != wantSeen {
		t.Fatalf("@agent_last_seen = %q, want %q", got, wantSeen)
	}
}

func TestRunAgentExplainTextAndJSON(t *testing.T) {
	cliTestEnv(t)
	const pane = "%6"
	fields := agentCLIExplainFields(pane, map[int]string{
		11: "claude",
		12: "working",
		15: "seshagy:claude",
	})
	listOut := []byte(strings.Join(fields, agentPaneSep) + "\n")
	sessionmgr.InstallAgentCLIFakeTmux(t, pane, listOut)

	textOut, err := captureStdout(t, func() error {
		return runAgent(context.Background(), []string{"explain", pane})
	})
	if err != nil {
		t.Fatalf("runAgent(explain) error = %v", err)
	}
	if !strings.Contains(textOut, pane) || !strings.Contains(textOut, "working") {
		t.Fatalf("explain text missing pane/state:\n%s", textOut)
	}

	jsonOut, err := captureStdout(t, func() error {
		return runAgent(context.Background(), []string{"explain", pane, "--json"})
	})
	if err != nil {
		t.Fatalf("runAgent(explain --json) error = %v", err)
	}
	var payload struct {
		Ok              bool   `json:"ok"`
		PaneID          string `json:"pane_id"`
		EffectiveStatus string `json:"effective_status"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(jsonOut)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, jsonOut)
	}
	if !payload.Ok || payload.PaneID != pane ||
		payload.EffectiveStatus != string(sessionmgr.AgentWorking) {
		t.Fatalf("explain payload = %#v", payload)
	}
}

func TestRunAgentTrackTextOutput(t *testing.T) {
	cliTestEnv(t)
	const pane = "%8"
	now := time.Unix(1_700_000_000, 0)
	sessionmgr.SetAgentTrackNowForTest(t, now)
	fields := agentCLIExplainFields(pane, map[int]string{
		11: "claude",
		12: "working",
		15: "seshagy:claude",
	})
	listOut := []byte(strings.Join(fields, agentPaneSep) + "\n")
	sessionmgr.InstallAgentCLIFakeTmux(t, pane, listOut)

	out, err := captureStdout(t, func() error {
		return runAgent(context.Background(), []string{"track", pane})
	})
	if err != nil {
		t.Fatalf("runAgent(track) error = %v", err)
	}
	want := fmt.Sprintf("%s: %s\n", pane, sessionmgr.AgentWorking)
	if out != want {
		t.Fatalf("track text = %q, want %q", out, want)
	}
}

func TestRunReportAgentJSONHonestyStaleSeq(t *testing.T) {
	cliTestEnv(t)
	const pane = "%21"
	f := sessionmgr.InstallAgentCLIFakeTmux(t, pane, nil)

	firstOut, err := captureStdout(t, func() error {
		return run([]string{
			"--report-agent",
			"--pane", pane,
			"--agent", "claude",
			"--state", "working",
			"--source", "hook",
			"--seq", "10",
			"--json",
		})
	})
	if err != nil {
		t.Fatalf("report seq 10 error = %v", err)
	}
	var firstPayload struct {
		Ok      bool `json:"ok"`
		Applied bool `json:"applied"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(firstOut)), &firstPayload); err != nil {
		t.Fatalf("first report json.Unmarshal() error = %v, out=%q", err, firstOut)
	}
	if !firstPayload.Ok || !firstPayload.Applied {
		t.Fatalf("first report payload = %#v", firstPayload)
	}

	staleOut, err := captureStdout(t, func() error {
		return run([]string{
			"--report-agent",
			"--pane", pane,
			"--agent", "claude",
			"--state", "idle",
			"--source", "hook",
			"--seq", "9",
			"--json",
		})
	})
	if err != nil {
		t.Fatalf("report seq 9 error = %v", err)
	}
	var stalePayload struct {
		Ok             bool   `json:"ok"`
		Applied        bool   `json:"applied"`
		RequestedState string `json:"requested_state"`
		State          string `json:"state"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(staleOut)), &stalePayload); err != nil {
		t.Fatalf("stale report json.Unmarshal() error = %v, out=%q", err, staleOut)
	}
	if !stalePayload.Ok || stalePayload.Applied ||
		stalePayload.RequestedState != "idle" || stalePayload.State != "working" {
		t.Fatalf("stale report payload = %#v", stalePayload)
	}
	if got := f.Get(pane, "@agent_state"); got != "working" {
		t.Fatalf("@agent_state after stale report = %q, want working", got)
	}
	if got := f.Get(pane, "@agent_seq"); got != "10" {
		t.Fatalf("@agent_seq after stale report = %q, want 10", got)
	}
}

func TestRunReportAgentJSONHonestyEqualSeq(t *testing.T) {
	cliTestEnv(t)
	const pane = "%24"
	f := sessionmgr.InstallAgentCLIFakeTmux(t, pane, nil)

	_, err := captureStdout(t, func() error {
		return run([]string{
			"--report-agent",
			"--pane", pane,
			"--agent", "claude",
			"--state", "working",
			"--source", "hook",
			"--seq", "40",
			"--json",
		})
	})
	if err != nil {
		t.Fatalf("setup report error = %v", err)
	}

	equalOut, err := captureStdout(t, func() error {
		return run([]string{
			"--report-agent",
			"--pane", pane,
			"--agent", "claude",
			"--state", "idle",
			"--source", "hook",
			"--seq", "40",
			"--json",
		})
	})
	if err != nil {
		t.Fatalf("equal seq report error = %v", err)
	}
	var equalPayload struct {
		Ok             bool   `json:"ok"`
		Applied        bool   `json:"applied"`
		RequestedState string `json:"requested_state"`
		State          string `json:"state"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(equalOut)), &equalPayload); err != nil {
		t.Fatalf("equal report json.Unmarshal() error = %v, out=%q", err, equalOut)
	}
	if !equalPayload.Ok || equalPayload.Applied ||
		equalPayload.RequestedState != "idle" || equalPayload.State != "working" {
		t.Fatalf("equal report payload = %#v", equalPayload)
	}
	if got := f.Get(pane, "@agent_state"); got != "working" {
		t.Fatalf("@agent_state after equal report = %q, want working", got)
	}
}

func TestRunReleaseAgentJSONHonestyStaleSeq(t *testing.T) {
	cliTestEnv(t)
	const pane = "%25"
	f := sessionmgr.InstallAgentCLIFakeTmux(t, pane, nil)

	_, err := captureStdout(t, func() error {
		return run([]string{
			"--report-agent",
			"--pane", pane,
			"--agent", "claude",
			"--state", "working",
			"--source", "hook",
			"--seq", "50",
			"--json",
		})
	})
	if err != nil {
		t.Fatalf("setup report error = %v", err)
	}

	staleOut, err := captureStdout(t, func() error {
		return run([]string{
			"--release-agent",
			"--pane", pane,
			"--source", "hook",
			"--seq", "49",
			"--json",
		})
	})
	if err != nil {
		t.Fatalf("stale release error = %v", err)
	}
	var stalePayload struct {
		Ok       bool `json:"ok"`
		Released bool `json:"released"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(staleOut)), &stalePayload); err != nil {
		t.Fatalf("stale release json.Unmarshal() error = %v, out=%q", err, staleOut)
	}
	if !stalePayload.Ok || stalePayload.Released {
		t.Fatalf("stale release payload = %#v", stalePayload)
	}
	if got := f.Get(pane, "@agent_name"); got != "claude" {
		t.Fatalf("@agent_name after stale release = %q, want claude", got)
	}
	if got := f.Get(pane, "@agent_seq"); got != "50" {
		t.Fatalf("@agent_seq after stale release = %q, want 50", got)
	}
}

func TestRunReleaseAgentJSONHonestyEqualSeq(t *testing.T) {
	cliTestEnv(t)
	const pane = "%26"
	f := sessionmgr.InstallAgentCLIFakeTmux(t, pane, nil)

	_, err := captureStdout(t, func() error {
		return run([]string{
			"--report-agent",
			"--pane", pane,
			"--agent", "claude",
			"--state", "working",
			"--source", "hook",
			"--seq", "60",
			"--json",
		})
	})
	if err != nil {
		t.Fatalf("setup report error = %v", err)
	}

	equalOut, err := captureStdout(t, func() error {
		return run([]string{
			"--release-agent",
			"--pane", pane,
			"--source", "hook",
			"--seq", "60",
			"--json",
		})
	})
	if err != nil {
		t.Fatalf("equal release error = %v", err)
	}
	var equalPayload struct {
		Ok       bool `json:"ok"`
		Released bool `json:"released"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(equalOut)), &equalPayload); err != nil {
		t.Fatalf("equal release json.Unmarshal() error = %v, out=%q", err, equalOut)
	}
	if !equalPayload.Ok || equalPayload.Released {
		t.Fatalf("equal release payload = %#v", equalPayload)
	}
	if got := f.Get(pane, "@agent_name"); got != "claude" {
		t.Fatalf("@agent_name after equal release = %q, want claude", got)
	}
	if got := f.Get(pane, "@agent_seq"); got != "60" {
		t.Fatalf("@agent_seq after equal release = %q, want 60", got)
	}
}

func TestRunReleaseAgentJSONHonestyWrongSource(t *testing.T) {
	cliTestEnv(t)
	const pane = "%22"
	f := sessionmgr.InstallAgentCLIFakeTmux(t, pane, nil)

	_, err := captureStdout(t, func() error {
		return run([]string{
			"--report-agent",
			"--pane", pane,
			"--agent", "claude",
			"--state", "working",
			"--source", "active",
			"--seq", "20",
			"--json",
		})
	})
	if err != nil {
		t.Fatalf("setup report error = %v", err)
	}

	releaseOut, err := captureStdout(t, func() error {
		return run([]string{
			"--release-agent",
			"--pane", pane,
			"--source", "stale",
			"--seq", "21",
			"--json",
		})
	})
	if err != nil {
		t.Fatalf("release wrong source error = %v", err)
	}
	var releasePayload struct {
		Ok       bool `json:"ok"`
		Released bool `json:"released"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(releaseOut)), &releasePayload); err != nil {
		t.Fatalf("release json.Unmarshal() error = %v, out=%q", err, releaseOut)
	}
	if !releasePayload.Ok || releasePayload.Released {
		t.Fatalf("release payload = %#v", releasePayload)
	}
	if got := f.Get(pane, "@agent_name"); got != "claude" {
		t.Fatalf("@agent_name after wrong-source release = %q, want claude", got)
	}
	if got := f.Get(pane, "@agent_source"); got != "active" {
		t.Fatalf("@agent_source after wrong-source release = %q, want active", got)
	}
}

func TestRunAgentSeenReleasedPaneJSON(t *testing.T) {
	cliTestEnv(t)
	const pane = "%27"
	f := sessionmgr.InstallAgentCLIFakeTmux(t, pane, nil)
	f.Set(pane, "@agent_seq", "90")

	out, err := captureStdout(t, func() error {
		return run([]string{"agent", "seen", pane, "--json"})
	})
	if err != nil {
		t.Fatalf("run(agent seen) error = %v", err)
	}
	var payload struct {
		Ok   bool `json:"ok"`
		Seen bool `json:"seen"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, out)
	}
	if !payload.Ok || payload.Seen {
		t.Fatalf("seen payload = %#v, want seen=false", payload)
	}
}

func TestRunReleaseAgentJSONHonestySeqLessAfterActiveSeq(t *testing.T) {
	cliTestEnv(t)
	const pane = "%29"
	f := sessionmgr.InstallAgentCLIFakeTmux(t, pane, nil)

	_, err := captureStdout(t, func() error {
		return run([]string{
			"--report-agent",
			"--pane", pane,
			"--agent", "claude",
			"--state", "working",
			"--source", "hook",
			"--seq", "80",
			"--json",
		})
	})
	if err != nil {
		t.Fatalf("setup report error = %v", err)
	}
	if got := f.Get(pane, "@agent_seq"); got != "80" {
		t.Fatalf("setup @agent_seq = %q, want 80", got)
	}

	seqLessOut, err := captureStdout(t, func() error {
		return run([]string{
			"--release-agent",
			"--pane", pane,
			"--source", "hook",
			"--json",
		})
	})
	if err != nil {
		t.Fatalf("seq-less release error = %v", err)
	}
	var seqLessPayload struct {
		Ok       bool   `json:"ok"`
		Released bool   `json:"released"`
		Pane     string `json:"pane"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(seqLessOut)), &seqLessPayload); err != nil {
		t.Fatalf("seq-less release json.Unmarshal() error = %v, out=%q", err, seqLessOut)
	}
	if !seqLessPayload.Ok || seqLessPayload.Released || seqLessPayload.Pane != pane {
		t.Fatalf("seq-less release payload = %#v", seqLessPayload)
	}
	if got := f.Get(pane, "@agent_name"); got != "claude" {
		t.Fatalf("@agent_name after seq-less release = %q, want claude", got)
	}
	if got := f.Get(pane, "@agent_seq"); got != "80" {
		t.Fatalf("@agent_seq after seq-less release = %q, want 80", got)
	}
}

func TestRunReportAgentJSONHonestySeqLessAfterActiveSeq(t *testing.T) {
	cliTestEnv(t)
	const pane = "%23"
	f := sessionmgr.InstallAgentCLIFakeTmux(t, pane, nil)

	_, err := captureStdout(t, func() error {
		return run([]string{
			"--report-agent",
			"--pane", pane,
			"--agent", "claude",
			"--state", "working",
			"--source", "hook",
			"--seq", "30",
			"--json",
		})
	})
	if err != nil {
		t.Fatalf("setup report error = %v", err)
	}
	if got := f.Get(pane, "@agent_seq"); got != "30" {
		t.Fatalf("setup @agent_seq = %q, want 30", got)
	}

	seqLessOut, err := captureStdout(t, func() error {
		return run([]string{
			"--report-agent",
			"--pane", pane,
			"--agent", "claude",
			"--state", "idle",
			"--source", "hook",
			"--json",
		})
	})
	if err != nil {
		t.Fatalf("seq-less report error = %v", err)
	}
	var seqLessPayload struct {
		Ok             bool   `json:"ok"`
		Applied        bool   `json:"applied"`
		RequestedState string `json:"requested_state"`
		State          string `json:"state"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(seqLessOut)), &seqLessPayload); err != nil {
		t.Fatalf("seq-less report json.Unmarshal() error = %v, out=%q", err, seqLessOut)
	}
	if !seqLessPayload.Ok || seqLessPayload.Applied ||
		seqLessPayload.RequestedState != "idle" || seqLessPayload.State != "working" {
		t.Fatalf("seq-less report payload = %#v", seqLessPayload)
	}
	if got := f.Get(pane, "@agent_state"); got != "working" {
		t.Fatalf("@agent_state after seq-less report = %q, want working", got)
	}
	if got := f.Get(pane, "@agent_seq"); got != "30" {
		t.Fatalf("@agent_seq after seq-less report = %q, want 30", got)
	}
}
