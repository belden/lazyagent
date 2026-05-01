# Mark and Export Events — Design

**Date:** 2026-05-01
**Status:** Draft, pending review

## Summary

Add a dired-inspired mark/unmark workflow to the Events pane and an
export popup that writes the marked events' raw payloads to a JSON
file on disk.

## User-facing behavior

### Marking

- **`m`** — toggle the mark on the event under the cursor, then move
  the cursor down one line. Holding/repeating `m` lets the user mark
  a contiguous range with one keystroke per event.
- **`M`** — mark every event currently visible in the events pane
  (i.e., every event that matches the active agent and event-type
  filters and is loaded into the events slice).
- **`U`** — clear all marks.

A marked event renders with `> ` in place of the existing two-space
left margin. Column alignment is preserved; nothing shifts. Example:

```
Events: 264
>   1  7a72251a  SessionStart  claude-opus-4-7
    2  7a72251a  UserPromptSubmit  ok, our credit_cards have an expir…
>   3  7a72251a  PreToolUse  Agent  [Explore] Find credit card expir…
    4  afeb60d2  PreToolUse  Bash  find /home/blyman/code/pp/paperle…
```

Marks are tied to an event's `ID`, not its position. Changing the
agent filter, the event-type filter, or scrolling does not lose
marks. Events that the user has filtered out simply aren't visible,
but they remain marked and counted for export. Marks are in-memory
only. Restarting lazyagent clears them.

### Exporting

- **`x`** — open the export popup. If no events are marked, do not
  open the popup; instead show a status-line message:
  `no events marked — press m to mark events`.

The popup contains:

- A text input pre-filled with a timestamped default like
  `lazyagent-events-2026-05-01-1430.json`.
- A `[confirm]` button.
- A `[cancel]` button.
- An overwrite confirmation row that appears only when the typed
  filename refers to an existing file: red `File exists, overwrite?`
  text plus a `[ ] yes` / `[x] no` checkbox. Until the box is
  flipped to `yes`, `[confirm]` is disabled.

Keyboard and mouse:

- `tab` / `shift+tab` cycle focus through input → (overwrite
  checkbox if present) → confirm → cancel.
- `enter` activates the focused element. From inside the input field,
  `enter` is equivalent to clicking confirm.
- `esc` always cancels.
- Mouse click focuses any element directly. Clicking confirm or
  cancel activates them.

Filename rules:

- Empty input or whitespace-only — confirm is disabled.
- Leading `~` is expanded to `$HOME`.
- Absolute paths are written as-is.
- Relative paths resolve against the user's current working
  directory (the directory lazyagent was launched from).
- The popup re-checks file existence whenever the input changes.

After a successful export:

- Marks are not cleared. The user explicitly chose `U` for that, and
  preserving marks lets the user re-export to a different filename
  or notice they missed an event.
- The popup closes.
- A status-line message confirms: `exported N events to <path>`.

On a write error, the popup stays open and renders the error in red
beneath the input.

## Export file format

The output file is always a JSON array, even for a single marked
event:

```json
[
  { ...payload of marked event 1... },
  { ...payload of marked event 2... }
]
```

Each array element is the parsed `Event.Payload`, matching exactly
what the Details pane shows when `J` toggles raw JSON
(`PayloadPretty()` in `internal/model/types.go:119`). Lazyagent
metadata (`AgentID`, `SessionID`, `Timestamp`, etc.) is not
included; the user wants the raw event data the agent emitted.

Edge cases:

- Empty `Payload` → `{}` in the array.
- `Payload` that fails to parse as JSON → `null` in the array.

Order: events appear in the same order they appear in the events
slice (chronological), regardless of the order the user marked them
in.

## Implementation

### New state on `eventsModel`

In `internal/tui/pane_events.go`:

```go
type eventsModel struct {
    // ... existing fields ...
    marked       map[string]struct{}     // event IDs that are marked
    markedEvents map[string]model.Event  // ID → cached Event for export
}
```

Two maps:

- `marked` is the source of truth for "is this event marked?" — used
  by render and by the popup-open guard.
- `markedEvents` caches the `Event` value at the time of marking so
  that export never has to re-query the DB. The events slice is a
  paginated window; an event the user marked may scroll out of the
  loaded set. Caching the value at mark time keeps export local and
  bounded by the user's mark count.

Both maps are cleared on `U` and on app shutdown.

### Mark operations

New methods on `eventsModel`:

- `toggleMark()` — flips the mark on the event under the cursor,
  updates both maps, then advances the cursor by one (`moveDown()`).
  No-op if the cursor is out of range.
- `markAllVisible()` — iterates the currently loaded `events` slice
  and adds each entry to both maps. Cursor unchanged.
- `unmarkAll()` — empties both maps. Cursor unchanged.
- `isMarked(id string) bool` — used by render.
- `markedCount() int` — used by `x` guard and status messages.
- `markedSnapshot() []model.Event` — returns marked events ordered
  to match their position in the loaded events slice.

### Rendering the marker

In `renderPlainEventLine` and `renderSelectedEventLine` in
`pane_events.go`, replace the hard-coded `"  "` prefix with a helper
that returns `"> "` if the event is marked or `"  "` otherwise. The
prefix width stays at 2, so all downstream column math is
unaffected.

### Keybindings

In `internal/tui/keys.go`, add:

```go
MarkToggle   key.Binding  // m
MarkAll      key.Binding  // M
UnmarkAll    key.Binding  // U
ExportMarked key.Binding  // x
```

In the events-pane key handler in `internal/tui/app.go`, route `m`,
`M`, `U` to the new `eventsModel` methods. Route `x` to the
popup-open path.

### Popup

New file `internal/tui/popup_export.go`:

```go
type exportPopup struct {
    active      bool
    input       textinput.Model
    focusIdx    int      // 0=input, 1=overwrite (if shown), 2=confirm, 3=cancel
    fileExists  bool
    overwriteOK bool
    err         string
}
```

The parent `Model` gains an `export exportPopup` field.

Behavior:

- `openExportPopup()` — initializes the input with the timestamped
  default, sets `active = true`, and gives the popup focus.
- `Update()` for the popup intercepts key and mouse events while
  `active`. Tab/Shift+Tab move `focusIdx`. Skipping the overwrite
  index when the row is hidden keeps the cycle natural.
- On every input change, `fileExists` is re-evaluated by `os.Stat`.
  When the file does not exist, `overwriteOK` is reset and the row
  hides.
- On confirm: expand `~`, write the array, close the popup, set the
  status line. On error, populate `err` and keep the popup open.

The parent `Update` and `handleMouseClick` need a guard at the top:
when the popup is active, route the event to the popup and return
early. This keeps the rest of the app blissfully unaware of popup
state.

Layout: lipgloss-rendered box centered on the screen, drawn over
the existing view via the same overlay pattern other Bubble Tea
apps use (a string composed after the main view, then trimmed and
written into the right cell range). If the project doesn't already
have a popup-overlay helper, we add a minimal one alongside the
popup file.

### Writing the file

A small helper in `popup_export.go`:

```go
func writeExport(path string, events []model.Event) error
```

- Build a `[]any` of length `len(events)`.
- For each event, attempt `json.Unmarshal(event.Payload)`. On
  success use the parsed value. On empty payload, use
  `map[string]any{}` (renders as `{}`). On parse failure, use
  `nil` (renders as `null`).
- `json.MarshalIndent` the slice with two-space indent.
- Write atomically: write to `<path>.tmp` in the same directory,
  then `os.Rename` over `<path>`. This avoids leaving a half-written
  file if the process is interrupted.

### Status line

Reuse the existing status-line plumbing for the two messages we add:

- `no events marked — press m to mark events`
- `exported N events to <path>`

## Edge cases and error handling

- **Mark a marked event** — `m` toggles, so the second press unmarks
  and still advances the cursor.
- **`M` with no events visible** — no-op. No status message needed;
  the empty pane is its own feedback.
- **Filter changes hide marked events** — marks remain in `marked`
  and `markedEvents`. `x` still works. `U` still clears them.
- **`x` with one event marked** — popup opens normally, export array
  has one element.
- **Filename contains a path that doesn't exist** (e.g.
  `out/data.json` with no `out/` directory) — write fails, error
  shown in popup; popup stays open.
- **Filename is a directory** — write fails, error shown in popup.
- **Filename is empty after trimming** — confirm stays disabled.
- **User confirms while file exists but overwrite is unchecked** —
  shouldn't be reachable because confirm is disabled; if reached
  (e.g. via stale state), treat as a no-op.
- **App crashes mid-export** — atomic rename means the user either
  has the old file or the new file, never a half-written one.

## Testing

Bubble Tea models are easy to drive in tests. Build on the patterns
already in `internal/tui/pane_events_test.go`.

New unit tests in `internal/tui/pane_events_test.go`:

- `m` toggles a mark and advances the cursor.
- `m` on a marked event unmarks it and still advances.
- `M` marks every loaded event.
- `U` clears all marks.
- Mark survives an agent-filter change (event still marked, even
  when not visible in the slice).
- `markedSnapshot()` returns events in slice order, not mark order.

New unit tests in a new `internal/tui/popup_export_test.go`:

- `writeExport` with one event yields a one-element array.
- `writeExport` with three events yields a three-element array in
  the order given.
- Empty payload becomes `{}` in the output.
- Unparseable payload becomes `null` in the output.
- Atomic write: a failed write does not leave a partial file at the
  target path (simulate by pointing at a directory).
- `~` expansion happens before path checks.

New integration test (Bubble Tea harness):

- Pressing `x` with zero marks shows the status message and does
  not open the popup.
- Pressing `x` with marks opens the popup; typing a name and
  hitting enter writes the file with the expected content.
- Existing-file flow: the overwrite checkbox appears, confirm is
  disabled until the box is flipped, flipping it enables confirm.
- `esc` cancels the popup without writing.

## Out of scope

- Persisting marks to disk across app restarts.
- Exporting in formats other than a JSON array.
- Exporting full `Event` structs with lazyagent metadata.
- Mouse drag selection.
- Marking events not yet loaded into the events slice.
