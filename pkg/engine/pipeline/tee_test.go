package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lynxbase/lynxdb/pkg/event"
)

func runTee(t *testing.T, rows []map[string]event.Value, format string) string {
	t.Helper()
	dest := filepath.Join(t.TempDir(), "tee.out")
	scan := NewRowScanIterator(rows, 1024)
	tee := NewTeeIterator(scan, dest, format)

	// CollectAll runs Init, drains, and Closes the iterator.
	if _, err := CollectAll(context.Background(), tee); err != nil {
		t.Fatalf("CollectAll: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read tee output: %v", err)
	}

	return string(data)
}

func TestTeeIterator_JSONDefault(t *testing.T) {
	out := runTee(t, []map[string]event.Value{
		{"level": event.StringValue("error"), "status": event.IntValue(500)},
	}, "")

	if !strings.Contains(out, `"level":"error"`) {
		t.Errorf("expected NDJSON output, got %q", out)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("expected trailing newline, got %q", out)
	}
}

func TestTeeIterator_CSV(t *testing.T) {
	out := runTee(t, []map[string]event.Value{
		{"level": event.StringValue("error"), "status": event.IntValue(500)},
		{"level": event.StringValue("warn"), "status": event.IntValue(404)},
	}, TeeFormatCSV)

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected header + 2 rows, got %d lines: %q", len(lines), out)
	}
	if lines[0] != "level,status" {
		t.Errorf("expected sorted header, got %q", lines[0])
	}
	if lines[1] != "error,500" {
		t.Errorf("expected first row, got %q", lines[1])
	}
	if lines[2] != "warn,404" {
		t.Errorf("expected second row, got %q", lines[2])
	}
}

func TestTeeIterator_Raw(t *testing.T) {
	out := runTee(t, []map[string]event.Value{
		{"_raw": event.StringValue("raw line one"), "level": event.StringValue("info")},
		{"level": event.StringValue("warn"), "status": event.IntValue(404)},
	}, TeeFormatRaw)

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), out)
	}
	if lines[0] != "raw line one" {
		t.Errorf("expected _raw value, got %q", lines[0])
	}
	if lines[1] != "level=warn\tstatus=404" {
		t.Errorf("expected key=value fallback, got %q", lines[1])
	}
}

func TestTeeIterator_UnknownFormat(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "tee.out")
	scan := NewRowScanIterator(nil, 1024)
	tee := NewTeeIterator(scan, dest, "xml")

	err := tee.Init(context.Background())
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
	if !strings.Contains(err.Error(), "unsupported format") {
		t.Errorf("expected unsupported-format error, got %v", err)
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Errorf("destination file should not be created on format error")
	}
}
