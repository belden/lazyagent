package tui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chojs23/lazyagent/internal/model"
	"github.com/chojs23/lazyagent/internal/store"
)

func testKey(text string) tea.KeyMsg {
	return tea.KeyPressMsg(tea.Key{Text: text})
}

func testRoutingTUIStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "tui-test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func seedLazyLoadingStore(t *testing.T) *store.Store {
	t.Helper()
	st := testRoutingTUIStore(t)
	ctx := t.Context()
	if err := st.WithTx(ctx, func(q *store.Queries) error {
		projectID, err := q.CreateProject(ctx, "lazy-proj", "Lazy", "/tmp/lazy", "")
		if err != nil {
			return err
		}
		if err := q.UpsertSession(ctx, "sess-1", "", projectID, "session", "claude", nil, 1000, ""); err != nil {
			return err
		}
		if err := q.UpsertAgent(ctx, "agent-1", "sess-1", "", "Main", "", "main", ""); err != nil {
			return err
		}
		_, err = q.InsertEvent(ctx, model.Event{
			AgentID:   "agent-1",
			SessionID: "sess-1",
			Type:      "message",
			Subtype:   "Stop",
			Timestamp: 1000,
			Payload:   `{"last_assistant_message":"hello"}`,
		})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return st
}

func loadSeededProjects(t *testing.T) Model {
	t.Helper()
	m := newModel(seedLazyLoadingStore(t), time.Second)
	updated, cmd := m.Update(m.loadProjectsCmd()())
	if cmd != nil {
		t.Fatal("project load should not return command")
	}
	return updated.(Model)
}

func seedTwoProjectStore(t *testing.T) *store.Store {
	t.Helper()
	st := testRoutingTUIStore(t)
	ctx := t.Context()
	if err := st.WithTx(ctx, func(q *store.Queries) error {
		projectA, err := q.CreateProject(ctx, "alpha", "Alpha", "/tmp/alpha", "")
		if err != nil {
			return err
		}
		projectB, err := q.CreateProject(ctx, "beta", "Beta", "/tmp/beta", "")
		if err != nil {
			return err
		}
		if err := q.UpsertSession(ctx, "alpha-session", "", projectA, "alpha", "claude", nil, 1000, ""); err != nil {
			return err
		}
		return q.UpsertSession(ctx, "beta-session", "", projectB, "beta", "claude", nil, 1000, "")
	}); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestSetFocusUpdatesLayout(t *testing.T) {
	m := newModel(nil, time.Second)
	m.width = 100
	m.height = 30

	m.setFocus(focusDetail)

	if m.focus != focusDetail {
		t.Fatalf("focus = %v, want %v", m.focus, focusDetail)
	}
	if m.detail.viewport.Width() == 0 || m.detail.viewport.Height() == 0 {
		t.Fatalf("detail viewport not sized after focus change: %dx%d", m.detail.viewport.Width(), m.detail.viewport.Height())
	}
}

func TestActivateProjectSelectionResetsAgentsAndSyncsSession(t *testing.T) {
	m := newModel(testRoutingTUIStore(t), time.Second)
	m.allProjects = []model.Project{{ID: 7, Name: "proj", Directory: "/tmp/proj"}}
	m.allSessions = []model.Session{{ID: "sess-1", ProjectID: 7, ProjectName: "proj", Runtime: "claude"}}
	m.projects.selectedSession = "sess-1"
	m.agents.selectedAgent = "agent-1"
	m.agents.cursor = 3

	cmd := m.activateProjectSelection()

	if cmd == nil {
		t.Fatal("activateProjectSelection should return reload command")
	}
	if m.agents.selectedAgent != "" {
		t.Fatalf("selectedAgent = %q, want empty", m.agents.selectedAgent)
	}
	if m.agents.cursor != 0 {
		t.Fatalf("agent cursor = %d, want 0", m.agents.cursor)
	}
	if m.session.session == nil || m.session.session.ID != "sess-1" {
		t.Fatalf("session pane did not sync selected session: %#v", m.session.session)
	}
}

func TestSelectedAgentLabelPrefersStoredName(t *testing.T) {
	st := testRoutingTUIStore(t)
	m := newModel(st, time.Second)
	m.agents.selectedAgent = "agent-1"

	ctx := t.Context()
	if err := st.WithTx(ctx, func(q *store.Queries) error {
		projectID, err := q.CreateProject(ctx, "proj", "proj", "/tmp/proj", "")
		if err != nil {
			return err
		}
		if err := q.UpsertSession(ctx, "sess-1", "", projectID, "", "claude", nil, 1, ""); err != nil {
			return err
		}
		return q.UpsertAgent(ctx, "agent-1", "sess-1", "", "Planner", "", "", "")
	}); err != nil {
		t.Fatal(err)
	}

	if got := m.selectedAgentLabel(); got != "Planner" {
		t.Fatalf("selectedAgentLabel = %q, want Planner", got)
	}
}

func TestApplyAgentSelectionUpdatesFilterLabel(t *testing.T) {
	m := newModel(testRoutingTUIStore(t), time.Second)
	m.projects.selectedSession = "sess-1"
	m.agents.selectedAgent = "agent-abcdef123456"

	cmd := m.applyAgentSelection()

	if cmd == nil {
		t.Fatal("applyAgentSelection should return reload command")
	}
	if got := m.filter.agentLabel; got != shortID("agent-abcdef123456") {
		t.Fatalf("agent label = %q, want %q", got, shortID("agent-abcdef123456"))
	}
}

func TestApplyAgentSelectionWithoutSessionDoesNotLoadEvents(t *testing.T) {
	m := newModel(testRoutingTUIStore(t), time.Second)
	m.agents.selectedAgent = "agent-abcdef123456"

	cmd := m.applyAgentSelection()

	if cmd != nil {
		t.Fatal("applyAgentSelection without a selected session should not return a load command")
	}
	if got := m.filter.agentLabel; got != shortID("agent-abcdef123456") {
		t.Fatalf("agent label = %q, want %q", got, shortID("agent-abcdef123456"))
	}
}

func TestInitialLoadFetchesProjectsOnly(t *testing.T) {
	st := seedLazyLoadingStore(t)
	m := newModel(st, time.Second)

	msg := m.loadProjectsCmd()()
	updated, cmd := m.Update(msg)
	if cmd != nil {
		t.Fatal("project load should not schedule another command")
	}
	m = updated.(Model)

	if len(m.allProjects) != 1 {
		t.Fatalf("projects len = %d, want 1", len(m.allProjects))
	}
	if len(m.allSessions) != 0 {
		t.Fatalf("sessions len = %d, want 0", len(m.allSessions))
	}
	if m.projects.currentSessionID() != "" {
		t.Fatalf("selected session = %q, want empty", m.projects.currentSessionID())
	}
	if len(m.projects.expandedProjs) != 0 {
		t.Fatalf("expanded projects = %#v, want none", m.projects.expandedProjs)
	}
	if len(m.agents.agents) != 0 || len(m.events.events) != 0 {
		t.Fatalf("initial load should not load agents/events: agents=%d events=%d", len(m.agents.agents), len(m.events.events))
	}
}

func TestOpeningProjectLoadsSessionsWithoutSelectingSession(t *testing.T) {
	m := loadSeededProjects(t)

	updated, cmd := m.updateProjects(testKey("enter"))
	if cmd == nil {
		t.Fatal("opening a project should return a session load command")
	}
	m = updated.(Model)
	updated, cmd = m.Update(cmd())
	if cmd != nil {
		t.Fatal("session load should not schedule another command")
	}
	m = updated.(Model)

	if !m.projects.expandedProjs[m.allProjects[0].ID] {
		t.Fatal("project should be expanded")
	}
	if len(m.allSessions) != 1 {
		t.Fatalf("sessions len = %d, want 1", len(m.allSessions))
	}
	if m.projects.currentSessionID() != "" {
		t.Fatalf("selected session = %q, want empty", m.projects.currentSessionID())
	}
	if len(m.agents.agents) != 0 || len(m.events.events) != 0 {
		t.Fatalf("opening project should not load agents/events: agents=%d events=%d", len(m.agents.agents), len(m.events.events))
	}
}

func TestSelectingSessionLoadsAgentsAndEvents(t *testing.T) {
	m := loadSeededProjects(t)
	updated, cmd := m.updateProjects(testKey("enter"))
	m = updated.(Model)
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	m.projects.cursor = 1

	updated, cmd = m.updateProjects(testKey("enter"))
	if cmd == nil {
		t.Fatal("selecting a session should return a session data load command")
	}
	m = updated.(Model)
	updated, cmd = m.Update(cmd())
	if cmd != nil {
		t.Fatal("session data load should not schedule another command")
	}
	m = updated.(Model)

	if got := m.projects.currentSessionID(); got != "sess-1" {
		t.Fatalf("selected session = %q, want sess-1", got)
	}
	if m.session.session == nil || m.session.session.ID != "sess-1" {
		t.Fatalf("session pane = %#v, want sess-1", m.session.session)
	}
	if len(m.agents.agents) != 1 {
		t.Fatalf("agents len = %d, want 1", len(m.agents.agents))
	}
	if len(m.events.events) != 1 {
		t.Fatalf("events len = %d, want 1", len(m.events.events))
	}
	if m.detail.event == nil || m.detail.event.ID == 0 {
		t.Fatalf("detail event = %#v, want loaded event", m.detail.event)
	}
}

func TestOpeningProjectLoadsOnlyThatProjectsSessions(t *testing.T) {
	m := newModel(seedTwoProjectStore(t), time.Second)
	updated, cmd := m.Update(m.loadProjectsCmd()())
	if cmd != nil {
		t.Fatal("project load should not return command")
	}
	m = updated.(Model)

	updated, cmd = m.updateProjects(testKey("enter"))
	if cmd == nil {
		t.Fatal("opening the first project should return a session load command")
	}
	m = updated.(Model)
	updated, cmd = m.Update(cmd())
	if cmd != nil {
		t.Fatal("project session load should not return command")
	}
	m = updated.(Model)

	if len(m.allSessions) != 1 {
		t.Fatalf("sessions len = %d, want 1", len(m.allSessions))
	}
	if m.allSessions[0].ID != "alpha-session" {
		t.Fatalf("loaded session = %q, want alpha-session", m.allSessions[0].ID)
	}
	for _, item := range m.projects.items {
		if item.sessionID == "beta-session" {
			t.Fatalf("closed project session should not be loaded into sidebar: %#v", m.projects.items)
		}
	}
}

func TestRefreshingProjectSessionsClearsStaleSelectedSession(t *testing.T) {
	m := newModel(nil, time.Second)
	m.allProjects = []model.Project{{ID: 1, Name: "proj"}}
	m.allSessions = []model.Session{{ID: "stale-session", ProjectID: 1, Runtime: "claude"}}
	m.projects.setData(m.allProjects, m.allSessions)
	m.projects.selectedSession = "stale-session"
	m.agents.setAgents([]model.Agent{{ID: "agent-1", SessionID: "stale-session"}})
	m.events.setEvents([]model.Event{{ID: 1, AgentID: "agent-1", SessionID: "stale-session"}}, 1, 0)
	m.syncDetailFromEvent()

	m.applyProjectSessions(1, nil)

	if got := m.projects.currentSessionID(); got != "" {
		t.Fatalf("selected session = %q, want empty", got)
	}
	if len(m.agents.agents) != 0 || len(m.events.events) != 0 || m.detail.event != nil {
		t.Fatalf("stale selected data not cleared: agents=%d events=%d detail=%#v", len(m.agents.agents), len(m.events.events), m.detail.event)
	}
}

func TestSelectEventAtSyncsDetailAndDisablesAutoFollow(t *testing.T) {
	m := newModel(nil, time.Second)
	m.agents.setAgents([]model.Agent{{ID: "agent-1", Name: "main"}})
	m.events.setEvents([]model.Event{{ID: 10, AgentID: "agent-1"}, {ID: 20, AgentID: "agent-1"}}, 2, 0)
	m.events.autoFollow = true

	m.selectEventAt(0)

	if m.events.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", m.events.cursor)
	}
	if m.events.autoFollow {
		t.Fatal("autoFollow should be disabled after selecting an explicit event")
	}
	if m.detail.event == nil || m.detail.event.ID != 10 {
		t.Fatalf("detail event = %#v, want ID 10", m.detail.event)
	}
}

func TestSyncEventSelectionAndMaybeLoadOlderReturnsCommandAtThreshold(t *testing.T) {
	m := newModel(nil, time.Second)
	m.agents.setAgents([]model.Agent{{ID: "agent-1"}})
	m.events.events = makeEvents(eventsPageSize)
	m.events.loadedOffset = 10
	m.events.cursor = 0

	cmd := m.syncEventSelectionAndMaybeLoadOlder()

	if cmd == nil {
		t.Fatal("expected load older command when cursor is near top and older events exist")
	}
	if m.detail.event == nil || m.detail.event.ID != 0 {
		t.Fatalf("detail event = %#v, want selected top event", m.detail.event)
	}
}

func TestUpdateProjectsDoubleGoesTop(t *testing.T) {
	m := newModel(nil, time.Second)
	m.projects.setData(
		[]model.Project{{ID: 1, Name: "proj", SessionCount: 2}},
		[]model.Session{
			{ID: "session-1", ProjectID: 1, Runtime: "claude", StartedAt: 1},
			{ID: "session-2", ProjectID: 1, Runtime: "claude", StartedAt: 2},
		},
	)
	m.projects.enter()
	m.projects.cursor = 1

	updated, cmd := m.updateProjects(testKey("g"))
	if cmd != nil {
		t.Fatal("first g should not return command")
	}
	m = updated.(Model)
	if m.lastKey != "g" {
		t.Fatalf("lastKey = %q, want g", m.lastKey)
	}

	updated, cmd = m.updateProjects(testKey("g"))
	if cmd != nil {
		t.Fatal("second g in projects should not return command")
	}
	m = updated.(Model)
	if m.projects.cursor != 0 {
		t.Fatalf("projects cursor = %d, want 0", m.projects.cursor)
	}
	if m.lastKey != "" {
		t.Fatalf("lastKey after gg = %q, want empty", m.lastKey)
	}
}

func TestUpdateProjectsHorizontalScrollClamps(t *testing.T) {
	m := newModel(nil, time.Second)
	m.projects.hScroll = 1

	updated, _ := m.updateProjects(testKey("h"))
	m = updated.(Model)
	if m.projects.hScroll != 0 {
		t.Fatalf("projects hScroll after h = %d, want 0", m.projects.hScroll)
	}

	updated, _ = m.updateProjects(testKey("l"))
	m = updated.(Model)
	if m.projects.hScroll != 4 {
		t.Fatalf("projects hScroll after l = %d, want 4", m.projects.hScroll)
	}
}

func TestUpdateEventsDoubleGRequestsOlderAndClearsLastKey(t *testing.T) {
	m := newModel(nil, time.Second)
	m.agents.setAgents([]model.Agent{{ID: "agent-1"}})
	m.events.events = makeEvents(eventsPageSize)
	m.events.loadedOffset = 10
	m.events.cursor = 5

	updated, cmd := m.updateEvents(testKey("g"))
	if cmd != nil {
		t.Fatal("first g should not return command")
	}
	m = updated.(Model)
	if m.lastKey != "g" {
		t.Fatalf("lastKey = %q, want g", m.lastKey)
	}

	updated, cmd = m.updateEvents(testKey("g"))
	if cmd == nil {
		t.Fatal("second g in events should return sync/load command")
	}
	m = updated.(Model)
	if m.events.cursor != 0 {
		t.Fatalf("events cursor = %d, want 0", m.events.cursor)
	}
	if m.lastKey != "" {
		t.Fatalf("lastKey after gg = %q, want empty", m.lastKey)
	}
}

func TestEventsPageKeys_RouteFromAnyFocus(t *testing.T) {
	pgup := tea.KeyPressMsg(tea.Key{Code: tea.KeyPgUp})
	pgdown := tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown})
	altPgUp := tea.KeyPressMsg(tea.Key{Code: tea.KeyPgUp, Mod: tea.ModAlt})
	altPgDown := tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown, Mod: tea.ModAlt})

	focuses := []focusPane{focusProjects, focusSession, focusAgents, focusEvents, focusDetail}

	tests := []struct {
		name    string
		msg     tea.KeyMsg
		startAt int
		viewH   int
		want    int
	}{
		{name: "pgup half page up", msg: pgup, startAt: 50, viewH: 20, want: 40},
		{name: "pgdown half page down", msg: pgdown, startAt: 50, viewH: 20, want: 60},
		{name: "alt+pgup full page up", msg: altPgUp, startAt: 50, viewH: 20, want: 30},
		{name: "alt+pgdown full page down", msg: altPgDown, startAt: 50, viewH: 20, want: 70},
	}

	for _, tt := range tests {
		for _, f := range focuses {
			t.Run(tt.name+"/"+focusName(f), func(t *testing.T) {
				m := newModel(nil, time.Second)
				m.focus = f
				m.agents.setAgents([]model.Agent{{ID: "agent-1"}})
				m.events.setEvents(makeEvents(100), 100, 0)
				m.events.autoFollow = false
				m.events.cursor = tt.startAt
				m.events.height = tt.viewH

				updated, _ := m.handleKey(tt.msg)
				m = updated.(Model)

				if m.events.cursor != tt.want {
					t.Fatalf("cursor = %d, want %d", m.events.cursor, tt.want)
				}
			})
		}
	}
}

func focusName(f focusPane) string {
	switch f {
	case focusProjects:
		return "projects"
	case focusSession:
		return "session"
	case focusAgents:
		return "agents"
	case focusEvents:
		return "events"
	case focusDetail:
		return "detail"
	}
	return "unknown"
}

func TestUpdateEventsHorizontalScrollPreservesSyncPath(t *testing.T) {
	m := newModel(nil, time.Second)
	m.agents.setAgents([]model.Agent{{ID: "agent-1"}})
	m.events.events = makeEvents(eventsPageSize)
	m.events.loadedOffset = 10
	m.events.cursor = 0
	m.events.hScroll = 1

	updated, cmd := m.updateEvents(testKey("h"))
	if cmd == nil {
		t.Fatal("events h should still return sync/load command")
	}
	m = updated.(Model)
	if m.events.hScroll != 0 {
		t.Fatalf("events hScroll after h = %d, want 0", m.events.hScroll)
	}

	updated, cmd = m.updateEvents(testKey("l"))
	if cmd == nil {
		t.Fatal("events l should still return sync/load command")
	}
	m = updated.(Model)
	if m.events.hScroll != 4 {
		t.Fatalf("events hScroll after l = %d, want 4", m.events.hScroll)
	}
}

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

	updated, _ := m.Update(testKey("m"))
	m = updated.(Model)
	if !m.events.isMarked(1) {
		t.Fatalf("m should mark event with ID 1")
	}

	updated, _ = m.Update(testKey("M"))
	m = updated.(Model)
	if m.events.markedCount() != 3 {
		t.Fatalf("M should mark all 3 events, got %d", m.events.markedCount())
	}

	updated, _ = m.Update(testKey("U"))
	m = updated.(Model)
	if m.events.markedCount() != 0 {
		t.Fatalf("U should unmark all, got %d", m.events.markedCount())
	}
}

func TestExportWithNoMarks_ShowsStatusOnly(t *testing.T) {
	m := newModel(nil, time.Second)
	m.focus = focusEvents
	m.events.setEvents([]model.Event{
		{ID: 1, Subtype: "PreToolUse"},
	}, 1, 0)
	m.events.autoFollow = false

	if m.events.markedCount() != 0 {
		t.Fatalf("precondition: no marks")
	}

	updated, _ := m.Update(testKey("x"))
	m = updated.(Model)

	if m.status == "" {
		t.Fatalf("status should be set")
	}
	if !strings.Contains(m.status, "no events marked") {
		t.Fatalf("status: got %q, want contains %q", m.status, "no events marked")
	}
}

func TestExportPopup_XOpensWhenMarksExist(t *testing.T) {
	m := newModel(nil, time.Second)
	m.focus = focusEvents
	m.events.setEvents([]model.Event{
		{ID: 1, Payload: `{"a":1}`},
		{ID: 2, Payload: `{"b":2}`},
	}, 2, 0)
	m.events.autoFollow = false
	m.events.cursor = 0
	updated, _ := m.Update(testKey("m"))
	m = updated.(Model)

	updated, _ = m.Update(testKey("x"))
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
	updated, _ := m.Update(testKey("m"))
	m = updated.(Model)
	updated, _ = m.Update(testKey("x"))
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	m = updated.(Model)

	if m.export.active {
		t.Fatal("popup should close on esc")
	}
}

func TestUpdateEventsEnterStillFocusesDetail(t *testing.T) {
	m := newModel(nil, time.Second)
	m.focus = focusEvents

	updated, cmd := m.updateEvents(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd != nil {
		t.Fatal("events enter should not return command")
	}
	m = updated.(Model)
	if m.focus != focusDetail {
		t.Fatalf("focus = %v, want %v", m.focus, focusDetail)
	}
}
