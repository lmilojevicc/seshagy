// Package logging provides privacy-bounded, file-only structured diagnostics.
package logging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/lmilojevicc/seshagy/internal/xdg"
)

const (
	SchemaVersion = 1
	MaxFileBytes  = 25 * 1024 * 1024
	RetainFiles   = 10
)

const (
	EventAppStart             Event = "app.start"
	EventAppStop              Event = "app.stop"
	EventLogLimitReached      Event = "log.limit_reached"
	EventLogRetention         Event = "log.retention"
	EventSourceLoad           Event = "source.load"
	EventSourceLoadFailed     Event = "source.load_failed"
	EventSourceLoadDegraded   Event = "source.load_degraded"
	EventSessionKill          Event = "session.kill"
	EventSessionFocusRestore  Event = "session.focus_restore"
	EventAgentReport          Event = "agent.report"
	EventAgentReportFailed    Event = "agent.report_failed"
	EventAgentRelease         Event = "agent.release"
	EventAgentReleaseFailed   Event = "agent.release_failed"
	EventManifestSweep        Event = "manifest.sweep"
	EventManifestStateChange  Event = "manifest.state_change"
	EventIntegrationInstall   Event = "integration.install"
	EventIntegrationUninstall Event = "integration.uninstall"
	EventTUIRefreshStale      Event = "tui.refresh_stale"
)

const (
	ComponentApp          Component = "app"
	ComponentLogging      Component = "logging"
	ComponentSession      Component = "sessionmgr"
	ComponentAgents       Component = "agents"
	ComponentManifest     Component = "manifest"
	ComponentIntegrations Component = "integrations"
	ComponentTUI          Component = "tui"
)

type (
	Event     string
	Component string
)

type Config struct {
	Level string
	File  string
}

type Resolved struct {
	LevelName string
	MinLevel  slog.Level
	Enabled   bool
	Explicit  bool
	File      string
	Directory string
}

type Status struct {
	Directory       string
	File            string
	Latest          string
	DirectoryExists bool
	FileExists      bool
	LatestPresent   bool
	FileType        string
	SizeBytes       int64
}

type Metadata struct {
	AppVersion string
}

var packageDefault atomic.Pointer[slog.Logger]

func init() {
	packageDefault.Store(slog.New(slog.DiscardHandler))
}

func Default() *slog.Logger { return packageDefault.Load() }

func Resolve(cfg Config, lookupEnv func(string) (string, bool)) (Resolved, error) {
	level := strings.TrimSpace(cfg.Level)
	file := strings.TrimSpace(cfg.File)
	if value, ok := lookupEnv("SESHAGY_LOG_LEVEL"); ok && strings.TrimSpace(value) != "" {
		level = strings.TrimSpace(value)
	}
	if value, ok := lookupEnv("SESHAGY_LOG_FILE"); ok && strings.TrimSpace(value) != "" {
		file = strings.TrimSpace(value)
	}
	level = strings.ToLower(level)
	if level == "" {
		level = "off"
	}
	resolved := Resolved{LevelName: level}
	switch level {
	case "off":
		resolved.MinLevel = slog.LevelError
	case "debug":
		resolved.Enabled, resolved.MinLevel = true, slog.LevelDebug
	case "info":
		resolved.Enabled, resolved.MinLevel = true, slog.LevelInfo
	case "warn":
		resolved.Enabled, resolved.MinLevel = true, slog.LevelWarn
	case "error":
		resolved.Enabled, resolved.MinLevel = true, slog.LevelError
	default:
		return Resolved{}, fmt.Errorf(
			"invalid log level %q (want off, debug, info, warn, or error)",
			level,
		)
	}
	if file != "" {
		resolved.Explicit = true
		resolved.File = xdg.ExpandHome(file)
		resolved.Directory = filepath.Dir(resolved.File)
	} else {
		resolved.Directory = filepath.Join(xdg.StateHome(), "seshagy", "log")
	}
	return resolved, nil
}

func Inspect(resolved Resolved) (Status, error) {
	status := Status{Directory: resolved.Directory, File: resolved.File, FileType: "missing"}
	if info, err := os.Stat(resolved.Directory); err == nil {
		status.DirectoryExists = info.IsDir()
	} else if !errors.Is(err, os.ErrNotExist) {
		return status, err
	}
	if resolved.Explicit {
		kind, size, exists, err := inspectPath(resolved.File)
		status.FileType, status.SizeBytes, status.FileExists = kind, size, exists
		return status, err
	}
	entries, err := os.ReadDir(resolved.Directory)
	if errors.Is(err, os.ErrNotExist) {
		return status, nil
	}
	if err != nil {
		return status, err
	}
	var newest os.FileInfo
	for _, entry := range entries {
		if !isDefaultLogName(entry.Name()) || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if newest == nil || info.ModTime().After(newest.ModTime()) ||
			(info.ModTime().Equal(newest.ModTime()) && info.Name() > newest.Name()) {
			newest = info
		}
	}
	if newest != nil {
		status.LatestPresent = true
		status.Latest = filepath.Join(resolved.Directory, newest.Name())
		status.FileType = "regular"
		status.FileExists = true
		status.SizeBytes = newest.Size()
	}
	return status, nil
}

func inspectPath(path string) (kind string, size int64, exists bool, err error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "missing", 0, false, nil
	}
	if err != nil {
		return "missing", 0, false, err
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		return "symlink", info.Size(), true, nil
	case info.Mode().IsRegular():
		return "regular", info.Size(), true, nil
	default:
		return "other", info.Size(), true, nil
	}
}

func ClassifyError(err error) string {
	if err == nil {
		return "unknown"
	}
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded), os.IsTimeout(err):
		return "timeout"
	case errors.Is(err, os.ErrPermission):
		return "permission"
	case errors.Is(err, os.ErrNotExist):
		return "not_found"
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return "exec"
	}
	var syntaxErr *json.SyntaxError
	var numErr *strconv.NumError
	if errors.As(err, &syntaxErr) || errors.As(err, &numErr) {
		return "parse"
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return "io"
	}
	return "unknown"
}

func LogAttrs(
	ctx context.Context,
	logger *slog.Logger,
	level slog.Level,
	event Event,
	component Component,
	attrs ...slog.Attr,
) {
	if logger == nil {
		logger = Default()
	}
	if !logger.Enabled(ctx, level) {
		return
	}
	base := []slog.Attr{
		slog.String("event", string(event)),
		slog.String("component", string(component)),
	}
	base = append(base, attrs...)
	logger.LogAttrs(ctx, level, string(event), base...)
}
