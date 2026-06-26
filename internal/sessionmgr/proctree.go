package sessionmgr

import (
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// procEntry is a single process from an OS process snapshot, trimmed to the
// fields needed for agent discovery: pid, parent pid, and comm (process name).
type procEntry struct {
	pid  int32
	ppid int32
	comm string
}

// procSnapshot returns the full process table as a pid -> procEntry map. It is
// the seam for reading the process tree; tests override it to inject a fake
// table (mirroring the tmuxOutput/tmuxRun pattern in exec.go).
var procSnapshot = defaultProcSnapshot

// snapshotCache memoizes a procSnapshot for the lifetime of a single
// ListAgents/ParseAgents call so the descendant fallback reads the process
// table at most once across all panes. The snapshot is built lazily — only
// when the first pane needs the fallback.
type snapshotCache struct {
	once  sync.Once
	table map[int32]procEntry
	err   error
}

func (c *snapshotCache) get() (map[int32]procEntry, error) {
	c.once.Do(func() {
		c.table, c.err = procSnapshot()
	})
	return c.table, c.err
}

// maxProcDepth bounds the descendant walk. The agent wrapper is a direct child
// of the pane shell (depth 1) but a wrapper-script layer can push it deeper.
const maxProcDepth = 4

// descendants returns the descendant PIDs of root via a bounded BFS. The
// visited set prevents cycles in inconsistent ppid data; maxProcDepth bounds
// the walk. PIDs 0 and 1 are never returned.
func descendants(table map[int32]procEntry, root int32) []int32 {
	children := make(map[int32][]int32)
	for pid, e := range table {
		if pid <= 1 {
			continue
		}
		children[e.ppid] = append(children[e.ppid], pid)
	}
	// Sort each group so the descendant order — and therefore which agent a
	// multi-agent pane resolves to — is deterministic across refreshes.
	for _, c := range children {
		sort.Slice(c, func(i, j int) bool { return c[i] < c[j] })
	}

	var out []int32
	visited := map[int32]bool{root: true}
	queue := []int32{root}
	for depth := 0; depth < maxProcDepth && len(queue) > 0; depth++ {
		var next []int32
		for _, pid := range queue {
			for _, child := range children[pid] {
				if child <= 1 || visited[child] {
					continue
				}
				visited[child] = true
				out = append(out, child)
				next = append(next, child)
			}
		}
		queue = next
	}
	return out
}

// detectAgent resolves the canonical agent name for a pane. It first tries the
// fast path (the foreground command from pane_current_command). When that
// fails — typically for node-based agents (pi, opencode, cursor-agent) that
// report as "node" — it falls back to walking the pane's process descendants
// and matching each comm against the same agentProcessNames table.
func detectAgent(command, panePID string, cache *snapshotCache) string {
	if agent := detectAgentName(command); agent != "" {
		return agent
	}
	if panePID == "" || cache == nil {
		return ""
	}
	pid, err := strconv.Atoi(panePID)
	if err != nil || pid <= 1 {
		return ""
	}
	table, err := cache.get()
	if err != nil || len(table) == 0 {
		return ""
	}
	for _, child := range descendants(table, int32(pid)) {
		if agent := detectAgentName(table[child].comm); agent != "" {
			return agent
		}
	}
	return ""
}

// defaultProcSnapshot reads the process table via ps. The comm field from ps
// preserves the wrapper name (e.g. "pi") rather than the exec'd binary (e.g.
// "node") which is what the kernel's p_comm reports — making ps essential for
// detecting node-based agents. BSD ps (macOS) uses -ax; procps ps (Linux) uses
// -e. The trailing = on each -o field suppresses the header line.
func defaultProcSnapshot() (map[int32]procEntry, error) {
	args := []string{"-o", "pid=,ppid=,comm="}
	switch runtime.GOOS {
	case "darwin", "freebsd", "netbsd", "openbsd":
		args = append([]string{"-ax"}, args...)
	default:
		args = append([]string{"-e"}, args...)
	}
	out, err := exec.Command("ps", args...).Output()
	if err != nil {
		return nil, err
	}
	return parsePsSnapshot(out), nil
}

// parsePsSnapshot parses `ps -o pid=,ppid=,comm=` output into a procEntry map.
// Each line is whitespace-separated: pid, ppid, then the comm (which may
// contain spaces and is everything after the second field).
func parsePsSnapshot(data []byte) map[int32]procEntry {
	table := make(map[int32]procEntry)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid <= 1 {
			continue
		}
		ppid, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		table[int32(pid)] = procEntry{
			pid:  int32(pid),
			ppid: int32(ppid),
			comm: strings.Join(fields[2:], " "),
		}
	}
	return table
}
