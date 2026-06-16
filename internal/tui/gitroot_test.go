package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindGitRoot_DirAtCwd(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, ok := findGitRoot(root)
	if !ok || got != root {
		t.Fatalf("findGitRoot(%q) = (%q, %v), want (%q, true)", root, got, ok, root)
	}
}

func TestFindGitRoot_WalkUp(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got, ok := findGitRoot(sub)
	if !ok || got != root {
		t.Fatalf("findGitRoot(%q) = (%q, %v), want (%q, true)", sub, got, ok, root)
	}
}

func TestFindGitRoot_WorktreeDotGitIsFile(t *testing.T) {
	root := t.TempDir()
	dotGit := filepath.Join(root, ".git")
	if err := os.WriteFile(dotGit, []byte("gitdir: /elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok := findGitRoot(root)
	if !ok || got != root {
		t.Fatalf("findGitRoot worktree = (%q, %v), want (%q, true)", got, ok, root)
	}
}

func TestFindGitRoot_NoMatch(t *testing.T) {
	root := t.TempDir()
	if _, ok := findGitRoot(root); ok {
		t.Fatalf("findGitRoot(%q) = (_, true), want (_, false)", root)
	}
}

func TestFindGitRoot_EmptyCwd(t *testing.T) {
	if _, ok := findGitRoot(""); ok {
		t.Fatal("findGitRoot(\"\") = (_, true), want (_, false)")
	}
}
