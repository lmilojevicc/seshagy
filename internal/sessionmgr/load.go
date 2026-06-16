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
	ModeAgents
	ModeCurrentAgents
	ModeZoxide
	ModeFD
)

func LoadWithOptions(ctx context.Context, mode SourceMode, opts LoadOptions) (LoadResult, error) {
	switch mode {
	case ModeSessions:
		items, err := ListSessions(ctx)
		return LoadResult{Items: items}, err
	case ModeAgents:
		items, err := ListAgents(ctx, "", opts)
		return LoadResult{Items: items}, err
	case ModeCurrentAgents:
		session, err := CurrentTmuxSession(ctx)
		if err != nil {
			return LoadResult{}, err
		}
		items, err := ListAgents(ctx, session, opts)
		return LoadResult{Items: items}, err
	case ModeZoxide:
		items, err := ListZoxideDirs(ctx)
		return LoadResult{Items: items}, err
	case ModeFD:
		items, err := ListFDirsWithCommand(ctx, opts.FDCommand)
		return LoadResult{Items: items}, err
	case ModeAll:
		fallthrough
	default:
		var out []Item
		var warnings []string
		sessions, err := ListSessions(ctx)
		if err != nil {
			return LoadResult{}, err
		}
		agents, err := ListAgents(ctx, "", opts)
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
		out = append(out, agents...)
		out = append(out, zoxide...)
		out = append(out, fd...)
		return LoadResult{Items: out, Warning: strings.Join(warnings, "; ")}, nil
	}
}
