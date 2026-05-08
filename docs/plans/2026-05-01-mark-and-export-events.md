# Mark and Export Events Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to
> implement this plan task-by-task.

**Goal:** Add a dired-inspired mark workflow (`m`/`M`/`U`) to the
Events pane and an `x`-triggered popup that exports the marked
events' raw payloads as a JSON array to a file on disk.

**Architecture:** Marks live on `eventsModel` as two maps keyed by
`Event.ID` (one for membership, one cached for export). A new
`exportPopup` lives on the parent `Model`; when active, it
intercepts key and mouse events at the top of `Update()` and
`handleMouseClick()`. The popup's text-input + confirm/cancel +
overwrite-checkbox UI is rendered as a centered overlay using the
existing `renderOverlayCentered` helper. Writes go through a small
pure `writeExport` function that's easy to unit-test.

**Tech Stack:** Go, Bubble Tea v2 (`charm.land/bubbletea/v2`),
Bubbles v2 (`charm.land/bubbles/v2/textinput`), Lipgloss v2,
`encoding/json`, `os`. No new dependencies.

**Design doc:** `docs/2026-05-01-mark-and-export-events-design.md`

**Conventions for every task:**
- Use Go's standard `testing` package (matching existing tests in
  `internal/tui/*_test.go`).
- All tests live alongside the code in the same package
  (`package tui` or `package tui_test` is fine if needed; existing
  tests use `package tui`).
- Run tests with `go test ./internal/tui/...` from the repo root.
- After every code edit, run
  `perl -i -lpe 's/  *$//' <file>` to strip trailing whitespace
  (per the user's global instruction).
- Each task ends with one `git commit`. Subject ≤50 chars, body
  ≤70 chars/line, active present voice. Include the
  `Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>` trailer.
- We're already on branch `feat/export-session`. Do not create a
  worktree.

---

## Task 1: Add mark state and `toggleMark`

Add the two mark maps to `eventsModel` plus a `toggleMark` method
that marks/unmarks the event under the cursor and advances the
cursor by one.

**Files:**
- Modify: `internal/tui/pane_events.go:15-28` (struct + constructor)
- Modify: `internal/tui/pane_events.go` (add new method block near
  `moveDown`)
- Test: `internal/tui/pane_events_test.go` (append new tests)

**Step 1: Write the failing tests**

Append to `internal/tui/pane_events_test.go`:

```go
func TestToggleMark_MarksAndAdvances(t *testing.T) {
	e := newEvents()
	e.setEvents(makeEvents(5), 5, 0)
	e.autoFollow = false
	e.cursor = 0

	e.toggleMark()

	if !e.isMarked(0) {
		t.Fatalf("event 0 should be marked")
	}
	if e.cursor != 1 {
		t.Fatalf("cursor: got %d, want 1", e.cursor)
	}
	if e.markedCount() != 1 {
		t.Fatalf("markedCount: got %d, want 1", e.markedCount())
	}
}

func TestToggleMark_TogglesOff(t *testing.T) {
	e := newEvents()
	e.setEvents(makeEvents(5), 5, 0)
	e.autoFollow = false
	e.cursor = 2

	e.toggleMark()
	// cursor is now at 3; go back and unmark
	e.cursor = 2
	e.toggleMark()

	if e.isMarked(2) {
		t.Fatalf("event 2 should be unmarked after second toggle")
	}
	if e.markedCount() != 0 {
		t.Fatalf("markedCount: got %d, want 0", e.markedCount())
	}
}

func TestToggleMark_AtEnd(t *testing.T) {
	e := newEvents()
	e.setEvents(makeEvents(3), 3, 0)
	e.autoFollow = false
	e.cursor = 2

	e.toggleMark()

	if !e.isMarked(2) {
		t.Fatal("last event should be marked")
	}
	if e.cursor != 2 {
		t.Fatalf("cursor should clamp at end: got %d, want 2", e.cursor)
	}
}

func TestToggleMark_EmptyEvents(t *testing.T) {
	e := newEvents()

	e.toggleMark()

	if e.markedCount() != 0 {
		t.Fatalf("markedCount: got %d, want 0", e.markedCount())
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/ -run TestToggleMark -v`
Expected: FAIL with "e.toggleMark undefined" (or similar
unresolved-symbol errors).

**Step 3: Implement minimal code**

Edit `internal/tui/pane_events.go`. Update the struct and
constructor:

```go
type eventsModel struct {
	events       []model.Event
	rawCount     int
	loadedOffset int
	cursor       int
	scroll       int
	hScroll      int
	autoFollow   bool
	height       int

	marked       map[int64]struct{}
	markedEvents map[int64]model.Event
}

func newEvents() eventsModel {
	return eventsModel{
		autoFollow:   true,
		marked:       map[int64]struct{}{},
		markedEvents: map[int64]model.Event{},
	}
}
```

Add new methods (place them after `moveDown` for adjacency):

```go
func (e *eventsModel) toggleMark() {
	if e.cursor < 0 || e.cursor >= len(e.events) {
		return
	}
	ev := e.events[e.cursor]
	if _, ok := e.marked[ev.ID]; ok {
		delete(e.marked, ev.ID)
		delete(e.markedEvents, ev.ID)
	} else {
		e.marked[ev.ID] = struct{}{}
		e.markedEvents[ev.ID] = ev
	}
	e.moveDown()
}

func (e *eventsModel) isMarked(id int64) bool {
	_, ok := e.marked[id]
	return ok
}

func (e *eventsModel) markedCount() int {
	return len(e.marked)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/ -run TestToggleMark -v`
Expected: PASS for all four tests.

Run the full pane_events test file to make sure nothing regressed:
`go test ./internal/tui/ -v -run "TestToggleMark|TestMove|TestSetEvents|TestSelectedEvent"`
Expected: all PASS.

**Step 5: Strip trailing whitespace and commit**

```bash
perl -i -lpe 's/  *$//' internal/tui/pane_events.go internal/tui/pane_events_test.go
git add internal/tui/pane_events.go internal/tui/pane_events_test.go
git commit -m "$(cat <<'EOF'
Add toggleMark to events model

Two maps on eventsModel track which events are marked and cache
the events themselves so export does not need a DB round-trip.
toggleMark flips the mark on the cursor's event and advances the
cursor by one, matching dired's m behavior.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Add `markAllVisible` and `unmarkAll`

**Files:**
- Modify: `internal/tui/pane_events.go`
- Test: `internal/tui/pane_events_test.go`

**Step 1: Write the failing tests**

```go
func TestMarkAllVisible_MarksEverythingLoaded(t *testing.T) {
	e := newEvents()
	e.setEvents(makeEvents(7), 7, 0)

	e.markAllVisible()

	if e.markedCount() != 7 {
		t.Fatalf("markedCount: got %d, want 7", e.markedCount())
	}
	for i := int64(0); i < 7; i++ {
		if !e.isMarked(i) {
			t.Fatalf("event %d should be marked", i)
		}
	}
}

func TestUnmarkAll_ClearsBothMaps(t *testing.T) {
	e := newEvents()
	e.setEvents(makeEvents(5), 5, 0)
	e.markAllVisible()

	e.unmarkAll()

	if e.markedCount() != 0 {
		t.Fatalf("markedCount: got %d, want 0", e.markedCount())
	}
	if len(e.markedEvents) != 0 {
		t.Fatalf("markedEvents map not cleared: len=%d",
			len(e.markedEvents))
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/ -run "TestMarkAllVisible|TestUnmarkAll" -v`
Expected: FAIL with undefined-method errors.

**Step 3: Implement**

Add to `internal/tui/pane_events.go`:

```go
func (e *eventsModel) markAllVisible() {
	for _, ev := range e.events {
		e.marked[ev.ID] = struct{}{}
		e.markedEvents[ev.ID] = ev
	}
}

func (e *eventsModel) unmarkAll() {
	e.marked = map[int64]struct{}{}
	e.markedEvents = map[int64]model.Event{}
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/ -run "TestMarkAllVisible|TestUnmarkAll" -v`
Expected: PASS.

**Step 5: Strip whitespace and commit**

```bash
perl -i -lpe 's/  *$//' internal/tui/pane_events.go internal/tui/pane_events_test.go
git add internal/tui/pane_events.go internal/tui/pane_events_test.go
git commit -m "$(cat <<'EOF'
Add markAllVisible and unmarkAll

markAllVisible marks every event in the loaded slice. unmarkAll
clears both mark maps. These back the M and U keybindings.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Marks survive filter changes

Verify (and lock in via test) that calling `setEvents` with a
different slice does not lose marks for events that are still in
the new slice, and that `markedEvents` remains populated for events
that were filtered out.

**Files:**
- Test: `internal/tui/pane_events_test.go`

**Step 1: Write the failing test**

```go
func TestMarksSurvive_SetEventsDoesNotClearMarks(t *testing.T) {
	e := newEvents()
	e.setEvents(makeEvents(10), 10, 0)
	e.cursor = 3
	e.toggleMark() // marks event 3

	// Simulate a filter change that removes event 3 from view.
	filtered := []model.Event{
		{ID: 1, Subtype: "PreToolUse"},
		{ID: 5, Subtype: "PreToolUse"},
		{ID: 7, Subtype: "PreToolUse"},
	}
	e.setEvents(filtered, 3, 0)

	if !e.isMarked(3) {
		t.Fatalf("mark should survive filter change")
	}
	if _, ok := e.markedEvents[3]; !ok {
		t.Fatalf("markedEvents cache should still hold event 3")
	}
	if e.markedCount() != 1 {
		t.Fatalf("markedCount: got %d, want 1", e.markedCount())
	}
}
```

**Step 2: Run test**

Run: `go test ./internal/tui/ -run TestMarksSurvive -v`
Expected: PASS (Task 1's implementation already supports this — we
never touched marks in `setEvents`). This test exists to guard
against regressions.

If it fails, that's a real regression — fix `setEvents` to leave
the maps alone before continuing.

**Step 3: Strip whitespace and commit**

```bash
perl -i -lpe 's/  *$//' internal/tui/pane_events_test.go
git add internal/tui/pane_events_test.go
git commit -m "$(cat <<'EOF'
Lock in mark survival across filter changes

Add a regression test asserting that setEvents with a filtered
slice keeps both mark maps intact for events that are no longer
visible.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: `markedSnapshot` returns events in slice order

The export needs marked events ordered chronologically (their
position in the loaded events slice), not insertion order into the
map.

**Files:**
- Modify: `internal/tui/pane_events.go`
- Test: `internal/tui/pane_events_test.go`

**Step 1: Write the failing test**

```go
func TestMarkedSnapshot_OrderedBySlicePosition(t *testing.T) {
	e := newEvents()
	e.setEvents(makeEvents(5), 5, 0)

	// Mark in scrambled order: 4, 1, 2.
	e.cursor = 4
	e.toggleMark()
	e.cursor = 1
	e.toggleMark()
	e.cursor = 2
	e.toggleMark()

	got := e.markedSnapshot()
	if len(got) != 3 {
		t.Fatalf("len: got %d, want 3", len(got))
	}
	wantIDs := []int64{1, 2, 4}
	for i, w := range wantIDs {
		if got[i].ID != w {
			t.Fatalf("got[%d].ID = %d, want %d", i, got[i].ID, w)
		}
	}
}

func TestMarkedSnapshot_IncludesEventsNotInSlice(t *testing.T) {
	e := newEvents()
	e.setEvents(makeEvents(5), 5, 0)

	e.cursor = 2
	e.toggleMark() // marks event 2

	// Filter out event 2.
	filtered := []model.Event{
		{ID: 0, Subtype: "PreToolUse"},
		{ID: 4, Subtype: "PreToolUse"},
	}
	e.setEvents(filtered, 2, 0)

	got := e.markedSnapshot()
	// Event 2 is not in the visible slice but is still marked.
	// It should appear in the snapshot. Where it appears relative
	// to the others is implementation-defined; we just assert it's
	// present and the count matches.
	if len(got) != 1 {
		t.Fatalf("len: got %d, want 1", len(got))
	}
	if got[0].ID != 2 {
		t.Fatalf("got[0].ID = %d, want 2", got[0].ID)
	}
}
```

**Step 2: Run test**

Run: `go test ./internal/tui/ -run TestMarkedSnapshot -v`
Expected: FAIL — `markedSnapshot` undefined.

**Step 3: Implement**

Add to `internal/tui/pane_events.go`:

```go
// markedSnapshot returns marked events ordered by their position
// in the currently loaded events slice. Marked events that are not
// in the slice are appended at the end in ascending ID order.
func (e *eventsModel) markedSnapshot() []model.Event {
	if len(e.marked) == 0 {
		return nil
	}
	out := make([]model.Event, 0, len(e.marked))
	seen := map[int64]bool{}
	for _, ev := range e.events {
		if _, ok := e.marked[ev.ID]; ok {
			out = append(out, ev)
			seen[ev.ID] = true
		}
	}
	// Append any marked events not in the visible slice.
	var orphans []model.Event
	for id, ev := range e.markedEvents {
		if !seen[id] {
			orphans = append(orphans, ev)
		}
	}
	sort.Slice(orphans, func(i, j int) bool {
		return orphans[i].ID < orphans[j].ID
	})
	return append(out, orphans...)
}
```

Add `"sort"` to the import block at the top of
`internal/tui/pane_events.go`.

**Step 4: Run tests**

Run: `go test ./internal/tui/ -run TestMarkedSnapshot -v`
Expected: PASS for both subtests.

**Step 5: Strip whitespace and commit**

```bash
perl -i -lpe 's/  *$//' internal/tui/pane_events.go internal/tui/pane_events_test.go
git add internal/tui/pane_events.go internal/tui/pane_events_test.go
git commit -m "$(cat <<'EOF'
Add markedSnapshot ordered by slice position

Returns marked events in the order they appear in the loaded
events slice, with marks for filtered-out events appended in ID
order at the end.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Render `>` prefix on marked event lines

Replace the hard-coded `"  "` prefix in the two render functions
with a helper that returns `"> "` when the event is marked.

**Files:**
- Modify: `internal/tui/pane_events.go:218-251`
- Test: `internal/tui/pane_events_test.go`

**Step 1: Write the failing test**

```go
func TestRenderEventLine_MarkedShowsArrow(t *testing.T) {
	e := newEvents()
	e.setEvents(makeEvents(3), 3, 0)
	e.cursor = 1
	e.toggleMark() // marks event 1

	ev := e.events[1]
	line := stripANSI(e.renderEventLine(ev, 1, false, false, nil, 1))

	if !strings.HasPrefix(line, "> ") {
		t.Fatalf("marked line should start with %q, got %q", "> ", line)
	}
}

func TestRenderEventLine_UnmarkedShowsSpaces(t *testing.T) {
	e := newEvents()
	e.setEvents(makeEvents(3), 3, 0)

	ev := e.events[0]
	line := stripANSI(e.renderEventLine(ev, 0, false, false, nil, 1))

	if !strings.HasPrefix(line, "  ") {
		t.Fatalf("unmarked line should start with two spaces, got %q",
			line)
	}
	if strings.HasPrefix(line, "> ") {
		t.Fatalf("unmarked line must not start with arrow")
	}
}
```

Note: `stripANSI` already exists in the test file (used by other
tests).

**Step 2: Run tests**

Run: `go test ./internal/tui/ -run "TestRenderEventLine_Marked|TestRenderEventLine_Unmarked" -v`
Expected: FAIL because the current implementation always uses
`"  "`.

**Step 3: Implement**

In `internal/tui/pane_events.go`, modify
`renderSelectedEventLine` and `renderPlainEventLine`. Add a small
helper above them:

```go
func markPrefix(marked bool) string {
	if marked {
		return "> "
	}
	return "  "
}
```

Then change the prefix lines:

In `renderSelectedEventLine` (currently
`return style.Render("  " + strings.Join(parts, "  "))`):

```go
return style.Render(markPrefix(marked) + strings.Join(parts, "  "))
```

In `renderPlainEventLine` (currently
`return "  " + strings.Join(parts, "  ")`):

```go
return markPrefix(marked) + strings.Join(parts, "  ")
```

This requires threading `marked` through the render call chain.
Update `renderEventLine`'s signature:

```go
func (e *eventsModel) renderEventLine(ev model.Event, index int, atCursor bool, focused bool, agentMap map[string]agentInfo, totalDigits int) string {
	numStr := fmt.Sprintf("%*d", totalDigits, index+1)
	subtype := truncate(orDefault(ev.Subtype, ev.Type), 20)
	agentLabel, agentInfo := eventAgentLabel(ev, agentMap)
	brief := eventview.Brief(ev)
	marked := e.isMarked(ev.ID)
	if atCursor {
		return renderSelectedEventLine(ev, focused, numStr,
			agentLabel, subtype, brief, marked)
	}
	return renderPlainEventLine(ev, numStr, subtype, agentLabel,
		agentInfo, brief, marked)
}
```

Update the two render-line function signatures to take a final
`marked bool` parameter and use `markPrefix(marked)`.

**Step 4: Run tests**

Run: `go test ./internal/tui/ -run "TestRenderEventLine" -v`
Expected: PASS for the new tests AND the existing
`TestRenderEventLine_AbsoluteNumbering`,
`TestRenderEventLineIncludesBriefText` (still pass — they don't
inspect the prefix).

Run the full TUI test suite:
`go test ./internal/tui/...`
Expected: all PASS. If any test breaks because it called
`renderSelectedEventLine`/`renderPlainEventLine` directly with the
old signature, fix the callers — they're test-only.

**Step 5: Strip whitespace and commit**

```bash
perl -i -lpe 's/  *$//' internal/tui/pane_events.go internal/tui/pane_events_test.go
git add internal/tui/pane_events.go internal/tui/pane_events_test.go
git commit -m "$(cat <<'EOF'
Render arrow prefix on marked event lines

A marked event line starts with > followed by one space in place
of the normal two-space indent. Column alignment is preserved.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Add `m`/`M`/`U` keybindings and route them

Wire the new keybindings through `keys.go` and `updateEvents`.
Marking is only active when the events pane is focused.

**Files:**
- Modify: `internal/tui/keys.go`
- Modify: `internal/tui/app.go:450-484` (`updateEvents`)
- Test: `internal/tui/app_routing_test.go` (or new file if patterns
  diverge)

**Step 1: Write the failing test**

Read `internal/tui/app_routing_test.go` first to match the existing
testing style. If the file doesn't already construct a `Model` with
events loaded, add a small helper:

```go
func TestMarkKeybindings_RouteWhenEventsFocused(t *testing.T) {
	m := newModel(nil, time.Second)
	m.focus = focusEvents
	m.events.setEvents([]model.Event{
		{ID: 1, Subtype: "PreToolUse"},
		{ID: 2, Subtype: "PreToolUse"},
		{ID: 3, Subtype: "PreToolUse"},
	}, 3, 0)
	m.events.autoFollow = false
	m.events.cursor = 0

	// Press m
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'm'})
	m = updated.(Model)
	if !m.events.isMarked(1) {
		t.Fatalf("m should mark event with ID 1")
	}

	// Press M (mark all)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'M'})
	m = updated.(Model)
	if m.events.markedCount() != 3 {
		t.Fatalf("M should mark all 3 events, got %d",
			m.events.markedCount())
	}

	// Press U (unmark all)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'U'})
	m = updated.(Model)
	if m.events.markedCount() != 0 {
		t.Fatalf("U should unmark all, got %d",
			m.events.markedCount())
	}
}
```

Important: check `app_routing_test.go` for the exact `tea.KeyMsg`
construction pattern that the codebase uses; the `tea.KeyPressMsg`
type may differ. Use whatever form existing tests use. If existing
tests use a helper like `keyMsg("m")`, use that.

**Step 2: Run test**

Run: `go test ./internal/tui/ -run TestMarkKeybindings -v`
Expected: FAIL — `m`, `M`, `U` are not handled.

**Step 3: Implement**

Edit `internal/tui/keys.go`. Add four binding fields:

```go
MarkToggle   key.Binding
MarkAll      key.Binding
UnmarkAll    key.Binding
ExportMarked key.Binding
```

In `defaultKeyMap()`:

```go
MarkToggle:   key.NewBinding(key.WithKeys("m"),
	key.WithHelp("m", "mark toggle")),
MarkAll:      key.NewBinding(key.WithKeys("M"),
	key.WithHelp("M", "mark all")),
UnmarkAll:    key.NewBinding(key.WithKeys("U"),
	key.WithHelp("U", "unmark all")),
ExportMarked: key.NewBinding(key.WithKeys("x"),
	key.WithHelp("x", "export marked")),
```

In `internal/tui/app.go`, modify `updateEvents` (line ~450). Add
new `case k` branches before the final `default` route:

```go
case "m":
	m.events.toggleMark()
	m.lastKey = k
	return m, m.syncEventSelectionAndMaybeLoadOlder()
case "M":
	m.events.markAllVisible()
	m.lastKey = k
	return m, nil
case "U":
	m.events.unmarkAll()
	m.lastKey = k
	return m, nil
```

`x` is handled in Task 8 — leave it alone for now.

Also extend `FullHelp()` in `keys.go` so the `?` overlay lists the
new bindings:

```go
return [][]key.Binding{
	{k.NextPane, k.PrevPane, k.PaneProjects, k.PaneSession,
		k.PaneAgents, k.PaneEvents, k.PaneDetail},
	{k.Search, k.CycleType, k.ToggleAuto, k.AgentAll,
		k.TokenUsage, k.Refresh, k.Quit},
	{k.MarkToggle, k.MarkAll, k.UnmarkAll, k.ExportMarked},
}
```

**Step 4: Run test**

Run: `go test ./internal/tui/ -run TestMarkKeybindings -v`
Expected: PASS.

Run full suite: `go test ./internal/tui/...`
Expected: PASS.

**Step 5: Strip whitespace and commit**

```bash
perl -i -lpe 's/  *$//' internal/tui/keys.go internal/tui/app.go internal/tui/app_routing_test.go
git add internal/tui/keys.go internal/tui/app.go internal/tui/app_routing_test.go
git commit -m "$(cat <<'EOF'
Wire m, M, U keybindings on the events pane

m toggles the mark on the cursor and advances. M marks every
loaded event. U clears all marks. x is reserved for the export
popup added in a later task.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Pure `writeExport` function and tests

Build the file-writing logic in isolation: a pure function that
takes a slice of events and a path and writes the JSON array. No
TUI involvement.

**Files:**
- Create: `internal/tui/popup_export.go`
- Create: `internal/tui/popup_export_test.go`

**Step 1: Write the failing tests**

Create `internal/tui/popup_export_test.go`:

```go
package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/chojs23/lazyagent/internal/model"
)

func TestWriteExport_SingleEvent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")
	events := []model.Event{
		{ID: 1, Payload: `{"foo":"bar"}`},
	}

	if err := writeExport(path, events); err != nil {
		t.Fatalf("writeExport: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var got []map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, data)
	}
	if len(got) != 1 {
		t.Fatalf("len: got %d, want 1", len(got))
	}
	if got[0]["foo"] != "bar" {
		t.Fatalf("got[0].foo: got %v, want bar", got[0]["foo"])
	}
}

func TestWriteExport_MultipleEventsPreserveOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")
	events := []model.Event{
		{ID: 1, Payload: `{"n":1}`},
		{ID: 2, Payload: `{"n":2}`},
		{ID: 3, Payload: `{"n":3}`},
	}

	if err := writeExport(path, events); err != nil {
		t.Fatalf("writeExport: %v", err)
	}

	data, _ := os.ReadFile(path)
	var got []map[string]any
	json.Unmarshal(data, &got)

	for i, want := range []float64{1, 2, 3} {
		if got[i]["n"] != want {
			t.Fatalf("got[%d].n: got %v, want %v",
				i, got[i]["n"], want)
		}
	}
}

func TestWriteExport_EmptyPayloadBecomesEmptyObject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")
	events := []model.Event{{ID: 1, Payload: ""}}

	if err := writeExport(path, events); err != nil {
		t.Fatalf("writeExport: %v", err)
	}

	data, _ := os.ReadFile(path)
	var got []any
	json.Unmarshal(data, &got)
	if len(got) != 1 {
		t.Fatalf("len: got %d, want 1", len(got))
	}
	obj, ok := got[0].(map[string]any)
	if !ok {
		t.Fatalf("got[0] should be object, got %T", got[0])
	}
	if len(obj) != 0 {
		t.Fatalf("got[0] should be empty object, got %v", obj)
	}
}

func TestWriteExport_UnparseableBecomesNull(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")
	events := []model.Event{{ID: 1, Payload: "not json {{"}}

	if err := writeExport(path, events); err != nil {
		t.Fatalf("writeExport: %v", err)
	}

	data, _ := os.ReadFile(path)
	var got []any
	json.Unmarshal(data, &got)
	if got[0] != nil {
		t.Fatalf("got[0] should be null, got %v", got[0])
	}
}

func TestWriteExport_AlwaysArrayEvenForSingleEvent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")
	events := []model.Event{{ID: 1, Payload: `{"k":"v"}`}}

	if err := writeExport(path, events); err != nil {
		t.Fatalf("writeExport: %v", err)
	}

	data, _ := os.ReadFile(path)
	if data[0] != '[' {
		t.Fatalf("output must start with [, got %q", data[:1])
	}
}

func TestWriteExport_FailsOnDirectoryPath(t *testing.T) {
	dir := t.TempDir()
	events := []model.Event{{ID: 1, Payload: "{}"}}

	err := writeExport(dir, events)
	if err == nil {
		t.Fatalf("writeExport should fail when path is a directory")
	}
}
```

**Step 2: Run tests**

Run: `go test ./internal/tui/ -run TestWriteExport -v`
Expected: FAIL — `writeExport` is undefined.

**Step 3: Implement**

Create `internal/tui/popup_export.go`:

```go
package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chojs23/lazyagent/internal/model"
)

// writeExport writes the marked events as a JSON array to path.
// Each element is the parsed Event.Payload. Empty payloads become
// {}. Unparseable payloads become null. The write is atomic: data
// is written to a temp file in the same directory, then renamed
// into place.
func writeExport(path string, events []model.Event) error {
	arr := make([]any, len(events))
	for i, ev := range events {
		arr[i] = parsePayload(ev.Payload)
	}

	data, err := json.MarshalIndent(arr, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".lazyagent-export-*.json.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

func parsePayload(payload string) any {
	if strings.TrimSpace(payload) == "" {
		return map[string]any{}
	}
	var v any
	if err := json.Unmarshal([]byte(payload), &v); err != nil {
		return nil
	}
	return v
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}
```

**Step 4: Run tests**

Run: `go test ./internal/tui/ -run TestWriteExport -v`
Expected: PASS for all.

Add a quick test for `expandHome`:

```go
func TestExpandHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	cases := []struct{ in, want string }{
		{"~", home},
		{"~/foo", filepath.Join(home, "foo")},
		{"/abs/path", "/abs/path"},
		{"rel/path", "rel/path"},
	}
	for _, c := range cases {
		got := expandHome(c.in)
		if got != c.want {
			t.Fatalf("expandHome(%q) = %q, want %q",
				c.in, got, c.want)
		}
	}
}
```

Run: `go test ./internal/tui/ -run TestExpandHome -v`
Expected: PASS.

**Step 5: Strip whitespace and commit**

```bash
perl -i -lpe 's/  *$//' internal/tui/popup_export.go internal/tui/popup_export_test.go
git add internal/tui/popup_export.go internal/tui/popup_export_test.go
git commit -m "$(cat <<'EOF'
Add writeExport for atomic JSON-array file writes

writeExport marshals the given events into a JSON array using
each event's parsed Payload, falling back to {} for empty payloads
and null for unparseable ones. The write goes through a temp file
then os.Rename so a crash never leaves a half-written file.
expandHome handles a leading ~ for filenames.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: `x` with no marks shows status message

Wire `x` in `updateEvents`. If `markedCount() == 0`, set the status
line and return; do not open a popup yet (popup arrives in
Task 9).

**Files:**
- Modify: `internal/tui/app.go:450-484`
- Test: `internal/tui/app_routing_test.go`

**Step 1: Write the failing test**

```go
func TestExportWithNoMarks_ShowsStatusOnly(t *testing.T) {
	m := newModel(nil, time.Second)
	m.focus = focusEvents
	m.events.setEvents([]model.Event{
		{ID: 1, Subtype: "PreToolUse"},
	}, 1, 0)
	m.events.autoFollow = false

	// Sanity: nothing marked.
	if m.events.markedCount() != 0 {
		t.Fatalf("precondition: no marks")
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'x'})
	m = updated.(Model)

	if m.status == "" {
		t.Fatalf("status should be set")
	}
	if !strings.Contains(m.status, "no events marked") {
		t.Fatalf("status: got %q, want contains %q",
			m.status, "no events marked")
	}
}
```

**Step 2: Run test**

Run: `go test ./internal/tui/ -run TestExportWithNoMarks -v`
Expected: FAIL — `x` is not handled.

**Step 3: Implement**

In `internal/tui/app.go`, in `updateEvents`, add an `x` case
alongside `m`/`M`/`U`:

```go
case "x":
	if m.events.markedCount() == 0 {
		m.status = "no events marked — press m to mark events"
		m.lastKey = k
		return m, nil
	}
	// Popup wiring arrives in Task 9.
	m.lastKey = k
	return m, nil
```

**Step 4: Run test**

Run: `go test ./internal/tui/ -run TestExportWithNoMarks -v`
Expected: PASS.

**Step 5: Strip whitespace and commit**

```bash
perl -i -lpe 's/  *$//' internal/tui/app.go internal/tui/app_routing_test.go
git add internal/tui/app.go internal/tui/app_routing_test.go
git commit -m "$(cat <<'EOF'
Show status hint on x with no marks

Pressing x in the events pane with zero marks sets a status-line
message instead of opening the popup. The popup itself arrives in
the next task.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: `exportPopup` state and headless lifecycle

Build the popup's state machine without rendering: open, type a
filename, confirm (call `writeExport`), close. Tests drive it
purely through method calls.

**Files:**
- Modify: `internal/tui/popup_export.go`
- Modify: `internal/tui/popup_export_test.go`

**Step 1: Write the failing tests**

Append to `popup_export_test.go`:

```go
func TestExportPopup_OpenSetsActiveAndDefaultFilename(t *testing.T) {
	p := newExportPopup()
	p.open()
	if !p.active {
		t.Fatal("popup should be active")
	}
	if p.filename() == "" {
		t.Fatal("filename should be pre-filled")
	}
	if !strings.HasSuffix(p.filename(), ".json") {
		t.Fatalf("default filename should end in .json, got %q",
			p.filename())
	}
}

func TestExportPopup_CancelClears(t *testing.T) {
	p := newExportPopup()
	p.open()
	p.cancel()
	if p.active {
		t.Fatal("popup should be inactive after cancel")
	}
}

func TestExportPopup_ConfirmWritesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")

	p := newExportPopup()
	p.open()
	p.setFilename(path)
	events := []model.Event{{ID: 1, Payload: `{"k":"v"}`}}

	if err := p.confirm(events); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if p.active {
		t.Fatal("popup should close on successful confirm")
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}
}

func TestExportPopup_ConfirmDisabledWhenFilenameEmpty(t *testing.T) {
	p := newExportPopup()
	p.open()
	p.setFilename("   ")
	if p.canConfirm() {
		t.Fatal("confirm should be disabled for whitespace filename")
	}
}

func TestExportPopup_OverwriteRequiredForExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exists.json")
	if err := os.WriteFile(path, []byte("[]"), 0644); err != nil {
		t.Fatal(err)
	}

	p := newExportPopup()
	p.open()
	p.setFilename(path)

	if !p.fileExists {
		t.Fatal("fileExists should be true")
	}
	if p.canConfirm() {
		t.Fatal("confirm should be disabled until overwriteOK is set")
	}

	p.overwriteOK = true
	if !p.canConfirm() {
		t.Fatal("confirm should be enabled once overwrite is OK")
	}
}
```

Note: `setFilename` is a test helper that pokes the textinput's
value directly so we can drive the popup without simulating
keystrokes.

**Step 2: Run tests**

Run: `go test ./internal/tui/ -run TestExportPopup -v`
Expected: FAIL — none of these methods exist.

**Step 3: Implement**

Append to `internal/tui/popup_export.go`:

```go
import (
	// ... existing imports
	"time"

	"charm.land/bubbles/v2/textinput"
)

type exportPopup struct {
	active      bool
	input       textinput.Model
	focusIdx    int
	fileExists  bool
	overwriteOK bool
	err         string
}

const (
	exportFocusInput = iota
	exportFocusOverwrite
	exportFocusConfirm
	exportFocusCancel
)

func newExportPopup() exportPopup {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 4096
	return exportPopup{input: ti}
}

func (p *exportPopup) open() {
	p.active = true
	p.focusIdx = exportFocusInput
	p.fileExists = false
	p.overwriteOK = false
	p.err = ""
	p.input.SetValue(defaultExportFilename(time.Now()))
	p.input.Focus()
}

func (p *exportPopup) cancel() {
	p.active = false
	p.input.SetValue("")
	p.input.Blur()
}

func (p *exportPopup) filename() string {
	return p.input.Value()
}

func (p *exportPopup) setFilename(s string) {
	p.input.SetValue(s)
	p.refreshFileExists()
}

func (p *exportPopup) refreshFileExists() {
	path := strings.TrimSpace(expandHome(p.filename()))
	if path == "" {
		p.fileExists = false
		p.overwriteOK = false
		return
	}
	info, err := os.Stat(path)
	exists := err == nil && !info.IsDir()
	if exists != p.fileExists {
		p.overwriteOK = false
	}
	p.fileExists = exists
}

func (p *exportPopup) canConfirm() bool {
	if strings.TrimSpace(p.filename()) == "" {
		return false
	}
	if p.fileExists && !p.overwriteOK {
		return false
	}
	return true
}

func (p *exportPopup) confirm(events []model.Event) error {
	if !p.canConfirm() {
		return fmt.Errorf("confirm not allowed in current state")
	}
	path := strings.TrimSpace(expandHome(p.filename()))
	if err := writeExport(path, events); err != nil {
		p.err = err.Error()
		return err
	}
	p.active = false
	p.input.Blur()
	return nil
}

func defaultExportFilename(now time.Time) string {
	return fmt.Sprintf("lazyagent-events-%s.json",
		now.Format("2006-01-02-1504"))
}
```

**Step 4: Run tests**

Run: `go test ./internal/tui/ -run TestExportPopup -v`
Expected: PASS.

**Step 5: Strip whitespace and commit**

```bash
perl -i -lpe 's/  *$//' internal/tui/popup_export.go internal/tui/popup_export_test.go
git add internal/tui/popup_export.go internal/tui/popup_export_test.go
git commit -m "$(cat <<'EOF'
Add exportPopup state machine

The popup tracks a textinput, focus index, file-exists check, and
overwrite-OK flag. open seeds a timestamped default filename;
confirm runs writeExport and closes on success. canConfirm
enforces non-empty filename and overwrite confirmation when the
target exists.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Popup keyboard handling — `enter`, `esc`, `tab`

Implement the popup's `Update` method that consumes key events
when active.

**Files:**
- Modify: `internal/tui/popup_export.go`
- Modify: `internal/tui/popup_export_test.go`

**Step 1: Write the failing tests**

```go
func TestExportPopup_EscCancels(t *testing.T) {
	p := newExportPopup()
	p.open()

	cmd := p.handleKey(tea.KeyPressMsg{Code: tea.KeyEsc}, nil)
	_ = cmd
	if p.active {
		t.Fatal("popup should close on esc")
	}
}

func TestExportPopup_TabCyclesFocus(t *testing.T) {
	p := newExportPopup()
	p.open()
	p.setFilename("/tmp/lazyagent-test-no-such-file.json")
	// File doesn't exist, so cycle is input → confirm → cancel.

	if p.focusIdx != exportFocusInput {
		t.Fatalf("initial focus: got %d, want input", p.focusIdx)
	}
	p.handleKey(tea.KeyPressMsg{Code: tea.KeyTab}, nil)
	if p.focusIdx != exportFocusConfirm {
		t.Fatalf("after tab: got %d, want confirm", p.focusIdx)
	}
	p.handleKey(tea.KeyPressMsg{Code: tea.KeyTab}, nil)
	if p.focusIdx != exportFocusCancel {
		t.Fatalf("after tab tab: got %d, want cancel", p.focusIdx)
	}
	p.handleKey(tea.KeyPressMsg{Code: tea.KeyTab}, nil)
	if p.focusIdx != exportFocusInput {
		t.Fatalf("after wrap: got %d, want input", p.focusIdx)
	}
}

func TestExportPopup_TabIncludesOverwriteWhenFileExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exists.json")
	os.WriteFile(path, []byte("[]"), 0644)

	p := newExportPopup()
	p.open()
	p.setFilename(path)

	p.handleKey(tea.KeyPressMsg{Code: tea.KeyTab}, nil)
	if p.focusIdx != exportFocusOverwrite {
		t.Fatalf("after tab: got %d, want overwrite", p.focusIdx)
	}
}

func TestExportPopup_EnterOnInputConfirmsWhenAllowed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")

	p := newExportPopup()
	p.open()
	p.setFilename(path)
	events := []model.Event{{ID: 1, Payload: `{"k":1}`}}

	p.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter}, events)

	if p.active {
		t.Fatal("popup should close on enter when confirm is allowed")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not written: %v", err)
	}
}

func TestExportPopup_SpaceTogglesOverwriteWhenFocused(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exists.json")
	os.WriteFile(path, []byte("[]"), 0644)

	p := newExportPopup()
	p.open()
	p.setFilename(path)
	p.focusIdx = exportFocusOverwrite

	p.handleKey(tea.KeyPressMsg{Code: ' '}, nil)
	if !p.overwriteOK {
		t.Fatal("space should toggle overwriteOK on")
	}
	p.handleKey(tea.KeyPressMsg{Code: ' '}, nil)
	if p.overwriteOK {
		t.Fatal("space again should toggle overwriteOK off")
	}
}
```

Note on the `tea.KeyPressMsg{Code: ...}` form: verify against
existing test code. If the project uses `tea.KeyMsg` or a different
helper, adapt accordingly. This whole task assumes the project
uses bubbletea v2's standard key-press representation.

**Step 2: Run tests**

Run: `go test ./internal/tui/ -run "TestExportPopup_(Esc|Tab|Enter|Space)" -v`
Expected: FAIL — `handleKey` is undefined.

**Step 3: Implement**

Add to `internal/tui/popup_export.go`:

```go
// handleKey processes a key event when the popup is active. The
// events slice is needed only for confirm; pass nil when not
// applicable. Returns a tea.Cmd in case future versions need one;
// for now it's always nil.
func (p *exportPopup) handleKey(msg tea.KeyMsg, events []model.Event) tea.Cmd {
	if !p.active {
		return nil
	}

	switch msg.Key().Code {
	case tea.KeyEsc:
		p.cancel()
		return nil

	case tea.KeyEnter:
		switch p.focusIdx {
		case exportFocusInput, exportFocusConfirm:
			if p.canConfirm() {
				_ = p.confirm(events)
			}
		case exportFocusCancel:
			p.cancel()
		case exportFocusOverwrite:
			p.overwriteOK = !p.overwriteOK
		}
		return nil

	case tea.KeyTab:
		p.advanceFocus(+1)
		return nil

	case tea.KeyShiftTab:
		p.advanceFocus(-1)
		return nil
	}

	// Space on overwrite toggles it without leaving focus.
	if msg.Key().Code == ' ' && p.focusIdx == exportFocusOverwrite {
		p.overwriteOK = !p.overwriteOK
		return nil
	}

	// Otherwise, when focus is on the input, forward to textinput.
	if p.focusIdx == exportFocusInput {
		var cmd tea.Cmd
		p.input, cmd = p.input.Update(msg)
		p.refreshFileExists()
		return cmd
	}
	return nil
}

func (p *exportPopup) advanceFocus(delta int) {
	order := []int{exportFocusInput, exportFocusConfirm,
		exportFocusCancel}
	if p.fileExists {
		order = []int{exportFocusInput, exportFocusOverwrite,
			exportFocusConfirm, exportFocusCancel}
	}
	pos := 0
	for i, f := range order {
		if f == p.focusIdx {
			pos = i
			break
		}
	}
	pos = (pos + delta + len(order)) % len(order)
	p.focusIdx = order[pos]
	if p.focusIdx == exportFocusInput {
		p.input.Focus()
	} else {
		p.input.Blur()
	}
}
```

You'll need `tea "charm.land/bubbletea/v2"` in the import block of
`popup_export.go`.

**Step 4: Run tests**

Run: `go test ./internal/tui/ -run TestExportPopup -v`
Expected: all PASS.

**Step 5: Strip whitespace and commit**

```bash
perl -i -lpe 's/  *$//' internal/tui/popup_export.go internal/tui/popup_export_test.go
git add internal/tui/popup_export.go internal/tui/popup_export_test.go
git commit -m "$(cat <<'EOF'
Add popup keyboard handling

handleKey processes esc, enter, tab, shift+tab, and space on the
overwrite checkbox. Tab cycles input → (overwrite if shown) →
confirm → cancel. Enter on the input or confirm runs the confirm
path; on cancel it closes; on overwrite it toggles the checkbox.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: Popup rendering

Render the popup as a centered overlay using the existing
`renderOverlayCentered` helper. Visual details (border, button
labels, red overwrite warning, disabled-confirm hint) live here.

**Files:**
- Modify: `internal/tui/popup_export.go`
- Modify: `internal/tui/popup_export_test.go`

**Step 1: Write the failing tests**

```go
func TestExportPopup_RenderShowsFilename(t *testing.T) {
	p := newExportPopup()
	p.open()
	p.setFilename("/tmp/foo.json")

	out := stripANSI(p.view(80))
	if !strings.Contains(out, "/tmp/foo.json") {
		t.Fatalf("view missing filename: %q", out)
	}
	if !strings.Contains(out, "confirm") {
		t.Fatalf("view missing confirm button: %q", out)
	}
	if !strings.Contains(out, "cancel") {
		t.Fatalf("view missing cancel button: %q", out)
	}
}

func TestExportPopup_RenderShowsOverwriteRowWhenFileExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exists.json")
	os.WriteFile(path, []byte("[]"), 0644)

	p := newExportPopup()
	p.open()
	p.setFilename(path)

	out := stripANSI(p.view(80))
	if !strings.Contains(out, "File exists") {
		t.Fatalf("view missing overwrite warning: %q", out)
	}
}

func TestExportPopup_RenderHidesOverwriteRowWhenFileMissing(t *testing.T) {
	p := newExportPopup()
	p.open()
	p.setFilename("/tmp/lazyagent-no-such-file.json")

	out := stripANSI(p.view(80))
	if strings.Contains(out, "File exists") {
		t.Fatalf("view should not show overwrite row: %q", out)
	}
}
```

**Step 2: Run tests**

Run: `go test ./internal/tui/ -run "TestExportPopup_Render" -v`
Expected: FAIL — `view` undefined.

**Step 3: Implement**

Add to `internal/tui/popup_export.go`:

```go
func (p *exportPopup) view(maxWidth int) string {
	if !p.active {
		return ""
	}

	width := min(max(maxWidth-20, 50), 80)

	label := lipgloss.NewStyle().Foreground(colorGray).
		Render("choose export filename: ")
	inputView := p.input.View()
	inputRow := label + inputView

	var rows []string
	rows = append(rows, inputRow)

	if p.fileExists {
		rows = append(rows, "")
		rows = append(rows, p.overwriteRow())
	}

	rows = append(rows, "")
	rows = append(rows, p.buttonRow())

	if p.err != "" {
		rows = append(rows, "")
		rows = append(rows, lipgloss.NewStyle().
			Foreground(colorRed).Render(p.err))
	}

	body := strings.Join(rows, "\n")
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorWhite).
		Padding(1, 2).
		Width(width).
		Render(body)
}

func (p *exportPopup) overwriteRow() string {
	check := "[ ] yes"
	if p.overwriteOK {
		check = "[x] yes"
	}
	row := "File exists, overwrite? " + check
	if p.focusIdx == exportFocusOverwrite {
		return lipgloss.NewStyle().
			Foreground(colorRed).Bold(true).Render(row)
	}
	return lipgloss.NewStyle().Foreground(colorRed).Render(row)
}

func (p *exportPopup) buttonRow() string {
	confirmStyle := lipgloss.NewStyle().Padding(0, 1)
	cancelStyle := confirmStyle

	if !p.canConfirm() {
		confirmStyle = confirmStyle.Foreground(colorGray)
	}
	if p.focusIdx == exportFocusConfirm {
		confirmStyle = confirmStyle.Reverse(true)
	}
	if p.focusIdx == exportFocusCancel {
		cancelStyle = cancelStyle.Reverse(true)
	}

	confirm := confirmStyle.Render("[confirm]")
	cancel := cancelStyle.Render("[cancel]")
	return confirm + "  " + cancel
}
```

`colorWhite`, `colorRed`, `colorGray` already exist in
`internal/tui/styles.go`. Confirm by searching that file before
using them.

**Step 4: Run tests**

Run: `go test ./internal/tui/ -run TestExportPopup -v`
Expected: PASS.

**Step 5: Strip whitespace and commit**

```bash
perl -i -lpe 's/  *$//' internal/tui/popup_export.go internal/tui/popup_export_test.go
git add internal/tui/popup_export.go internal/tui/popup_export_test.go
git commit -m "$(cat <<'EOF'
Render export popup with input, buttons, overwrite row

view returns the popup as a bordered, padded box. The overwrite
row only shows when the target file exists. Focus is indicated
by reverse video on buttons and bold red on the overwrite row.
The error string, if any, renders red below the buttons.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 12: Wire popup into Model — open, intercept, render

The parent `Model` gains an `export exportPopup` field. `x` opens
it. `Update` and `handleMouseClick` route to the popup when
active. The main `View` overlays the popup centered.

**Files:**
- Modify: `internal/tui/app.go`
- Modify: `internal/tui/mouse.go`
- Test: `internal/tui/app_routing_test.go` and/or
  `internal/tui/popup_export_test.go`

**Step 1: Write the failing tests**

```go
func TestExportPopup_XOpensWhenMarksExist(t *testing.T) {
	m := newModel(nil, time.Second)
	m.focus = focusEvents
	m.events.setEvents([]model.Event{
		{ID: 1, Payload: `{"a":1}`},
		{ID: 2, Payload: `{"b":2}`},
	}, 2, 0)
	m.events.autoFollow = false
	m.events.cursor = 0
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'm'})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'x'})
	m = updated.(Model)

	if !m.export.active {
		t.Fatal("popup should be active after x with marks")
	}
}

func TestExportPopup_EscClosesPopupAfterOpen(t *testing.T) {
	m := newModel(nil, time.Second)
	m.focus = focusEvents
	m.events.setEvents([]model.Event{
		{ID: 1, Payload: `{"a":1}`},
	}, 1, 0)
	m.events.autoFollow = false
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'm'})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'x'})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)

	if m.export.active {
		t.Fatal("popup should close on esc")
	}
}
```

**Step 2: Run tests**

Run: `go test ./internal/tui/ -run TestExportPopup_X -v`
Expected: FAIL — `Model.export` field doesn't exist or popup isn't
opened.

**Step 3: Implement**

In `internal/tui/app.go`:

1. Add the field to `Model` (after the `tokens` field around line 70):

```go
export exportPopup
```

2. Initialize it in `newModel`:

```go
export: newExportPopup(),
```

3. In `Update`, add a popup-active intercept BEFORE the existing
   tokens/debug overlay handling. Place this just after the
   `tea.MouseWheelMsg` case and before
   `if m.filter.searchMode`:

```go
if m.export.active {
	if msg, ok := msg.(tea.KeyMsg); ok {
		events := m.events.markedSnapshot()
		cmd := m.export.handleKey(msg, events)
		if !m.export.active && m.export.err == "" {
			m.status = fmt.Sprintf(
				"exported %d events to %s",
				len(events),
				strings.TrimSpace(expandHome(m.export.filename())))
		}
		return m, cmd
	}
	if click, ok := msg.(tea.MouseClickMsg); ok {
		// Mouse handling deferred to Task 13.
		_ = click
		return m, nil
	}
	return m, nil
}
```

You'll need `"strings"` in the import block of `app.go` if it's
not already there — check first.

4. In `updateEvents`, replace the placeholder `x` case from Task 8
   so it opens the popup when marks exist:

```go
case "x":
	if m.events.markedCount() == 0 {
		m.status = "no events marked — press m to mark events"
		m.lastKey = k
		return m, nil
	}
	m.export.open()
	m.lastKey = k
	return m, nil
```

5. In `View`, render the popup as a centered overlay. Find the
   final `View` return path (around line 833 in app.go where
   `lipgloss.JoinVertical(...)` builds the full screen). After the
   final composed string is built, overlay the popup:

```go
full = renderOverlayCentered(full, m.width, m.height,
	m.export.view(m.width))
```

This needs to happen AFTER any other overlays (errorOverlay,
tokens, debug) so the popup sits on top.

**Step 4: Run tests**

Run: `go test ./internal/tui/ -run "TestExportPopup_X|TestExportPopup_Esc" -v`
Expected: PASS.

Run the full TUI suite: `go test ./internal/tui/...`
Expected: PASS.

**Step 5: Strip whitespace and commit**

```bash
perl -i -lpe 's/  *$//' internal/tui/app.go
git add internal/tui/app.go internal/tui/app_routing_test.go internal/tui/popup_export_test.go
git commit -m "$(cat <<'EOF'
Wire export popup into Model

The parent Model gains an exportPopup field. Pressing x with at
least one marked event opens the popup; while active, the popup
intercepts key events at the top of Update. The main View
overlays the popup centered on top of everything else. On a
successful confirm the status line reports the export count and
path.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 13: Mouse click hit-testing inside the popup

Click on the input, the overwrite checkbox, the confirm button, or
the cancel button to focus/activate.

**Files:**
- Modify: `internal/tui/popup_export.go`
- Modify: `internal/tui/app.go`
- Test: `internal/tui/popup_export_test.go`

**Step 1: Write the failing test**

```go
func TestExportPopup_MouseClickFocusesElement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exists.json")
	os.WriteFile(path, []byte("[]"), 0644)

	p := newExportPopup()
	p.open()
	p.setFilename(path)

	// Click on the overwrite row and check focus moves there.
	// We don't validate exact pixel coordinates here; just that
	// handleClick with hitOverwrite changes focus.
	p.handleClick(hitOverwrite, nil)
	if p.focusIdx != exportFocusOverwrite {
		t.Fatalf("after overwrite click: got %d, want overwrite",
			p.focusIdx)
	}
	// A second click toggles overwriteOK.
	p.handleClick(hitOverwrite, nil)
	if !p.overwriteOK {
		t.Fatal("second click on overwrite should toggle on")
	}
}

func TestExportPopup_ClickConfirmActivates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")

	p := newExportPopup()
	p.open()
	p.setFilename(path)
	events := []model.Event{{ID: 1, Payload: "{}"}}

	p.handleClick(hitConfirm, events)

	if p.active {
		t.Fatal("clicking confirm should close popup on success")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not written: %v", err)
	}
}
```

**Step 2: Run test**

Run: `go test ./internal/tui/ -run "TestExportPopup_(MouseClick|ClickConfirm)" -v`
Expected: FAIL — `handleClick` and `hit*` constants undefined.

**Step 3: Implement**

Add to `internal/tui/popup_export.go`:

```go
type popupHit int

const (
	hitNone popupHit = iota
	hitInput
	hitOverwrite
	hitConfirm
	hitCancel
)

// handleClick processes a click on a known popup element. The
// caller is responsible for translating screen coordinates into a
// popupHit; this keeps geometry concerns in app.go and lets us
// test the state transitions in isolation.
func (p *exportPopup) handleClick(hit popupHit, events []model.Event) {
	if !p.active {
		return
	}
	switch hit {
	case hitInput:
		p.focusIdx = exportFocusInput
		p.input.Focus()
	case hitOverwrite:
		if p.fileExists {
			if p.focusIdx == exportFocusOverwrite {
				p.overwriteOK = !p.overwriteOK
			} else {
				p.focusIdx = exportFocusOverwrite
				p.input.Blur()
			}
		}
	case hitConfirm:
		p.focusIdx = exportFocusConfirm
		p.input.Blur()
		if p.canConfirm() {
			_ = p.confirm(events)
		}
	case hitCancel:
		p.cancel()
	}
}
```

In `app.go`, replace the deferred mouse-click handling in the
popup-active block from Task 12 with real hit-testing:

```go
if click, ok := msg.(tea.MouseClickMsg); ok {
	hit := m.export.hitTest(click.X, click.Y, m.width, m.height)
	events := m.events.markedSnapshot()
	m.export.handleClick(hit, events)
	if !m.export.active && m.export.err == "" {
		m.status = fmt.Sprintf(
			"exported %d events to %s",
			len(events),
			strings.TrimSpace(expandHome(m.export.filename())))
	}
	return m, nil
}
```

Add a `hitTest` method on `exportPopup` in `popup_export.go`:

```go
// hitTest returns which popup element the screen coordinate falls
// on. The popup is rendered centered via renderOverlayCentered, so
// we recompute its origin from the same width math used in view().
func (p *exportPopup) hitTest(x, y, screenW, screenH int) popupHit {
	if !p.active {
		return hitNone
	}
	rendered := p.view(screenW)
	if rendered == "" {
		return hitNone
	}

	w := lipgloss.Width(rendered)
	h := lipgloss.Height(rendered)
	originX := max((screenW-w)/2, 0)
	originY := max((screenH-h)/2, 0)

	if x < originX || x >= originX+w ||
		y < originY || y >= originY+h {
		return hitNone
	}

	// Inside the popup: rough row-based mapping. Layout is:
	// row 0 = top border
	// row 1 = top padding
	// row 2 = input
	// row 3 = blank
	// row 4 = overwrite (or confirm if no overwrite)
	// row 5 = blank or confirm row
	// ...
	// We compute by counting the rows we rendered.
	relY := y - originY
	rows := strings.Split(rendered, "\n")
	// Find the line that contains "[confirm]" / "[cancel]" — that
	// row is the button row. The line containing "File exists" (if
	// any) is the overwrite row. The line containing the input is
	// just below the top padding.
	var inputRow, overwriteRow, buttonRow int = -1, -1, -1
	for i, line := range rows {
		plain := stripANSI(line)
		switch {
		case inputRow < 0 && strings.Contains(plain,
			"choose export filename:"):
			inputRow = i
		case overwriteRow < 0 && strings.Contains(plain,
			"File exists"):
			overwriteRow = i
		case buttonRow < 0 && strings.Contains(plain,
			"[confirm]"):
			buttonRow = i
		}
	}

	switch relY {
	case inputRow:
		return hitInput
	case overwriteRow:
		return hitOverwrite
	case buttonRow:
		// X-split between confirm and cancel.
		plain := stripANSI(rows[buttonRow])
		idx := strings.Index(plain, "[cancel]")
		if idx < 0 {
			return hitConfirm
		}
		// Adjust idx for popup origin and any leading padding.
		if x-originX >= idx {
			return hitCancel
		}
		return hitConfirm
	}
	return hitNone
}
```

`stripANSI` already exists as a test helper but you'll likely need
a non-test version. Check `internal/tui/helpers.go`. If only the
test file has it, lift the implementation into `helpers.go` (or a
new file) and have both call sites use it.

**Step 4: Run tests**

Run: `go test ./internal/tui/ -run "TestExportPopup_(MouseClick|ClickConfirm)" -v`
Expected: PASS.

Run full suite: `go test ./internal/tui/...`
Expected: PASS.

**Step 5: Strip whitespace and commit**

```bash
perl -i -lpe 's/  *$//' internal/tui/popup_export.go internal/tui/app.go internal/tui/helpers.go internal/tui/popup_export_test.go
git add internal/tui/popup_export.go internal/tui/app.go internal/tui/helpers.go internal/tui/popup_export_test.go
git commit -m "$(cat <<'EOF'
Handle mouse clicks inside the export popup

handleClick reacts to a popupHit enum supplied by the parent
Model. hitTest translates screen coordinates into a popupHit by
locating the input, overwrite, and button rows in the rendered
output. This keeps geometry and state separate so the state
transitions are easy to test in isolation.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 14: End-to-end exercise

A test that drives the full flow: load events, mark two, press `x`,
type a filename, confirm, verify the file's contents.

**Files:**
- Test: `internal/tui/popup_export_test.go`

**Step 1: Write the failing test**

```go
func TestExportFullFlow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "flow.json")

	m := newModel(nil, time.Second)
	m.focus = focusEvents
	m.events.setEvents([]model.Event{
		{ID: 1, Payload: `{"k":"first"}`},
		{ID: 2, Payload: `{"k":"second"}`},
		{ID: 3, Payload: `{"k":"third"}`},
	}, 3, 0)
	m.events.autoFollow = false
	m.events.cursor = 0

	// Mark events 1 and 3 (indices 0 and 2).
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'm'}) // mark 1, cursor → 1
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j'}) // cursor → 2
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'm'}) // mark 3, cursor → 2 (clamp)
	m = updated.(Model)

	if m.events.markedCount() != 2 {
		t.Fatalf("markedCount: got %d, want 2",
			m.events.markedCount())
	}

	// Open popup.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'x'})
	m = updated.(Model)
	if !m.export.active {
		t.Fatal("popup should be active")
	}

	// Set filename and confirm via enter.
	m.export.setFilename(path)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)

	if m.export.active {
		t.Fatal("popup should close")
	}

	// Verify file contents.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var got []map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, data)
	}
	if len(got) != 2 {
		t.Fatalf("len: got %d, want 2", len(got))
	}
	if got[0]["k"] != "first" || got[1]["k"] != "third" {
		t.Fatalf("contents: got %v", got)
	}

	// Verify status message.
	if !strings.Contains(m.status, "exported 2 events") {
		t.Fatalf("status: got %q", m.status)
	}
	if !strings.Contains(m.status, path) {
		t.Fatalf("status missing path: %q", m.status)
	}
}
```

**Step 2: Run test**

Run: `go test ./internal/tui/ -run TestExportFullFlow -v`
Expected: PASS (this is just an integration check — every
underlying piece was tested in earlier tasks). If it FAILS,
investigate which earlier piece doesn't compose correctly and fix
it before continuing.

**Step 3: (No implementation) Strip whitespace and commit**

```bash
perl -i -lpe 's/  *$//' internal/tui/popup_export_test.go
git add internal/tui/popup_export_test.go
git commit -m "$(cat <<'EOF'
Add end-to-end test for mark + export flow

Marks two of three events, opens the popup with x, types a
filename, confirms with enter, and asserts the file contents,
order, and status message.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 15: Manual smoke test and final commit

Run the actual binary against a real session DB and validate the
feature works end-to-end with a real terminal.

**Files:** None.

**Step 1:** Build the binary.

```bash
go build -o /tmp/lazyagent ./cmd/lazyagent
```

Expected: build succeeds.

**Step 2:** Run lazyagent against the user's existing data.

Tell the user to launch `/tmp/lazyagent`, focus the events pane
(`4`), and exercise:

- `m` marks the current event with `>` and advances.
- `j`, `m` again — second event marked.
- `M` marks every visible event.
- `U` clears all marks.
- `x` with no marks — status line shows the hint.
- `x` with marks — popup opens with a timestamped default filename.
- Edit the filename, hit `enter`, file is written.
- Press `x` again, type the same filename, see the overwrite row,
  flip it on, confirm — file is overwritten.
- Cancel with `esc` — no file written.
- Filter changes (`a`, `t`) preserve marks.

**Step 3:** If anything looks off, file a follow-up note (no extra
commit needed); if everything works, no commit needed for this
task.

---

## Out of scope (do not implement)

- Persisting marks across app restarts.
- Exporting in formats other than a JSON array.
- Exporting full `Event` structs with lazyagent metadata.
- Mouse drag selection.
- Marking events not yet loaded into the events slice.

## Risk notes for the implementer

- **Bubble Tea v2 key types:** the codebase uses
  `charm.land/bubbletea/v2`. Verify the exact `KeyMsg` shape
  (`tea.KeyPressMsg{Code: ...}` vs other forms) by reading
  existing tests in `internal/tui/app_routing_test.go` before
  writing your own. Adapt the test snippets in this plan to the
  project's actual style if they diverge.
- **Mouse coordinate system:** `mouse.go` already does the
  pane-level hit-testing using the same `paneSizes`/`mainH` math
  the renderer uses. The popup hit-test in Task 13 uses a
  different approach (locate rows by content) because the popup
  is overlaid late in `View()`, after layout math has already run.
  If overlay positions ever drift, prefer fixing the math in
  `hitTest` rather than re-architecting.
- **`setEvents` and offset shifts:** existing tests
  (`TestSetEvents_OffsetShift_*`) verify that cursor and scroll
  compensate for new events arriving. Marks must remain untouched
  by `setEvents`; Task 3's regression test guards this.
- **Status-line clobbering:** other features (`r`, `t`, `a`, etc.)
  set `m.status`. Our export status will be replaced on the next
  filter change or refresh — that's fine, matches the existing
  pattern.
