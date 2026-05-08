# Web Mark and Export Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Bring the TUI's mark-and-export feature to the read-only web UI: shift-clickable checkboxes per event row plus an `[export]` button that downloads marked events' raw payloads as JSON.

**Architecture:** All client-side. No server changes — the `internal/web` read-only contract stays. New checkbox column on each event row drives a `Set<eventId>` of marks; an export-slot in the events header renders mark count, `[export]`, and `[clear]` once at least one event is marked. `[export]` fetches each marked event from the existing `GET /api/events/{id}` endpoint (the list endpoint omits payloads), assembles a JSON array of payloads, and triggers a browser download via a temporary `<a download>` Blob URL.

**Tech Stack:** Plain JavaScript (no framework), CSS, Go-served static assets at `internal/web/static/{index.html,app.js,style.css}`. No JS test infrastructure exists in this repo and we are not adding any — every task ends with a manual smoke check the human operator runs.

**Reference design:** `docs/plans/2026-05-08-web-mark-export-design.md`

---

## Pre-flight

This plan assumes you are working in the worktree at `.worktrees/web-mark-export` on branch `feat/web-mark-export`. Confirm:

```bash
git -C . rev-parse --abbrev-ref HEAD
```

Expected: `feat/web-mark-export`

```bash
go test ./...
```

Expected: all packages pass (this is the baseline before any change).

---

## Task 1: Locate and inventory the touchpoints in `app.js`

**Files:**
- Read: `internal/web/static/app.js`
- Read: `internal/web/static/style.css`
- Read: `internal/web/static/index.html`

**Step 1: Read each file end-to-end**

Run:

```bash
wc -l internal/web/static/app.js internal/web/static/style.css internal/web/static/index.html
```

Read every file in full so you understand:

- where event `<li>`s are constructed and inserted (the row renderer)
- where the events list refresh runs and what state it lives in (the auto-refresh / polling fetch)
- where session-change handling clears UI state (so we know where to call `clearMarks()`)
- where existing buttons/styles live so the new `[export]` and `[clear]` buttons match

**Step 2: Write down (in your own scratch notes, not committed) the exact identifiers you will need**

You should be able to answer:

- What is the variable / array name that holds the currently rendered events?
- What is the function that builds one event `<li>`?
- What is the function that re-renders the list?
- What handler runs on session change and where would `clearMarks()` slot in?
- What is the existing event-id property on each `<li>` (data attribute? closure capture?) and how is the row-click handler wired?

If any of these are not obvious from one pass, read again. Do not start coding until you can name all five.

**Step 3: No commit (read-only inventory).**

---

## Task 2: Add the export slot to the events header markup

**Files:**
- Modify: `internal/web/static/index.html`

The design specifies the new order inside `<header class="events-head">`:

```html
<header class="events-head">
  <h3 class="collapsible" data-toggle="events-block">
    <span class="caret">▶</span>
    Events <span id="events-count" class="count-badge"></span>
  </h3>
  <div class="export"></div>
  <div class="filters">
    <select id="filter-type">…</select>
    <input id="filter-search" type="text" placeholder="search payload" />
  </div>
</header>
```

**Step 1: Add the `<div class="export"></div>` between the `<h3>` and the `<div class="filters">`**

Use Edit on `internal/web/static/index.html` to insert one line. Do not change the surrounding markup.

**Step 2: Strip trailing whitespace**

Run:

```bash
perl -i -lpe 's/  *$//' internal/web/static/index.html
```

**Step 3: Smoke check (no JS yet, just markup)**

Run:

```bash
go run ./cmd/lazyagent web --no-browser
```

(or whatever invocation the project uses for the web mode — confirm by reading `cmd/lazyagent` if needed). In another terminal, `curl -s http://127.0.0.1:7777/ | grep -A2 'events-head'` and confirm the new `<div class="export">` is present. Stop the server with Ctrl-C.

**Step 4: Commit**

```bash
git add internal/web/static/index.html
git commit -m "Add export slot to events header markup

The new div.export sits between the events title and the filters
block. It is empty until JavaScript begins populating it once the
mark-and-export feature ships in subsequent commits."
```

Subject is 50 chars or fewer; body wraps at 70.

---

## Task 3: Add CSS for checkbox column, marked row accent, and export slot

**Files:**
- Modify: `internal/web/static/style.css`

Per the design (Styles section):

- `.event-mark` — small inline checkbox, no margin shift on the row content
- `.events li.marked` — left border accent: `border-left: 3px solid var(--accent)` (use whatever the existing accent custom-property is named; if there is none, pick a color consistent with the existing palette and define a `--accent` variable at the top of `style.css`)
- `.events-head .export` — `display: flex; gap: 0.5rem; align-items: center;`
- `.mark-count` — muted text (re-use the existing `.muted` color variable)
- `.export-btn`, `.clear-btn` — match existing button styles (look at how `#refresh-btn` or similar are styled and either reuse a class or copy its declarations)
- `.export-status` — small muted text

**Step 1: Read `style.css` and identify the existing accent / muted / button conventions**

If the existing `.events li` uses padding to leave room for content, account for that when adding `border-left: 3px solid …` (the border eats 3px; you may need a matching `padding-left` reduction on `.marked` or accept a slight visual shift — match what feels natural in the existing palette).

**Step 2: Append the new rules**

Add a clearly delimited block at the bottom of `style.css`:

```css
/* mark and export */
.event-mark {
  margin: 0 0.5rem 0 0;
  vertical-align: middle;
}
.events li.marked {
  border-left: 3px solid var(--accent);
}
.events-head .export {
  display: flex;
  gap: 0.5rem;
  align-items: center;
}
.mark-count {
  color: var(--muted);
  font-size: 0.85rem;
}
.export-btn,
.clear-btn {
  /* match existing button styles — adjust to existing tokens */
}
.export-status {
  color: var(--muted);
  font-size: 0.85rem;
}
```

If `--accent` and `--muted` are not defined elsewhere, define them at `:root` (look near the top of `style.css`).

For `.export-btn` and `.clear-btn`, copy the rules from the existing button selector you identified in Task 1. Do not invent new visual styling — match what is already there.

**Step 3: Strip trailing whitespace**

```bash
perl -i -lpe 's/  *$//' internal/web/static/style.css
```

**Step 4: Smoke check**

Run the dev server again, load the page, confirm nothing visibly regressed (the `.export` div is still empty, so you should see the events header render the same as before). The new rules are inert until app.js starts emitting matching markup.

**Step 5: Commit**

```bash
git add internal/web/static/style.css
git commit -m "Add CSS for mark column and export slot

Checkbox styling, the marked-row left-border accent, the export
slot's flex layout, and the supporting muted-text classes all land
together. The selectors are inert until app.js begins emitting the
matching markup."
```

---

## Task 4: Add `marks` state, `markAnchor`, and a `clearMarks()` helper

**Files:**
- Modify: `internal/web/static/app.js`

**Step 1: Decide on placement**

Find the top-of-file state block (where the events array, current session, etc. live) identified in Task 1. The new state belongs there so it has the same scope as the events array.

**Step 2: Add the state**

```js
const marks = new Set();   // event IDs (numbers)
let markAnchor = null;     // last single-clicked event ID

function clearMarks() {
  marks.clear();
  markAnchor = null;
  renderExportSlot();
  // Re-render the events list so .marked classes drop and checkboxes uncheck.
  renderEvents();   // use whatever the existing list-render function is named
}
```

`renderExportSlot` and the actual render function name will be wired in subsequent tasks; you may leave a `// TODO` next to `renderExportSlot()` in this task and remove it once Task 7 lands. Or, equivalently, define a no-op stub `function renderExportSlot() {}` at file scope now and replace it in Task 7. Pick the stub approach — it keeps each commit independently runnable.

**Step 3: Wire `clearMarks()` into the session-change handler**

In the function that handles a session selection / switch, call `clearMarks()` at the top so the moment a new session loads, the previous marks are gone. Project change clears the session, so this transitively clears project-change too — confirm by tracing the project-change path.

**Step 4: Strip trailing whitespace**

```bash
perl -i -lpe 's/  *$//' internal/web/static/app.js
```

**Step 5: Smoke check**

Reload the page, switch sessions a couple of times. Open the browser devtools console, type `marks` and `markAnchor` — both should exist (`marks` is an empty Set, `markAnchor` is `null`). No visible UI change yet.

**Step 6: Commit**

```bash
git add internal/web/static/app.js
git commit -m "Add marks state and clearMarks helper

A Set holds the marked event IDs and a separate variable tracks the
shift-click anchor. clearMarks empties both and re-renders, and is
called when a session is selected so each session starts fresh."
```

---

## Task 5: Add the checkbox column to event row rendering

**Files:**
- Modify: `internal/web/static/app.js`

**Step 1: Locate the row builder**

In the function that builds an event `<li>` (identified in Task 1), prepend the checkbox to the row content. The design says:

```html
<input type="checkbox" class="event-mark" data-event-id="${id}">
```

The exact mechanism depends on whether the existing code uses `innerHTML` template strings or DOM-construction (`document.createElement`). Match the existing style — do not switch idioms.

If template-string style:

```js
const li = document.createElement("li");
li.className = "event";   // existing class, don't drop it
if (marks.has(ev.id)) {
  li.classList.add("marked");
}
li.innerHTML = `
  <input type="checkbox" class="event-mark" data-event-id="${ev.id}"${marks.has(ev.id) ? " checked" : ""}>
  ${/* existing row content */}
`;
```

If DOM-construction style, use `document.createElement("input")`, set `type`, `className`, `dataset.eventId`, and `checked`.

**Step 2: Wire the checkbox click handler**

After the `<li>` is built and inserted, attach the handler. Do this at the same point the existing row click handler is attached (so that if list rendering uses event delegation, you delegate too).

```js
const checkbox = li.querySelector(".event-mark");
checkbox.addEventListener("click", (e) => {
  e.stopPropagation();   // do not let the row-click selection fire
  onMarkClick(e, ev.id);
});
```

`onMarkClick` is defined in Task 6 — for now, define a stub at file scope:

```js
function onMarkClick(_e, _id) {
  /* implemented in Task 6 */
}
```

**Step 3: Add the `.marked` class toggling helper**

Add a small helper near the row builder that updates a single row's marked state without re-rendering the whole list (will be useful in Task 6 for shift-click range, where touching N rows individually is cheaper than re-rendering everyone):

```js
function setRowMarkedClass(id, isMarked) {
  const cb = document.querySelector(`.event-mark[data-event-id="${id}"]`);
  if (!cb) return;
  cb.checked = isMarked;
  const li = cb.closest("li");
  if (li) li.classList.toggle("marked", isMarked);
}
```

**Step 4: Strip trailing whitespace**

```bash
perl -i -lpe 's/  *$//' internal/web/static/app.js
```

**Step 5: Smoke check**

Reload, every event row now shows a checkbox. Clicking it does nothing user-visible yet (handler is a stub) but it must NOT trigger event selection in the detail pane (the `stopPropagation` should already work). Click the row body (not the checkbox) — detail selection must still work. Both behaviors confirmed before moving on.

**Step 6: Commit**

```bash
git add internal/web/static/app.js
git commit -m "Render mark checkbox on each event row

Each event li now starts with a checkbox bound to data-event-id. The
checkbox initial checked state and the row's marked class are driven
by the marks set. Click on the checkbox stops propagation so the
existing row-select handler is not affected."
```

---

## Task 6: Implement `onMarkClick` and `applyRange`

**Files:**
- Modify: `internal/web/static/app.js`

**Step 1: Replace the `onMarkClick` stub with the real implementation**

Per the design:

```js
function onMarkClick(ev, id) {
  // Determine the new state from the checkbox itself.
  const newState = ev.target.checked;

  if (ev.shiftKey && markAnchor !== null && markAnchor !== id) {
    const anchorIdx = events.findIndex(e => e.id === markAnchor);
    const targetIdx = events.findIndex(e => e.id === id);
    if (anchorIdx !== -1 && targetIdx !== -1) {
      applyRange(anchorIdx, targetIdx, newState);
      renderExportSlot();
      return;
    }
    // Anchor not in current view → fall through to plain-click behavior.
  }

  // Plain click: toggle this one and reset the anchor.
  if (newState) {
    marks.add(id);
  } else {
    marks.delete(id);
  }
  markAnchor = id;
  setRowMarkedClass(id, newState);
  renderExportSlot();
}
```

The variable name `events` is a placeholder — substitute the actual events-array variable identified in Task 1.

**Step 2: Add `applyRange`**

```js
function applyRange(aIdx, bIdx, state) {
  const lo = Math.min(aIdx, bIdx);
  const hi = Math.max(aIdx, bIdx);
  for (let i = lo; i <= hi; i++) {
    const id = events[i].id;
    if (state) {
      marks.add(id);
    } else {
      marks.delete(id);
    }
    setRowMarkedClass(id, state);
  }
  // Anchor unchanged — design specifies anchor survives shift-clicks.
}
```

Note: when shift-clicking, the browser flips the checkbox's `checked` state *before* firing the click event, which is why `ev.target.checked` is the correct "new state" — for the shift-clicked box itself it has already been toggled. The range then matches that state. Read the design's "Shift-click again on a checked row above the anchor unchecks the range" line — that flow depends on this exact semantic.

**Step 3: Strip trailing whitespace**

```bash
perl -i -lpe 's/  *$//' internal/web/static/app.js
```

**Step 4: Smoke check (manual QA)**

This is the meatiest mark-related smoke check. Run through every case:

- Plain-click an unchecked row → row gets `.marked`, checkbox is checked, anchor is now this row. Confirm via console: `markAnchor` is the id you just clicked, `marks.has(id)` is true.
- Plain-click again → row unchecks, leaves `marks`, anchor unchanged (still that id).
- Plain-click a different row → that one becomes anchor, first stays in whatever state it was.
- Shift-click below the anchor on an unchecked row → every row from anchor through clicked row checks. Anchor unchanged.
- Shift-click again on a row above the anchor that is currently checked → every row from that row through the anchor unchecks. Anchor unchanged.
- Shift-click with `markAnchor === null` (try by reloading and shift-clicking before any plain click) → behaves as plain click.
- Click the row body, not the checkbox → detail pane updates as before, marks state untouched.

If any of these fail, fix before committing.

**Step 5: Commit**

```bash
git add internal/web/static/app.js
git commit -m "Wire shift-click range marking with anchor

A plain click toggles the row and sets the anchor. A shift-click
sets every row between the anchor and the clicked row to the
clicked row's new state, leaving the anchor intact. If the anchor
is missing from the current view, shift-click degrades to a plain
click."
```

---

## Task 7: Render the export slot

**Files:**
- Modify: `internal/web/static/app.js`

**Step 1: Replace the `renderExportSlot` stub with the real implementation**

```js
function renderExportSlot() {
  const slot = document.querySelector(".events-head .export");
  if (!slot) return;
  if (marks.size === 0) {
    slot.innerHTML = "";
    return;
  }
  slot.innerHTML = `
    <span class="mark-count">${marks.size} marked</span>
    <button class="export-btn" type="button">export</button>
    <button class="clear-btn" type="button">clear</button>
  `;
  slot.querySelector(".export-btn").addEventListener("click", exportMarks);
  slot.querySelector(".clear-btn").addEventListener("click", clearMarks);
}
```

`exportMarks` is defined in Task 8 — for now define a stub at file scope:

```js
function exportMarks() {
  /* implemented in Task 8 */
}
```

**Step 2: Make sure the slot renders on first paint too**

Find the post-events-fetch render path. After the event list re-renders, the slot's `marks.size` text might be stale if a previously marked event scrolled out of the loaded list — the count should still be accurate (it tracks `marks`, not visible rows). Add a `renderExportSlot()` call at the end of the events-render function so size displays correctly even with hidden marked events.

**Step 3: Strip trailing whitespace**

```bash
perl -i -lpe 's/  *$//' internal/web/static/app.js
```

**Step 4: Smoke check**

- No marks → slot is empty.
- Mark one event → slot renders `1 marked`, `[export]`, `[clear]`.
- Mark a second → slot updates to `2 marked`.
- Click `[clear]` → slot empties, all `.marked` rows unchecked, anchor cleared (verify `markAnchor === null` in console).
- Apply a filter that hides marked events → slot still says `N marked` (count includes hidden marked events).

**Step 5: Commit**

```bash
git add internal/web/static/app.js
git commit -m "Render the export slot when marks exist

The slot stays empty when no events are marked and otherwise shows
the count, an export button, and a clear button. Clear empties the
mark set; export is wired to a stub that the next commit fills in."
```

---

## Task 8: Implement the export action and `flashStatus`

**Files:**
- Modify: `internal/web/static/app.js`

**Step 1: Replace the `exportMarks` stub with the real implementation**

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

function flashStatus(text) {
  const slot = document.querySelector(".events-head .export");
  if (!slot) return;
  // Drop any existing status flash before adding a new one.
  const existing = slot.querySelector(".export-status");
  if (existing) existing.remove();
  const span = document.createElement("span");
  span.className = "export-status";
  span.textContent = text;
  slot.appendChild(span);
  setTimeout(() => {
    if (span.parentNode === slot) span.remove();
  }, 3000);
}
```

Notes on details that matter:
- `URL.revokeObjectURL` runs *after* `a.click()` returns. Some browsers skip the download if the URL is revoked first.
- `a.remove()` between `click()` and `revokeObjectURL` is fine — the click has been dispatched synchronously.
- `Promise.allSettled` (not `Promise.all`) so a single failed fetch does not drop the whole download.
- The detail endpoint returns `{payload: <object>, raw: <string>}` per `internal/web/views.go`. Unparseable payloads come back with no `payload` field and a non-empty `raw` — those map to `null`. Parseable empty payloads (e.g. `{}`) come back with `payload: {}` and map to `{}`. The list endpoint omits payload entirely, which is why we must hit the per-event endpoint.

**Step 2: Strip trailing whitespace**

```bash
perl -i -lpe 's/  *$//' internal/web/static/app.js
```

**Step 3: Smoke check — golden path**

Mark two events in the same session, click `[export]`. Expect:

- A `lazyagent-events-YYYY-MM-DD-HHMM.json` file downloads.
- Open it: a top-level JSON array with two elements, sorted by event id ascending. Each element is the parsed payload (or `null` / `{}` per the rules).
- The slot flashes `exported 2 events` for ~3 seconds, then disappears.
- The marks remain (export does not clear them — clear is a separate action).

**Step 4: Smoke check — failure path**

Pick a marked event and have the server return an error for it (easiest: stop the server mid-flight, or hand-edit a known event id in the marks Set via console to a non-existent id like `999999`). Click `[export]`. Expect:

- File downloads with only the successful payloads.
- Slot flashes `exported N events (M failed)`.

**Step 5: Smoke check — interaction with auto-refresh**

Mark an event, wait through one auto-refresh tick. Marks must survive (no flicker, no re-rendering of the export slot from `0 marked`). Confirm in console: `marks.size` unchanged, `markAnchor` unchanged.

**Step 6: Commit**

```bash
git add internal/web/static/app.js
git commit -m "Implement client-side export of marked events

Export fetches each marked event from the existing detail endpoint
in parallel via Promise.allSettled, assembles a JSON array of the
parsed payloads, and triggers a browser download via a temporary
anchor and Blob URL. Failed fetches are dropped and counted in the
status flash. Status text is removed after three seconds."
```

---

## Task 9: Full-feature manual QA pass

**Files:** none (verification only)

**Step 1: Run through the smoke checklist from the design doc**

Reproduce each item from `docs/plans/2026-05-08-web-mark-export-design.md` "Testing" section, exactly. Each should pass:

- Single click toggles a row, sets anchor.
- Shift-click below the anchor checks the range.
- Shift-click again on a checked row above the anchor unchecks the range.
- Anchor survives across non-shift clicks (last single click wins).
- `[clear]` empties marks; slot collapses; rows lose `.marked`.
- `[export]` with two marks downloads a 2-element JSON array of parsed payloads; filename matches `lazyagent-events-…`.
- Switching sessions clears marks.
- Filter change keeps marks; export still includes hidden marked events.
- Auto-refresh tick keeps marks intact.
- 404 on a deleted event during export → status reports failed count; valid payloads still in the file.

**Step 2: Run the Go test suite**

```bash
go test ./...
```

Expected: all packages pass, no regressions. (We did not touch any Go code, but verifying the suite is green protects against accidental drift.)

**Step 3: If anything fails, open a fix in a new commit**

Do not amend prior task commits — add a follow-up. Track which task the fix belongs to in the commit body.

**Step 4: No commit unless a fix was needed.**

---

## Task 10: Update the README keybindings or feature list

**Files:**
- Modify: `README.md`

**Step 1: Find where the README documents the web UI (if it does)**

```bash
grep -n -i 'web\|browser\|mark\|export' README.md | head -30
```

**Step 2: Add a one-paragraph description of the web mark-and-export flow**

If the README has a section describing TUI keybindings (`m`, `M`, `x`, `U` from the prior feature), add a sibling section for the web UI:

> **Web UI:** Each event row has a checkbox in the leftmost column. Click to mark; shift-click to mark a contiguous range (Gmail-style: the new state matches the shift-clicked box). Once at least one event is marked, an `[export]` and `[clear]` button appear in the events header — `[export]` downloads a JSON array of the marked events' raw payloads, `[clear]` empties the marks. Switching sessions clears marks. Marks are in-memory only (page reload loses them).

If no web UI section exists, add a small one near the TUI section.

**Step 3: Strip trailing whitespace**

```bash
perl -i -lpe 's/  *$//' README.md
```

**Step 4: Commit**

```bash
git add README.md
git commit -m "Document web mark and export

The web UI now mirrors the TUI's mark-and-export feature: per-row
checkboxes with shift-click range selection, an export button that
downloads a JSON array of the marked events' raw payloads, and a
clear button. Marks are in-memory only and clear on session change."
```

---

## Wrap-up

At this point:

- `feat/web-mark-export` has nine implementation commits plus a documentation commit.
- The Go test suite is still green.
- The manual smoke checklist from the design doc passes end to end.
- No server changes shipped; the read-only contract on `internal/web/server.go` is intact.

Recommended final action before merging: re-read the diff of every changed file (`git diff feature-export-session...feat/web-mark-export -- internal/web/static`) and confirm there is no dead code, no leftover `console.log`, no stub function that survived past its task. Then open a PR against `feature-export-session` (or whichever branch is the integration target — confirm with the human operator).

## Out of scope (deferred)

Per the design doc, none of the below ships in this plan. Do not add them under any pretext:

- Persisting marks across page reloads.
- A server-side write endpoint.
- Keyboard shortcuts for mark / unmark / export (mouse-only for v1).
- Cross-session marking.
- Loading older events on demand for shift-click range.
- Caching payloads at mark time to skip the per-event fetch.

If any of these come up during execution as "wouldn't it be easier to also…", the answer is no — close that branch in your head and stay on the plan.
