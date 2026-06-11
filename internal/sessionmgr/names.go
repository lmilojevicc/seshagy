package sessionmgr

import (
	"os"
	"path/filepath"
	"strings"
)

func SessionNameFromDir(dir string) string {
	cleaned := filepath.Clean(dir)
	name := filepath.Base(cleaned)
	if name == "." || name == string(os.PathSeparator) || name == "" {
		if wd, err := os.Getwd(); err == nil {
			name = filepath.Base(wd)
		}
	}
	if strings.HasPrefix(name, ".") {
		name = "dot_" + strings.TrimPrefix(name, ".")
	}
	return sanitizeSessionName(name)
}

func sanitizeSessionName(name string) string {
	if name == "" {
		return "session"
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range name {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' ||
			r == '_'
		if ok {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "session"
	}
	return out
}

func ExpandHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}

func ContractHome(path string) string {
	if home, err := os.UserHomeDir(); err == nil {
		if path == home {
			return "~"
		}
		if strings.HasPrefix(path, home+string(os.PathSeparator)) {
			return "~" + strings.TrimPrefix(path, home)
		}
	}
	return path
}
