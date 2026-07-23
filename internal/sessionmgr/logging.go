package sessionmgr

import (
	"context"
	"log/slog"
	"time"

	"github.com/lmilojevicc/seshagy/internal/logging"
)

func sessionKillStart(ctx context.Context) time.Time {
	logger := logging.Default()
	if logger.Enabled(ctx, slog.LevelInfo) || logger.Enabled(ctx, slog.LevelError) {
		return time.Now()
	}
	return time.Time{}
}

func logSessionKill(ctx context.Context, backend BackendKind, started time.Time, err error) {
	if started.IsZero() {
		return
	}
	level, result := slog.LevelInfo, "success"
	attrs := []slog.Attr{
		slog.String("backend", string(backend)),
		slog.String("result", result),
		slog.Int64("duration_ms", time.Since(started).Milliseconds()),
	}
	if err != nil {
		level, result = slog.LevelError, "failed"
		attrs[1] = slog.String("result", result)
		attrs = append(attrs, slog.String("error_class", logging.ClassifyError(err)))
	}
	logging.LogAttrs(
		ctx,
		logging.Default(),
		level,
		logging.EventSessionKill,
		logging.ComponentSession,
		attrs...)
}
