package logging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"
)

var (
	nowFunc      = time.Now
	randomIDFunc = randomID
	defaultLogRE = regexp.MustCompile(
		`^seshagy-([0-9]{8}T[0-9]{6}\.[0-9]{9}Z)-([1-9][0-9]*)-([0-9a-f]{16})\.jsonl$`,
	)
)

var activeRuntimes struct {
	sync.Mutex
	base  *slog.Logger
	stack []*Runtime
}

type Runtime struct {
	logger *slog.Logger
	file   *os.File
	writer *cappedWriter

	mu     sync.Mutex
	active bool
	closed bool
}

func (r *Runtime) Logger() *slog.Logger { return r.logger }

func Open(resolved Resolved, metadata Metadata) (*Runtime, error) {
	if !resolved.Enabled {
		return &Runtime{logger: slog.New(slog.DiscardHandler)}, nil
	}
	runID, err := randomIDFunc()
	if err != nil {
		return nil, fmt.Errorf("create log run id: %w", err)
	}
	var file *os.File
	var path string
	var retention retentionResult
	if resolved.Explicit {
		file, err = openExplicit(resolved.File)
	} else {
		file, path, err = openDefault(resolved.Directory, runID)
		if err == nil {
			retention = pruneDefaultLogs(resolved.Directory, path)
		}
	}
	if err != nil {
		return nil, err
	}
	appVersion := metadata.AppVersion
	if appVersion == "" {
		appVersion = "unknown"
	}
	marker, err := limitMarker(MaxFileBytes, runID, appVersion)
	if err != nil {
		_ = unlockFile(file)
		_ = file.Close()
		return nil, err
	}
	writer, err := newCappedWriter(file, MaxFileBytes, marker)
	if err != nil {
		_ = unlockFile(file)
		_ = file.Close()
		return nil, err
	}
	jsonHandler := slog.NewJSONHandler(
		writer,
		&slog.HandlerOptions{Level: resolved.MinLevel, AddSource: false},
	)
	logger := slog.New(newSafetyHandler(jsonHandler)).With(
		slog.Int("schema_version", SchemaVersion),
		slog.String("run_id", runID),
		slog.String("app_version", appVersion),
	)
	runtime := &Runtime{logger: logger, file: file, writer: writer}
	if retention.warningCount > 0 {
		LogAttrs(context.Background(), logger, slog.LevelWarn, EventLogRetention, ComponentLogging,
			slog.Int("item_count", retention.itemCount),
			slog.Int("skipped_count", retention.skippedCount),
			slog.Int("warning_count", retention.warningCount),
			slog.String("error_class", retention.errorClass),
		)
	}
	return runtime, nil
}

func (r *Runtime) Activate() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active || r.closed {
		return
	}
	activeRuntimes.Lock()
	if len(activeRuntimes.stack) == 0 {
		activeRuntimes.base = packageDefault.Load()
	}
	activeRuntimes.stack = append(activeRuntimes.stack, r)
	packageDefault.Store(r.logger)
	activeRuntimes.Unlock()
	r.active = true
}

func (r *Runtime) Shutdown() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	if r.active {
		activeRuntimes.Lock()
		for i, active := range activeRuntimes.stack {
			if active != r {
				continue
			}
			activeRuntimes.stack = append(activeRuntimes.stack[:i], activeRuntimes.stack[i+1:]...)
			break
		}
		if count := len(activeRuntimes.stack); count > 0 {
			packageDefault.Store(activeRuntimes.stack[count-1].logger)
		} else {
			packageDefault.Store(activeRuntimes.base)
			activeRuntimes.base = nil
		}
		activeRuntimes.Unlock()
		r.active = false
	}
	if r.file == nil {
		return nil
	}
	var errs []error
	if r.writer != nil && r.writer.Err() != nil {
		errs = append(errs, r.writer.Err())
	}
	if err := r.file.Sync(); err != nil {
		errs = append(errs, err)
	}
	if err := unlockFile(r.file); err != nil {
		errs = append(errs, err)
	}
	if err := r.file.Close(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func randomID() (string, error) {
	var data [8]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(data[:]), nil
}

func openDefault(directory, runID string) (*os.File, string, error) {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, "", err
	}
	name := fmt.Sprintf(
		"seshagy-%s-%d-%s.jsonl",
		nowFunc().UTC().Format("20060102T150405.000000000Z"),
		os.Getpid(),
		runID,
	)
	path := filepath.Join(directory, name)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return nil, "", err
	}
	if err := tryLockFile(file); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, "", fmt.Errorf("lock log destination: %w", err)
	}
	return file, path, nil
}

func openExplicit(path string) (*os.File, error) {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, errors.New("log destination must be a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := tryLockFile(file); err != nil {
		_ = file.Close()
		return nil, errors.New("log destination busy")
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = unlockFile(file)
		_ = file.Close()
		return nil, errors.New("log destination must be a regular file")
	}
	if err := file.Chmod(0o600); err != nil {
		_ = unlockFile(file)
		_ = file.Close()
		return nil, err
	}
	if err := file.Truncate(0); err != nil {
		_ = unlockFile(file)
		_ = file.Close()
		return nil, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = unlockFile(file)
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

type retentionResult struct {
	itemCount    int
	skippedCount int
	warningCount int
	errorClass   string
}

type logEntry struct {
	path string
	info os.FileInfo
}

func pruneDefaultLogs(directory, current string) retentionResult {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return retentionResult{warningCount: 1, errorClass: ClassifyError(err)}
	}
	logs := make([]logEntry, 0, len(entries))
	for _, entry := range entries {
		if !isDefaultLogName(entry.Name()) || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		logs = append(logs, logEntry{path: filepath.Join(directory, entry.Name()), info: info})
	}
	sort.Slice(logs, func(i, j int) bool {
		if logs[i].info.ModTime().Equal(logs[j].info.ModTime()) {
			return logs[i].info.Name() > logs[j].info.Name()
		}
		return logs[i].info.ModTime().After(logs[j].info.ModTime())
	})
	result := retentionResult{itemCount: len(logs), errorClass: "unknown"}
	for i, entry := range logs {
		if i < RetainFiles || entry.path == current {
			continue
		}
		skipped, err := removeUnlockedLog(entry.path)
		if skipped {
			result.skippedCount++
			continue
		}
		if err != nil {
			result.warningCount++
			result.errorClass = ClassifyError(err)
		}
	}
	return result
}

func isDefaultLogName(name string) bool {
	matches := defaultLogRE.FindStringSubmatch(name)
	if matches == nil {
		return false
	}
	if _, err := time.Parse("20060102T150405.000000000Z", matches[1]); err != nil {
		return false
	}
	pid, err := strconv.ParseUint(matches[2], 10, 64)
	return err == nil && pid > 0
}

func limitMarker(limit int64, runID, appVersion string) ([]byte, error) {
	line, err := json.Marshal(map[string]any{
		"time": nowFunc().UTC().
			Format(time.RFC3339Nano),
		"level":          "WARN",
		"msg":            string(EventLogLimitReached),
		"schema_version": SchemaVersion,
		"run_id":         runID,
		"app_version":    appVersion,
		"event": string(
			EventLogLimitReached,
		),
		"component":  string(ComponentLogging),
		"byte_limit": limit,
	})
	if err != nil {
		return nil, err
	}
	return append(line, '\n'), nil
}

type cappedWriter struct {
	mu      sync.Mutex
	writer  io.Writer
	limit   int64
	marker  []byte
	written int64
	stopped bool
	err     error
}

func newCappedWriter(writer io.Writer, limit int64, marker []byte) (*cappedWriter, error) {
	if int64(len(marker)) > limit {
		return nil, errors.New("log byte limit is too small for limit marker")
	}
	return &cappedWriter{writer: writer, limit: limit, marker: append([]byte(nil), marker...)}, nil
}

func (w *cappedWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped || w.err != nil {
		return len(data), nil
	}
	if w.written+int64(len(data))+int64(len(w.marker)) > w.limit {
		w.writeUnderlying(w.marker)
		w.stopped = true
		return len(data), nil
	}
	w.writeUnderlying(data)
	return len(data), nil
}

func (w *cappedWriter) writeUnderlying(data []byte) {
	if w.err != nil {
		return
	}
	n, err := w.writer.Write(data)
	w.written += int64(n)
	if err != nil {
		w.err = err
	} else if n != len(data) {
		w.err = io.ErrShortWrite
	}
}

func (w *cappedWriter) Err() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.err
}
