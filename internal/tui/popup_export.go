package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
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
	return fmt.Sprintf("lazyagent-events-%s.json", now.Format("2006-01-02-1504"))
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
