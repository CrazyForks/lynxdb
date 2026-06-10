package sigmaqueries

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lynxbase/lynxdb/pkg/event"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/parser"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/run"
	"github.com/lynxbase/lynxdb/test/integration/sigmacompat"
)

// walkLynxFlowFixtures discovers *.lynxflow golden files and calls fn for each
// non-blank, non-comment line.
func walkLynxFlowFixtures(t *testing.T, fn func(t *testing.T, fixture, line string, lineNo int)) {
	t.Helper()

	fixtures, err := filepath.Glob(filepath.Join("testdata", "golden", "*.lynxflow"))
	if err != nil {
		t.Fatalf("glob lynxflow fixtures: %v", err)
	}
	if len(fixtures) == 0 {
		t.Fatal("no golden LynxFlow fixtures discovered")
	}

	for _, fixture := range fixtures {
		data, err := os.ReadFile(fixture)
		if err != nil {
			t.Fatalf("read %s: %v", fixture, err)
		}
		for i, raw := range strings.Split(string(data), "\n") {
			lineNo := i + 1
			line := strings.TrimSpace(raw)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			t.Run(filepath.Base(fixture)+"/"+strconv.Itoa(lineNo), func(t *testing.T) {
				fn(t, fixture, line, lineNo)
			})
		}
	}
}

// TestParseLynxFlowGoldens verifies that every line in every .lynxflow golden
// parses cleanly with the LynxFlow parser. This mirrors TestParseEveryGoldenLine
// for .spl2 files.
func TestParseLynxFlowGoldens(t *testing.T) {
	walkLynxFlowFixtures(t, func(t *testing.T, fixture, line string, _ int) {
		q, diags := parser.Parse(line)
		if q == nil {
			t.Fatalf("parse returned nil AST for %s: %s", fixture, line)
		}
		for _, d := range diags {
			if d.Severity == parser.SeverityError {
				t.Fatalf("parse error for %s: %s\n  LynxFlow: %s", fixture, d.Message, line)
			}
		}
	})
}

// TestLynxFlowConformance executes every non-minimal, non-index .lynxflow
// golden against the sigmacompat deterministic dataset and asserts the same
// expected_match_count as the SPL2 compat_manifest.json.
//
// Minimal and index variants are tested by TestParseLynxFlowGoldens (parse-only)
// because their match semantics are identical to the default variant.
func TestLynxFlowConformance(t *testing.T) {
	manifest, err := EmbeddedCompatManifest()
	if err != nil {
		t.Fatalf("load compat manifest: %v", err)
	}

	// Build fixture name -> expected count lookup from manifest.
	expected := make(map[string]int, len(manifest.Fixtures))
	for _, f := range manifest.Fixtures {
		expected[f.Name] = f.ExpectedMatchCount
	}

	// Only run conformance on the canonical (non-minimal, non-index) fixtures.
	// These are the 9 base fixtures that have matching sigmacompat datasets.
	for _, fixtureName := range sigmacompat.FixtureNames {
		fixtureName := fixtureName
		t.Run(fixtureName, func(t *testing.T) {
			// Read the .lynxflow golden file.
			lfPath := filepath.Join("testdata", "golden", fixtureName+".lynxflow")
			data, err := os.ReadFile(lfPath)
			if err != nil {
				t.Fatalf("read %s: %v", lfPath, err)
			}
			query := strings.TrimSpace(string(data))

			// Get expected match count.
			wantCount, ok := expected[fixtureName]
			if !ok {
				t.Fatalf("fixture %s not found in compat_manifest.json", fixtureName)
			}

			// Build the event store from the sigmacompat deterministic dataset.
			dataset := sigmacompat.DatasetFor(fixtureName)
			if len(dataset) == 0 {
				t.Fatalf("empty dataset for fixture %s", fixtureName)
			}

			events := sigmaEventsToLynxDB(t, dataset)
			eventMap := map[string][]*event.Event{"main": events}

			// Execute via the LynxFlow engine.
			rows, err := run.Execute(
				context.Background(),
				query,
				eventMap,
				run.Options{
					DefaultSource: "main",
					Now:           time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				},
			)
			if err != nil {
				t.Fatalf("lynxflow Execute(%s): %v\n  Query: %s", fixtureName, err, query)
			}

			gotCount := len(rows)
			if gotCount != wantCount {
				t.Errorf("fixture %s: match count mismatch: got %d, want %d\n  Query: %s",
					fixtureName, gotCount, wantCount, query)
			}
		})
	}
}

// sigmaEventsToLynxDB converts sigmacompat.Event slice to []*event.Event
// suitable for run.Execute. Each event is parsed from its JSON Raw field.
func sigmaEventsToLynxDB(t *testing.T, dataset []sigmacompat.Event) []*event.Event {
	t.Helper()

	events := make([]*event.Event, 0, len(dataset))
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	for i, de := range dataset {
		var fields map[string]any
		if err := json.Unmarshal([]byte(de.Raw), &fields); err != nil {
			t.Fatalf("unmarshal event %d: %v\n  raw: %s", i, err, de.Raw)
		}

		ev := &event.Event{
			Time:   ts.Add(time.Duration(i) * time.Millisecond),
			Raw:    de.Raw,
			Source: "sigmacompat",
			Index:  "main",
			Fields: make(map[string]event.Value, len(fields)),
		}
		for k, v := range fields {
			ev.Fields[k] = event.ValueFromInterface(v)
		}
		events = append(events, ev)
	}

	return events
}
