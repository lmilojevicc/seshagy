package sessionmgr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const DefaultFDCommand = `fd -H -a -d 2 -t d -E .Trash . "$HOME"`

func ListZoxideDirs(ctx context.Context) ([]Item, error) {
	if !commandOnPath("zoxide") {
		return nil, nil
	}
	out, err := optionalCommandOutput(plainCommand(ctx, "zoxide", "query", "-l"))
	if err != nil {
		return nil, fmt.Errorf("zoxide query: %w", err)
	}
	return dirItems(out, KindZoxide), nil
}

func ListFDirsWithCommand(ctx context.Context, command string) ([]Item, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		command = DefaultFDCommand
	}
	out, err := optionalCommandOutput(shellCommand(ctx, command))
	if err != nil {
		return nil, fmt.Errorf("fd command: %w", err)
	}
	items := dirItems(out, KindFD)
	sort.SliceStable(items, func(i, j int) bool { return items[i].Path < items[j].Path })
	return items, nil
}

func commandOnPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// optionalCommandOutput runs cmd and returns its stdout. A non-zero exit (for
// example zoxide reporting an empty database) is treated as "no results" with a
// nil error, but a failure to start the command (missing binary, etc.) is
// surfaced so callers can report it instead of silently showing nothing.
func optionalCommandOutput(cmd *exec.Cmd) ([]byte, error) {
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out, nil
		}
		return nil, err
	}
	return out, nil
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
		out, err := plainCommand(
			ctx,
			"eza",
			"-lah",
			"--icons=always",
			"--sort=name",
			"--group-directories-first",
			"--color=always",
			dir,
		).CombinedOutput()
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
