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
