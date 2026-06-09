package pipeline

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/lynxbase/lynxdb/pkg/event"
)

// Tee output formats.
const (
	TeeFormatJSON = "json" // NDJSON, one object per row (default)
	TeeFormatCSV  = "csv"  // RFC 4180, header from the first row's sorted keys
	TeeFormatRaw  = "raw"  // _raw field per line; key=value fallback
)

// ValidTeeFormat reports whether the given tee output format is supported.
func ValidTeeFormat(format string) bool {
	switch format {
	case TeeFormatJSON, TeeFormatCSV, TeeFormatRaw:
		return true
	}

	return false
}

// TeeIterator implements the tee pipeline operator — a side-effect passthrough
// that writes each batch to a destination file, then yields the batch unchanged.
type TeeIterator struct {
	child  Iterator
	dest   string
	format string
	writer *os.File
	enc    *json.Encoder
	csv    *csv.Writer
	header []string // CSV column order, fixed by the first row
}

func NewTeeIterator(child Iterator, dest string, format string) *TeeIterator {
	if format == "" {
		format = TeeFormatJSON
	}

	return &TeeIterator{child: child, dest: dest, format: format}
}

func (t *TeeIterator) Init(ctx context.Context) error {
	if !ValidTeeFormat(t.format) {
		return fmt.Errorf("tee: unsupported format %q (want json, csv, or raw)", t.format)
	}

	f, err := os.Create(t.dest)
	if err != nil {
		return fmt.Errorf("tee: cannot create %s: %w", t.dest, err)
	}

	t.writer = f
	switch t.format {
	case TeeFormatJSON:
		t.enc = json.NewEncoder(f)
	case TeeFormatCSV:
		t.csv = csv.NewWriter(f)
	}

	if err := t.child.Init(ctx); err != nil {
		t.writer = nil
		t.enc = nil
		t.csv = nil
		return errors.Join(
			fmt.Errorf("tee: init child: %w", err),
			wrapTeeCloseError(t.dest, f.Close()),
		)
	}

	return nil
}

func (t *TeeIterator) Next(ctx context.Context) (*Batch, error) {
	batch, err := t.child.Next(ctx)
	if batch != nil && t.writer != nil {
		for i := 0; i < batch.Len; i++ {
			if encErr := t.writeRow(batch.Row(i)); encErr != nil {
				return batch, fmt.Errorf("tee: write to %s: %w", t.dest, encErr)
			}
		}
	}

	return batch, err
}

func (t *TeeIterator) writeRow(row map[string]event.Value) error {
	switch t.format {
	case TeeFormatCSV:
		return t.writeCSVRow(row)
	case TeeFormatRaw:
		return t.writeRawRow(row)
	default:
		return t.enc.Encode(teeToMap(row))
	}
}

func (t *TeeIterator) writeCSVRow(row map[string]event.Value) error {
	if t.header == nil {
		t.header = make([]string, 0, len(row))
		for k := range row {
			t.header = append(t.header, k)
		}
		sort.Strings(t.header)
		if err := t.csv.Write(t.header); err != nil {
			return err
		}
	}

	record := make([]string, len(t.header))
	for i, col := range t.header {
		if v, ok := row[col]; ok {
			record[i] = teeValueString(v)
		}
	}

	return t.csv.Write(record)
}

// teeValueString renders a value for csv/raw output; nulls become empty.
func teeValueString(v event.Value) string {
	if v.IsNull() {
		return ""
	}

	return v.String()
}

func (t *TeeIterator) writeRawRow(row map[string]event.Value) error {
	if raw, ok := row["_raw"]; ok {
		if s := teeValueString(raw); s != "" {
			_, err := fmt.Fprintln(t.writer, s)
			return err
		}
	}

	// Fallback: sorted key=value pairs, tab-separated. _raw is omitted —
	// this path only runs when it is absent or empty.
	keys := make([]string, 0, len(row))
	for k := range row {
		if k == "_raw" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + teeValueString(row[k])
	}
	_, err := fmt.Fprintln(t.writer, strings.Join(parts, "\t"))

	return err
}

func (t *TeeIterator) Close() error {
	var closeErr error
	if t.writer != nil {
		if t.csv != nil {
			t.csv.Flush()
			closeErr = wrapTeeCloseError(t.dest, t.csv.Error())
		}
		closeErr = errors.Join(closeErr, wrapTeeCloseError(t.dest, t.writer.Close()))
	}

	childErr := t.child.Close()
	return errors.Join(closeErr, childErr)
}

func (t *TeeIterator) Schema() []FieldInfo { return t.child.Schema() }

func (t *TeeIterator) Child() Iterator { return t.child }

func wrapTeeCloseError(dest string, err error) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("tee: close %s: %w", dest, err)
}

// teeToMap converts a pipeline row (map[string]event.Value) to a JSON-friendly map.
func teeToMap(row map[string]event.Value) map[string]interface{} {
	out := make(map[string]interface{}, len(row))
	for k, v := range row {
		out[k] = v.Interface()
	}

	return out
}
