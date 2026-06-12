// Package explain renders a user-facing EXPLAIN tree from a logical IR plan.
//
// Unlike plan.Dump (which is a debug format), EXPLAIN is the rich render
// described in RFC-002 §11: per-node index interaction (term-index pushdown,
// bloom terms, field predicates, column pruning), spill capability markers,
// and an Annotations section showing desugar rewrites and optimizer rules.
//
// EXPLAIN ANALYZE extends the base tree with per-node runtime statistics
// (rows, batches, wall time) collected during physical execution.
package explain

import (
	"fmt"
	"strings"
	"time"

	"github.com/lynxbase/lynxdb/pkg/logical"
	"github.com/lynxbase/lynxdb/pkg/logical/opt"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/desugar"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/format"
)

// Info carries metadata from earlier pipeline stages that the EXPLAIN renderer
// needs to produce the Annotations section.
type Info struct {
	Rewrites []desugar.Rewrite
	Applied  []opt.Applied
}

// NodeStats holds per-node runtime statistics collected by the physical
// StatsIterator wrapper during EXPLAIN ANALYZE execution.
type NodeStats struct {
	Rows     int64
	Batches  int64
	WallTime time.Duration
}

// Render produces the user-facing EXPLAIN string for the given plan.
// When stats is non-nil, per-node runtime statistics are appended to each
// node line (EXPLAIN ANALYZE mode).
func Render(p *logical.Plan, info Info, stats map[logical.Node]*NodeStats) string {
	if p == nil {
		return "(empty plan)\n"
	}
	var b strings.Builder
	counter := 1

	if len(p.Lets) > 0 {
		keys := sortedKeys(p.Lets)
		for _, name := range keys {
			fmt.Fprintf(&b, "Let $%s\n", name)
			counter = renderNode(&b, p.Lets[name].Root, 1, counter, stats)
			b.WriteByte('\n')
		}
	}

	renderNode(&b, p.Root, 0, counter, stats)
	renderAnnotations(&b, info)
	return b.String()
}

// ---------------------------------------------------------------------------
// Spill capability classification
// ---------------------------------------------------------------------------

// spillCapable returns true for node types that can spill to disk.
// Per RFC-002 §11: Sort, Aggregate, Join, Dedup, Tail (Limit with Tail=true).
func spillCapable(n logical.Node) bool {
	switch nd := n.(type) {
	case *logical.Sort:
		return true
	case *logical.Aggregate:
		return true
	case *logical.Join:
		return true
	case *logical.Dedup:
		return true
	case *logical.Limit:
		return nd.Tail
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// Tree rendering
// ---------------------------------------------------------------------------

// renderNode renders a single node and its children recursively.
// It returns the next counter value (for numbering).
func renderNode(b *strings.Builder, n logical.Node, depth int, counter int, stats map[logical.Node]*NodeStats) int {
	if n == nil {
		indent(b, depth)
		fmt.Fprintf(b, "%d. (nil)\n", counter)
		return counter + 1
	}

	// Print the node line.
	indent(b, depth)
	fmt.Fprintf(b, "%d. %s", counter, nodeOneLiner(n))
	if spillCapable(n) {
		b.WriteString("  [spill-capable]")
	}
	if stats != nil {
		if ns, ok := stats[n]; ok {
			fmt.Fprintf(b, "  rows=%d batches=%d time=%s", ns.Rows, ns.Batches, formatDuration(ns.WallTime))
		}
	}
	b.WriteByte('\n')
	myCounter := counter
	counter++
	_ = myCounter

	// Pushdown sub-lines for Scan.
	if s, ok := n.(*Scan); ok {
		counter = renderPushdown(b, &s.Pushdown, depth+1, counter)
	}

	// For Join: left children, then "Right:" sub-plan.
	if j, ok := n.(*Join); ok {
		for _, c := range n.Children() {
			counter = renderNode(b, c, depth+1, counter, stats)
		}
		indent(b, depth+1)
		b.WriteString("Right:\n")
		counter = renderNode(b, j.Right, depth+2, counter, stats)
		return counter
	}

	// For Union: all inputs.
	for _, c := range n.Children() {
		counter = renderNode(b, c, depth+1, counter, stats)
	}
	return counter
}

// renderPushdown renders Scan pushdown annotations as numbered sub-lines.
func renderPushdown(b *strings.Builder, pd *logical.Pushdown, depth int, counter int) int {
	if pd.TimeBounds != nil {
		indent(b, depth)
		fmt.Fprintf(b, "time_bounds: %s\n", timeBoundsString(pd.TimeBounds))
	}
	for _, rt := range pd.RawTerms {
		indent(b, depth)
		fmt.Fprintf(b, "raw_term: %q via FST\n", rt)
	}
	for _, tg := range pd.TokenGlobs {
		indent(b, depth)
		fmt.Fprintf(b, "token_glob: %q via FST expansion\n", tg)
	}
	for _, bt := range pd.BloomTerms {
		indent(b, depth)
		fmt.Fprintf(b, "bloom_term: %q\n", bt)
	}
	for _, fp := range pd.FieldPredicates {
		indent(b, depth)
		fmt.Fprintf(b, "field_predicate: %s\n", format.Expr(fp))
	}
	if len(pd.Columns) > 0 {
		indent(b, depth)
		fmt.Fprintf(b, "columns: [%s]\n", strings.Join(pd.Columns, ", "))
	}
	return counter
}

// ---------------------------------------------------------------------------
// One-liner per node type
// ---------------------------------------------------------------------------

func nodeOneLiner(n logical.Node) string {
	switch nd := n.(type) {
	case *logical.Scan:
		return scanOneLiner(nd)
	case *logical.Empty:
		return "Empty()"
	case *logical.Filter:
		return "Filter(" + format.Expr(nd.Expr) + ")"
	case *logical.Parse:
		return parseOneLiner(nd)
	case *logical.Project:
		return nd.String()
	case *logical.Extend:
		return nd.String()
	case *logical.Aggregate:
		return aggregateOneLiner(nd)
	case *logical.TopK:
		return nd.String()
	case *logical.Sort:
		return nd.String()
	case *logical.Limit:
		return nd.String()
	case *logical.Dedup:
		return nd.String()
	case *logical.Join:
		return nd.String()
	case *logical.Union:
		return nd.String()
	case *logical.Explode:
		return nd.String()
	case *logical.Describe:
		return "Describe()"
	case *logical.Helper:
		return nd.String()
	case *logical.Materialize:
		return nd.String()
	case *logical.Tee:
		return nd.String()
	default:
		return fmt.Sprintf("%T(?)", n)
	}
}

func scanOneLiner(s *Scan) string {
	var b strings.Builder
	b.WriteString("Scan(")
	for i, src := range s.Sources {
		if i > 0 {
			b.WriteString(", ")
		}
		switch src.Kind {
		case 0: // SourceName
			b.WriteString(src.Name)
		default:
			// Fall through to the existing String method for other kinds.
			b.WriteString(s.String()[len("Scan("):])
			// That would double up; just use the node's own String instead.
		}
	}
	// Simpler: use the node's existing String() which already handles
	// all source kinds, time range, and reverse.
	return s.String()
}

func parseOneLiner(p *logical.Parse) string {
	var b strings.Builder
	b.WriteString("Parse(")
	if len(p.FirstOf) > 0 {
		b.WriteString("first_of(")
		b.WriteString(strings.Join(p.FirstOf, ", "))
		b.WriteByte(')')
	} else {
		b.WriteString(p.Format)
	}
	if p.From != "" {
		b.WriteString(", from=")
		b.WriteString(p.From)
	}
	if p.OnError != "" && p.OnError != "propagate" {
		b.WriteString(", on_error=")
		b.WriteString(p.OnError)
	}
	b.WriteByte(')')
	return b.String()
}

func aggregateOneLiner(a *logical.Aggregate) string {
	// Delegate to the node's String() which already includes [partial],
	// [eventstats], [streamstats window=N], [topk=N].
	return a.String()
}

// ---------------------------------------------------------------------------
// Annotations section
// ---------------------------------------------------------------------------

func renderAnnotations(b *strings.Builder, info Info) {
	hasRewrites := len(info.Rewrites) > 0
	hasApplied := len(info.Applied) > 0
	if !hasRewrites && !hasApplied {
		return
	}

	b.WriteString("\nAnnotations:\n")

	if hasRewrites {
		b.WriteString("  Desugar rewrites:\n")
		for _, rw := range info.Rewrites {
			if rw.Before == "" {
				fmt.Fprintf(b, "    %s: => %s\n", rw.Reason, rw.After)
			} else {
				fmt.Fprintf(b, "    %s: %s => %s\n", rw.Reason, rw.Before, rw.After)
			}
		}
	}

	if hasApplied {
		b.WriteString("  Optimizer rules:\n")
		for _, a := range info.Applied {
			fmt.Fprintf(b, "    %s (%dx)\n", a.Rule, a.Count)
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// Type aliases for less verbose switch statements.
type (
	Scan = logical.Scan
	Join = logical.Join
)

func indent(b *strings.Builder, depth int) {
	for i := 0; i < depth; i++ {
		b.WriteString("  ")
	}
}

func timeBoundsString(tb *logical.TimeBounds) string {
	if tb == nil {
		return ""
	}
	var b strings.Builder
	b.WriteByte('[')
	if tb.Start != nil {
		b.WriteString(format.Expr(tb.Start))
	}
	if tb.End != nil {
		b.WriteString("..")
		b.WriteString(format.Expr(tb.End))
	}
	b.WriteByte(']')
	if tb.Snap != "" {
		b.WriteString("[@")
		b.WriteString(tb.Snap)
		b.WriteByte(']')
	}
	return b.String()
}

func formatDuration(d time.Duration) string {
	if d < time.Microsecond {
		return fmt.Sprintf("%dns", d.Nanoseconds())
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%.1fus", float64(d.Nanoseconds())/1e3)
	}
	if d < time.Second {
		return fmt.Sprintf("%.1fms", float64(d.Nanoseconds())/1e6)
	}
	return fmt.Sprintf("%.3fs", d.Seconds())
}

func sortedKeys(m map[string]*logical.Plan) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
