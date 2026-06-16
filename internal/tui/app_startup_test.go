package tui

import (
	"testing"
	"time"
)

func TestNewModelCapturesStartupGitRoot(t *testing.T) {
	m := newModelWithGitRoot(nil, time.Second, "/tmp/proj")
	if m.startupGitRoot != "/tmp/proj" {
		t.Fatalf("startupGitRoot = %q, want %q", m.startupGitRoot, "/tmp/proj")
	}
	if m.defaultsApplied {
		t.Fatal("defaultsApplied should start false")
	}
}

func TestNewModelEmptyGitRootStaysEmpty(t *testing.T) {
	m := newModelWithGitRoot(nil, time.Second, "")
	if m.startupGitRoot != "" {
		t.Fatalf("startupGitRoot = %q, want empty", m.startupGitRoot)
	}
}
