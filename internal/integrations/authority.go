package integrations

type AuthorityKind string

const (
	LifecycleAuthority AuthorityKind = "lifecycle"
	SessionOnly        AuthorityKind = "session-only"
)

func Authority(target Target) AuthorityKind {
	switch target {
	case TargetPi, TargetOpencode, TargetKimi,
		TargetClaude, TargetCodex, TargetCopilot,
		TargetDroid, TargetQodercli, TargetCursor:
		return LifecycleAuthority
	default:
		return SessionOnly
	}
}
