# Default the Projects Pane Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** On startup, auto-select the Project matching the current
directory's git root, and auto-select that project's
most-recently-active session if any exists.

**Architecture:** Detect git root once at process start, stash it on
the `Model`. The first `projectsMsg` triggers `applyStartupDefaults`
(matches the root against `Project.Directory`, expands the project,
moves the cursor, fires a session-load). The resulting
`projectSessionsMsg` triggers `maybeAutoSelectActiveSession`. A
`defaultsApplied bool` guards against re-running on refresh ticks.

**Tech Stack:** Go, charmbracelet bubbletea, sqlite-backed store. All
work lives under `internal/tui/`, plus a tiny plumbing change in
`cmd/lazyagent/main.go`.

**Worktree:** `.worktrees/default-project-pane` on branch
`feature/default-project-pane`.

**Design doc:** `docs/plans/2026-06-16-default-project-pane-design.md`.

---

## Conventions

- TDD throughout: failing test first, run it red, minimal code to
  green, run it again, commit.
- After any file write, run
  `perl -i -lpe 's/  *$//' <file>` to strip trailing whitespace (per
  user's global rule).
- Run scoped tests during a task
  (`go test ./internal/tui/... -run TestXxx -v`); run the whole
  package once at the end of each task before commit.
- Commit subjects ≤50 chars; bodies wrap at 70.
- All `go test` invocations run from the worktree root
  (`/home/blyman/code/github/chojs23/lazyagent/.worktrees/default-project-pane`).

---

## Task 1: `findGitRoot` helper

**Files:**
- Create: `internal/tui/gitroot.go`
- Create: `internal/tui/gitroot_test.go`

**Step 1: Write the failing tests**

Add to `internal/tui/gitroot_test.go`:

```go
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
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/tui/ -run TestFindGitRoot -v
```

Expected: build failure — `findGitRoot` undefined.

**Step 3: Write minimal implementation**

`internal/tui/gitroot.go`:

```go
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
```

**Step 4: Verify tests pass**

```bash
perl -i -lpe 's/  *$//' internal/tui/gitroot.go internal/tui/gitroot_test.go
go test ./internal/tui/ -run TestFindGitRoot -v
```

Expected: 5 PASS, 0 FAIL.

**Step 5: Commit**

```bash
git add internal/tui/gitroot.go internal/tui/gitroot_test.go
git commit -m "Add findGitRoot helper

findGitRoot walks up from a starting path looking for a .git entry,
accepting either a directory or a file so git worktrees match too.
Returns the containing directory on hit or empty/false at the
filesystem root."
```

---

## Task 2: Plumb `startupGitRoot` and `defaultsApplied` onto `Model`

**Files:**
- Modify: `internal/tui/app.go` — `Model` struct, `newModel`, `Run`
- Modify: `cmd/lazyagent/main.go` — `tui.Run` call site

**Step 1: Write the failing test**

Add to `internal/tui/app_routing_test.go` (at the end of the file):

```go
func TestNewModelCapturesStartupGitRoot(t *testing.T) {
	m := newModelWithCWD(nil, time.Second, "/tmp/proj")
	if m.startupGitRoot != "/tmp/proj" {
		t.Fatalf("startupGitRoot = %q, want %q", m.startupGitRoot, "/tmp/proj")
	}
	if m.defaultsApplied {
		t.Fatal("defaultsApplied should start false")
	}
}

func TestNewModelEmptyCWDLeavesRootEmpty(t *testing.T) {
	m := newModelWithCWD(nil, time.Second, "")
	if m.startupGitRoot != "" {
		t.Fatalf("startupGitRoot = %q, want empty", m.startupGitRoot)
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/tui/ -run TestNewModelCapturesStartupGitRoot -v
```

Expected: build failure — `newModelWithCWD` undefined.

**Step 3: Write minimal implementation**

In `internal/tui/app.go`:

a. Add to `Model` struct (alongside other top-level fields, e.g. near
`allSessions`):

```go
	startupGitRoot  string
	defaultsApplied bool
```

b. Rename the existing `newModel` body into a private constructor
keyed off cwd. Keep `newModel` as a thin wrapper for existing callers
(tests pass `""` for cwd to opt out of auto-selection). Replace:

```go
func newModel(st *store.Store, refreshInterval time.Duration) Model {
	m := Model{
		// ...existing fields...
	}
	setGlobalDebug(m.debug)
	return m
}
```

with:

```go
func newModel(st *store.Store, refreshInterval time.Duration) Model {
	return newModelWithCWD(st, refreshInterval, "")
}

func newModelWithCWD(st *store.Store, refreshInterval time.Duration, cwd string) Model {
	root := ""
	if cwd != "" {
		if gr, ok := findGitRoot(cwd); ok {
			if resolved, err := filepath.EvalSymlinks(gr); err == nil {
				root = resolved
			} else {
				root = gr
			}
		}
	}
	m := Model{
		store:           st,
		refreshInterval: refreshInterval,
		keys:            defaultKeyMap(),
		help:            help.New(),
		projects:        newProjects(),
		session:         newSessionInfo(),
		agents:          newAgents(),
		events:          newEvents(),
		detail:          newDetail(),
		filter:          newFilter(),
		focus:           focusProjects,
		status:          "Loading...",
		debug:           &debugOverlay{},
		export:          newExportPopup(),
		startupGitRoot:  root,
	}
	setGlobalDebug(m.debug)
	return m
}
```

c. Add `"path/filepath"` to the import block if not already present
(it isn't in `app.go`).

d. Update `Run` to capture cwd:

```go
func Run(st *store.Store, refreshInterval time.Duration) error {
	cwd, _ := os.Getwd()
	p := tea.NewProgram(newModelWithCWD(st, refreshInterval, cwd))
	_, err := p.Run()
	return err
}
```

Add `"os"` to the imports.

**Step 4: Verify tests pass**

```bash
perl -i -lpe 's/  *$//' internal/tui/app.go
go test ./internal/tui/ -run TestNewModel -v
go test ./internal/tui/...
go build ./...
```

Expected: new tests PASS, all existing TUI tests still PASS, build OK.

**Step 5: Commit**

```bash
git add internal/tui/app.go internal/tui/app_routing_test.go
git commit -m "Capture startup git root on Model

newModelWithCWD resolves the cwd's git root once at construction and
stashes it on the Model alongside a defaultsApplied flag. Run wires
os.Getwd() through; existing newModel callers (tests) pass an empty
cwd and opt out of auto-selection."
```

---

## Task 3: `applyStartupDefaults` expands the matching project

**Files:**
- Modify: `internal/tui/app.go` — new helper + projectsMsg branch
- Modify: `internal/tui/pane_projects.go` — add a cursor-relocation
  helper if needed
- Modify: `internal/tui/app_routing_test.go` — new tests

**Step 1: Write the failing tests**

Append to `internal/tui/app_routing_test.go`:

```go
func TestApplyStartupDefaults_MatchExpandsProjectAndLoadsSessions(t *testing.T) {
	st := testRoutingTUIStore(t)
	ctx := t.Context()
	var projectID int64
	if err := st.WithTx(ctx, func(q *store.Queries) error {
		var err error
		projectID, err = q.CreateProject(ctx, "alpha", "Alpha", "/tmp/alpha", "")
		return err
	}); err != nil {
		t.Fatal(err)
	}

	m := newModelWithCWD(st, time.Second, "/tmp/alpha")
	updated, cmd := m.Update(m.loadProjectsCmd()())
	m = updated.(Model)

	if !m.defaultsApplied {
		t.Fatal("defaultsApplied should be true after first projectsMsg")
	}
	if !m.projects.expandedProjs[projectID] {
		t.Fatalf("project %d not expanded", projectID)
	}
	item := m.projects.currentItem()
	if item == nil || item.kind != "project" || item.projectID != projectID {
		t.Fatalf("cursor not on matching project: %+v", item)
	}
	if cmd == nil {
		t.Fatal("expected projectSessionsMsg load command")
	}
}

func TestApplyStartupDefaults_NoMatchLeavesPaneUnselected(t *testing.T) {
	st := testRoutingTUIStore(t)
	ctx := t.Context()
	if err := st.WithTx(ctx, func(q *store.Queries) error {
		_, err := q.CreateProject(ctx, "alpha", "Alpha", "/tmp/alpha", "")
		return err
	}); err != nil {
		t.Fatal(err)
	}

	m := newModelWithCWD(st, time.Second, "/tmp/other")
	updated, _ := m.Update(m.loadProjectsCmd()())
	m = updated.(Model)

	if !m.defaultsApplied {
		t.Fatal("defaultsApplied should latch even on no-match")
	}
	if m.projects.cursor != 0 || len(m.projects.expandedProjs) != 0 {
		t.Fatalf("unexpected pane state: cursor=%d expanded=%v",
			m.projects.cursor, m.projects.expandedProjs)
	}
	if m.projects.selectedSession != "" {
		t.Fatalf("selectedSession = %q, want empty", m.projects.selectedSession)
	}
}

func TestApplyStartupDefaults_EmptyGitRootSkips(t *testing.T) {
	st := testRoutingTUIStore(t)
	ctx := t.Context()
	if err := st.WithTx(ctx, func(q *store.Queries) error {
		_, err := q.CreateProject(ctx, "alpha", "Alpha", "/tmp/alpha", "")
		return err
	}); err != nil {
		t.Fatal(err)
	}

	m := newModelWithCWD(st, time.Second, "")
	updated, _ := m.Update(m.loadProjectsCmd()())
	m = updated.(Model)

	if m.defaultsApplied {
		t.Fatal("defaultsApplied should stay false when no startup git root")
	}
	if len(m.projects.expandedProjs) != 0 {
		t.Fatalf("expanded = %v, want empty", m.projects.expandedProjs)
	}
}

func TestApplyStartupDefaults_DoesNotReapplyOnSecondProjectsMsg(t *testing.T) {
	st := testRoutingTUIStore(t)
	ctx := t.Context()
	var projectID int64
	if err := st.WithTx(ctx, func(q *store.Queries) error {
		var err error
		projectID, err = q.CreateProject(ctx, "alpha", "Alpha", "/tmp/alpha", "")
		return err
	}); err != nil {
		t.Fatal(err)
	}

	m := newModelWithCWD(st, time.Second, "/tmp/alpha")
	updated, _ := m.Update(m.loadProjectsCmd()())
	m = updated.(Model)

	// User collapses the project manually.
	m.projects.expandedProjs[projectID] = false

	// Simulate a refresh tick reloading projects.
	updated, _ = m.Update(m.loadProjectsCmd()())
	m = updated.(Model)

	if m.projects.expandedProjs[projectID] {
		t.Fatal("refresh tick re-expanded the project; defaults should latch")
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/tui/ -run TestApplyStartupDefaults -v
```

Expected: tests FAIL (defaults never applied, cursor never moves, no
follow-up command).

**Step 3: Add cursor-locator helper on projectsModel**

Append to `internal/tui/pane_projects.go`:

```go
// indexOfProject returns the items index of a project's row, or -1.
func (p *projectsModel) indexOfProject(projectID int64) int {
	for i, item := range p.items {
		if item.kind == "project" && item.projectID == projectID {
			return i
		}
	}
	return -1
}

// indexOfSession returns the items index of a session's row, or -1.
func (p *projectsModel) indexOfSession(sessionID string) int {
	for i, item := range p.items {
		if item.kind == "session" && item.sessionID == sessionID {
			return i
		}
	}
	return -1
}
```

**Step 4: Add the helper and wire it into `projectsMsg`**

In `internal/tui/app.go`, append a new helper:

```go
// applyStartupDefaults runs once after the first projectsMsg. If the
// current working directory's git root matches a known project, the
// project is expanded and the cursor moves to its row. Returns a
// command to load that project's sessions, or nil.
func (m *Model) applyStartupDefaults() tea.Cmd {
	if m.defaultsApplied || m.startupGitRoot == "" {
		return nil
	}
	m.defaultsApplied = true

	projectID, ok := m.matchStartupProject()
	if !ok {
		return nil
	}
	m.projects.expandedProjs[projectID] = true
	m.projects.rebuildItems()
	if idx := m.projects.indexOfProject(projectID); idx >= 0 {
		m.projects.cursor = idx
	}
	return m.loadProjectSessionsCmd(projectID)
}

// matchStartupProject returns the ID of the project whose Directory
// matches startupGitRoot, normalizing symlinks where possible.
func (m *Model) matchStartupProject() (int64, bool) {
	want := m.startupGitRoot
	for _, proj := range m.allProjects {
		if proj.Directory == "" {
			continue
		}
		if proj.Directory == want {
			return proj.ID, true
		}
		if resolved, err := filepath.EvalSymlinks(proj.Directory); err == nil && resolved == want {
			return proj.ID, true
		}
	}
	return 0, false
}
```

Change the `projectsMsg` branch in `Update` from:

```go
	case projectsMsg:
		if msg.err != nil {
			m.reportError("Refresh failed", msg.err)
			return m, nil
		}
		m.lastError = nil
		m.applyProjects(msg.projects)
		return m, nil
```

to:

```go
	case projectsMsg:
		if msg.err != nil {
			m.reportError("Refresh failed", msg.err)
			return m, nil
		}
		m.lastError = nil
		m.applyProjects(msg.projects)
		return m, m.applyStartupDefaults()
```

`m` is a value receiver here, so the assignment must happen via the
existing `Model` value. Confirm the surrounding code: the `Update`
method already uses `m` as a value and returns `m`, so calling
`m.applyStartupDefaults()` on a value receiver mutates the local
copy. To match the rest of the file's style, the helper takes a
pointer receiver and is called as `(&m).applyStartupDefaults()` —
but since the helper is already declared on `*Model`, plain
`m.applyStartupDefaults()` works because Go takes the address of the
addressable local automatically. Verify by running the build.

**Step 5: Verify tests pass**

```bash
perl -i -lpe 's/  *$//' internal/tui/app.go internal/tui/pane_projects.go internal/tui/app_routing_test.go
go test ./internal/tui/ -run TestApplyStartupDefaults -v
go test ./internal/tui/...
```

Expected: 4 new tests PASS, all existing tests still PASS.

**Step 6: Commit**

```bash
git add internal/tui/app.go internal/tui/pane_projects.go internal/tui/app_routing_test.go
git commit -m "Auto-expand matching project on startup

After the first projectsMsg, applyStartupDefaults matches the
captured git root against Project.Directory (with EvalSymlinks),
expands the project, moves the cursor to its row, and fires a
session load. A defaultsApplied flag latches the behavior so refresh
ticks never re-apply it."
```

---

## Task 4: `maybeAutoSelectActiveSession` picks the active conversation

**Files:**
- Modify: `internal/tui/app.go` — new helper + projectSessionsMsg
  branch
- Modify: `internal/tui/app_routing_test.go` — new tests

**Step 1: Write the failing tests**

Append:

```go
func TestAutoSelectActiveSession_PicksMostRecentActive(t *testing.T) {
	st := testRoutingTUIStore(t)
	ctx := t.Context()
	var projectID int64
	if err := st.WithTx(ctx, func(q *store.Queries) error {
		var err error
		projectID, err = q.CreateProject(ctx, "alpha", "Alpha", "/tmp/alpha", "")
		if err != nil {
			return err
		}
		// Stopped session (most recent overall) — should be skipped.
		if err := q.UpsertSession(ctx, "stopped-recent", "", projectID, "stopped", "claude", nil, 3000, ""); err != nil {
			return err
		}
		if err := q.UpdateSessionStatus(ctx, "stopped-recent", "stopped"); err != nil {
			return err
		}
		// Older active.
		if err := q.UpsertSession(ctx, "active-old", "", projectID, "old", "claude", nil, 1000, ""); err != nil {
			return err
		}
		// Newer active — should win.
		return q.UpsertSession(ctx, "active-new", "", projectID, "new", "claude", nil, 2000, "")
	}); err != nil {
		t.Fatal(err)
	}

	m := newModelWithCWD(st, time.Second, "/tmp/alpha")
	updated, cmd := m.Update(m.loadProjectsCmd()())
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected sessions-load cmd")
	}
	updated, cmd = m.Update(cmd())
	m = updated.(Model)

	if m.projects.selectedSession != "active-new" {
		t.Fatalf("selectedSession = %q, want active-new", m.projects.selectedSession)
	}
	if cmd == nil {
		t.Fatal("expected session-data load cmd after auto-selecting session")
	}
	item := m.projects.currentItem()
	if item == nil || item.kind != "session" || item.sessionID != "active-new" {
		t.Fatalf("cursor not on selected session: %+v", item)
	}
}

func TestAutoSelectActiveSession_NoActiveLeavesSelectionEmpty(t *testing.T) {
	st := testRoutingTUIStore(t)
	ctx := t.Context()
	var projectID int64
	if err := st.WithTx(ctx, func(q *store.Queries) error {
		var err error
		projectID, err = q.CreateProject(ctx, "alpha", "Alpha", "/tmp/alpha", "")
		if err != nil {
			return err
		}
		if err := q.UpsertSession(ctx, "sess-1", "", projectID, "s", "claude", nil, 1000, ""); err != nil {
			return err
		}
		return q.UpdateSessionStatus(ctx, "sess-1", "stopped")
	}); err != nil {
		t.Fatal(err)
	}

	m := newModelWithCWD(st, time.Second, "/tmp/alpha")
	updated, cmd := m.Update(m.loadProjectsCmd()())
	m = updated.(Model)
	updated, _ = m.Update(cmd())
	m = updated.(Model)

	if m.projects.selectedSession != "" {
		t.Fatalf("selectedSession = %q, want empty", m.projects.selectedSession)
	}
	// Cursor should still be on the project row.
	item := m.projects.currentItem()
	if item == nil || item.kind != "project" || item.projectID != projectID {
		t.Fatalf("cursor not on project: %+v", item)
	}
}

func TestAutoSelectActiveSession_IgnoredForOtherProjects(t *testing.T) {
	st := testRoutingTUIStore(t)
	ctx := t.Context()
	var projectA int64
	if err := st.WithTx(ctx, func(q *store.Queries) error {
		var err error
		projectA, err = q.CreateProject(ctx, "alpha", "Alpha", "/tmp/alpha", "")
		if err != nil {
			return err
		}
		projectB, err := q.CreateProject(ctx, "beta", "Beta", "/tmp/beta", "")
		if err != nil {
			return err
		}
		return q.UpsertSession(ctx, "b-sess", "", projectB, "b", "claude", nil, 1000, "")
	}); err != nil {
		t.Fatal(err)
	}

	m := newModelWithCWD(st, time.Second, "/tmp/alpha")
	updated, cmd := m.Update(m.loadProjectsCmd()())
	m = updated.(Model)

	// Force a sessions msg for beta — it should be ignored for selection.
	updated, _ = m.Update(projectSessionsMsg{
		projectID: projectA + 999, // unrelated id
		sessions:  []model.Session{{ID: "b-sess", ProjectID: projectA + 999, Status: "active", LastActivity: 5000}},
	})
	m = updated.(Model)

	if m.projects.selectedSession != "" {
		t.Fatalf("selectedSession = %q for unrelated project, want empty",
			m.projects.selectedSession)
	}

	// Drain the alpha sessions cmd just so the test exercises both paths.
	if cmd != nil {
		updated, _ = m.Update(cmd())
		m = updated.(Model)
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/tui/ -run TestAutoSelectActiveSession -v
```

Expected: FAIL — no auto-selection happens.

**Step 3: Write the helper and wire it in**

In `internal/tui/app.go`, add helper:

```go
// maybeAutoSelectActiveSession picks the most-recently-active session
// for the startup-matched project, if any. Runs only for the matching
// projectID and only the first time defaults are applied.
func (m *Model) maybeAutoSelectActiveSession(projectID int64) tea.Cmd {
	if m.startupGitRoot == "" {
		return nil
	}
	wantID, ok := m.matchStartupProject()
	if !ok || wantID != projectID {
		return nil
	}
	if m.projects.selectedSession != "" {
		return nil
	}
	var best *model.Session
	for i := range m.allSessions {
		sess := &m.allSessions[i]
		if sess.ProjectID != projectID || sess.Status != "active" {
			continue
		}
		if best == nil || sess.LastActivity > best.LastActivity {
			best = sess
		}
	}
	if best == nil {
		return nil
	}
	m.projects.selectedSession = best.ID
	m.projects.rebuildItems()
	if idx := m.projects.indexOfSession(best.ID); idx >= 0 {
		m.projects.cursor = idx
	}
	m.syncSessionPane()
	return m.loadSelectedSessionDataCmd()
}
```

Change the `projectSessionsMsg` branch in `Update` from:

```go
	case projectSessionsMsg:
		if msg.err != nil {
			m.reportError("Load sessions failed", msg.err)
			return m, nil
		}
		m.lastError = nil
		m.applyProjectSessions(msg.projectID, msg.sessions)
		return m, nil
```

to:

```go
	case projectSessionsMsg:
		if msg.err != nil {
			m.reportError("Load sessions failed", msg.err)
			return m, nil
		}
		m.lastError = nil
		m.applyProjectSessions(msg.projectID, msg.sessions)
		return m, m.maybeAutoSelectActiveSession(msg.projectID)
```

**Step 4: Verify tests pass**

```bash
perl -i -lpe 's/  *$//' internal/tui/app.go internal/tui/app_routing_test.go
go test ./internal/tui/ -run TestAutoSelectActiveSession -v
go test ./internal/tui/...
```

Expected: 3 new tests PASS, all prior tests still PASS.

**Step 5: Commit**

```bash
git add internal/tui/app.go internal/tui/app_routing_test.go
git commit -m "Auto-select most recently active session on startup

After the startup-matched project's sessions load,
maybeAutoSelectActiveSession scans them for status='active' and
picks the one with the largest LastActivity. The cursor moves to
that session's row and a session-data load fires. Stopped-only
projects leave the cursor on the project row with no selection."
```

---

## Task 5: End-to-end test driving the full sequence

**Files:**
- Modify: `internal/tui/app_routing_test.go` — single integration test

**Step 1: Write the failing test**

Append:

```go
func TestStartupDefaults_EndToEnd(t *testing.T) {
	st := testRoutingTUIStore(t)
	ctx := t.Context()
	var projectID int64
	if err := st.WithTx(ctx, func(q *store.Queries) error {
		var err error
		projectID, err = q.CreateProject(ctx, "alpha", "Alpha", "/tmp/alpha", "")
		if err != nil {
			return err
		}
		if err := q.UpsertSession(ctx, "sess-active", "", projectID, "a", "claude", nil, 1000, ""); err != nil {
			return err
		}
		return q.UpsertAgent(ctx, "agent-1", "sess-active", "", "main", "", "main", "")
	}); err != nil {
		t.Fatal(err)
	}

	m := newModelWithCWD(st, time.Second, "/tmp/alpha")

	// 1) projectsMsg → expand + load sessions
	updated, cmd := m.Update(m.loadProjectsCmd()())
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("step 1 expected a session-load cmd")
	}

	// 2) projectSessionsMsg → pick active session + load session data
	updated, cmd = m.Update(cmd())
	m = updated.(Model)
	if m.projects.selectedSession != "sess-active" {
		t.Fatalf("selectedSession = %q, want sess-active",
			m.projects.selectedSession)
	}
	if cmd == nil {
		t.Fatal("step 2 expected a session-data load cmd")
	}

	// 3) sessionDataMsg → events/agents apply
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if len(m.agents.agents) == 0 {
		t.Fatal("agents not loaded after end-to-end auto-selection")
	}
}
```

**Step 2: Run it**

```bash
go test ./internal/tui/ -run TestStartupDefaults_EndToEnd -v
```

Expected: PASS (no implementation change needed — this is integration
coverage). If it fails, fix Task 3/4 before continuing.

**Step 3: Run the full TUI package**

```bash
go test ./internal/tui/...
```

Expected: all PASS.

**Step 4: Commit**

```bash
git add internal/tui/app_routing_test.go
git commit -m "Cover startup defaults end to end

The integration test drives a fresh Model through projectsMsg,
projectSessionsMsg, and sessionDataMsg in sequence and asserts the
cursor lands on the active session and agents load."
```

---

## Task 6: Smoke-test the whole binary and update the design doc

**Files:**
- Modify: `docs/plans/2026-06-16-default-project-pane-design.md` —
  mark the design as implemented (single-line note at the top)

**Step 1: Build the binary**

```bash
go build ./...
```

Expected: no errors.

**Step 2: Run the full test suite**

```bash
go test ./...
```

Expected: all PASS.

**Step 3: Manual smoke test (optional but recommended)**

```bash
go build -o /tmp/lazyagent-default-pane ./cmd/lazyagent
cd /home/blyman/code/github/chojs23/lazyagent  # a known project dir
/tmp/lazyagent-default-pane tui
```

Confirm:
- The Projects pane has the lazyagent project expanded with the
  cursor on it (or on an active session if one exists).
- Quit (`q`), then run the binary from a non-git directory
  (`cd /tmp && /tmp/lazyagent-default-pane tui`) — confirm the pane
  is unselected (current behavior).

Clean up: `rm /tmp/lazyagent-default-pane`.

**Step 4: Note implementation in design doc**

Prepend a single line under the title of
`docs/plans/2026-06-16-default-project-pane-design.md`:

```markdown
> **Status:** Implemented on branch `feature/default-project-pane`.
```

Strip trailing whitespace:

```bash
perl -i -lpe 's/  *$//' docs/plans/2026-06-16-default-project-pane-design.md
```

**Step 5: Commit**

```bash
git add docs/plans/2026-06-16-default-project-pane-design.md
git commit -m "Mark default-project-pane design as implemented"
```

---

## Verification checklist

Before declaring done, confirm with the user:

- [ ] `go test ./...` is clean.
- [ ] `go build ./...` is clean.
- [ ] Smoke test from inside the project shows the Projects pane
      pre-selected.
- [ ] Smoke test from outside any git repo shows today's
      behavior (no selection).
- [ ] The branch has six small commits in leaf-to-root order matching
      the tasks above.
