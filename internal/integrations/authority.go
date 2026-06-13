package integrations

type AuthorityKind string

const (
	LifecycleAuthority AuthorityKind = "lifecycle"
	SessionOnly        AuthorityKind = "session-only"
)

func Authority(target Target) AuthorityKind {
	switch target {
	case TargetPi, TargetOpencode, TargetKimi:
		return LifecycleAuthority
	default:
		return SessionOnly
	}
}
