package sessionmgr

import (
	"context"
	"os/exec"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// psEtime is the seam for the batched process start-time query; tests replace it
// to inject etime output (mirroring the procSnapshot pattern in proctree.go).
var psEtime = defaultPsEtime

// ProcessStartTimes resolves when each pid started, using ONE batched ps call so
// a list of agents costs a single subprocess. Pids ps cannot report (a process
// that exited between discovery and the query) are simply left out of the
// result, which the caller renders as "no age".
func ProcessStartTimes(
	ctx context.Context,
	pids []int,
	now time.Time,
) (map[int]time.Time, error) {
	pids = uniquePIDs(pids)
	if len(pids) == 0 {
		return nil, nil
	}
	out, err := psEtime(ctx, pids)
	if err != nil {
		return nil, err
	}
	return parsePsEtime(out, now), nil
}

// defaultPsEtime reads elapsed process times with `ps -o pid=,etime= -p <list>`.
// List form is portable across macOS (BSD ps) and Linux (procps ps); the
// trailing = on each -o field suppresses the header line.
func defaultPsEtime(ctx context.Context, pids []int) ([]byte, error) {
	ids := make([]string, len(pids))
	for i, pid := range pids {
		ids[i] = strconv.Itoa(pid)
	}
	return exec.CommandContext(ctx, "ps", "-o", "pid=,etime=", "-p", strings.Join(ids, ",")).
		Output()
}

// parsePsEtime parses `ps -o pid=,etime=` output into pid → start time. now is
// the reference the elapsed time is subtracted from.
func parsePsEtime(data []byte, now time.Time) map[int]time.Time {
	starts := make(map[int]time.Time)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid <= 1 {
			continue
		}
		elapsed, ok := parseEtime(fields[1])
		if !ok {
			continue
		}
		starts[pid] = now.Add(-elapsed)
	}
	return starts
}

// parseEtime parses ps's elapsed-time format `[[dd-]hh:]mm:ss`.
func parseEtime(value string) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	days := 0
	if dash := strings.IndexByte(value, '-'); dash >= 0 {
		parsed, err := strconv.Atoi(value[:dash])
		if err != nil || parsed < 0 {
			return 0, false
		}
		days = parsed
		value = value[dash+1:]
	}
	parts := strings.Split(value, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	nums := make([]int, 0, len(parts))
	for _, part := range parts {
		parsed, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || parsed < 0 {
			return 0, false
		}
		nums = append(nums, parsed)
	}
	hours, minutes, seconds := 0, nums[0], nums[1]
	if len(nums) == 3 {
		hours, minutes, seconds = nums[0], nums[1], nums[2]
	}
	if minutes > 59 || seconds > 59 {
		return 0, false
	}
	return time.Duration(days)*24*time.Hour +
		time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second, true
}

// uniquePIDs drops duplicates and unparsable pids and sorts what is left, so one
// ps call carries each pid once and the argument list is deterministic.
func uniquePIDs(pids []int) []int {
	unique := make([]int, 0, len(pids))
	for _, pid := range pids {
		if pid > 1 {
			unique = append(unique, pid)
		}
	}
	sort.Ints(unique)
	return slices.Compact(unique)
}
