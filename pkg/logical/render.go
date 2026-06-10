// Package logical (render.go) reconstructs parseable LynxFlow query text from
// a logical plan or a prefix of one. This is used by the distributed query
// coordinator to serialize shard-level sub-plans as query text for transmission.
//
// Design decision: we render to LynxFlow TEXT (not binary) because the shard
// boundary already uses language-agnostic text transport (the shard re-parses).
// This mirrors the existing SPL2 approach in buildShardQueryText.
//
// Round-trip contract: parse -> desugar -> lower -> optimize -> RenderPipeline
// -> re-parse -> desugar -> lower produces a plan whose Dump() matches the
// original (modulo pushdown re-derivation by the optimizer on the shard).
//
// What is NOT round-trippable:
//   - Pushdown annotations (bloom terms, field predicates, columns) are dropped;
//     each shard re-derives them via its own optimizer pass.
//   - Schema caches are not preserved (rebuilt on lower).
//   - TopKHint on Aggregate is advisory and not rendered.
//   - Aggregate.Partial flag is not rendered; the shard's optimizer sets it.
//   - Empty nodes are rendered as "from * | where false" (re-lowers to Empty).
package logical

import (
	"fmt"
	"strings"

	"github.com/lynxbase/lynxdb/pkg/lynxflow/ast"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/format"
)

// RenderPipeline produces parseable LynxFlow text from a sequence of logical
// nodes. The nodes must be provided in pipeline order: the first node should
// be a Scan (or Empty), and each subsequent node receives input from the
// previous. If nodes is empty, returns an empty string.
//
// The rendered text is a single pipeline (no CTEs). CTE references in Scan
// sources are rendered as $name references; the caller is responsible for
// rendering CTE definitions separately if needed.
func RenderPipeline(nodes ...Node) string {
	if len(nodes) == 0 {
		return ""
	}
	var b strings.Builder
	first := true
	for _, n := range nodes {
		text := renderNode(n)
		if text == "" {
			continue
		}
		if !first {
			b.WriteString(" | ")
		}
		b.WriteString(text)
		first = false
	}
	return b.String()
}

// RenderPlan produces parseable LynxFlow text from a complete Plan.
// It renders CTEs as let declarations and the main pipeline.
func RenderPlan(p *Plan) string {
	if p == nil {
		return ""
	}
	var b strings.Builder
	if len(p.Lets) > 0 {
		keys := sortedKeys(p.Lets)
		for _, name := range keys {
			b.WriteString("let $")
			b.WriteString(name)
			b.WriteString(" = ")
			chain := linearize(p.Lets[name].Root)
			for i, n := range chain {
				text := renderNode(n)
				if text == "" {
					continue
				}
				if i > 0 {
					b.WriteString(" | ")
				}
				b.WriteString(text)
			}
			b.WriteString(";\n")
		}
	}
	chain := linearize(p.Root)
	for i, n := range chain {
		text := renderNode(n)
		if text == "" {
			continue
		}
		if i > 0 {
			b.WriteString(" | ")
		}
		b.WriteString(text)
	}
	return b.String()
}

// linearize walks the plan tree from root to leaf and returns nodes in
// pipeline order (leaf first). The logical plan is a linear chain for most
// nodes (unaryNode.Input), with Scan/Empty as leaves.
func linearize(root Node) []Node {
	if root == nil {
		return nil
	}
	var chain []Node
	cur := root
	for cur != nil {
		chain = append(chain, cur)
		children := cur.Children()
		if len(children) == 0 {
			break
		}
		// For linear pipelines, follow the single child.
		cur = children[0]
	}
	// Reverse to get pipeline order (leaf = Scan first).
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}

// renderNode produces the LynxFlow text for a single logical node.
func renderNode(n Node) string {
	switch n := n.(type) {
	case *Scan:
		return renderScan(n)
	case *Empty:
		return "from * | where false"
	case *Filter:
		return "where " + format.Expr(n.Expr)
	case *Parse:
		return renderParse(n)
	case *Project:
		return renderProject(n)
	case *Extend:
		return renderExtend(n)
	case *Aggregate:
		return renderAggregate(n)
	case *Sort:
		return renderSort(n)
	case *TopK:
		return renderTopK(n)
	case *Limit:
		return renderLimit(n)
	case *Dedup:
		return renderDedup(n)
	case *Join:
		return renderJoin(n)
	case *Union:
		return renderUnion(n)
	case *Explode:
		return renderExplode(n)
	case *Describe:
		return "describe"
	case *Helper:
		return renderHelper(n)
	case *Materialize:
		return renderMaterialize(n)
	case *Tee:
		return fmt.Sprintf("tee %s", quoteStr(n.Sink))
	default:
		return ""
	}
}

func renderScan(n *Scan) string {
	var b strings.Builder
	b.WriteString("from")
	if len(n.Sources) > 0 {
		b.WriteByte(' ')
		for i, s := range n.Sources {
			if i > 0 {
				b.WriteString(", ")
			}
			switch s.Kind {
			case ast.SourceStar:
				b.WriteByte('*')
			case ast.SourceCTE:
				b.WriteString("$" + s.Name)
			case ast.SourceNegated:
				b.WriteString("!" + s.Pattern)
			case ast.SourceGlob:
				b.WriteString(s.Pattern)
			default:
				b.WriteString(s.Name)
			}
		}
	}
	if n.TimeRange != nil {
		renderTimeBounds(&b, n.TimeRange)
	}
	return b.String()
}

func renderTimeBounds(b *strings.Builder, tb *TimeBounds) {
	b.WriteString(" [")
	if tb.Start != nil {
		b.WriteString(format.Expr(tb.Start))
	}
	if tb.End != nil {
		b.WriteString("..")
		b.WriteString(format.Expr(tb.End))
	}
	b.WriteByte(']')
	if tb.Snap != "" {
		b.WriteString("[")
		b.WriteString(tb.Snap)
		b.WriteByte(']')
	}
}

func renderParse(n *Parse) string {
	var b strings.Builder
	b.WriteString("parse ")
	if len(n.FirstOf) > 0 {
		b.WriteString("first_of(")
		b.WriteString(strings.Join(n.FirstOf, ", "))
		b.WriteByte(')')
	} else {
		b.WriteString(n.Format)
	}
	if n.From != "" {
		b.WriteString(" from ")
		b.WriteString(n.From)
	}
	if len(n.Captures) > 0 {
		b.WriteString(" into (")
		for i, c := range n.Captures {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(c.Name)
			if c.Type != "" {
				b.WriteString(" as ")
				b.WriteString(c.Type)
			}
		}
		b.WriteByte(')')
	}
	if n.Prefix != "" {
		b.WriteString(" prefix ")
		b.WriteString(n.Prefix)
	}
	if n.OnError != "" {
		b.WriteString(" on_error ")
		b.WriteString(n.OnError)
	}
	return b.String()
}

func renderProject(n *Project) string {
	// Determine mode: all keeps, all drops, or mixed.
	hasKeep := false
	hasDrop := false
	hasRename := false
	hasStarExcept := false
	for _, c := range n.Cols {
		switch c.Action {
		case ProjectKeep:
			hasKeep = true
			if c.StarExcept {
				hasStarExcept = true
			}
		case ProjectDrop:
			hasDrop = true
		case ProjectRename:
			hasRename = true
		}
	}

	var b strings.Builder
	if hasStarExcept || (hasDrop && !hasKeep) {
		b.WriteString("drop ")
		first := true
		for _, c := range n.Cols {
			if c.Action == ProjectDrop {
				if !first {
					b.WriteString(", ")
				}
				b.WriteString(c.Name)
				first = false
			}
		}
	} else if hasKeep {
		b.WriteString("keep ")
		first := true
		for _, c := range n.Cols {
			if c.Action == ProjectKeep {
				if !first {
					b.WriteString(", ")
				}
				b.WriteString(c.Name)
				first = false
			}
		}
	}
	if hasRename {
		if b.Len() > 0 {
			b.WriteString(" | ")
		}
		b.WriteString("rename ")
		first := true
		for _, c := range n.Cols {
			if c.Action == ProjectRename {
				if !first {
					b.WriteString(", ")
				}
				b.WriteString(c.From)
				b.WriteString(" as ")
				b.WriteString(c.Name)
				first = false
			}
		}
	}
	return b.String()
}

func renderExtend(n *Extend) string {
	var b strings.Builder
	b.WriteString("extend ")
	for i, a := range n.Assignments {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(a.Name)
		b.WriteString(" = ")
		b.WriteString(format.Expr(a.Value))
	}
	return b.String()
}

func renderAggregate(n *Aggregate) string {
	var b strings.Builder

	// Choose command name based on window variant.
	switch {
	case n.Window != nil && n.Window.Variant == WindowEventstats:
		b.WriteString("eventstats ")
	case n.Window != nil && n.Window.Variant == WindowStreamstats:
		b.WriteString("streamstats ")
		if n.Window.Window != nil {
			b.WriteString(fmt.Sprintf("window=%d ", *n.Window.Window))
		}
		if n.Window.Current != nil {
			if *n.Window.Current {
				b.WriteString("current=true ")
			} else {
				b.WriteString("current=false ")
			}
		}
	default:
		b.WriteString("stats ")
	}

	for i, a := range n.Aggs {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(format.Expr(a.Func))
		if a.Alias != "" {
			b.WriteString(" as ")
			b.WriteString(a.Alias)
		}
	}
	if len(n.Keys) > 0 || n.TimeBin != nil {
		b.WriteString(" by ")
		first := true
		for _, k := range n.Keys {
			if !first {
				b.WriteString(", ")
			}
			if k.Expr != nil {
				b.WriteString(format.Expr(k.Expr))
			} else {
				b.WriteString(k.Name)
			}
			first = false
		}
		if n.TimeBin != nil {
			if !first {
				b.WriteString(", ")
			}
			b.WriteString("bin(_time, ")
			b.WriteString(format.Expr(n.TimeBin.Duration))
			b.WriteByte(')')
		}
	}
	return b.String()
}

func renderSort(n *Sort) string {
	var b strings.Builder
	b.WriteString("sort ")
	for i, k := range n.Keys {
		if i > 0 {
			b.WriteString(", ")
		}
		if k.Desc {
			b.WriteByte('-')
		}
		b.WriteString(format.Expr(k.Expr))
	}
	return b.String()
}

func renderTopK(n *TopK) string {
	// TopK is a fused sort+head. Render as sort + head.
	var b strings.Builder
	b.WriteString("sort ")
	for i, k := range n.SortKeys {
		if i > 0 {
			b.WriteString(", ")
		}
		if k.Desc {
			b.WriteByte('-')
		}
		b.WriteString(format.Expr(k.Expr))
	}
	b.WriteString(fmt.Sprintf(" | head %d", n.K))
	return b.String()
}

func renderLimit(n *Limit) string {
	if n.Tail {
		return fmt.Sprintf("tail %d", n.N)
	}
	return fmt.Sprintf("head %d", n.N)
}

func renderDedup(n *Dedup) string {
	var b strings.Builder
	b.WriteString("dedup ")
	if n.N > 1 {
		b.WriteString(fmt.Sprintf("%d ", n.N))
	}
	for i, f := range n.Fields {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(f)
	}
	return b.String()
}

func renderJoin(n *Join) string {
	var b strings.Builder
	b.WriteString("join ")
	if n.Type != "" && n.Type != "inner" {
		b.WriteString("type=")
		b.WriteString(n.Type)
		b.WriteByte(' ')
	}
	if len(n.On) > 0 {
		b.WriteString("on ")
		b.WriteString(strings.Join(n.On, ", "))
	}
	if n.Right != nil {
		b.WriteString(" with [")
		rightChain := linearize(n.Right)
		for i, rn := range rightChain {
			text := renderNode(rn)
			if text == "" {
				continue
			}
			if i > 0 {
				b.WriteString(" | ")
			}
			b.WriteString(text)
		}
		b.WriteByte(']')
	}
	return b.String()
}

func renderUnion(n *Union) string {
	var b strings.Builder
	b.WriteString("union ")
	for i, inp := range n.Inputs {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteByte('[')
		chain := linearize(inp)
		for j, cn := range chain {
			text := renderNode(cn)
			if text == "" {
				continue
			}
			if j > 0 {
				b.WriteString(" | ")
			}
			b.WriteString(text)
		}
		b.WriteByte(']')
	}
	return b.String()
}

func renderExplode(n *Explode) string {
	var b strings.Builder
	b.WriteString("explode ")
	b.WriteString(n.Field)
	if n.As != "" {
		b.WriteString(" as ")
		b.WriteString(n.As)
	}
	return b.String()
}

func renderHelper(n *Helper) string {
	var b strings.Builder
	b.WriteString(n.Name)
	if len(n.Positional) > 0 || len(n.Options) > 0 {
		b.WriteByte(' ')
	}
	for i, p := range n.Positional {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(format.Expr(p))
	}
	for k, v := range n.Options {
		if len(n.Positional) > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(format.Expr(v))
	}
	return b.String()
}

func renderMaterialize(n *Materialize) string {
	return fmt.Sprintf("materialize %s", quoteStr(n.Name))
}

func quoteStr(s string) string {
	return fmt.Sprintf("%q", s)
}
