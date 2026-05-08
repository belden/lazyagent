package tui

import (
	"context"
	"fmt"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/chojs23/lazyagent/internal/applog"
	"github.com/chojs23/lazyagent/internal/model"
	"github.com/chojs23/lazyagent/internal/store"
)

type focusPane int

const (
	focusProjects focusPane = iota
	focusSession
	focusAgents
	focusEvents
	focusDetail
	paneCount = 5
)

type projectsMsg struct {
	projects []model.Project
	err      error
}

type projectSessionsMsg struct {
	projectID int64
	sessions  []model.Session
	err       error
}

type sessionDataMsg struct {
	sessionID string
	agents    []model.Agent
	events    []model.Event
	rawCount  int
	offset    int
	err       error
}

type maintenanceMsg struct {
	err error
}

type moreEventsMsg struct {
	events []model.Event
	offset int
	err    error
}

type tickMsg time.Time
type spinnerTickMsg time.Time

type Model struct {
	store           *store.Store
	refreshInterval time.Duration
	keys            keyMap
	help            help.Model

	projects projectsModel
	session  sessionInfoModel
	agents   agentsModel
	events   eventsModel
	detail   detailModel
	filter   filterModel

	focus      focusPane
	status     string
	statusHold int
	width      int
	height    int
	lastError error
	lastKey   string

	errorOverlay errorOverlay
	debug        *debugOverlay
	tokens       tokensOverlay
	export       exportPopup

	allProjects []model.Project
	allSessions []model.Session
}

func Run(st *store.Store, refreshInterval time.Duration) error {
	p := tea.NewProgram(newModel(st, refreshInterval))
	_, err := p.Run()
	return err
}

func newModel(st *store.Store, refreshInterval time.Duration) Model {
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
	}
	setGlobalDebug(m.debug)
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadProjectsCmd(), tickCmd(m.refreshInterval), spinnerTickCmd())
}

func (m *Model) syncLayout() {
	if m.width == 0 || m.height == 0 {
		return
	}

	sz := m.calcSizes()
	m.projects.height = sz.projH
	m.syncSessionPane()
	m.agents.height = sz.agentH
	m.events.height = sz.eventsH
	m.events.clampScroll()
	m.detail.viewport.SetWidth(max(sz.rightW-4, 10))
	m.detail.viewport.SetHeight(max(sz.detailH-3, 4))
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle non-key messages first so they are processed regardless of
	// whether the search input is active.
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.syncLayout()
		return m, nil

	case projectsMsg:
		if msg.err != nil {
			m.reportError("Refresh failed", msg.err)
			return m, nil
		}
		m.lastError = nil
		m.applyProjects(msg.projects)
		return m, nil

	case projectSessionsMsg:
		if msg.err != nil {
			m.reportError("Load sessions failed", msg.err)
			return m, nil
		}
		m.lastError = nil
		m.applyProjectSessions(msg.projectID, msg.sessions)
		return m, nil

	case sessionDataMsg:
		if msg.err != nil {
			m.reportError("Load session failed", msg.err)
			return m, nil
		}
		if msg.sessionID != m.projects.currentSessionID() {
			return m, nil
		}
		m.lastError = nil
		m.applySessionData(msg)
		return m, nil

	case maintenanceMsg:
		if msg.err != nil {
			m.reportError("Refresh failed", msg.err)
			return m, nil
		}
		return m, nil

	case moreEventsMsg:
		if msg.err != nil {
			m.reportError("Load older events failed", msg.err)
			return m, nil
		}
		if len(msg.events) > 0 {
			m.events.prependEvents(msg.events, msg.offset)
			m.syncDetailFromEvent()
		}
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.refreshVisibleDataCmd(), tickCmd(m.refreshInterval))

	case spinnerTickMsg:
		m.errorOverlay.update(time.Time(msg))
		m.agents.tick()
		m.projects.tick()
		return m, spinnerTickCmd()

	case tea.MouseClickMsg:
		if m.tokens.visible || m.debug.isVisible() || m.errorOverlay.visible {
			return m, nil
		}
		if m.export.active {
			// Fall through to the popup-active intercept below.
			break
		}
		return m.handleMouseClick(msg)

	case tea.MouseWheelMsg:
		if m.tokens.visible || m.debug.isVisible() || m.errorOverlay.visible || m.export.active {
			return m, nil
		}
		return m.handleMouseWheel(msg)
	}

	if m.export.active {
		if msg, ok := msg.(tea.KeyMsg); ok {
			events := m.events.markedSnapshot()
			cmd := m.export.handleKey(msg, events)
			if !m.export.active && m.export.confirmed {
				m.status = fmt.Sprintf(
					"exported %d events to %s",
					len(events),
					expandHome(m.export.filename()))
				m.statusHold = 3
				m.export.confirmed = false
			}
			return m, cmd
		}
		if click, ok := msg.(tea.MouseClickMsg); ok {
			hit := m.export.hitTest(click.X, click.Y, m.width, m.height)
			events := m.events.markedSnapshot()
			m.export.handleClick(hit, events)
			if m.export.confirmed {
				m.export.confirmed = false
				m.status = fmt.Sprintf(
					"exported %d events to %s",
					len(events),
					expandHome(m.export.filename()))
				m.statusHold = 3
			}
			return m, nil
		}
		return m, nil
	}

	// While the search input is focused, route remaining messages
	// (primarily key events) to the search handler.
	if m.filter.searchMode {
		return m.updateSearch(msg)
	}

	// When the tokens overlay is open, capture keys for its navigation.
	if m.tokens.visible {
		if msg, ok := msg.(tea.KeyMsg); ok {
			switch msg.String() {
			case "j", "down":
				m.tokens.scrollDown(1, m.width, m.height)
				return m, nil
			case "k", "up":
				m.tokens.scrollUp(1)
				return m, nil
			case "ctrl+d":
				m.tokens.halfPageDown(m.width, m.height)
				return m, nil
			case "ctrl+u":
				m.tokens.halfPageUp(m.width, m.height)
				return m, nil
			case "b", "esc", "q":
				m.tokens.close()
				return m, nil
			}
			return m, nil
		}
	}

	// When the debug overlay is open, capture keys for its navigation
	// but allow the toggle key to pass through to handleKey.
	if m.debug.isVisible() {
		if msg, ok := msg.(tea.KeyMsg); ok {
			switch msg.String() {
			case "j", "down":
				m.debug.scrollDown(1)
				return m, nil
			case "k", "up":
				m.debug.scrollUp(1)
				return m, nil
			case "G":
				m.debug.scrollToNewest()
				return m, nil
			case "g":
				if m.lastKey == "g" {
					m.debug.scrollToOldest()
					m.lastKey = ""
					return m, nil
				}
				m.lastKey = "g"
				return m, nil
			case "c":
				m.debug.clear()
				return m, nil
			case "`", "esc":
				m.debug.toggle()
				return m, nil
			}
			return m, nil
		}
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// forward to detail viewport
	if m.focus == focusDetail {
		var cmd tea.Cmd
		m.detail.viewport, cmd = m.detail.viewport.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.NextPane):
		m.setFocus((m.focus + 1) % paneCount)
		return m, nil
	case key.Matches(msg, m.keys.PrevPane):
		m.setFocus((m.focus + paneCount - 1) % paneCount)
		return m, nil
	case key.Matches(msg, m.keys.PaneProjects):
		m.setFocus(focusProjects)
		return m, nil
	case key.Matches(msg, m.keys.PaneSession):
		m.setFocus(focusSession)
		return m, nil
	case key.Matches(msg, m.keys.PaneAgents):
		m.setFocus(focusAgents)
		return m, nil
	case key.Matches(msg, m.keys.PaneEvents):
		m.setFocus(focusEvents)
		return m, nil
	case key.Matches(msg, m.keys.PaneDetail):
		m.setFocus(focusDetail)
		return m, nil
	case msg.Key().Code == tea.KeyEscape && m.filter.searchQuery != "" && m.focus != focusDetail:
		m.filter.clearSearch()
		m.status = "Search: off"
		return m, m.loadSelectedSessionDataCmd()
	case key.Matches(msg, m.keys.Search):
		m.filter.enterSearch()
		m.status = "Type search query, enter to apply, esc to cancel"
		m.syncLayout()
		return m, nil
	case key.Matches(msg, m.keys.CycleType):
		m.filter.cycleType()
		m.status = "Filter: " + m.filter.typeLabel()
		return m, m.loadSelectedSessionDataCmd()
	case key.Matches(msg, m.keys.CycleTypeRev):
		m.filter.cycleTypeReverse()
		m.status = "Filter: " + m.filter.typeLabel()
		return m, m.loadSelectedSessionDataCmd()
	case key.Matches(msg, m.keys.ToggleAuto):
		m.events.toggleAutoFollow()
		m.status = "Auto-follow: " + onOff(m.events.autoFollow)
		return m, nil
	case key.Matches(msg, m.keys.Refresh):
		m.status = "Refreshing..."
		return m, m.refreshVisibleDataCmd()
	case key.Matches(msg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll
		m.syncLayout()
		return m, nil
	case key.Matches(msg, m.keys.AgentAll):
		if m.focus == focusAgents {
			m.agents.selectedAgent = ""
			m.filter.setAgentLabel("all")
			m.status = "Agent filter: all"
			return m, m.loadSelectedSessionDataCmd()
		}
	case key.Matches(msg, m.keys.DebugLog):
		m.debug.toggle()
		return m, nil
	case key.Matches(msg, m.keys.TokenUsage):
		session := m.selectedSessionSummary()
		m.tokens.toggle(session, m.store)
		return m, nil
	case key.Matches(msg, m.keys.Delete):
		return m.handleDelete()
	case key.Matches(msg, m.keys.ClearEvt):
		return m.handleClearEvents()
	}

	// pane-specific navigation
	switch m.focus {
	case focusProjects:
		return m.updateProjects(msg)
	case focusSession:
		return m.updateSession(msg)
	case focusAgents:
		return m.updateAgents(msg)
	case focusEvents:
		return m.updateEvents(msg)
	case focusDetail:
		k := msg.String()
		switch k {
		case "esc":
			m.setFocus(focusEvents)
			m.lastKey = k
			return m, nil
		case "J":
			m.detail.toggleJSON()
			m.lastKey = k
			return m, nil
		case "e":
			m.detail.toggleExpand()
			m.lastKey = k
			return m, nil
		case "G":
			m.detail.viewport.GotoBottom()
			m.lastKey = k
			return m, nil
		case "g":
			if m.lastKey == "g" {
				m.detail.viewport.GotoTop()
				m.lastKey = ""
				return m, nil
			}
			m.lastKey = "g"
			return m, nil
		}
		m.lastKey = k
		var cmd tea.Cmd
		m.detail.viewport, cmd = m.detail.viewport.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) updateProjects(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if result := m.handleCommonVerticalNav(k,
		m.projects.moveDown,
		m.projects.moveUp,
		func() { m.projects.halfPageDown(m.projects.height) },
		func() { m.projects.halfPageUp(m.projects.height) },
		m.projects.goBottom,
		m.projects.goTop,
	); result != navUnhandled {
		if result == navAwaitMore {
			return m, nil
		}
		return m, nil
	}
	if m.handleHorizontalNav(k, &m.projects.hScroll) {
		return m, nil
	}
	switch k {
	case "enter", "space":
		item := m.projects.currentItem()
		if m.projects.enter() {
			m.lastKey = k
			return m, m.activateProjectSelection()
		}
		if item != nil && item.kind == "project" && m.projects.expandedProjs[item.projectID] {
			m.lastKey = k
			return m, m.loadProjectSessionsCmd(item.projectID)
		}
	}
	m.lastKey = k
	return m, nil
}

func (m Model) updateSession(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if result := m.handleCommonVerticalNav(k,
		m.session.moveDown,
		m.session.moveUp,
		func() { m.session.halfPageDown(m.session.height) },
		func() { m.session.halfPageUp(m.session.height) },
		m.session.goBottom,
		m.session.goTop,
	); result != navUnhandled {
		if result == navAwaitMore {
			return m, nil
		}
		return m, nil
	}
	if m.handleHorizontalNav(k, &m.session.hScroll) {
		return m, nil
	}
	m.lastKey = k
	return m, nil
}

func (m Model) updateAgents(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if result := m.handleCommonVerticalNav(k,
		m.agents.moveDown,
		m.agents.moveUp,
		func() { m.agents.halfPageDown(m.agents.height) },
		func() { m.agents.halfPageUp(m.agents.height) },
		m.agents.goBottom,
		m.agents.goTop,
	); result != navUnhandled {
		if result == navAwaitMore {
			return m, nil
		}
		return m, nil
	}
	if m.handleHorizontalNav(k, &m.agents.hScroll) {
		return m, nil
	}
	switch k {
	case "enter", "space":
		m.agents.enter()
		m.lastKey = k
		return m, m.applyAgentSelection()
	}
	m.lastKey = k
	return m, nil
}

func (m Model) updateEvents(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if result := m.handleCommonVerticalNav(k,
		m.events.moveDown,
		m.events.moveUp,
		func() { m.events.halfPageDown(m.events.height) },
		func() { m.events.halfPageUp(m.events.height) },
		m.events.goBottom,
		m.events.goTop,
	); result != navUnhandled {
		if result == navAwaitMore {
			return m, nil
		}
		return m, m.syncEventSelectionAndMaybeLoadOlder()
	}
	if m.handleHorizontalNav(k, &m.events.hScroll) {
		return m, m.syncEventSelectionAndMaybeLoadOlder()
	}
	switch k {
	case "z":
		if m.lastKey == "z" {
			m.events.centerCursor()
			m.lastKey = ""
			return m, nil
		}
		m.lastKey = "z"
		return m, nil
	case "enter":
		m.setFocus(focusDetail)
		m.lastKey = k
		return m, nil
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
	case "x":
		if m.events.markedCount() == 0 {
			m.status = "no events marked — press m to mark events"
			m.lastKey = k
			return m, nil
		}
		m.export.open()
		m.lastKey = k
		return m, nil
	}
	m.lastKey = k
	return m, m.syncEventSelectionAndMaybeLoadOlder()
}

type navResult int

const (
	navUnhandled navResult = iota
	navHandled
	navAwaitMore
)

func (m *Model) handleCommonVerticalNav(k string, moveDown, moveUp, halfDown, halfUp, goBottom, goTop func()) navResult {
	switch k {
	case "j", "down":
		moveDown()
	case "k", "up":
		moveUp()
	case "ctrl+d":
		halfDown()
	case "ctrl+u":
		halfUp()
	case "G":
		goBottom()
	case "g":
		if m.lastKey == "g" {
			goTop()
			m.lastKey = ""
			return navHandled
		}
		m.lastKey = "g"
		return navAwaitMore
	default:
		return navUnhandled
	}
	m.lastKey = k
	return navHandled
}

func (m *Model) handleHorizontalNav(k string, hScroll *int) bool {
	switch k {
	case "l", "right":
		*hScroll += 4
	case "h", "left":
		*hScroll = max(*hScroll-4, 0)
	default:
		return false
	}
	m.lastKey = k
	return true
}

func (m Model) updateSearch(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Key().Code {
		case tea.KeyEnter:
			m.filter.commitSearch()
			m.status = "Search: " + orDefault(m.filter.searchQuery, "off")
			m.syncLayout()
			return m, m.loadSelectedSessionDataCmd()
		case tea.KeyEscape:
			m.filter.cancelSearch()
			m.status = "Search cancelled"
			m.syncLayout()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.filter.searchInput, cmd = m.filter.searchInput.Update(msg)
	return m, cmd
}

func (m *Model) syncDetailFromEvent() {
	ev := m.events.selectedEvent()
	m.detail.setEvent(ev, m.agents.agents)
}

func (m *Model) setFocus(pane focusPane) {
	m.focus = pane
	m.syncLayout()
}

func (m *Model) activateProjectSelection() tea.Cmd {
	m.agents.selectedAgent = ""
	m.agents.cursor = 0
	m.agents.setAgents(nil)
	m.events.setEvents(nil, 0, 0)
	m.syncSessionPane()
	m.syncDetailFromEvent()
	return m.loadSelectedSessionDataCmd()
}

func (m *Model) selectedAgentLabel() string {
	agentLabel := "all"
	id := m.agents.selectedAgentID()
	if id == "" {
		return agentLabel
	}
	agentLabel = shortID(id)
	if m.store == nil {
		return agentLabel
	}
	if a, _ := m.store.Read().GetAgentByID(context.Background(), id); a != nil && a.Name != "" {
		return a.Name
	}
	return agentLabel
}

func (m *Model) applyAgentSelection() tea.Cmd {
	m.filter.setAgentLabel(m.selectedAgentLabel())
	return m.loadSelectedSessionDataCmd()
}

func (m *Model) selectEventAt(index int) {
	if index < 0 || index >= len(m.events.events) {
		return
	}
	m.events.cursor = index
	m.events.autoFollow = false
	m.events.clampScroll()
	m.syncDetailFromEvent()
}

func (m *Model) syncEventSelectionAndMaybeLoadOlder() tea.Cmd {
	m.syncDetailFromEvent()
	if m.events.needsOlder() {
		return m.loadOlderEventsCmd()
	}
	return nil
}

func (m *Model) applyProjects(projects []model.Project) {
	m.allProjects = projects
	m.projects.setData(m.allProjects, m.allSessions)
	m.syncSessionPane()
	if m.statusHold == 0 {
		m.status = fmt.Sprintf("P:%d S:%d E:%d/%d A:%d",
			len(m.allProjects), len(m.allSessions), len(m.events.events), m.events.rawCount, len(m.agents.agents))
	}
}

func (m *Model) applyProjectSessions(projectID int64, sessions []model.Session) {
	m.allSessions = replaceProjectSessions(m.allSessions, projectID, sessions)
	m.projects.setData(m.allProjects, m.allSessions)
	if selected := m.projects.currentSessionID(); selected != "" && !sessionExists(m.allSessions, selected) {
		m.projects.selectedSession = ""
		m.agents.setAgents(nil)
		m.events.setEvents(nil, 0, 0)
		m.syncDetailFromEvent()
	}
	m.syncSessionPane()
	if m.statusHold == 0 {
		m.status = fmt.Sprintf("P:%d S:%d E:%d/%d A:%d",
			len(m.allProjects), len(m.allSessions), len(m.events.events), m.events.rawCount, len(m.agents.agents))
	}
}

func (m *Model) applySessionData(d sessionDataMsg) {
	m.agents.setAgents(d.agents)
	m.events.setEvents(d.events, d.rawCount, d.offset)
	m.syncDetailFromEvent()
	if m.statusHold > 0 {
		m.statusHold--
	} else {
		m.status = fmt.Sprintf("P:%d S:%d E:%d/%d A:%d",
			len(m.allProjects), len(m.allSessions), len(d.events), d.rawCount, len(d.agents))
	}
}

func replaceProjectSessions(existing []model.Session, projectID int64, sessions []model.Session) []model.Session {
	out := make([]model.Session, 0, len(existing)+len(sessions))
	for _, session := range existing {
		if session.ProjectID != projectID {
			out = append(out, session)
		}
	}
	out = append(out, sessions...)
	return out
}

func sessionExists(sessions []model.Session, sessionID string) bool {
	for _, session := range sessions {
		if session.ID == sessionID {
			return true
		}
	}
	return false
}

func (m *Model) reportError(context string, err error) {
	if err == nil {
		return
	}
	applog.Error(context, err)
	m.debug.add("%s: %s", context, err.Error())
	m.lastError = err
	m.status = context + ": " + err.Error()
	m.errorOverlay.show(context, err.Error())
}

func (m *Model) syncSessionPane() {
	session := m.selectedSessionSummary()
	project := m.selectedProjectSummary(session)
	m.session.setSession(session, project)
}

func (m Model) selectedSessionSummary() *model.Session {
	sessionID := m.projects.currentSessionID()
	if sessionID == "" {
		return nil
	}
	for i := range m.allSessions {
		if m.allSessions[i].ID == sessionID {
			return &m.allSessions[i]
		}
	}
	return nil
}

func (m Model) selectedProjectSummary(session *model.Session) *model.Project {
	if session == nil {
		return nil
	}
	for i := range m.allProjects {
		if m.allProjects[i].ID == session.ProjectID {
			return &m.allProjects[i]
		}
	}
	return nil
}

func (m Model) handleDelete() (tea.Model, tea.Cmd) {
	if m.focus != focusProjects {
		return m, nil
	}
	item := m.projects.currentItem()
	if item == nil {
		return m, nil
	}
	ctx := context.Background()
	switch item.kind {
	case "session":
		if err := m.store.WithTx(ctx, func(q *store.Queries) error {
			return q.DeleteSession(ctx, item.sessionID)
		}); err != nil {
			m.reportError("Delete failed", err)
			return m, nil
		}
		m.projects.selectedSession = ""
		m.agents.setAgents(nil)
		m.events.setEvents(nil, 0, 0)
		m.syncSessionPane()
		m.syncDetailFromEvent()
		m.status = "Session deleted"
	case "project":
		if err := m.store.WithTx(ctx, func(q *store.Queries) error {
			return q.DeleteProject(ctx, item.projectID)
		}); err != nil {
			m.reportError("Delete failed", err)
			return m, nil
		}
		m.projects.selectedSession = ""
		m.agents.setAgents(nil)
		m.events.setEvents(nil, 0, 0)
		m.syncSessionPane()
		m.syncDetailFromEvent()
		m.status = "Project deleted"
	}
	return m, m.refreshVisibleDataCmd()
}

func (m Model) handleClearEvents() (tea.Model, tea.Cmd) {
	sid := m.projects.currentSessionID()
	if sid == "" {
		return m, nil
	}
	ctx := context.Background()
	if err := m.store.WithTx(ctx, func(q *store.Queries) error {
		return q.ClearSessionEvents(ctx, sid)
	}); err != nil {
		m.reportError("Clear failed", err)
		return m, nil
	}
	m.status = "Events cleared"
	return m, m.loadSelectedSessionDataCmd()
}

type paneSizes struct {
	sidebarW int
	rightW   int
	projH    int
	sessionH int
	agentH   int
	eventsH  int
	detailH  int
}

func (m Model) footerViews() (string, string) {
	filterBar := m.filter.view(m.width)
	statusContent := m.status
	if helpLine := m.help.View(m.keys); helpLine != "" {
		statusContent += " " + helpLine
	}
	statusLine := statusBarStyle.Width(m.width).Render(statusContent)
	return filterBar, statusLine
}

func (m Model) footerHeight() int {
	if m.width <= 0 {
		return 0
	}
	filterBar, statusLine := m.footerViews()
	return lipgloss.Height(filterBar) + lipgloss.Height(statusLine)
}

func (m Model) calcSizes() paneSizes {
	sidebarW := max(m.width/4, 24)
	rightW := m.width - sidebarW
	mainH := max(m.height-m.footerHeight(), 3)
	leftH := mainH
	rightH := mainH

	// left: projects vs session vs agents
	var projH, sessionH, agentH int
	switch m.focus {
	case focusProjects:
		projH, sessionH, agentH = splitThreeHeights(leftH, [3]int{5, 2, 3}, [3]int{6, 4, 4})
	case focusSession:
		projH, sessionH, agentH = splitThreeHeights(leftH, [3]int{3, 4, 3}, [3]int{6, 4, 4})
	case focusAgents:
		projH, sessionH, agentH = splitThreeHeights(leftH, [3]int{3, 2, 5}, [3]int{6, 4, 4})
	default:
		projH, sessionH, agentH = splitThreeHeights(leftH, [3]int{4, 2, 3}, [3]int{6, 4, 4})
	}

	// right: events vs detail — 7:3 ratio based on focus
	var eventsH, detailH int
	switch m.focus {
	case focusEvents:
		eventsH = max(rightH*70/100, 6)
		detailH = max(rightH-eventsH, 4)
	case focusDetail:
		detailH = max(rightH*70/100, 6)
		eventsH = max(rightH-detailH, 4)
	default:
		eventsH = max(rightH*55/100, 6)
		detailH = max(rightH-eventsH, 4)
	}

	return paneSizes{sidebarW, rightW, projH, sessionH, agentH, eventsH, detailH}
}

func splitThreeHeights(total int, weights [3]int, mins [3]int) (int, int, int) {
	base := mins[0] + mins[1] + mins[2]
	if total <= base {
		first := max(total*weights[0]/(weights[0]+weights[1]+weights[2]), 1)
		second := max((total-first)*weights[1]/(weights[1]+weights[2]), 1)
		third := max(total-first-second, 1)
		return first, second, third
	}

	remaining := total - base
	weightSum := weights[0] + weights[1] + weights[2]
	first := mins[0] + remaining*weights[0]/weightSum
	second := mins[1] + remaining*weights[1]/weightSum
	third := total - first - second
	return first, second, third
}

func (m Model) View() tea.View {
	if m.width == 0 || m.height == 0 {
		v := tea.NewView("Loading...")
		v.AltScreen = true
		return v
	}

	sz := m.calcSizes()

	projView := m.projects.view(sz.sidebarW, sz.projH, m.focus == focusProjects)
	sessionView := m.session.view(sz.sidebarW, sz.sessionH, m.focus == focusSession)
	agentView := m.agents.view(sz.sidebarW, sz.agentH, m.focus == focusAgents)

	agentMap := buildAgentMap(m.agents.agents)
	eventsView := m.events.view(sz.rightW, sz.eventsH, m.focus == focusEvents, agentMap)
	detailView := m.detail.view(sz.rightW, sz.detailH, m.focus == focusDetail)

	left := lipgloss.JoinVertical(lipgloss.Left, projView, sessionView, agentView)
	right := lipgloss.JoinVertical(lipgloss.Left, eventsView, detailView)
	main := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	filterBar, statusLine := m.footerViews()

	full := lipgloss.JoinVertical(lipgloss.Left, main, filterBar, statusLine)
	if m.debug.isVisible() {
		full = renderOverlay(full, m.width, m.height, m.debug.view(m.width, m.height))
	}
	if m.errorOverlay.visible {
		full = renderOverlay(full, m.width, m.height, m.errorOverlay.view(m.width))
	}
	if m.tokens.visible {
		full = renderOverlayCentered(full, m.width, m.height, m.tokens.viewFullScreen(m.width, m.height))
	}
	if m.export.active {
		full = renderOverlayCentered(full, m.width, m.height, m.export.view(m.width))
	}
	if lipgloss.Height(full) > m.height {
		full = lipgloss.NewStyle().MaxHeight(m.height).Render(full)
	}

	v := tea.NewView(full)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m Model) loadProjectsCmd() tea.Cmd {
	st := m.store
	if st == nil {
		return nil
	}
	return func() tea.Msg {
		ctx := context.Background()
		projects, err := st.Read().ListProjects(ctx)
		if err != nil {
			return projectsMsg{err: err}
		}
		return projectsMsg{projects: projects}
	}
}

func (m Model) loadProjectSessionsCmd(projectID int64) tea.Cmd {
	st := m.store
	if st == nil || projectID == 0 {
		return nil
	}
	return func() tea.Msg {
		ctx := context.Background()
		sessions, err := st.Read().ListSessionsForProject(ctx, projectID)
		if err != nil {
			return projectSessionsMsg{projectID: projectID, err: err}
		}
		return projectSessionsMsg{projectID: projectID, sessions: sessions}
	}
}

func (m Model) loadSelectedSessionDataCmd() tea.Cmd {
	sessionID := m.projects.currentSessionID()
	if sessionID == "" {
		return nil
	}
	return m.loadSessionDataCmd(sessionID)
}

func (m Model) loadSessionDataCmd(sessionID string) tea.Cmd {
	st := m.store
	if st == nil || sessionID == "" {
		return nil
	}
	baseFilter := m.currentEventFilter()
	// Preserve the number of already-loaded events on refresh so pagination
	// progress is not lost when the periodic tick reloads data.
	loadedCount := len(m.events.events)

	return func() tea.Msg {
		ctx := context.Background()
		q := st.Read()

		agents, err := q.ListAgentsForSessionTree(ctx, sessionID)
		if err != nil {
			return sessionDataMsg{sessionID: sessionID, err: err}
		}

		rawCount, err := q.CountEventsForSessionTree(ctx, sessionID)
		if err != nil {
			return sessionDataMsg{sessionID: sessionID, err: err}
		}

		// When filters are active the SQL OFFSET must be relative to the
		// filtered result set, not the total event count. Use a filtered
		// count so pagination and needsOlder() work correctly.
		filteredCount := rawCount
		if eventFilterActive(baseFilter) {
			filteredCount, err = q.CountFilteredEventsForSessionTree(ctx, sessionID, baseFilter)
			if err != nil {
				return sessionDataMsg{sessionID: sessionID, err: err}
			}
		}

		// Load from the end so the user sees the latest events first.
		// On refresh, preserve the number of already-loaded events.
		pageLimit, offset := currentRefreshEventWindow(filteredCount, loadedCount)
		filter := baseFilter
		filter.Limit = pageLimit
		filter.Offset = offset
		events, err := q.ListEventsForSessionTree(ctx, sessionID, filter)
		if err != nil {
			return sessionDataMsg{sessionID: sessionID, err: err}
		}

		return sessionDataMsg{
			sessionID: sessionID,
			agents:    agents,
			events:    events,
			rawCount:  rawCount,
			offset:    offset,
		}
	}
}

func (m Model) refreshVisibleDataCmd() tea.Cmd {
	cmds := []tea.Cmd{m.runMaintenanceCmd(), m.loadProjectsCmd()}
	for projectID := range m.projects.expandedProjs {
		if m.projects.expandedProjs[projectID] {
			cmds = append(cmds, m.loadProjectSessionsCmd(projectID))
		}
	}
	if cmd := m.loadSelectedSessionDataCmd(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...)
}

func (m Model) runMaintenanceCmd() tea.Cmd {
	st := m.store
	if st == nil {
		return nil
	}
	return func() tea.Msg {
		ctx := context.Background()
		// Auto-stop sessions that have been idle for over 5 minutes
		// with no active child sessions. Handles ungraceful shutdowns.
		if _, err := st.ReapStaleSessions(ctx, 5*60*1000); err != nil {
			return maintenanceMsg{err: err}
		}
		return maintenanceMsg{}
	}
}

func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) loadOlderEventsCmd() tea.Cmd {
	st := m.store
	sessionID := m.projects.currentSessionID()
	baseFilter := m.currentEventFilter()
	currentOffset := m.events.loadedOffset

	return func() tea.Msg {
		if sessionID == "" || currentOffset <= 0 {
			return moreEventsMsg{}
		}
		ctx := context.Background()
		q := st.Read()

		limit, newOffset := currentOlderEventsWindow(currentOffset)
		filter := baseFilter
		filter.Limit = limit
		filter.Offset = newOffset
		events, err := q.ListEventsForSessionTree(ctx, sessionID, filter)
		if err != nil {
			return moreEventsMsg{err: err}
		}
		return moreEventsMsg{events: events, offset: newOffset}
	}
}

func (m Model) currentEventFilter() model.EventFilter {
	filter := model.EventFilter{
		Type:   m.filter.typeValue(),
		Search: m.filter.searchQuery,
	}
	if agentID := m.agents.selectedAgentID(); agentID != "" {
		filter.AgentIDs = []string{agentID}
	}
	return filter
}

func eventFilterActive(filter model.EventFilter) bool {
	return len(filter.AgentIDs) > 0 || filter.Type != "" || filter.Search != ""
}

func currentRefreshEventWindow(filteredCount, loadedCount int) (limit, offset int) {
	limit = max(eventsPageSize, loadedCount)
	offset = max(0, filteredCount-limit)
	return limit, offset
}

func currentOlderEventsWindow(currentOffset int) (limit, offset int) {
	offset = max(0, currentOffset-eventsPageSize)
	limit = currentOffset - offset
	return limit, offset
}

func spinnerTickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return spinnerTickMsg(t)
	})
}

type agentInfo struct {
	index int
	name  string
}

func buildAgentMap(agents []model.Agent) map[string]agentInfo {
	m := make(map[string]agentInfo, len(agents))
	for i, a := range agents {
		name := shortID(a.ID)
		if a.Name != "" {
			name = a.Name
		}
		m[a.ID] = agentInfo{index: i, name: name}
	}
	return m
}
