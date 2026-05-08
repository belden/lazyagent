package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
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
		t.Fatalf("default filename should end in .json, got %q", p.filename())
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

func TestExportPopup_EscCancels(t *testing.T) {
	p := newExportPopup()
	p.open()

	p.handleKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}), nil)
	if p.active {
		t.Fatal("popup should close on esc")
	}
}

func TestExportPopup_TabCyclesFocus(t *testing.T) {
	p := newExportPopup()
	p.open()
	p.setFilename("/tmp/lazyagent-test-no-such-file.json")

	if p.focusIdx != exportFocusInput {
		t.Fatalf("initial focus: got %d, want input", p.focusIdx)
	}
	p.handleKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}), nil)
	if p.focusIdx != exportFocusConfirm {
		t.Fatalf("after tab: got %d, want confirm", p.focusIdx)
	}
	p.handleKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}), nil)
	if p.focusIdx != exportFocusCancel {
		t.Fatalf("after tab tab: got %d, want cancel", p.focusIdx)
	}
	p.handleKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}), nil)
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

	p.handleKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}), nil)
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

	p.handleKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}), events)

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

	p.handleKey(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace}), nil)
	if !p.overwriteOK {
		t.Fatal("space should toggle overwriteOK on")
	}
	p.handleKey(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace}), nil)
	if p.overwriteOK {
		t.Fatal("space again should toggle overwriteOK off")
	}
}

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

func TestExportPopup_MouseClickFocusesElement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exists.json")
	os.WriteFile(path, []byte("[]"), 0644)

	p := newExportPopup()
	p.open()
	p.setFilename(path)

	p.handleClick(hitOverwrite, nil)
	if p.focusIdx != exportFocusOverwrite {
		t.Fatalf("after overwrite click: got %d, want overwrite",
			p.focusIdx)
	}
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

// TestUpdate_PopupActiveInterceptsClicks verifies that while the
// export popup is active, MouseClickMsg events are routed to the
// popup intercept rather than handleMouseClick on the underlying
// pane. We assert the events-pane cursor and focus do not change
// and that the popup remains active after an arbitrary click.
func TestUpdate_PopupActiveInterceptsClicks(t *testing.T) {
	m := newModel(nil, time.Second)
	m.width = 80
	m.height = 24
	m.focus = focusEvents
	m.events.setEvents([]model.Event{
		{ID: 1, Payload: `{"k":"a"}`},
		{ID: 2, Payload: `{"k":"b"}`},
		{ID: 3, Payload: `{"k":"c"}`},
	}, 3, 0)
	m.events.autoFollow = false
	m.events.cursor = 1

	// Mark an event so the popup can open with content.
	updated, _ := m.Update(testKey("m"))
	m = updated.(Model)

	// Open the export popup.
	updated, _ = m.Update(testKey("x"))
	m = updated.(Model)
	if !m.export.active {
		t.Fatal("popup should be active after pressing x")
	}

	preCursor := m.events.cursor
	preFocus := m.focus

	// Send an arbitrary click that, without the intercept, would
	// hit the events pane and possibly move the cursor or refocus.
	click := tea.MouseClickMsg(tea.Mouse{
		X: 0, Y: 0, Button: tea.MouseLeft,
	})
	updated, _ = m.Update(click)
	m = updated.(Model)

	if !m.export.active {
		t.Fatal("popup should remain active after click outside it")
	}
	if m.events.cursor != preCursor {
		t.Fatalf("events cursor changed: got %d, want %d",
			m.events.cursor, preCursor)
	}
	if m.focus != preFocus {
		t.Fatalf("focus changed: got %v, want %v",
			m.focus, preFocus)
	}
}

// TestUpdate_PopupActiveDropsWheel verifies that while the popup
// is active, MouseWheelMsg events are silently dropped instead of
// scrolling the pane underneath.
func TestUpdate_PopupActiveDropsWheel(t *testing.T) {
	m := newModel(nil, time.Second)
	m.width = 80
	m.height = 24
	m.focus = focusEvents
	m.events.setEvents([]model.Event{
		{ID: 1, Payload: `{"k":"a"}`},
		{ID: 2, Payload: `{"k":"b"}`},
		{ID: 3, Payload: `{"k":"c"}`},
	}, 3, 0)
	m.events.autoFollow = false
	m.events.cursor = 0

	updated, _ := m.Update(testKey("m"))
	m = updated.(Model)
	updated, _ = m.Update(testKey("x"))
	m = updated.(Model)
	if !m.export.active {
		t.Fatal("popup should be active")
	}

	preCursor := m.events.cursor
	wheel := tea.MouseWheelMsg(tea.Mouse{
		X: 0, Y: 0, Button: tea.MouseWheelDown,
	})
	updated, _ = m.Update(wheel)
	m = updated.(Model)

	if !m.export.active {
		t.Fatal("popup should remain active after wheel")
	}
	if m.events.cursor != preCursor {
		t.Fatalf("events cursor scrolled: got %d, want %d",
			m.events.cursor, preCursor)
	}
}

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

	updated, _ := m.Update(testKey("m"))
	m = updated.(Model)
	updated, _ = m.Update(testKey("j"))
	m = updated.(Model)
	updated, _ = m.Update(testKey("m"))
	m = updated.(Model)

	if m.events.markedCount() != 2 {
		t.Fatalf("markedCount: got %d, want 2",
			m.events.markedCount())
	}

	updated, _ = m.Update(testKey("x"))
	m = updated.(Model)
	if !m.export.active {
		t.Fatal("popup should be active")
	}

	m.export.setFilename(path)
	updated, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = updated.(Model)

	if m.export.active {
		t.Fatal("popup should close")
	}

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

	if !strings.Contains(m.status, "exported 2 events") {
		t.Fatalf("status: got %q", m.status)
	}
	if !strings.Contains(m.status, path) {
		t.Fatalf("status missing path: %q", m.status)
	}
}
