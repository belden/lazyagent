# Default the Projects pane to the current directory

## Problem

When lazyagent starts, the Projects pane is focused but nothing is
selected. The user must expand a project and pick a session manually
before the right-hand panes show anything useful. In practice the
correct project is almost always the one matching the shell's current
directory, and the correct session is whichever conversation is
currently running there.

## Goal

On startup, if the current working directory is inside a known git
project, auto-select that project. If that project has an active
session, auto-select the most recently active one as the current
conversation. Everything else stays as today.

## Scope

In scope:

- Detect the cwd's git root by walking up looking for `.git`.
- Match that root against `Project.Directory` (symlink-normalized).
- Expand the matched project, place the cursor on it.
- If the project has any session with `Status == "active"`, set the
  selected session to the one with the largest `LastActivity` and move
  the cursor to that session's row.

Out of scope:

- CLI flag or env-var overrides.
- Fallback to a non-git cwd or to "most recent project."
- Persisting a "last selected" preference across runs.
- Any UI affordance to indicate that auto-selection happened.

If the conditions are not met, the Projects pane behaves exactly as it
does today.

## Matching rules

1. At process start, compute `cwd := os.Getwd()`. Walk up the directory
   tree looking for a `.git` entry (file or directory, since worktrees
   use a `.git` file). The first hit is the git root. If none is
   found, skip auto-selection entirely.
2. Stash the root on the `Model` as `startupGitRoot` so it survives the
   async wait for project data. Normalize once via
   `filepath.EvalSymlinks`. On EvalSymlinks error, fall back to the raw
   path; never block startup on a symlink read.
3. On the first `projectsMsg` after start, scan `m.allProjects` for one
   whose `Directory` (also EvalSymlinks-normalized) equals
   `startupGitRoot`. If multiple match, take the first. Skip any
   project whose `Directory` is empty.
4. Auto-selection runs exactly once, gated by `defaultsApplied bool` on
   the Model. Periodic refreshes via `tickMsg` must not re-trigger it
   even if the user later clears their own selection.

## Sequencing

Session data is lazy-loaded per project, so the auto-selection spans
two message round-trips. The flow:

1. `newModel`: capture `startupGitRoot`. Initialize
   `defaultsApplied = false`.
2. `Init` returns `loadProjectsCmd()` as today.
3. On the first `projectsMsg`, after the existing `applyProjects` runs,
   call a new `applyStartupDefaults` helper:
   - If `defaultsApplied || startupGitRoot == ""`, return.
   - Find the matching project. If none, set
     `defaultsApplied = true` and return.
   - Set `m.projects.expandedProjs[projectID] = true`.
   - Rebuild items, then move the cursor to that project's row.
   - Set `defaultsApplied = true`.
   - Return `loadProjectSessionsCmd(projectID)` as an extra command,
     batched with the existing return from the `projectsMsg` branch.
4. On the resulting `projectSessionsMsg`, after the existing
   `applyProjectSessions` runs, call a new
   `maybeAutoSelectActiveSession` helper:
   - If `projectID` is not the startup-matched project, return.
   - Filter the project's sessions to those with `Status == "active"`,
     pick the one with the largest `LastActivity`.
   - If none, leave the cursor on the project row and return.
   - Otherwise set `m.projects.selectedSession = chosen.ID`, relocate
     the cursor to that session's row, and return
     `m.loadSelectedSessionDataCmd()` so events and agents load.

Splitting the work into two helpers keeps each one a pure function over
already-loaded state and avoids leaking startup-defaults logic into the
generic refresh paths.

## Edge cases

- Symlinks: normalize both sides with `filepath.EvalSymlinks` before
  comparing. On error, fall back to raw string equality.
- Worktrees: the walk-up detector accepts `.git` as either a file or a
  directory. This handles `git worktree add` checkouts without
  shelling out to the `git` binary.
- Empty `Project.Directory`: skip these entries in the match loop.
- Cursor relocation: `rebuildItems` can shift item indices. Find the
  target row by scanning the items slice for the matching `projectID`
  (or `sessionID` in step 4) at use-time. Do not cache pre-rebuild
  indices.

## Tests

New file `internal/tui/app_startup_defaults_test.go`, table-driven over
the Model's message sequence:

1. Git root matches a project with one active session: selected session
   set, cursor on that session's row, follow-up command to load events
   and agents fires.
2. Git root matches a project, no active sessions: project expanded,
   cursor on project row, `selectedSession` empty.
3. Git root matches a project, multiple active sessions: the one with
   the largest `LastActivity` wins.
4. Git root present but no matching project: no expansion, no
   selection.
5. No git root (startup outside any repo): no auto-selection.
6. Second `projectsMsg` (refresh tick) after the first run: defaults do
   not re-apply, even if the user has since cleared their selection.

Plus a small helper test for `findGitRoot(cwd)`: exact `.git` directory
hit, walked-up hit, `.git` file hit (worktree case), and reaching the
filesystem root with no match.
