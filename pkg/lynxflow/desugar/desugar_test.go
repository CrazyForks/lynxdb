package desugar

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/lynxbase/lynxdb/pkg/lynxflow/ast"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/parser"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/registry"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mustParse(t *testing.T, input string) *ast.Query {
	t.Helper()
	q, diags := parser.Parse(input)
	if len(diags) > 0 {
		t.Fatalf("Parse(%q): %d diag(s):", input, len(diags))
	}
	if q == nil {
		t.Fatalf("Parse(%q): returned nil", input)
	}
	return q
}

// hasSugarStage walks the pipeline checking for any sugar-class stage names.
func hasSugarStage(q *ast.Query) string {
	for _, s := range q.Pipeline.Stages {
		if IsSugarStageName(s.Name) {
			return s.Name
		}
	}
	for _, l := range q.Lets {
		for _, s := range l.Pipeline.Stages {
			if IsSugarStageName(s.Name) {
				return s.Name
			}
		}
	}
	// Also check union sub-pipelines.
	return hasSugarStageInStages(q.Pipeline.Stages)
}

func hasSugarStageInStages(stages []ast.Stage) string {
	for _, s := range stages {
		if IsSugarStageName(s.Name) {
			return s.Name
		}
		if s.Union != nil {
			for _, src := range s.Union.Sources {
				if src.Pipeline != nil {
					found := hasSugarStageInStages(src.Pipeline.Stages)
					if found != "" {
						return found
					}
				}
			}
		}
		if s.Join != nil && s.Join.Right != nil && s.Join.Right.Pipeline != nil {
			found := hasSugarStageInStages(s.Join.Right.Pipeline.Stages)
			if found != "" {
				return found
			}
		}
	}
	return ""
}

// hasSugarTerms checks if the from stage still has search sugar terms.
func hasSugarTerms(q *ast.Query) bool {
	if q.Pipeline.Source != nil && q.Pipeline.Source.SugarTerms != nil {
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// 1. Golden expansion tests
// ---------------------------------------------------------------------------

func TestGoldenExpansions(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantPipe      string // expected String() of the desugared pipeline
		wantRewBefore string // expected Rewrite.Before (if non-empty, check first rewrite)
		wantRewAfter  string // expected Rewrite.After (if non-empty, check first rewrite)
		wantReason    string
	}{
		// --- Search sugar §3.1 ---
		{
			name:       "bare_word",
			input:      `from app timeout`,
			wantPipe:   `from app | where has(_raw, "timeout")`,
			wantReason: "search-sugar",
		},
		{
			name:       "quoted_phrase",
			input:      `from app "connection reset"`,
			wantPipe:   `from app | where contains(_raw, "connection reset")`,
			wantReason: "search-sugar",
		},
		{
			name:       "key_eq_value_string",
			input:      `from app level="ERROR"`,
			wantPipe:   `from app | where (level == "ERROR")`,
			wantReason: "search-sugar",
		},
		{
			name:       "key_eq_value_int",
			input:      `from app status=500`,
			wantPipe:   `from app | where (status == 500)`,
			wantReason: "search-sugar",
		},
		{
			name:       "key_gte",
			input:      `from app status>=500`,
			wantPipe:   `from app | where (status >= 500)`,
			wantReason: "search-sugar",
		},
		{
			name:       "key_neq",
			input:      `from app status!=200`,
			wantPipe:   `from app | where (status != 200)`,
			wantReason: "search-sugar",
		},
		{
			name:       "key_lt",
			input:      `from app status<400`,
			wantPipe:   `from app | where (status < 400)`,
			wantReason: "search-sugar",
		},
		{
			name:       "glob_value",
			input:      `from app host=web*`,
			wantPipe:   `from app | where glob(host, "web*")`,
			wantReason: "search-sugar",
		},
		{
			name:       "key_in",
			input:      `from app level in ("ERROR", "WARN")`,
			wantPipe:   `from app | where (level in ["ERROR", "WARN"])`,
			wantReason: "search-sugar",
		},
		{
			name:       "juxtaposition_and",
			input:      `from app timeout status>=500`,
			wantPipe:   `from app | where (has(_raw, "timeout") and (status >= 500))`,
			wantReason: "search-sugar",
		},
		{
			name:       "or_terms",
			input:      `from app error or timeout`,
			wantPipe:   `from app | where (has(_raw, "error") or has(_raw, "timeout"))`,
			wantReason: "search-sugar",
		},
		{
			name:       "not_term",
			input:      `from app not debug`,
			wantPipe:   `from app | where (not has(_raw, "debug"))`,
			wantReason: "search-sugar",
		},
		{
			name:       "parens_in_sugar",
			input:      `from app (error or timeout) status>=500`,
			wantPipe:   `from app | where ((has(_raw, "error") or has(_raw, "timeout")) and (status >= 500))`,
			wantReason: "search-sugar",
		},
		{
			name:       "no_sugar_no_rewrite",
			input:      `from app | where status >= 500`,
			wantPipe:   `from app | where (status >= 500)`,
			wantReason: "", // no search-sugar rewrite expected
		},

		// --- Sugar stages §9.1 ---
		{
			name:       "top_default",
			input:      `from app | top uri`,
			wantPipe:   `from app | stats count() as count by uri | sort -count | head 10`,
			wantReason: "sugar:top",
		},
		{
			name:       "top_with_n",
			input:      `from app | top 5 uri`,
			wantPipe:   `from app | stats count() as count by uri | sort -count | head 5`,
			wantReason: "sugar:top",
		},
		{
			name:       "rare_default",
			input:      `from app | rare service`,
			wantPipe:   `from app | stats count() as count by service | sort count | head 10`,
			wantReason: "sugar:rare",
		},
		{
			name:       "rare_with_n",
			input:      `from app | rare 3 service`,
			wantPipe:   `from app | stats count() as count by service | sort count | head 3`,
			wantReason: "sugar:rare",
		},
		{
			name:       "every_simple",
			input:      `from app | every 5m stats count()`,
			wantPipe:   `from app | stats count() by bin(_time, 5m)`,
			wantReason: "sugar:every",
		},
		{
			name:       "every_with_by",
			input:      `from app | every 5m by service stats count()`,
			wantPipe:   `from app | stats count() by service, bin(_time, 5m)`,
			wantReason: "sugar:every",
		},
		{
			name:       "rate_default",
			input:      `from app | rate`,
			wantPipe:   `from app | stats count() as rate by bin(_time, 1m)`,
			wantReason: "sugar:rate",
		},
		{
			name:       "rate_per_by",
			input:      `from app | rate per 5m by service`,
			wantPipe:   `from app | stats count() as rate by service, bin(_time, 5m)`,
			wantReason: "sugar:rate",
		},
		{
			name:       "latency_simple",
			input:      `from app | latency dur`,
			wantPipe:   `from app | stats p50(dur), p95(dur), p99(dur), count()`,
			wantReason: "sugar:latency",
		},
		{
			name:       "latency_every_by",
			input:      `from app | latency dur every 5m by endpoint`,
			wantPipe:   `from app | stats p50(dur), p95(dur), p99(dur), count() by endpoint, bin(_time, 5m)`,
			wantReason: "sugar:latency",
		},
		{
			name:       "percentiles_simple",
			input:      `from app | percentiles duration_ms`,
			wantPipe:   `from app | stats p50(duration_ms) as p50_duration_ms, p75(duration_ms) as p75_duration_ms, p90(duration_ms) as p90_duration_ms, p95(duration_ms) as p95_duration_ms, p99(duration_ms) as p99_duration_ms`,
			wantReason: "sugar:percentiles",
		},
		{
			name:       "percentiles_by",
			input:      `from app | percentiles duration_ms by service`,
			wantPipe:   `from app | stats p50(duration_ms) as p50_duration_ms, p75(duration_ms) as p75_duration_ms, p90(duration_ms) as p90_duration_ms, p95(duration_ms) as p95_duration_ms, p99(duration_ms) as p99_duration_ms by service`,
			wantReason: "sugar:percentiles",
		},
		{
			name:       "proportion_simple",
			input:      `from app | proportion status >= 500 as error_rate`,
			wantPipe:   `from app | stats count(where (status >= 500)) as error_rate_num, count() as error_rate_den | extend error_rate = (error_rate_num / error_rate_den)`,
			wantReason: "sugar:proportion",
		},
		{
			name:       "proportion_every_by",
			input:      `from app | proportion status >= 500 as error_rate every 5m by service`,
			wantPipe:   `from app | stats count(where (status >= 500)) as error_rate_num, count() as error_rate_den by service, bin(_time, 5m) | extend error_rate = (error_rate_num / error_rate_den)`,
			wantReason: "sugar:proportion",
		},
		{
			name:       "impact_default",
			input:      `from app | impact by service`,
			wantPipe:   `from app | stats count() as n by service | eventstats sum(n) as total_n | extend pct_n = (n / total_n) | sort -pct_n`,
			wantReason: "sugar:impact",
		},
		{
			name:       "impact_sum",
			input:      `from app | impact sum(bytes) by host`,
			wantPipe:   `from app | stats sum(bytes) as sum_bytes by host | eventstats sum(sum_bytes) as total_sum_bytes | extend pct_sum_bytes = (sum_bytes / total_sum_bytes) | sort -pct_sum_bytes`,
			wantReason: "sugar:impact",
		},
		{
			name:       "baseline",
			input:      `from app | baseline error_rate window=12 by service`,
			wantPipe:   `from app | streamstats window=12 current=false avg(error_rate) as baseline_error_rate, stdev(error_rate) as stdev_error_rate by service | extend delta_error_rate = (error_rate - baseline_error_rate), z_error_rate = if((stdev_error_rate > 0), (delta_error_rate / stdev_error_rate), null)`,
			wantReason: "sugar:baseline",
		},
		{
			name:       "changes",
			input:      `from app | changes version by service`,
			wantPipe:   `from app | sort _time | streamstats current=false last(version) as previous_version by service | where (exists(previous_version) and (version != previous_version))`,
			wantReason: "sugar:changes",
		},
		{
			name:       "exemplars_global",
			input:      `from app | exemplars`,
			wantPipe:   `from app | sort -_time | head 3`,
			wantReason: "sugar:exemplars",
		},
		{
			name:       "exemplars_with_n",
			input:      `from app | exemplars 5`,
			wantPipe:   `from app | sort -_time | head 5`,
			wantReason: "sugar:exemplars",
		},
		{
			name:       "exemplars_by",
			input:      `from app | exemplars 5 by endpoint`,
			wantPipe:   `from app | sort -_time | dedup 5 endpoint`,
			wantReason: "sugar:exemplars",
		},

		// --- Implicit source ---
		{
			name:       "implicit_source",
			input:      `| stats count()`,
			wantPipe:   `from main | stats count()`,
			wantReason: "implicit-source",
		},

		// --- RFC-002 §13 sugar examples ---
		{
			name:       "ex10_every_by_service",
			input:      `from app | every 5m by service stats count()`,
			wantPipe:   `from app | stats count() by service, bin(_time, 5m)`,
			wantReason: "sugar:every",
		},
		{
			name:       "ex11_proportion_by_service",
			input:      `from app | proportion status >= 500 as error_rate by service`,
			wantPipe:   `from app | stats count(where (status >= 500)) as error_rate_num, count() as error_rate_den by service | extend error_rate = (error_rate_num / error_rate_den)`,
			wantReason: "sugar:proportion",
		},
		{
			name:       "ex12_latency_every_by_endpoint",
			input:      `from app | latency dur every 5m by endpoint`,
			wantPipe:   `from app | stats p50(dur), p95(dur), p99(dur), count() by endpoint, bin(_time, 5m)`,
			wantReason: "sugar:latency",
		},
	}

	opts := Options{DefaultSource: "main"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := mustParse(t, tt.input)
			out, rewrites := Desugar(q, opts)

			got := out.Pipeline.String()
			if got != tt.wantPipe {
				t.Errorf("Desugar pipeline:\n  got:  %s\n  want: %s", got, tt.wantPipe)
			}

			// Check reason.
			if tt.wantReason != "" {
				found := false
				for _, r := range rewrites {
					if r.Reason == tt.wantReason {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected rewrite reason %q, got reasons: %v", tt.wantReason, rewriteReasons(rewrites))
				}
			}

			// Check Rewrite Before/After strings if specified.
			if tt.wantRewBefore != "" && len(rewrites) > 0 {
				if rewrites[0].Before != tt.wantRewBefore {
					t.Errorf("Rewrite.Before:\n  got:  %s\n  want: %s", rewrites[0].Before, tt.wantRewBefore)
				}
			}
			if tt.wantRewAfter != "" && len(rewrites) > 0 {
				if rewrites[0].After != tt.wantRewAfter {
					t.Errorf("Rewrite.After:\n  got:  %s\n  want: %s", rewrites[0].After, tt.wantRewAfter)
				}
			}

			// Verify no remaining sugar stages.
			if sugar := hasSugarStage(out); sugar != "" {
				t.Errorf("output still contains sugar stage %q", sugar)
			}
			if hasSugarTerms(out) {
				t.Errorf("output still contains search sugar terms")
			}
		})
	}
}

func rewriteReasons(rw []Rewrite) []string {
	reasons := make([]string, len(rw))
	for i, r := range rw {
		reasons[i] = r.Reason
	}
	return reasons
}

// ---------------------------------------------------------------------------
// 2. Corpus: desugar all 63 corpus queries
// ---------------------------------------------------------------------------

type corpusEntry struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	LynxFlow string   `json:"lynxflow"`
	Features []string `json:"features"`
}

func loadCorpus(t *testing.T) []corpusEntry {
	t.Helper()
	f, err := os.Open("../testdata/corpus/corpus.jsonl")
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}
	defer f.Close()

	var entries []corpusEntry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		var e corpusEntry
		if err := json.Unmarshal([]byte(text), &e); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		entries = append(entries, e)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan corpus: %v", err)
	}
	return entries
}

func TestCorpusDesugar(t *testing.T) {
	entries := loadCorpus(t)
	if len(entries) < 50 {
		t.Fatalf("corpus has %d entries, want >= 50", len(entries))
	}

	opts := Options{DefaultSource: "main"}

	for _, e := range entries {
		t.Run(e.ID+"_"+e.Name, func(t *testing.T) {
			q, diags := parser.Parse(e.LynxFlow)
			if len(diags) > 0 {
				t.Skipf("parse diags for %s (skip desugar test): %v", e.ID, diags)
				return
			}

			out, _ := Desugar(q, opts)

			// Assert no remaining sugar-class stages.
			if sugar := hasSugarStage(out); sugar != "" {
				t.Errorf("[%s] output still contains sugar stage %q", e.ID, sugar)
			}

			// Assert no remaining search sugar terms on from.
			if hasSugarTerms(out) {
				t.Errorf("[%s] output still contains search sugar terms", e.ID)
			}

			// Idempotence: re-desugar should be identical.
			out2, rewrites2 := Desugar(out, opts)
			if out.Pipeline.String() != out2.Pipeline.String() {
				t.Errorf("[%s] idempotence failed:\n  first:  %s\n  second: %s", e.ID, out.Pipeline.String(), out2.Pipeline.String())
			}
			// Re-desugaring core output should produce zero rewrites
			// (except possibly implicit-source if the output gained one).
			for _, r := range rewrites2 {
				if r.Reason != "implicit-source" {
					t.Errorf("[%s] re-desugar produced unexpected rewrite: %q", e.ID, r.Reason)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 3. Input immutability
// ---------------------------------------------------------------------------

func TestInputImmutability(t *testing.T) {
	input := `from app timeout status>=500 | top 5 uri | latency dur every 1h by service`
	q := mustParse(t, input)
	origStr := q.String()

	opts := Options{DefaultSource: "main"}
	_, _ = Desugar(q, opts)

	afterStr := q.String()
	if origStr != afterStr {
		t.Errorf("input AST was mutated:\n  before: %s\n  after:  %s", origStr, afterStr)
	}
}

// ---------------------------------------------------------------------------
// 4. Facets prefix-clone correctness
// ---------------------------------------------------------------------------

func TestFacetsPrefixClone(t *testing.T) {
	input := `from x[-1h] | where a == 1 | facets s, h limit=2`
	q := mustParse(t, input)
	opts := Options{DefaultSource: "main"}
	out, rewrites := Desugar(q, opts)

	_ = out

	// There should be a sugar:facets rewrite.
	found := false
	for _, r := range rewrites {
		if r.Reason == "sugar:facets" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected sugar:facets rewrite")
	}

	// The output should contain a union stage with sub-pipelines.
	// Each sub-pipeline should include the where stage.
	var unionStage *ast.Stage
	for i := range out.Pipeline.Stages {
		if out.Pipeline.Stages[i].Name == "union" {
			unionStage = &out.Pipeline.Stages[i]
			break
		}
	}
	if unionStage == nil {
		// With 2 fields: first field inline, second field in union.
		// Check that the inline stages include stats+sort+head+extend+keep (5 stages)
		// and the union contains the second branch.
		t.Log("Output pipeline:", out.Pipeline.String())
		// The where a == 1 should be in the prefix stages.
		pipeStr := out.Pipeline.String()
		if !strings.Contains(pipeStr, "where (a == 1)") {
			t.Errorf("inline branch missing prefix where stage: %s", pipeStr)
		}
		return
	}

	// Check that union sub-pipelines contain the where stage.
	for i, src := range unionStage.Union.Sources {
		if src.Pipeline == nil {
			t.Errorf("union source %d has nil pipeline", i)
			continue
		}
		subPipStr := src.Pipeline.String()
		if !strings.Contains(subPipStr, "where") {
			t.Errorf("union branch %d missing prefix where stage: %s", i, subPipStr)
		}
	}
}

// ---------------------------------------------------------------------------
// 5. Fuzz: parse+desugar corpus-seeded inputs
// ---------------------------------------------------------------------------

func FuzzDesugar(f *testing.F) {
	// Seed with corpus queries.
	corpus, err := os.Open("../testdata/corpus/corpus.jsonl")
	if err == nil {
		sc := bufio.NewScanner(corpus)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			text := strings.TrimSpace(sc.Text())
			if text == "" {
				continue
			}
			var e corpusEntry
			if json.Unmarshal([]byte(text), &e) == nil {
				f.Add(e.LynxFlow)
			}
		}
		corpus.Close()
	}

	// Additional seeds.
	f.Add(`from app error | top 10 uri`)
	f.Add(`| rate per 5m by host`)
	f.Add(`from x | baseline f window=5`)
	f.Add(`from y "phrase" key=val | exemplars 3 by k`)

	opts := Options{DefaultSource: "main"}

	f.Fuzz(func(t *testing.T, input string) {
		q, diags := parser.Parse(input)
		if len(diags) > 0 || q == nil {
			return // skip inputs that don't parse
		}

		// Property: no panic.
		out, _ := Desugar(q, opts)
		if out == nil {
			t.Fatal("Desugar returned nil")
		}

		// Property: desugar twice == desugar once (idempotence on String()).
		out2, _ := Desugar(out, opts)
		if out.Pipeline.String() != out2.Pipeline.String() {
			t.Errorf("idempotence failed:\n  first:  %s\n  second: %s",
				out.Pipeline.String(), out2.Pipeline.String())
		}
	})
}

// ---------------------------------------------------------------------------
// Additional edge cases
// ---------------------------------------------------------------------------

func TestIdempotence_CoreOnly(t *testing.T) {
	// A query with no sugar should return zero rewrites (except implicit-source).
	input := `from app | where status >= 500 | stats count() by service | sort -count | head 10`
	q := mustParse(t, input)
	opts := Options{DefaultSource: "main"}
	out, rewrites := Desugar(q, opts)

	if len(rewrites) != 0 {
		t.Errorf("expected 0 rewrites for core-only query, got %d: %v", len(rewrites), rewriteReasons(rewrites))
	}

	if q.Pipeline.String() != out.Pipeline.String() {
		t.Errorf("core-only query changed:\n  before: %s\n  after:  %s", q.Pipeline.String(), out.Pipeline.String())
	}
}

func TestImplicitSource_NoDefaultSource(t *testing.T) {
	input := `| stats count()`
	q := mustParse(t, input)
	opts := Options{} // no default source
	out, rewrites := Desugar(q, opts)

	// Should not add an implicit source.
	if out.Pipeline.Source != nil {
		t.Errorf("expected no source when DefaultSource is empty, got: %s", out.Pipeline.Source.String())
	}
	if len(rewrites) != 0 {
		t.Errorf("expected 0 rewrites, got %d", len(rewrites))
	}
}

func TestSearchSugarWithRange(t *testing.T) {
	input := `from nginx[-1h] timeout status>=500`
	q := mustParse(t, input)
	opts := Options{DefaultSource: "main"}
	out, _ := Desugar(q, opts)

	// The from should keep the time range but lose the sugar terms.
	if out.Pipeline.Source == nil {
		t.Fatal("expected from stage")
	}
	if out.Pipeline.Source.SugarTerms != nil {
		t.Error("from still has sugar terms")
	}
	if len(out.Pipeline.Source.TimeRanges) == 0 {
		t.Error("from lost time ranges")
	}

	// First stage should be where.
	if len(out.Pipeline.Stages) == 0 || out.Pipeline.Stages[0].Name != "where" {
		t.Errorf("first stage should be where, got: %v", out.Pipeline.Stages)
	}
}

func TestPercentilesDottedField(t *testing.T) {
	// Field names with dots should have dots replaced with _ in aliases.
	input := `from app | percentiles response.time`
	q := mustParse(t, input)
	opts := Options{DefaultSource: "main"}
	out, _ := Desugar(q, opts)

	pipeStr := out.Pipeline.String()
	// Should use p50_response_time (dots -> underscores).
	if !strings.Contains(pipeStr, "p50_response_time") {
		t.Errorf("expected p50_response_time in output, got: %s", pipeStr)
	}
}

func TestChangesNoBy(t *testing.T) {
	input := `from app | changes config_version`
	q := mustParse(t, input)
	opts := Options{DefaultSource: "main"}
	out, _ := Desugar(q, opts)

	pipeStr := out.Pipeline.String()
	if !strings.Contains(pipeStr, "sort _time") {
		t.Errorf("expected sort _time in output, got: %s", pipeStr)
	}
	if !strings.Contains(pipeStr, "previous_config_version") {
		t.Errorf("expected previous_config_version in output, got: %s", pipeStr)
	}
}

func TestUnionSubPipelineDesugared(t *testing.T) {
	// Sugar inside union sub-pipelines should also be desugared.
	input := `from app | union [from other | top 5 uri]`
	q := mustParse(t, input)
	opts := Options{DefaultSource: "main"}
	out, _ := Desugar(q, opts)

	// The union sub-pipeline should contain stats+sort+head, not top.
	if sugar := hasSugarStage(out); sugar != "" {
		t.Errorf("output still contains sugar stage %q", sugar)
	}
}

func TestJoinSubPipelineDesugared(t *testing.T) {
	input := `from app | join type=left on id with [from other | rare 3 status]`
	q := mustParse(t, input)
	opts := Options{DefaultSource: "main"}
	out, _ := Desugar(q, opts)

	if sugar := hasSugarStage(out); sugar != "" {
		t.Errorf("output still contains sugar stage %q", sugar)
	}
}

func TestCTEDesugar(t *testing.T) {
	input := `let $errs = from app error | top 5 service; from $errs | head 10`
	q := mustParse(t, input)
	opts := Options{DefaultSource: "main"}
	out, rewrites := Desugar(q, opts)

	// CTE should be desugared.
	if len(out.Lets) == 0 {
		t.Fatal("expected let bindings")
	}

	// CTE body should not contain sugar.
	cteStages := out.Lets[0].Pipeline.Stages
	for _, s := range cteStages {
		if IsSugarStageName(s.Name) {
			t.Errorf("CTE still contains sugar stage %q", s.Name)
		}
	}

	// Should have rewrites for both search-sugar and sugar:top.
	reasons := rewriteReasons(rewrites)
	hasSearchSugar := false
	hasTopSugar := false
	for _, r := range reasons {
		if r == "search-sugar" {
			hasSearchSugar = true
		}
		if r == "sugar:top" {
			hasTopSugar = true
		}
	}
	if !hasSearchSugar {
		t.Error("expected search-sugar rewrite")
	}
	if !hasTopSugar {
		t.Error("expected sugar:top rewrite")
	}
}

// ---------------------------------------------------------------------------
// Sugar stage class check
// ---------------------------------------------------------------------------

func TestSugarStageNames(t *testing.T) {
	// All sugar-class operators should be recognized.
	for _, op := range registry.Operators() {
		if op.Class == registry.ClassSugar {
			if !IsSugarStageName(op.Name) {
				t.Errorf("IsSugarStageName(%q) = false, want true", op.Name)
			}
		}
	}
	// Core operators should not be sugar.
	for _, op := range registry.Operators() {
		if op.Class == registry.ClassCore {
			if IsSugarStageName(op.Name) {
				t.Errorf("IsSugarStageName(%q) = true for core operator", op.Name)
			}
		}
	}
}
