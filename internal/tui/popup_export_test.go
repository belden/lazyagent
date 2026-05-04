package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
