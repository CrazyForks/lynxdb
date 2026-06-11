// Package desugar implements the AST -> AST lowering pass for LynxFlow v2
// (RFC-002 Phase 2d). It rewrites sugar-class stages and search-sugar terms
// into core-only stages, producing Rewrite records that document every
// transformation for --show-rewritten / meta.rewrites / EXPLAIN.
//
// The input AST is never mutated; the output is a freshly built tree.
// Running Desugar on already-core output is idempotent (zero rewrites).
package desugar

import (
	"fmt"
	"strings"
	"time"

	"github.com/lynxbase/lynxdb/pkg/lynxflow/ast"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/registry"
)

// Rewrite documents a single desugaring transformation.
type Rewrite struct {
	Before string   // String() of the original sugar construct
	After  string   // String() of the expanded core stages
	Reason string   // stable tag: "implicit-source", "search-sugar", "sugar:top", etc.
	Span   ast.Span // source span of the sugar construct
}

// Options controls desugaring behavior.
type Options struct {
	// DefaultSource is the source name used when the pipeline has no explicit
	// from stage (e.g. "main").
	DefaultSource string
}

// Desugar lowers all sugar in q to core stages. The input q is not mutated.
// The returned Query contains only core, helper, and management stages.
// Each transformation appends a Rewrite to the returned slice.
func Desugar(q *ast.Query, opts Options) (*ast.Query, []Rewrite) {
	d := &desugarer{opts: opts}
	out := d.desugarQuery(q)
	return out, d.rewrites
}

// ---------------------------------------------------------------------------
// Internal desugarer
// ---------------------------------------------------------------------------

type desugarer struct {
	opts     Options
	rewrites []Rewrite
}

func (d *desugarer) addRewrite(before, after, reason string, span ast.Span) {
	d.rewrites = append(d.rewrites, Rewrite{
		Before: before,
		After:  after,
		Reason: reason,
		Span:   span,
	})
}

// ---------------------------------------------------------------------------
// Query / Let / Pipeline
// ---------------------------------------------------------------------------

func (d *desugarer) desugarQuery(q *ast.Query) *ast.Query {
	out := &ast.Query{Pos: q.Pos}
	for _, l := range q.Lets {
		out.Lets = append(out.Lets, d.desugarLet(l))
	}
	out.Pipeline = d.desugarPipeline(q.Pipeline, true)
	return out
}

func (d *desugarer) desugarLet(l ast.Let) ast.Let {
	return ast.Let{
		Name:     l.Name,
		NameSpan: l.NameSpan,
		Pipeline: d.desugarPipeline(l.Pipeline, true),
		Pos:      l.Pos,
	}
}

func (d *desugarer) desugarPipeline(p ast.Pipeline, topLevel bool) ast.Pipeline {
	out := ast.Pipeline{Pos: p.Pos}

	// 1. Implicit source: if no from stage, insert from <default>.
	if p.Source == nil && topLevel && d.opts.DefaultSource != "" {
		synthFrom := &ast.FromStage{
			Sources: []ast.SourceAtom{{
				Kind: ast.SourceName,
				Name: d.opts.DefaultSource,
				Pos:  ast.Span{Start: p.Pos.Start, End: p.Pos.Start},
			}},
			Pos: ast.Span{Start: p.Pos.Start, End: p.Pos.Start},
		}
		out.Source = synthFrom
		d.addRewrite("",
			synthFrom.String(),
			"implicit-source",
			p.Pos,
		)
	} else if p.Source != nil {
		// Desugar from: strip sugar terms and insert a where stage.
		out.Source = d.desugarFrom(p.Source)
		sugarWhere := d.desugarFromSugarTerms(p.Source)
		out.Stages = append(out.Stages, sugarWhere...)
	}

	// 2. Desugar stages one at a time; sugar stages may expand to multiple stages.
	// stageIdx refers to the index in the ORIGINAL pipeline stages, used by
	// facets to find prefix stages.
	for i, s := range p.Stages {
		expanded := d.desugarStage(s, p, i)
		out.Stages = append(out.Stages, expanded...)
	}

	return out
}

// ---------------------------------------------------------------------------
// From stage: extract search sugar terms into a where stage
// ---------------------------------------------------------------------------

func (d *desugarer) desugarFrom(f *ast.FromStage) *ast.FromStage {
	// Clone the from stage without sugar terms.
	out := &ast.FromStage{
		Sources:    cloneSourceAtoms(f.Sources),
		TimeRanges: cloneTimeRanges(f.TimeRanges),
		Pos:        f.Pos,
		// SugarTerms left nil — we extract them to a where stage.
	}
	return out
}

func (d *desugarer) desugarFromSugarTerms(f *ast.FromStage) []ast.Stage {
	if f == nil || f.SugarTerms == nil {
		return nil
	}
	expr := d.desugarSearchExpr(f.SugarTerms)
	if expr == nil {
		return nil
	}

	whereStage := ast.Stage{
		Name:    "where",
		NamePos: f.SugarTerms.SearchExprSpan(),
		Where:   &ast.WherePayload{Expr: expr},
		Pos:     f.SugarTerms.SearchExprSpan(),
	}

	d.addRewrite(
		f.SugarTerms.String(),
		whereStage.String(),
		"search-sugar",
		f.SugarTerms.SearchExprSpan(),
	)

	return []ast.Stage{whereStage}
}

// desugarSearchExpr converts a SearchExpr tree to a regular Expr tree per §3.1.
func (d *desugarer) desugarSearchExpr(se ast.SearchExpr) ast.Expr {
	if se == nil {
		return nil
	}
	switch s := se.(type) {
	case *ast.SearchBareWord:
		// bare word -> has(_raw, "word")
		return &ast.Call{
			Callee: "has",
			Args: []ast.Expr{
				&ast.Ident{Name: "_raw", Pos: s.Pos},
				&ast.Literal{Kind: ast.LitString, Raw: fmt.Sprintf("%q", s.Word), Value: s.Word, Pos: s.Pos},
			},
			Pos: s.Pos,
		}

	case *ast.SearchPhrase:
		// quoted phrase -> contains(_raw, "phrase")
		return &ast.Call{
			Callee: "contains",
			Args: []ast.Expr{
				&ast.Ident{Name: "_raw", Pos: s.Pos},
				&ast.Literal{Kind: ast.LitString, Raw: fmt.Sprintf("%q", s.Text), Value: s.Text, Pos: s.Pos},
			},
			Pos: s.Pos,
		}

	case *ast.SearchKeyValue:
		return d.desugarKeyValue(s)

	case *ast.SearchIn:
		// key in (a, b) -> key in [a, b]
		elems := make([]ast.Expr, len(s.Values))
		for i, v := range s.Values {
			elems[i] = cloneExpr(v)
		}
		return &ast.In{
			LHS: &ast.Ident{Name: s.Key, Pos: s.Pos},
			RHS: &ast.Array{Elems: elems, Pos: s.Pos},
			Pos: s.Pos,
		}

	case *ast.SearchBinary:
		left := d.desugarSearchExpr(s.Left)
		right := d.desugarSearchExpr(s.Right)
		if left == nil || right == nil {
			if left != nil {
				return left
			}
			return right
		}
		op := ast.OpAnd
		if s.Op == "or" {
			op = ast.OpOr
		}
		return &ast.Binary{
			Op:    op,
			Left:  left,
			Right: right,
			Pos:   s.Pos,
		}

	case *ast.SearchNot:
		operand := d.desugarSearchExpr(s.Operand)
		if operand == nil {
			return nil
		}
		return &ast.Unary{
			Op:      ast.OpNot,
			Operand: operand,
			Pos:     s.Pos,
		}

	case *ast.SearchParen:
		inner := d.desugarSearchExpr(s.Inner)
		if inner == nil {
			return nil
		}
		return &ast.Paren{
			Inner: inner,
			Pos:   s.Pos,
		}
	}
	return nil
}

func (d *desugarer) desugarKeyValue(kv *ast.SearchKeyValue) ast.Expr {
	key := &ast.Ident{Name: kv.Key, Pos: kv.Pos}
	val := cloneExpr(kv.Value)

	// Glob value -> glob(key, "pattern")
	if gv, ok := kv.Value.(*ast.SearchGlobValue); ok && kv.Op == "=" {
		return &ast.Call{
			Callee: "glob",
			Args: []ast.Expr{
				key,
				&ast.Literal{Kind: ast.LitString, Raw: fmt.Sprintf("%q", gv.Pattern), Value: gv.Pattern, Pos: gv.Pos},
			},
			Pos: kv.Pos,
		}
	}

	// Map search ops to binary ops.
	var op ast.BinaryOp
	switch kv.Op {
	case "=":
		op = ast.OpEq
	case "!=":
		op = ast.OpNotEq
	case "<":
		op = ast.OpLt
	case "<=":
		op = ast.OpLtEq
	case ">":
		op = ast.OpGt
	case ">=":
		op = ast.OpGtEq
	default:
		op = ast.OpEq
	}

	return &ast.Binary{
		Op:    op,
		Left:  key,
		Right: val,
		Pos:   kv.Pos,
	}
}

// ---------------------------------------------------------------------------
// Stage desugaring dispatch
// ---------------------------------------------------------------------------

func (d *desugarer) desugarStage(s ast.Stage, pip ast.Pipeline, stageIdx int) []ast.Stage {
	// Check if this is a sugar stage.
	op, found := registry.LookupOperator(s.Name)
	if found && op.Class == registry.ClassSugar {
		return d.expandSugar(s, pip, stageIdx)
	}

	// Non-sugar: clone the stage. If it contains sub-pipelines (join, union),
	// recurse into those.
	return []ast.Stage{d.cloneStage(s)}
}

func (d *desugarer) expandSugar(s ast.Stage, pip ast.Pipeline, stageIdx int) []ast.Stage {
	switch s.Name {
	case "top":
		return d.expandTopRare(s, true)
	case "rare":
		return d.expandTopRare(s, false)
	case "every":
		return d.expandEvery(s)
	case "rate":
		return d.expandRate(s)
	case "latency":
		return d.expandLatency(s)
	case "percentiles":
		return d.expandPercentiles(s)
	case "proportion":
		return d.expandProportion(s)
	case "facets":
		return d.expandFacets(s, pip, stageIdx)
	case "impact":
		return d.expandImpact(s)
	case "baseline":
		return d.expandBaseline(s)
	case "changes":
		return d.expandChanges(s)
	case "exemplars":
		return d.expandExemplars(s)
	}

	// Unknown sugar — pass through (should not happen).
	return []ast.Stage{d.cloneStage(s)}
}

// ---------------------------------------------------------------------------
// top / rare
// ---------------------------------------------------------------------------

func (d *desugarer) expandTopRare(s ast.Stage, isTop bool) []ast.Stage {
	var payload *ast.TopRarePayload
	if isTop {
		payload = s.Top
	} else {
		payload = s.Rare
	}
	if payload == nil {
		return []ast.Stage{d.cloneStage(s)}
	}

	n := int64(10)
	if payload.N != nil {
		n = *payload.N
	}

	field := cloneExpr(payload.Field)

	// stats count() as count by <field>
	statsStage := ast.Stage{
		Name:    "stats",
		NamePos: s.NamePos,
		Stats: &ast.StatsPayload{
			Aggs: []ast.AggExpr{{
				Func:  &ast.Call{Callee: "count", Pos: s.Pos},
				Alias: "count",
				Pos:   s.Pos,
			}},
			By: []ast.Expr{field},
		},
		Pos: s.Pos,
	}

	// eventstats sum(count) as _total — compute grand total across all rows
	eventstatsStage := ast.Stage{
		Name:    "eventstats",
		NamePos: s.NamePos,
		Eventstats: &ast.StatsPayload{
			Aggs: []ast.AggExpr{{
				Func: &ast.Call{
					Callee: "sum",
					Args:   []ast.Expr{&ast.Ident{Name: "count", Pos: s.Pos}},
					Pos:    s.Pos,
				},
				Alias: "_total",
				Pos:   s.Pos,
			}},
		},
		Pos: s.Pos,
	}

	// extend percent = round(count * 100.0 / _total, 2)
	extendStage := ast.Stage{
		Name:    "extend",
		NamePos: s.NamePos,
		Extend: &ast.AssignPayload{
			Assignments: []ast.Assignment{{
				Name: "percent",
				Value: &ast.Call{
					Callee: "round",
					Args: []ast.Expr{
						&ast.Binary{
							Op: ast.OpDiv,
							Left: &ast.Binary{
								Op:    ast.OpMul,
								Left:  &ast.Ident{Name: "count", Pos: s.Pos},
								Right: &ast.Literal{Kind: ast.LitFloat, Raw: "100.0", Value: 100.0, Pos: s.Pos},
								Pos:   s.Pos,
							},
							Right: &ast.Ident{Name: "_total", Pos: s.Pos},
							Pos:   s.Pos,
						},
						&ast.Literal{Kind: ast.LitInt, Raw: "2", Value: int64(2), Pos: s.Pos},
					},
					Pos: s.Pos,
				},
				Pos: s.Pos,
			}},
		},
		Pos: s.Pos,
	}

	// drop _total — remove internal helper field
	dropStage := ast.Stage{
		Name:    "drop",
		NamePos: s.NamePos,
		Drop: &ast.FieldPatternsPayload{
			Patterns: []ast.FieldPattern{{Name: "_total", Pos: s.Pos}},
		},
		Pos: s.Pos,
	}

	// sort -count / sort +count
	sortKey := ast.SortKey{Field: &ast.Ident{Name: "count", Pos: s.Pos}, Desc: isTop, Pos: s.Pos}
	sortStage := ast.Stage{
		Name:    "sort",
		NamePos: s.NamePos,
		Sort:    &ast.SortPayload{Keys: []ast.SortKey{sortKey}},
		Pos:     s.Pos,
	}

	// head N
	headStage := ast.Stage{
		Name:    "head",
		NamePos: s.NamePos,
		Head:    &ast.IntPayload{N: n, Pos: s.Pos},
		Pos:     s.Pos,
	}

	result := []ast.Stage{statsStage, eventstatsStage, extendStage, dropStage, sortStage, headStage}
	reason := "sugar:top"
	if !isTop {
		reason = "sugar:rare"
	}
	d.addRewrite(s.String(), renderStages(result), reason, s.Pos)
	return result
}

// ---------------------------------------------------------------------------
// every
// ---------------------------------------------------------------------------

func (d *desugarer) expandEvery(s ast.Stage) []ast.Stage {
	ev := s.Every
	if ev == nil {
		return []ast.Stage{d.cloneStage(s)}
	}

	// Build by list: [user keys...,] bin(_time, <span>)
	by := cloneExprs(ev.By)
	binCall := &ast.Call{
		Callee: "bin",
		Args: []ast.Expr{
			&ast.Ident{Name: "_time", Pos: s.Pos},
			cloneExpr(ev.Span),
		},
		Pos: s.Pos,
	}
	by = append(by, binCall)

	// stats <aggs> by <keys>
	statsStage := ast.Stage{
		Name:    "stats",
		NamePos: s.NamePos,
		Stats: &ast.StatsPayload{
			Aggs: cloneAggs(ev.Aggs),
			By:   by,
		},
		Pos: s.Pos,
	}

	result := []ast.Stage{statsStage}
	d.addRewrite(s.String(), renderStages(result), "sugar:every", s.Pos)
	return result
}

// ---------------------------------------------------------------------------
// rate
// ---------------------------------------------------------------------------

func (d *desugarer) expandRate(s ast.Stage) []ast.Stage {
	r := s.Rate
	if r == nil {
		return []ast.Stage{d.cloneStage(s)}
	}

	// Default per = 1m
	per := r.Per
	if per == nil {
		per = &ast.Literal{Kind: ast.LitDuration, Raw: "1m", Value: time.Minute, Pos: s.Pos}
	} else {
		per = cloneExpr(per)
	}

	// Build by list: [user keys...,] bin(_time, <per>)
	by := cloneExprs(r.By)
	binCall := &ast.Call{
		Callee: "bin",
		Args: []ast.Expr{
			&ast.Ident{Name: "_time", Pos: s.Pos},
			per,
		},
		Pos: s.Pos,
	}
	by = append(by, binCall)

	// stats count() as rate by <keys>
	statsStage := ast.Stage{
		Name:    "stats",
		NamePos: s.NamePos,
		Stats: &ast.StatsPayload{
			Aggs: []ast.AggExpr{{
				Func:  &ast.Call{Callee: "count", Pos: s.Pos},
				Alias: "rate",
				Pos:   s.Pos,
			}},
			By: by,
		},
		Pos: s.Pos,
	}

	result := []ast.Stage{statsStage}
	d.addRewrite(s.String(), renderStages(result), "sugar:rate", s.Pos)
	return result
}

// ---------------------------------------------------------------------------
// latency
// ---------------------------------------------------------------------------

func (d *desugarer) expandLatency(s ast.Stage) []ast.Stage {
	l := s.Latency
	if l == nil {
		return []ast.Stage{d.cloneStage(s)}
	}

	field := cloneExpr(l.Field)

	aggs := []ast.AggExpr{
		{Func: &ast.Call{Callee: "p50", Args: []ast.Expr{cloneExpr(field)}, Pos: s.Pos}, Pos: s.Pos},
		{Func: &ast.Call{Callee: "p95", Args: []ast.Expr{cloneExpr(field)}, Pos: s.Pos}, Pos: s.Pos},
		{Func: &ast.Call{Callee: "p99", Args: []ast.Expr{cloneExpr(field)}, Pos: s.Pos}, Pos: s.Pos},
		{Func: &ast.Call{Callee: "count", Pos: s.Pos}, Pos: s.Pos},
	}

	by := cloneExprs(l.By)
	if l.Every != nil {
		binCall := &ast.Call{
			Callee: "bin",
			Args: []ast.Expr{
				&ast.Ident{Name: "_time", Pos: s.Pos},
				cloneExpr(l.Every),
			},
			Pos: s.Pos,
		}
		by = append(by, binCall)
	}

	statsStage := ast.Stage{
		Name:    "stats",
		NamePos: s.NamePos,
		Stats: &ast.StatsPayload{
			Aggs: aggs,
			By:   by,
		},
		Pos: s.Pos,
	}

	result := []ast.Stage{statsStage}
	d.addRewrite(s.String(), renderStages(result), "sugar:latency", s.Pos)
	return result
}

// ---------------------------------------------------------------------------
// percentiles
// ---------------------------------------------------------------------------

func (d *desugarer) expandPercentiles(s ast.Stage) []ast.Stage {
	p := s.Percentiles
	if p == nil {
		return []ast.Stage{d.cloneStage(s)}
	}

	field := cloneExpr(p.Field)
	fieldName := sanitizeFieldName(exprFieldName(p.Field))

	percentiles := []struct {
		fn    string
		alias string
	}{
		{"p50", "p50_" + fieldName},
		{"p75", "p75_" + fieldName},
		{"p90", "p90_" + fieldName},
		{"p95", "p95_" + fieldName},
		{"p99", "p99_" + fieldName},
	}

	aggs := make([]ast.AggExpr, len(percentiles))
	for i, pc := range percentiles {
		aggs[i] = ast.AggExpr{
			Func:  &ast.Call{Callee: pc.fn, Args: []ast.Expr{cloneExpr(field)}, Pos: s.Pos},
			Alias: pc.alias,
			Pos:   s.Pos,
		}
	}

	statsStage := ast.Stage{
		Name:    "stats",
		NamePos: s.NamePos,
		Stats: &ast.StatsPayload{
			Aggs: aggs,
			By:   cloneExprs(p.By),
		},
		Pos: s.Pos,
	}

	result := []ast.Stage{statsStage}
	d.addRewrite(s.String(), renderStages(result), "sugar:percentiles", s.Pos)
	return result
}

// ---------------------------------------------------------------------------
// proportion
// ---------------------------------------------------------------------------

func (d *desugarer) expandProportion(s ast.Stage) []ast.Stage {
	pr := s.Proportion
	if pr == nil {
		return []ast.Stage{d.cloneStage(s)}
	}

	name := pr.Alias
	numAlias := name + "_num"
	denAlias := name + "_den"

	by := cloneExprs(pr.By)
	if pr.Every != nil {
		binCall := &ast.Call{
			Callee: "bin",
			Args: []ast.Expr{
				&ast.Ident{Name: "_time", Pos: s.Pos},
				cloneExpr(pr.Every),
			},
			Pos: s.Pos,
		}
		by = append(by, binCall)
	}

	// stats count(where <pred>) as <name>_num, count() as <name>_den [by ...]
	statsStage := ast.Stage{
		Name:    "stats",
		NamePos: s.NamePos,
		Stats: &ast.StatsPayload{
			Aggs: []ast.AggExpr{
				{
					Func:      &ast.Call{Callee: "count", Pos: s.Pos},
					WhereCond: cloneExpr(pr.Predicate),
					Alias:     numAlias,
					Pos:       s.Pos,
				},
				{
					Func:  &ast.Call{Callee: "count", Pos: s.Pos},
					Alias: denAlias,
					Pos:   s.Pos,
				},
			},
			By: by,
		},
		Pos: s.Pos,
	}

	// extend <name> = <name>_num / <name>_den
	extendStage := ast.Stage{
		Name:    "extend",
		NamePos: s.NamePos,
		Extend: &ast.AssignPayload{
			Assignments: []ast.Assignment{{
				Name: name,
				Value: &ast.Binary{
					Op:    ast.OpDiv,
					Left:  &ast.Ident{Name: numAlias, Pos: s.Pos},
					Right: &ast.Ident{Name: denAlias, Pos: s.Pos},
					Pos:   s.Pos,
				},
				Pos: s.Pos,
			}},
		},
		Pos: s.Pos,
	}

	result := []ast.Stage{statsStage, extendStage}
	d.addRewrite(s.String(), renderStages(result), "sugar:proportion", s.Pos)
	return result
}

// ---------------------------------------------------------------------------
// facets
// ---------------------------------------------------------------------------

func (d *desugarer) expandFacets(s ast.Stage, pip ast.Pipeline, stageIdx int) []ast.Stage {
	f := s.Facets
	if f == nil || len(f.Fields) == 0 {
		return []ast.Stage{d.cloneStage(s)}
	}

	limit := int64(10)
	if f.Limit != nil {
		limit = *f.Limit
	}

	// Prefix stages: all stages before the facets stage in the pipeline.
	// These need to be cloned into each union branch.
	var prefixStages []ast.Stage
	for i := 0; i < stageIdx; i++ {
		prefixStages = append(prefixStages, d.cloneStage(pip.Stages[i]))
	}

	// Clone the from stage for sub-pipelines.
	var fromClone *ast.FromStage
	if pip.Source != nil {
		fromClone = d.desugarFrom(pip.Source)
	}

	// Build per-field pipelines.
	var facetPipelines []facetBranch
	for _, fieldExpr := range f.Fields {
		fieldName := exprFieldName(fieldExpr)
		branch := d.buildFacetBranch(fieldExpr, fieldName, limit, s.Pos)
		facetPipelines = append(facetPipelines, branch)
	}

	if len(facetPipelines) == 0 {
		return []ast.Stage{d.cloneStage(s)}
	}

	// First field: inline its stages.
	// Remaining fields: union sub-pipelines with prefix + branch stages.
	result := facetPipelines[0].stages

	if len(facetPipelines) > 1 {
		var unionSources []ast.SubPipeline
		for _, branch := range facetPipelines[1:] {
			subPip := ast.Pipeline{
				Source: cloneFromStage(fromClone),
				Pos:    s.Pos,
			}
			// Add prefix stages (before facets).
			for _, ps := range prefixStages {
				subPip.Stages = append(subPip.Stages, d.cloneStage(ps))
			}
			// Add search-sugar derived where if original from had sugar terms.
			if pip.Source != nil && pip.Source.SugarTerms != nil {
				whereSugar := d.buildSearchSugarWhere(pip.Source)
				if whereSugar != nil {
					subPip.Stages = append(subPip.Stages, *whereSugar)
				}
			}
			subPip.Stages = append(subPip.Stages, branch.stages...)
			unionSources = append(unionSources, ast.SubPipeline{
				Pipeline: &subPip,
				Pos:      s.Pos,
			})
		}

		unionStage := ast.Stage{
			Name:    "union",
			NamePos: s.NamePos,
			Union:   &ast.UnionPayload{Sources: unionSources},
			Pos:     s.Pos,
		}
		result = append(result, unionStage)
	}

	d.addRewrite(s.String(), renderStages(result), "sugar:facets", s.Pos)
	return result
}

type facetBranch struct {
	stages []ast.Stage
}

func (d *desugarer) buildFacetBranch(fieldExpr ast.Expr, fieldName string, limit int64, span ast.Span) facetBranch {
	field := cloneExpr(fieldExpr)

	// stats count() as count by <field>
	statsStage := ast.Stage{
		Name:    "stats",
		NamePos: span,
		Stats: &ast.StatsPayload{
			Aggs: []ast.AggExpr{{
				Func:  &ast.Call{Callee: "count", Pos: span},
				Alias: "count",
				Pos:   span,
			}},
			By: []ast.Expr{field},
		},
		Pos: span,
	}

	// sort -count
	sortStage := ast.Stage{
		Name:    "sort",
		NamePos: span,
		Sort: &ast.SortPayload{
			Keys: []ast.SortKey{{
				Field: &ast.Ident{Name: "count", Pos: span},
				Desc:  true,
				Pos:   span,
			}},
		},
		Pos: span,
	}

	// head <limit>
	headStage := ast.Stage{
		Name:    "head",
		NamePos: span,
		Head:    &ast.IntPayload{N: limit, Pos: span},
		Pos:     span,
	}

	// extend _facet = "<field>", _value = string(<field>)
	extendStage := ast.Stage{
		Name:    "extend",
		NamePos: span,
		Extend: &ast.AssignPayload{
			Assignments: []ast.Assignment{
				{
					Name:  "_facet",
					Value: &ast.Literal{Kind: ast.LitString, Raw: fmt.Sprintf("%q", fieldName), Value: fieldName, Pos: span},
					Pos:   span,
				},
				{
					Name: "_value",
					Value: &ast.Call{
						Callee: "string",
						Args:   []ast.Expr{cloneExpr(fieldExpr)},
						Pos:    span,
					},
					Pos: span,
				},
			},
		},
		Pos: span,
	}

	// keep _facet, _value, count
	keepStage := ast.Stage{
		Name:    "keep",
		NamePos: span,
		Keep: &ast.FieldPatternsPayload{
			Patterns: []ast.FieldPattern{
				{Name: "_facet", Pos: span},
				{Name: "_value", Pos: span},
				{Name: "count", Pos: span},
			},
		},
		Pos: span,
	}

	return facetBranch{stages: []ast.Stage{statsStage, sortStage, headStage, extendStage, keepStage}}
}

func (d *desugarer) buildSearchSugarWhere(f *ast.FromStage) *ast.Stage {
	if f == nil || f.SugarTerms == nil {
		return nil
	}
	expr := d.desugarSearchExpr(f.SugarTerms)
	if expr == nil {
		return nil
	}
	return &ast.Stage{
		Name:    "where",
		NamePos: f.SugarTerms.SearchExprSpan(),
		Where:   &ast.WherePayload{Expr: expr},
		Pos:     f.SugarTerms.SearchExprSpan(),
	}
}

// ---------------------------------------------------------------------------
// impact
// ---------------------------------------------------------------------------

func (d *desugarer) expandImpact(s ast.Stage) []ast.Stage {
	im := s.Impact
	if im == nil {
		return []ast.Stage{d.cloneStage(s)}
	}

	// Default agg: count(); naming: count() -> "n", sum(bytes) -> "sum_bytes"
	var agg ast.AggExpr
	var vName string
	if im.Agg != nil {
		agg = cloneAgg(*im.Agg)
		vName = impactVarName(agg)
	} else {
		agg = ast.AggExpr{
			Func: &ast.Call{Callee: "count", Pos: s.Pos},
			Pos:  s.Pos,
		}
		vName = "n"
	}
	agg.Alias = vName

	totalName := "total_" + vName
	pctName := "pct_" + vName

	// stats <agg> as v by keys
	statsStage := ast.Stage{
		Name:    "stats",
		NamePos: s.NamePos,
		Stats: &ast.StatsPayload{
			Aggs: []ast.AggExpr{agg},
			By:   cloneExprs(im.By),
		},
		Pos: s.Pos,
	}

	// eventstats sum(v) as total_v
	eventstatsStage := ast.Stage{
		Name:    "eventstats",
		NamePos: s.NamePos,
		Eventstats: &ast.StatsPayload{
			Aggs: []ast.AggExpr{{
				Func:  &ast.Call{Callee: "sum", Args: []ast.Expr{&ast.Ident{Name: vName, Pos: s.Pos}}, Pos: s.Pos},
				Alias: totalName,
				Pos:   s.Pos,
			}},
		},
		Pos: s.Pos,
	}

	// extend pct_v = v / total_v
	extendStage := ast.Stage{
		Name:    "extend",
		NamePos: s.NamePos,
		Extend: &ast.AssignPayload{
			Assignments: []ast.Assignment{{
				Name: pctName,
				Value: &ast.Binary{
					Op:    ast.OpDiv,
					Left:  &ast.Ident{Name: vName, Pos: s.Pos},
					Right: &ast.Ident{Name: totalName, Pos: s.Pos},
					Pos:   s.Pos,
				},
				Pos: s.Pos,
			}},
		},
		Pos: s.Pos,
	}

	// sort -pct_v
	sortStage := ast.Stage{
		Name:    "sort",
		NamePos: s.NamePos,
		Sort: &ast.SortPayload{
			Keys: []ast.SortKey{{
				Field: &ast.Ident{Name: pctName, Pos: s.Pos},
				Desc:  true,
				Pos:   s.Pos,
			}},
		},
		Pos: s.Pos,
	}

	result := []ast.Stage{statsStage, eventstatsStage, extendStage, sortStage}
	d.addRewrite(s.String(), renderStages(result), "sugar:impact", s.Pos)
	return result
}

func impactVarName(agg ast.AggExpr) string {
	// count() -> "n"; sum(bytes) -> "sum_bytes"
	if call, ok := agg.Func.(*ast.Call); ok {
		if call.Callee == "count" && len(call.Args) == 0 {
			return "n"
		}
		if len(call.Args) > 0 {
			argName := exprFieldName(call.Args[0])
			if argName != "" {
				return call.Callee + "_" + argName
			}
		}
		return call.Callee
	}
	return "v"
}

// ---------------------------------------------------------------------------
// baseline
// ---------------------------------------------------------------------------

func (d *desugarer) expandBaseline(s ast.Stage) []ast.Stage {
	bl := s.Baseline
	if bl == nil {
		return []ast.Stage{d.cloneStage(s)}
	}

	field := cloneExpr(bl.Field)
	fieldName := exprFieldName(bl.Field)
	baselineName := "baseline_" + fieldName
	stdevName := "stdev_" + fieldName
	deltaName := "delta_" + fieldName
	zName := "z_" + fieldName
	window := int(bl.Window)
	currentFalse := false

	// streamstats current=false window=N avg(f) as baseline_f, stdev(f) as stdev_f [by keys]
	streamstatsStage := ast.Stage{
		Name:    "streamstats",
		NamePos: s.NamePos,
		Streamstats: &ast.StreamstatsPayload{
			StatsPayload: ast.StatsPayload{
				Aggs: []ast.AggExpr{
					{
						Func:  &ast.Call{Callee: "avg", Args: []ast.Expr{cloneExpr(field)}, Pos: s.Pos},
						Alias: baselineName,
						Pos:   s.Pos,
					},
					{
						Func:  &ast.Call{Callee: "stdev", Args: []ast.Expr{cloneExpr(field)}, Pos: s.Pos},
						Alias: stdevName,
						Pos:   s.Pos,
					},
				},
				By: cloneExprs(bl.By),
			},
			Window:  &window,
			Current: &currentFalse,
		},
		Pos: s.Pos,
	}

	// extend delta_f = f - baseline_f, z_f = if(stdev_f > 0, delta_f / stdev_f, null)
	extendStage := ast.Stage{
		Name:    "extend",
		NamePos: s.NamePos,
		Extend: &ast.AssignPayload{
			Assignments: []ast.Assignment{
				{
					Name: deltaName,
					Value: &ast.Binary{
						Op:    ast.OpSub,
						Left:  cloneExpr(field),
						Right: &ast.Ident{Name: baselineName, Pos: s.Pos},
						Pos:   s.Pos,
					},
					Pos: s.Pos,
				},
				{
					Name: zName,
					Value: &ast.Call{
						Callee: "if",
						Args: []ast.Expr{
							&ast.Binary{
								Op:    ast.OpGt,
								Left:  &ast.Ident{Name: stdevName, Pos: s.Pos},
								Right: &ast.Literal{Kind: ast.LitInt, Raw: "0", Value: int64(0), Pos: s.Pos},
								Pos:   s.Pos,
							},
							&ast.Binary{
								Op:    ast.OpDiv,
								Left:  &ast.Ident{Name: deltaName, Pos: s.Pos},
								Right: &ast.Ident{Name: stdevName, Pos: s.Pos},
								Pos:   s.Pos,
							},
							&ast.Literal{Kind: ast.LitNull, Raw: "null", Pos: s.Pos},
						},
						Pos: s.Pos,
					},
					Pos: s.Pos,
				},
			},
		},
		Pos: s.Pos,
	}

	result := []ast.Stage{streamstatsStage, extendStage}
	d.addRewrite(s.String(), renderStages(result), "sugar:baseline", s.Pos)
	return result
}

// ---------------------------------------------------------------------------
// changes
// ---------------------------------------------------------------------------

func (d *desugarer) expandChanges(s ast.Stage) []ast.Stage {
	ch := s.Changes
	if ch == nil {
		return []ast.Stage{d.cloneStage(s)}
	}

	field := cloneExpr(ch.Field)
	fieldName := exprFieldName(ch.Field)
	previousName := "previous_" + fieldName
	currentFalse := false

	// sort +_time
	sortStage := ast.Stage{
		Name:    "sort",
		NamePos: s.NamePos,
		Sort: &ast.SortPayload{
			Keys: []ast.SortKey{{
				Field: &ast.Ident{Name: "_time", Pos: s.Pos},
				Desc:  false,
				Pos:   s.Pos,
			}},
		},
		Pos: s.Pos,
	}

	// streamstats current=false last(f) as previous_f [by keys]
	streamstatsStage := ast.Stage{
		Name:    "streamstats",
		NamePos: s.NamePos,
		Streamstats: &ast.StreamstatsPayload{
			StatsPayload: ast.StatsPayload{
				Aggs: []ast.AggExpr{{
					Func:  &ast.Call{Callee: "last", Args: []ast.Expr{cloneExpr(field)}, Pos: s.Pos},
					Alias: previousName,
					Pos:   s.Pos,
				}},
				By: cloneExprs(ch.By),
			},
			Current: &currentFalse,
		},
		Pos: s.Pos,
	}

	// where exists(previous_f) and f != previous_f
	whereStage := ast.Stage{
		Name:    "where",
		NamePos: s.NamePos,
		Where: &ast.WherePayload{
			Expr: &ast.Binary{
				Op: ast.OpAnd,
				Left: &ast.Call{
					Callee: "exists",
					Args:   []ast.Expr{&ast.Ident{Name: previousName, Pos: s.Pos}},
					Pos:    s.Pos,
				},
				Right: &ast.Binary{
					Op:    ast.OpNotEq,
					Left:  cloneExpr(field),
					Right: &ast.Ident{Name: previousName, Pos: s.Pos},
					Pos:   s.Pos,
				},
				Pos: s.Pos,
			},
		},
		Pos: s.Pos,
	}

	result := []ast.Stage{sortStage, streamstatsStage, whereStage}
	d.addRewrite(s.String(), renderStages(result), "sugar:changes", s.Pos)
	return result
}

// ---------------------------------------------------------------------------
// exemplars
// ---------------------------------------------------------------------------

func (d *desugarer) expandExemplars(s ast.Stage) []ast.Stage {
	ex := s.Exemplars
	if ex == nil {
		return []ast.Stage{d.cloneStage(s)}
	}

	n := int64(3)
	if ex.N != nil {
		n = *ex.N
	}

	// sort -_time
	sortStage := ast.Stage{
		Name:    "sort",
		NamePos: s.NamePos,
		Sort: &ast.SortPayload{
			Keys: []ast.SortKey{{
				Field: &ast.Ident{Name: "_time", Pos: s.Pos},
				Desc:  true,
				Pos:   s.Pos,
			}},
		},
		Pos: s.Pos,
	}

	var result []ast.Stage
	if len(ex.By) > 0 {
		// dedup N keys
		dedupStage := ast.Stage{
			Name:    "dedup",
			NamePos: s.NamePos,
			Dedup: &ast.DedupPayload{
				N:      n,
				Fields: cloneExprs(ex.By),
			},
			Pos: s.Pos,
		}
		result = []ast.Stage{sortStage, dedupStage}
	} else {
		// global: head N
		headStage := ast.Stage{
			Name:    "head",
			NamePos: s.NamePos,
			Head:    &ast.IntPayload{N: n, Pos: s.Pos},
			Pos:     s.Pos,
		}
		result = []ast.Stage{sortStage, headStage}
	}

	d.addRewrite(s.String(), renderStages(result), "sugar:exemplars", s.Pos)
	return result
}

// ---------------------------------------------------------------------------
// Stage cloning (for non-sugar stages with sub-pipelines)
// ---------------------------------------------------------------------------

func (d *desugarer) cloneStage(s ast.Stage) ast.Stage {
	out := ast.Stage{
		Name:     s.Name,
		NamePos:  s.NamePos,
		Pos:      s.Pos,
		HasError: s.HasError,
	}

	// Clone each payload type. Most are pass-through; join/union need recursion.
	switch {
	case s.Where != nil:
		out.Where = &ast.WherePayload{Expr: cloneExpr(s.Where.Expr)}
	case s.Extend != nil:
		out.Extend = cloneAssignPayload(s.Extend)
	case s.Rename != nil:
		out.Rename = cloneRenamePayload(s.Rename)
	case s.Stats != nil:
		out.Stats = cloneStatsPayload(s.Stats)
	case s.Eventstats != nil:
		out.Eventstats = cloneStatsPayload(s.Eventstats)
	case s.Streamstats != nil:
		out.Streamstats = cloneStreamstatsPayload(s.Streamstats)
	case s.Sort != nil:
		out.Sort = cloneSortPayload(s.Sort)
	case s.Head != nil:
		out.Head = &ast.IntPayload{N: s.Head.N, Pos: s.Head.Pos}
	case s.Tail != nil:
		out.Tail = &ast.IntPayload{N: s.Tail.N, Pos: s.Tail.Pos}
	case s.Dedup != nil:
		out.Dedup = &ast.DedupPayload{N: s.Dedup.N, Fields: cloneExprs(s.Dedup.Fields)}
	case s.Keep != nil:
		out.Keep = cloneFieldPatternsPayload(s.Keep)
	case s.Drop != nil:
		out.Drop = cloneFieldPatternsPayload(s.Drop)
	case s.Join != nil:
		out.Join = d.cloneJoinPayload(s.Join)
	case s.Union != nil:
		out.Union = d.cloneUnionPayload(s.Union)
	case s.Explode != nil:
		out.Explode = &ast.ExplodePayload{Array: cloneExpr(s.Explode.Array), As: s.Explode.As, AsPos: s.Explode.AsPos}
	case s.Describe != nil:
		out.Describe = &ast.DescribePayload{}
	case s.Parse != nil:
		out.Parse = cloneParsePayload(s.Parse)
	case s.Materialize != nil:
		out.Materialize = cloneMaterializePayload(s.Materialize)
	case s.Tee != nil:
		out.Tee = &ast.TeePayload{Sink: s.Tee.Sink}
	case s.Use != nil:
		out.Use = &ast.UsePayload{Fragment: s.Use.Fragment}
	case s.Compare != nil:
		out.Compare = cloneComparePayload(s.Compare)
	case s.Transaction != nil:
		out.Transaction = cloneTransactionPayload(s.Transaction)
	case s.Correlate != nil:
		out.Correlate = cloneCorrelatePayload(s.Correlate)
	case s.Rollup != nil:
		out.Rollup = cloneRollupPayload(s.Rollup)
	case s.Xyseries != nil:
		out.Xyseries = cloneXYSeriesPayload(s.Xyseries)
	case s.Patterns != nil:
		out.Patterns = cloneGenericOptionsPayload(s.Patterns)
	case s.Outliers != nil:
		out.Outliers = cloneGenericOptionsPayload(s.Outliers)
	case s.Sessionize != nil:
		out.Sessionize = cloneGenericOptionsPayload(s.Sessionize)
	case s.Trace != nil:
		out.Trace = cloneGenericOptionsPayload(s.Trace)
	case s.Topology != nil:
		out.Topology = cloneGenericOptionsPayload(s.Topology)
	case s.Generic != nil:
		out.Generic = cloneGenericOptionsPayload(s.Generic)
	}

	return out
}

func (d *desugarer) cloneJoinPayload(j *ast.JoinPayload) *ast.JoinPayload {
	out := &ast.JoinPayload{
		Type:     j.Type,
		TypeSpan: j.TypeSpan,
		On:       cloneExprs(j.On),
	}
	if j.Right != nil {
		r := d.cloneSubPipeline(*j.Right)
		out.Right = &r
	}
	return out
}

func (d *desugarer) cloneUnionPayload(u *ast.UnionPayload) *ast.UnionPayload {
	out := &ast.UnionPayload{}
	for _, s := range u.Sources {
		out.Sources = append(out.Sources, d.cloneSubPipeline(s))
	}
	return out
}

func (d *desugarer) cloneSubPipeline(sp ast.SubPipeline) ast.SubPipeline {
	if sp.CTERef != "" {
		return ast.SubPipeline{CTERef: sp.CTERef, Pos: sp.Pos}
	}
	if sp.Pipeline != nil {
		pip := d.desugarPipeline(*sp.Pipeline, false)
		return ast.SubPipeline{Pipeline: &pip, Pos: sp.Pos}
	}
	return ast.SubPipeline{Pos: sp.Pos}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// renderStages renders a list of stages as a pipe-separated string.
func renderStages(stages []ast.Stage) string {
	var b strings.Builder
	for i, s := range stages {
		if i > 0 {
			b.WriteString(" | ")
		}
		b.WriteString(s.String())
	}
	return b.String()
}

// exprFieldName extracts a simple field name from an expression (Ident).
func exprFieldName(e ast.Expr) string {
	if id, ok := e.(*ast.Ident); ok {
		return id.Name
	}
	if m, ok := e.(*ast.Member); ok {
		return exprFieldName(m.Object) + "." + m.Field
	}
	return e.String()
}

// sanitizeFieldName replaces dots with underscores for alias names.
func sanitizeFieldName(name string) string {
	return strings.ReplaceAll(name, ".", "_")
}

// ---------------------------------------------------------------------------
// Expression / AST cloning utilities
// ---------------------------------------------------------------------------

func cloneExpr(e ast.Expr) ast.Expr {
	if e == nil {
		return nil
	}
	switch x := e.(type) {
	case *ast.Ident:
		return &ast.Ident{Name: x.Name, Quoted: x.Quoted, Pos: x.Pos}
	case *ast.Literal:
		return &ast.Literal{Kind: x.Kind, Raw: x.Raw, Value: x.Value, Pos: x.Pos}
	case *ast.Binary:
		return &ast.Binary{Op: x.Op, Left: cloneExpr(x.Left), Right: cloneExpr(x.Right), Pos: x.Pos}
	case *ast.Unary:
		return &ast.Unary{Op: x.Op, Operand: cloneExpr(x.Operand), Pos: x.Pos}
	case *ast.Call:
		args := make([]ast.Expr, len(x.Args))
		for i, a := range x.Args {
			args[i] = cloneExpr(a)
		}
		return &ast.Call{Receiver: cloneExpr(x.Receiver), SafeNav: x.SafeNav, Callee: x.Callee, Bang: x.Bang, Args: args, Pos: x.Pos}
	case *ast.In:
		return &ast.In{LHS: cloneExpr(x.LHS), RHS: cloneExpr(x.RHS), Pos: x.Pos}
	case *ast.Between:
		return &ast.Between{X: cloneExpr(x.X), Lo: cloneExpr(x.Lo), Hi: cloneExpr(x.Hi), Pos: x.Pos}
	case *ast.Member:
		return &ast.Member{Object: cloneExpr(x.Object), Field: x.Field, Pos: x.Pos}
	case *ast.SafeMember:
		return &ast.SafeMember{Object: cloneExpr(x.Object), Field: x.Field, Pos: x.Pos}
	case *ast.Index:
		return &ast.Index{Object: cloneExpr(x.Object), Idx: cloneExpr(x.Idx), Pos: x.Pos}
	case *ast.Lambda:
		return &ast.Lambda{Param: x.Param, Body: cloneExpr(x.Body), Pos: x.Pos}
	case *ast.Paren:
		return &ast.Paren{Inner: cloneExpr(x.Inner), Pos: x.Pos}
	case *ast.Array:
		elems := make([]ast.Expr, len(x.Elems))
		for i, e := range x.Elems {
			elems[i] = cloneExpr(e)
		}
		return &ast.Array{Elems: elems, Pos: x.Pos}
	case *ast.Object:
		entries := make([]ast.ObjectEntry, len(x.Entries))
		for i, ent := range x.Entries {
			entries[i] = ast.ObjectEntry{Key: ent.Key, KeySpan: ent.KeySpan, Value: cloneExpr(ent.Value)}
		}
		return &ast.Object{Entries: entries, Pos: x.Pos}
	case *ast.ErrorExpr:
		return &ast.ErrorExpr{Message: x.Message, Pos: x.Pos}
	case *ast.SearchGlobValue:
		return &ast.SearchGlobValue{Pattern: x.Pattern, Pos: x.Pos}
	}
	return e
}

func cloneExprs(exprs []ast.Expr) []ast.Expr {
	if exprs == nil {
		return nil
	}
	out := make([]ast.Expr, len(exprs))
	for i, e := range exprs {
		out[i] = cloneExpr(e)
	}
	return out
}

func cloneAgg(a ast.AggExpr) ast.AggExpr {
	return ast.AggExpr{
		Func:      cloneExpr(a.Func),
		WhereCond: cloneExpr(a.WhereCond),
		Alias:     a.Alias,
		AliasSpan: a.AliasSpan,
		Pos:       a.Pos,
	}
}

func cloneAggs(aggs []ast.AggExpr) []ast.AggExpr {
	if aggs == nil {
		return nil
	}
	out := make([]ast.AggExpr, len(aggs))
	for i, a := range aggs {
		out[i] = cloneAgg(a)
	}
	return out
}

func cloneSourceAtoms(atoms []ast.SourceAtom) []ast.SourceAtom {
	if atoms == nil {
		return nil
	}
	out := make([]ast.SourceAtom, len(atoms))
	copy(out, atoms)
	return out
}

func cloneTimeRanges(trs []ast.TimeRange) []ast.TimeRange {
	if trs == nil {
		return nil
	}
	out := make([]ast.TimeRange, len(trs))
	for i, tr := range trs {
		out[i] = ast.TimeRange{
			Start:    cloneExpr(tr.Start),
			End:      cloneExpr(tr.End),
			Snap:     tr.Snap,
			SnapSpan: tr.SnapSpan,
			Pos:      tr.Pos,
		}
	}
	return out
}

func cloneFromStage(f *ast.FromStage) *ast.FromStage {
	if f == nil {
		return nil
	}
	return &ast.FromStage{
		Sources:    cloneSourceAtoms(f.Sources),
		TimeRanges: cloneTimeRanges(f.TimeRanges),
		Pos:        f.Pos,
	}
}

func cloneStatsPayload(sp *ast.StatsPayload) *ast.StatsPayload {
	return &ast.StatsPayload{
		Aggs: cloneAggs(sp.Aggs),
		By:   cloneExprs(sp.By),
	}
}

func cloneStreamstatsPayload(sp *ast.StreamstatsPayload) *ast.StreamstatsPayload {
	out := &ast.StreamstatsPayload{
		StatsPayload: *cloneStatsPayload(&sp.StatsPayload),
	}
	if sp.Window != nil {
		w := *sp.Window
		out.Window = &w
	}
	if sp.Current != nil {
		c := *sp.Current
		out.Current = &c
	}
	return out
}

func cloneAssignPayload(ap *ast.AssignPayload) *ast.AssignPayload {
	out := &ast.AssignPayload{}
	for _, a := range ap.Assignments {
		out.Assignments = append(out.Assignments, ast.Assignment{
			Name:     a.Name,
			NameSpan: a.NameSpan,
			Value:    cloneExpr(a.Value),
			Pos:      a.Pos,
		})
	}
	return out
}

func cloneRenamePayload(rp *ast.RenamePayload) *ast.RenamePayload {
	out := &ast.RenamePayload{}
	for _, r := range rp.Renames {
		out.Renames = append(out.Renames, ast.RenameEntry{
			Old: r.Old, OldSpan: r.OldSpan,
			New: r.New, NewSpan: r.NewSpan,
			Pos: r.Pos,
		})
	}
	return out
}

func cloneSortPayload(sp *ast.SortPayload) *ast.SortPayload {
	out := &ast.SortPayload{}
	for _, k := range sp.Keys {
		out.Keys = append(out.Keys, ast.SortKey{
			Field: cloneExpr(k.Field),
			Desc:  k.Desc,
			Pos:   k.Pos,
		})
	}
	return out
}

func cloneFieldPatternsPayload(fp *ast.FieldPatternsPayload) *ast.FieldPatternsPayload {
	out := &ast.FieldPatternsPayload{StarExcept: fp.StarExcept}
	for _, p := range fp.Patterns {
		out.Patterns = append(out.Patterns, ast.FieldPattern{Name: p.Name, Glob: p.Glob, Pos: p.Pos})
	}
	return out
}

func cloneParsePayload(pp *ast.ParsePayload) *ast.ParsePayload {
	out := &ast.ParsePayload{
		Format:     pp.Format,
		FormatPos:  pp.FormatPos,
		FirstOfPos: pp.FirstOfPos,
		Prefix:     pp.Prefix,
		OnError:    pp.OnError,
		From:       cloneExpr(pp.From),
	}
	out.FirstOf = append(out.FirstOf, pp.FirstOf...)
	out.FormatArgs = cloneExprs(pp.FormatArgs)
	for _, c := range pp.Into {
		out.Into = append(out.Into, ast.CaptureField{Name: c.Name, Type: c.Type, Pos: c.Pos})
	}
	return out
}

func cloneMaterializePayload(mp *ast.MaterializePayload) *ast.MaterializePayload {
	return &ast.MaterializePayload{
		Name:      mp.Name,
		Retention: cloneExpr(mp.Retention),
	}
}

func cloneComparePayload(cp *ast.ComparePayload) *ast.ComparePayload {
	return &ast.ComparePayload{
		Previous: cp.Previous,
		Shift:    cloneExpr(cp.Shift),
	}
}

func cloneTransactionPayload(tp *ast.TransactionPayload) *ast.TransactionPayload {
	return &ast.TransactionPayload{
		Fields:     cloneExprs(tp.Fields),
		MaxSpan:    cloneExpr(tp.MaxSpan),
		StartsWith: cloneExpr(tp.StartsWith),
		EndsWith:   cloneExpr(tp.EndsWith),
	}
}

func cloneCorrelatePayload(cp *ast.CorrelatePayload) *ast.CorrelatePayload {
	return &ast.CorrelatePayload{
		Field1: cloneExpr(cp.Field1),
		Field2: cloneExpr(cp.Field2),
		Method: cp.Method,
	}
}

func cloneRollupPayload(rp *ast.RollupPayload) *ast.RollupPayload {
	return &ast.RollupPayload{
		Resolutions: cloneExprs(rp.Resolutions),
		By:          cloneExprs(rp.By),
	}
}

func cloneXYSeriesPayload(xp *ast.XYSeriesPayload) *ast.XYSeriesPayload {
	return &ast.XYSeriesPayload{
		X:     cloneExpr(xp.X),
		Y:     cloneExpr(xp.Y),
		Value: cloneExpr(xp.Value),
	}
}

func cloneGenericOptionsPayload(gp *ast.GenericOptionsPayload) *ast.GenericOptionsPayload {
	out := &ast.GenericOptionsPayload{
		Positionals: cloneExprs(gp.Positionals),
	}
	for _, o := range gp.Options {
		out.Options = append(out.Options, ast.Option{
			Name:     o.Name,
			NameSpan: o.NameSpan,
			Value:    cloneExpr(o.Value),
			ValuePos: o.ValuePos,
		})
	}
	return out
}

// isSugarStage returns true if the stage name is a sugar-class operator.
func isSugarStage(name string) bool {
	op, found := registry.LookupOperator(name)
	return found && op.Class == registry.ClassSugar
}

// IsSugarStageName is exported for test use.
func IsSugarStageName(name string) bool {
	return isSugarStage(name)
}
