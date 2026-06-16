package tui

import (
	"os"
	"path/filepath"
)

// findGitRoot walks up from start looking for a .git entry (file or
// directory; worktrees use a file). Returns the directory containing
// .git and true on hit, or ("", false) if the filesystem root is
// reached without one.
func findGitRoot(start string) (string, bool) {
	if start == "" {
		return "", false
	}
	dir := filepath.Clean(start)
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
