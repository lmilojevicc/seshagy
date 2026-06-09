package sessionmgr

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func ListZoxideDirs(ctx context.Context) ([]Item, error) {
	if _, err := exec.LookPath("zoxide"); err != nil {
		return nil, nil
	}
	out, err := plainCommand(ctx, "zoxide", "query", "-l").Output()
	if err != nil {
		return nil, nil
	}
	return dirItems(out, KindZoxide), nil
}

func ListFDirs(ctx context.Context) ([]Item, error) {
	if _, err := exec.LookPath("fd"); err != nil {
		return nil, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil
	}
	out, err := plainCommand(ctx, "fd", "-H", "-a", "-d", "2", "-t", "d", "-E", ".Trash", ".", home).Output()
	if err != nil {
		return nil, nil
	}
	items := dirItems(out, KindFD)
	sort.SliceStable(items, func(i, j int) bool { return items[i].Path < items[j].Path })
	return items, nil
}

func dirItems(out []byte, kind Kind) []Item {
	lines := bytes.Split(bytes.TrimSpace(out), []byte("\n"))
	items := make([]Item, 0, len(lines))
	for _, line := range lines {
		path := strings.TrimSpace(string(line))
		if path == "" {
			continue
		}
		label := ContractHome(path)
		items = append(items, Item{Kind: kind, Name: label, Target: ExpandHome(label), Path: label})
	}
	return items
}

func ListDirectoryPreview(ctx context.Context, dir string, maxLines int) (string, error) {
	dir = ExpandHome(dir)
	if _, err := os.Stat(dir); err != nil {
		return "", err
	}
	if _, err := exec.LookPath("eza"); err == nil {
		out, err := plainCommand(ctx, "eza", "-lah", "--icons=always", "--sort=name", "--group-directories-first", "--color=always", dir).CombinedOutput()
		return limitLines(string(out), maxLines), err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, entry := range entries {
		info, _ := entry.Info()
		kind := " "
		if entry.IsDir() {
			kind = "/"
		}
		size := ""
		if info != nil && !entry.IsDir() {
			size = fmt.Sprintf("%8d", info.Size())
		}
		fmt.Fprintf(&b, "%s %-40s %s\n", kind, entry.Name(), size)
	}
	return limitLines(b.String(), maxLines), nil
}

func RunYaziCommand(cwdFile string) *exec.Cmd {
	if cwdFile == "" {
		cwdFile = filepath.Join(os.TempDir(), "seshagy-yazi-cwd")
	}
	return exec.Command("yazi", "--cwd-file", cwdFile)
}

func limitLines(s string, max int) string {
	if max <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= max {
		return s
	}
	return strings.Join(lines[:max], "\n")
}
