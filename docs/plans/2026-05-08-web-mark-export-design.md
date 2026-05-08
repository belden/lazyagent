# Web UI — Mark and Export Events Design

**Date:** 2026-05-08
**Status:** Draft, pending implementation

## Summary

Bring the TUI's mark-and-export feature to the web UI. Users can
check off events with the mouse, shift-click to mark a contiguous
range, and download the marked events' raw payloads as a JSON file
through the browser. No server changes; the read-only contract on
`internal/web` is preserved.

## User-facing behavior

### Marking

- Each event row gains a leftmost checkbox cell. Plain click on the
  checkbox toggles that event's mark and sets it as the **anchor**.
- Shift-click on a checkbox sets every event between the anchor and
  the shift-clicked row to the **new state of the shift-clicked
  box** (Gmail-style: shift-click an unchecked box checks the whole
  range; shift-click a checked box unchecks it). The anchor stays
  put across shift-clicks.
- If shift-click happens with no anchor yet (or the anchor isn't in
  the currently loaded list), it acts like a plain click.
- Clicking the checkbox does not select the event in the detail
  pane; it `stopPropagation`s so the row's existing click handler
  is unaffected.

A marked row gets a CSS `.marked` class on its `<li>` so the row
can render an accent (left border, similar in spirit to the TUI's
`>` indicator).

### Export trigger

A new slot lives in the events header, between the collapsible
title and the existing filters block:

```html
<header class="events-head">
  <h3 class="collapsible">…</h3>
  <div class="export"></div>
  <div class="filters">…</div>
</header>
```

When zero events are marked, the slot is empty and collapses via
flex. When at least one is marked, the slot renders:

```html
<span class="mark-count">2 marked</span>
<button class="export-btn" type="button">export</button>
<button class="clear-btn" type="button">clear</button>
```

`[clear]` empties the mark set and the anchor, and re-renders the
list. `[export]` triggers a browser download (see below) and
flashes `exported N events` in the slot for three seconds.

### Exporting

Click `[export]`:

1. Snapshot `marks` as an array, sorted by event ID ascending so
   the file order is chronological (matches the events list).
2. Fetch each event's full payload in parallel via the existing
   `GET /api/events/{id}` endpoint:
   ```js
   const results = await Promise.allSettled(
     ids.map(id => fetch(`/api/events/${id}`).then(r => r.json()))
   );
   ```
3. Build a JSON array of payloads:
   - The detail endpoint returns `{payload: <object>, raw: <string>}`.
     Unparseable payloads come back under `raw` with no `payload`
     field; those become `null` in the export.
   - Empty payloads become `{}`.
   - Failed fetches (404 / 500 / network) are dropped from the
     output and counted as failures in the status flash.
4. `JSON.stringify(arr, null, 2)` → `Blob` (`application/json`) →
   temporary `<a href="blob:..." download="lazyagent-events-YYYY-MM-DD-HHMM.json">`
   appended to `document.body`, `.click()`, `URL.revokeObjectURL`,
   removed. Filename pattern matches the TUI default.
5. Status flash:
   - All succeeded: `exported 2 events`.
   - Partial: `exported 2 events (1 failed)`.

The browser handles the save dialog (or auto-saves to the user's
Downloads folder, per their browser settings). No app-level
filename modal.

### State lifecycle

- **Auto-refresh:** the periodic events fetch leaves marks intact.
  New events arrive unmarked; events that scroll out of the loaded
  window stay in `marks` and still export.
- **Filter change** (type dropdown, search): marks intact. Hidden
  marked events still count and still export.
- **Session change:** marks and anchor clear. Each session gets
  fresh state.
- **Project change:** clears the session, which clears marks.
- **Page reload:** marks gone. In-memory only, matching the TUI.

## Implementation

### Client state

In `internal/web/static/app.js`:

```js
const marks = new Set();   // event IDs (numbers)
let markAnchor = null;     // last single-clicked event ID
```

Both reset by a helper `clearMarks()` which is called on session
change and on `[clear]`.

### Rendering

Where `app.js` builds an event `<li>`:

- Prepend `<input type="checkbox" class="event-mark" data-event-id="${id}">`.
- Set `checked` from `marks.has(id)`.
- Add `marked` to the `<li>` class list when marked.
- Wire `click` on the checkbox: `stopPropagation`, then dispatch
  to `onMarkClick(ev, id)`.

`onMarkClick(ev, id)`:
- If `ev.shiftKey && markAnchor != null && markAnchor !== id` and
  both IDs are present in the currently loaded events array:
  call `applyRange(markAnchor, id, ev.target.checked)`.
- Else: toggle `marks.has(id)`; set `markAnchor = id`.
- Re-render the export slot and update `.marked` classes on the
  affected rows.

`applyRange(a, b, state)`:
- Find indices of `a` and `b` in the current `events` array.
- For every event between them (inclusive), set `marks.add` if
  `state` else `marks.delete`.
- Anchor unchanged.

### Export slot

`renderExportSlot()` rebuilds the contents of `<div class="export">`
based on `marks.size`:

- `0`: clear innerHTML.
- `>0`: render mark count, `[export]`, `[clear]`. Wire click
  handlers to `exportMarks()` and `clearMarks()`.

A `flashStatus(text)` helper appends a `<span class="export-status">`
into the slot, then `setTimeout(..., 3000)` removes it.

### Export action

```js
async function exportMarks() {
  const ids = [...marks].sort((a, b) => a - b);
  const results = await Promise.allSettled(
    ids.map(id => fetch(`/api/events/${id}`).then(r => {
      if (!r.ok) throw new Error(`HTTP ${r.status}`);
      return r.json();
    }))
  );

  const payloads = [];
  let failed = 0;
  for (const r of results) {
    if (r.status !== "fulfilled") { failed++; continue; }
    const ev = r.value;
    if (ev.payload !== undefined) {
      payloads.push(ev.payload);
    } else if (ev.raw && ev.raw !== "") {
      payloads.push(null);    // unparseable payload
    } else {
      payloads.push({});      // empty payload
    }
  }

  const json = JSON.stringify(payloads, null, 2);
  const blob = new Blob([json], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = defaultExportFilename();
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);

  if (failed > 0) {
    flashStatus(`exported ${payloads.length} events (${failed} failed)`);
  } else {
    flashStatus(`exported ${payloads.length} events`);
  }
}

function defaultExportFilename() {
  const d = new Date();
  const pad = n => String(n).padStart(2, "0");
  return `lazyagent-events-${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}-${pad(d.getHours())}${pad(d.getMinutes())}.json`;
}
```

### Styles

Add to `style.css`:

- `.event-mark` — small inline checkbox, no margin shift on the
  row content.
- `.events li.marked` — left border accent (e.g.
  `border-left: 3px solid var(--accent)`).
- `.events-head .export` — flex slot, `gap: 0.5rem`,
  `align-items: center`.
- `.mark-count` — muted text.
- `.export-btn`, `.clear-btn` — match existing button styles.
- `.export-status` — small muted text, fade-out optional.

### Server changes

None. The read-only contract documented in
`internal/web/server.go:1-7` stays.

## Edge cases

- **Mark on a row, then auto-refresh removes that row** (event got
  deleted upstream): the ID stays in `marks`. On export, the fetch
  for that ID 404s and is counted as a failure in the status.
- **Shift-click with anchor not in loaded list**: treat as plain
  click. We don't load older events on demand for shift-click.
- **Browser blocks the download** (very rare for same-origin Blob
  downloads): no special handling; the user sees the browser's own
  prompt.
- **Filename collision in the user's download directory**: browser
  handles it (most append `(1)` automatically, some prompt).
- **Session switch with marks set**: `clearMarks()` runs.
  `[export]` and `[clear]` slot vanishes since `marks.size === 0`.

## Testing

No JS test infrastructure exists in this repo, and we won't add it
for this feature. Manual QA only. Smoke checklist:

- Single click toggles a row, sets anchor.
- Shift-click below the anchor checks the range.
- Shift-click again on a checked row above the anchor unchecks
  the range.
- Anchor survives across non-shift clicks (last single click wins).
- `[clear]` empties marks; slot collapses; rows lose `.marked`.
- `[export]` with two marks downloads a 2-element JSON array of
  parsed payloads; filename matches `lazyagent-events-…`.
- Switching sessions clears marks.
- Filter change keeps marks; export still includes hidden marked
  events.
- Auto-refresh tick keeps marks intact.
- 404 on a deleted event during export → status reports failed
  count; valid payloads still in the file.

## Out of scope

- Persisting marks across page reloads.
- Server-side write endpoint.
- Keyboard shortcuts for mark/unmark/export (mouse-only for v1).
- Cross-session marking.
- Loading older events on demand for shift-click range.
- Caching payloads at mark time (revisit if export latency hurts).

## Risk notes

- **`Promise.all` vs `Promise.allSettled`**: use `allSettled` so a
  single failed fetch doesn't drop the whole download.
- **`URL.revokeObjectURL` timing**: revoke only after `click()`
  returns. Some browsers will skip the download if the URL is
  revoked first.
- **Existing row click handler**: don't break the event-detail
  selection. The checkbox `click` handler must `stopPropagation`.
- **List view does not include payload**: confirmed via
  `internal/web/views.go:236-257` (eventView omits payload).
  Export must hit `/api/events/{id}` per marked event.
