package sessionmgr

import (
	"context"
	"errors"
	"testing"
	"time"
)

// setPsEtimeForTest swaps the ps etime seam for the duration of a test.
func setPsEtimeForTest(
	t *testing.T,
	fn func(context.Context, []int) ([]byte, error),
) {
	t.Helper()
	orig := psEtime
	psEtime = fn
	t.Cleanup(func() { psEtime = orig })
}

func TestParseEtime(t *testing.T) {
	tests := []struct {
		in    string
		want  time.Duration
		valid bool
	}{
		{in: "47", valid: false},
		{in: "", valid: false},
		{in: "00:47", want: 47 * time.Second, valid: true},
		{in: "12:00", want: 12 * time.Minute, valid: true},
		{in: "59:59", want: 59*time.Minute + 59*time.Second, valid: true},
		{in: "02:04:00", want: 2*time.Hour + 4*time.Minute, valid: true},
		{in: "23:59:59", want: 23*time.Hour + 59*time.Minute + 59*time.Second, valid: true},
		{in: "01-03:03:46", want: 27*time.Hour + 3*time.Minute + 46*time.Second, valid: true},
		{in: "4-00:00:01", want: 96*time.Hour + time.Second, valid: true},
		{in: "4-1:2:3", want: 97*time.Hour + 2*time.Minute + 3*time.Second, valid: true},
		// Malformed values are rejected rather than guessed at.
		{in: "12:60", valid: false},
		{in: "12:00:60", valid: false},
		{in: "1:2:3:4", valid: false},
		{in: "x:00", valid: false},
		{in: "-1:00", valid: false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, ok := parseEtime(tt.in)
			if ok != tt.valid {
				t.Fatalf("parseEtime(%q) ok = %v, want %v", tt.in, ok, tt.valid)
			}
			if ok && got != tt.want {
				t.Fatalf("parseEtime(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParsePsEtimeSubtractsElapsedFromNow(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	raw := []byte("  200 12:00\n" +
		" 100 01-03:03:46\n" +
		" 300 00:00\n" +
		" 400 \n" + // ps printed no etime for a process that just exited
		"garbage line\n")
	starts := parsePsEtime(raw, now)
	if len(starts) != 3 {
		t.Fatalf("parsePsEtime() = %d entries, want 3", len(starts))
	}
	if got := starts[200]; !got.Equal(now.Add(-12 * time.Minute)) {
		t.Errorf("starts[200] = %v, want now-12m", got)
	}
	if got := starts[300]; !got.Equal(now) {
		t.Errorf("starts[300] = %v, want now", got)
	}
	// "12:00" is minutes:seconds, "01-03:03:46" is days-hours:minutes:seconds.
	if got := starts[100]; !got.Equal(now.Add(-(27*time.Hour + 3*time.Minute + 46*time.Second))) {
		t.Errorf("starts[100] = %v, want now-27h03m46s", got)
	}
}

func TestProcessStartTimesBatchesEveryPidIntoOneCall(t *testing.T) {
	var calls [][]int
	setPsEtimeForTest(t, func(_ context.Context, pids []int) ([]byte, error) {
		calls = append(calls, pids)
		return []byte("100 00:10\n200 00:20\n"), nil
	})
	now := time.Now()
	starts, err := ProcessStartTimes(context.Background(), []int{200, 100, 200, 0}, now)
	if err != nil {
		t.Fatalf("ProcessStartTimes() error = %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("ps called %d times, want 1", len(calls))
	}
	// Duplicates and non-process pids are dropped, and the list is sorted so
	// the argument string is deterministic.
	if got := calls[0]; len(got) != 2 || got[0] != 100 || got[1] != 200 {
		t.Fatalf("ps pids = %v, want [100 200]", got)
	}
	if len(starts) != 2 {
		t.Fatalf("ProcessStartTimes() = %d entries, want 2", len(starts))
	}
}

func TestProcessStartTimesSkipsTheCallWithoutPids(t *testing.T) {
	called := false
	setPsEtimeForTest(t, func(context.Context, []int) ([]byte, error) {
		called = true
		return nil, nil
	})
	starts, err := ProcessStartTimes(context.Background(), []int{0, 1}, time.Now())
	if err != nil {
		t.Fatalf("ProcessStartTimes() error = %v", err)
	}
	if called {
		t.Fatal("ps was called with no usable pids")
	}
	if len(starts) != 0 {
		t.Fatalf("ProcessStartTimes() = %v, want no entries", starts)
	}
}

func TestProcessStartTimesReportsPsFailure(t *testing.T) {
	setPsEtimeForTest(t, func(context.Context, []int) ([]byte, error) {
		return nil, errors.New("ps: command not found")
	})
	if _, err := ProcessStartTimes(context.Background(), []int{100}, time.Now()); err == nil {
		t.Fatal("ProcessStartTimes() error = nil, want the ps failure")
	}
}
