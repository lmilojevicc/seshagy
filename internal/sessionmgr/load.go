package sessionmgr

import (
	"context"
	"strings"
)

type SourceMode int

type LoadOptions struct {
	FDCommand        string
	ManifestFallback bool
}

// LoadResult is the outcome of a source load. Warning carries non-fatal source
// failures (for example zoxide or fd unavailable) while Items still holds any
// data that loaded successfully.
type LoadResult struct {
	Items   []Item
	Warning string
}

const (
	ModeAll SourceMode = iota
	ModeSessions
	ModeZoxide
	ModeFD
	ModeAgents
	ModeCurrentAgents
)

// LoadWithOptions is a compatibility wrapper that detects the active
// multiplexer from the environment and delegates to LoadWithBackend. Existing
// callers under $TMUX keep identical behaviour.
func LoadWithOptions(ctx context.Context, mode SourceMode, opts LoadOptions) (LoadResult, error) {
	return LoadWithBackend(ctx, Detect(), mode, opts)
}

// LoadWithBackend dispatches item loading across sources, routing the
// multiplexer-backed modes (sessions, agents) through the supplied backend.
// Directory modes (zoxide, fd) are backend-independent. The capture-pane
// manifest fallback runs only for the tmux backend, since herdr owns agent
// state detection under herdr.
func LoadWithBackend(
	ctx context.Context,
	mux Multiplexer,
	mode SourceMode,
	opts LoadOptions,
) (LoadResult, error) {
	runManifest := mux.Kind() == BackendTmux && opts.ManifestFallback
	switch mode {
	case ModeSessions:
		items, err := mux.ListSessions(ctx)
		return LoadResult{Items: items}, err
	case ModeZoxide:
		items, err := ListZoxideDirs(ctx)
		return LoadResult{Items: items}, err
	case ModeFD:
		items, err := ListFDirsWithCommand(ctx, opts.FDCommand)
		return LoadResult{Items: items}, err
	case ModeAgents:
		items, err := mux.ListAgents(ctx, "")
		if err == nil && runManifest {
			ApplyManifestFallback(ctx, items)
		}
		return LoadResult{Items: items}, err
	case ModeCurrentAgents:
		session, err := mux.CurrentSession(ctx)
		if err != nil {
			return LoadResult{}, err
		}
		items, err := mux.ListAgents(ctx, session)
		if err == nil && runManifest {
			ApplyManifestFallback(ctx, items)
		}
		return LoadResult{Items: items}, err
	case ModeAll:
		fallthrough
	default:
		var out []Item
		var warnings []string
		sessions, err := mux.ListSessions(ctx)
		if err != nil {
			return LoadResult{}, err
		}
		zoxide, err := ListZoxideDirs(ctx)
		if err != nil {
			warnings = append(warnings, err.Error())
		}
		fd, err := ListFDirsWithCommand(ctx, opts.FDCommand)
		if err != nil {
			warnings = append(warnings, err.Error())
		}
		out = append(out, sessions...)
		out = append(out, zoxide...)
		out = append(out, fd...)
		agents, err := mux.ListAgents(ctx, "")
		if err != nil {
			warnings = append(warnings, err.Error())
		}
		if err == nil && runManifest {
			ApplyManifestFallback(ctx, agents)
		}
		out = append(out, agents...)
		return LoadResult{Items: out, Warning: strings.Join(warnings, "; ")}, nil
	}
}
