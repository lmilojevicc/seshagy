package sessionmgr

import "context"

type SourceMode int

type LoadOptions struct {
	FDCommand string
}

const (
	ModeAll SourceMode = iota
	ModeSessions
	ModeAgents
	ModeCurrentAgents
	ModeZoxide
	ModeFD
)

func Load(ctx context.Context, mode SourceMode) ([]Item, error) {
	return LoadWithOptions(ctx, mode, LoadOptions{})
}

func LoadWithOptions(ctx context.Context, mode SourceMode, opts LoadOptions) ([]Item, error) {
	switch mode {
	case ModeSessions:
		return ListSessions(ctx)
	case ModeAgents:
		return ListAgents(ctx, "")
	case ModeCurrentAgents:
		session, err := CurrentTmuxSession(ctx)
		if err != nil {
			return nil, err
		}
		return ListAgents(ctx, session)
	case ModeZoxide:
		return ListZoxideDirs(ctx)
	case ModeFD:
		return ListFDirsWithCommand(ctx, opts.FDCommand)
	case ModeAll:
		fallthrough
	default:
		var out []Item
		sessions, err := ListSessions(ctx)
		if err != nil {
			return nil, err
		}
		agents, err := ListAgents(ctx, "")
		if err != nil {
			return nil, err
		}
		zoxide, _ := ListZoxideDirs(ctx)
		fd, _ := ListFDirsWithCommand(ctx, opts.FDCommand)
		out = append(out, sessions...)
		out = append(out, agents...)
		out = append(out, zoxide...)
		out = append(out, fd...)
		return out, nil
	}
}
