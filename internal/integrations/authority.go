package integrations

type AuthorityKind string

const (
	LifecycleAuthority AuthorityKind = "lifecycle"
	SessionOnly        AuthorityKind = "session-only"
)

func Authority(target Target) AuthorityKind {
	switch target {
	case TargetPi, TargetOpencode, TargetKimi, TargetKilo, TargetHermes,
		TargetClaude, TargetCodex, TargetCopilot,
		TargetDroid, TargetQodercli, TargetCursor, TargetGrok:
		return LifecycleAuthority
	default:
		return SessionOnly
	}
}
