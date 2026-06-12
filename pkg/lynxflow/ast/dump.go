package ast

import (
	"fmt"
	"strings"
)

// Dump produces a deterministic, readable multi-line AST dump suitable for
// golden-test assertions. Spans are OMITTED (they change with reformatting);
// only structural information — node kinds, names, literal values, operator
// names — is included.
//
// The output is indented with two spaces per nesting level.
func Dump(q *Query) string {
	var b strings.Builder
	d := dumper{w: &b}
	d.dumpQuery(q)
	return b.String()
}

// DumpExpr produces a deterministic AST dump for a single expression.
func DumpExpr(e Expr) string {
	var b strings.Builder
	d := dumper{w: &b}
	d.dumpExpr(e, 0)
	return b.String()
}

type dumper struct {
	w *strings.Builder
}

func (d *dumper) indent(depth int) {
	for i := 0; i < depth; i++ {
		d.w.WriteString("  ")
	}
}

func (d *dumper) line(depth int, format string, args ...interface{}) {
	d.indent(depth)
	fmt.Fprintf(d.w, format, args...)
	d.w.WriteByte('\n')
}

func (d *dumper) dumpQuery(q *Query) {
	d.line(0, "Query")
	for _, l := range q.Lets {
		d.dumpLet(&l, 1)
	}
	d.dumpPipeline(&q.Pipeline, 1)
}

func (d *dumper) dumpLet(l *Let, depth int) {
	d.line(depth, "Let $%s", l.Name)
	d.dumpPipeline(&l.Pipeline, depth+1)
}

func (d *dumper) dumpPipeline(p *Pipeline, depth int) {
	d.line(depth, "Pipeline")
	if p.Source != nil {
		d.dumpFromStage(p.Source, depth+1)
	}
	for i := range p.Stages {
		d.dumpStage(&p.Stages[i], depth+1)
	}
}

func (d *dumper) dumpFromStage(f *FromStage, depth int) {
	d.line(depth, "From")
	for _, src := range f.Sources {
		d.dumpSourceAtom(&src, depth+1)
	}
	for _, tr := range f.TimeRanges {
		d.dumpTimeRange(&tr, depth+1)
	}
	if f.SugarTerms != nil {
		d.line(depth+1, "SugarTerms")
		d.dumpSearchExpr(f.SugarTerms, depth+2)
	}
}

func (d *dumper) dumpSourceAtom(s *SourceAtom, depth int) {
	switch s.Kind {
	case SourceStar:
		d.line(depth, "Source *")
	case SourceCTE:
		d.line(depth, "Source $%s", s.Name)
	case SourceNegated:
		if s.Pattern != "" {
			d.line(depth, "Source !%s", s.Pattern)
		} else {
			d.line(depth, "Source !%s", s.Name)
		}
	case SourceGlob:
		d.line(depth, "Source glob(%s)", s.Pattern)
	default:
		if s.Quoted {
			d.line(depth, "Source `%s`", s.Name)
		} else {
			d.line(depth, "Source %s", s.Name)
		}
	}
}

func (d *dumper) dumpTimeRange(tr *TimeRange, depth int) {
	d.line(depth, "TimeRange")
	if tr.Start != nil {
		d.line(depth+1, "Start")
		d.dumpExpr(tr.Start, depth+2)
	}
	if tr.End != nil {
		d.line(depth+1, "End")
		d.dumpExpr(tr.End, depth+2)
	}
	if tr.Snap != "" {
		d.line(depth+1, "Snap %s", tr.Snap)
	}
}

func (d *dumper) dumpSearchExpr(se SearchExpr, depth int) {
	if se == nil {
		d.line(depth, "<nil>")
		return
	}
	switch s := se.(type) {
	case *SearchBareWord:
		if s.Glob {
			d.line(depth, "BareWord %q glob", s.Word)
		} else {
			d.line(depth, "BareWord %q", s.Word)
		}
	case *SearchPhrase:
		d.line(depth, "Phrase %q", s.Text)
	case *SearchKeyValue:
		d.line(depth, "KeyValue %s%s", s.Key, s.Op)
		if s.Value != nil {
			d.dumpExpr(s.Value, depth+1)
		}
	case *SearchIn:
		d.line(depth, "SearchIn %s", s.Key)
		for _, v := range s.Values {
			d.dumpExpr(v, depth+1)
		}
	case *SearchBinary:
		d.line(depth, "SearchBinary %s", s.Op)
		d.dumpSearchExpr(s.Left, depth+1)
		d.dumpSearchExpr(s.Right, depth+1)
	case *SearchNot:
		d.line(depth, "SearchNot")
		d.dumpSearchExpr(s.Operand, depth+1)
	case *SearchParen:
		d.line(depth, "SearchParen")
		d.dumpSearchExpr(s.Inner, depth+1)
	default:
		d.line(depth, "<unknown-search-expr>")
	}
}

func (d *dumper) dumpStage(s *Stage, depth int) {
	d.line(depth, "Stage %s", s.Name)
	switch {
	case s.Where != nil:
		d.dumpExpr(s.Where.Expr, depth+1)
	case s.Extend != nil:
		for _, a := range s.Extend.Assignments {
			d.line(depth+1, "Assign %s", a.Name)
			d.dumpExpr(a.Value, depth+2)
		}
	case s.Rename != nil:
		for _, r := range s.Rename.Renames {
			d.line(depth+1, "Rename %s as %s", r.Old, r.New)
		}
	case s.Stats != nil:
		d.dumpStatsPayload(s.Stats, depth+1)
	case s.Eventstats != nil:
		d.dumpStatsPayload(s.Eventstats, depth+1)
	case s.Streamstats != nil:
		if s.Streamstats.Window != nil {
			d.line(depth+1, "Window %d", *s.Streamstats.Window)
		}
		if s.Streamstats.Current != nil {
			d.line(depth+1, "Current %v", *s.Streamstats.Current)
		}
		d.dumpStatsPayload(&s.Streamstats.StatsPayload, depth+1)
	case s.Sort != nil:
		for _, k := range s.Sort.Keys {
			prefix := "+"
			if k.Desc {
				prefix = "-"
			}
			d.line(depth+1, "SortKey %s", prefix)
			d.dumpExpr(k.Field, depth+2)
		}
	case s.Head != nil:
		d.line(depth+1, "N %d", s.Head.N)
	case s.Tail != nil:
		d.line(depth+1, "N %d", s.Tail.N)
	case s.Dedup != nil:
		if s.Dedup.N > 1 {
			d.line(depth+1, "N %d", s.Dedup.N)
		}
		for _, f := range s.Dedup.Fields {
			d.dumpExpr(f, depth+1)
		}
	case s.Keep != nil:
		d.dumpFieldPatterns(s.Keep, depth+1)
	case s.Drop != nil:
		d.dumpFieldPatterns(s.Drop, depth+1)
	case s.Join != nil:
		if s.Join.Type != "" {
			d.line(depth+1, "Type %s", s.Join.Type)
		}
		if len(s.Join.On) > 0 {
			d.line(depth+1, "On")
			for _, f := range s.Join.On {
				d.dumpExpr(f, depth+2)
			}
		}
		if s.Join.Right != nil {
			d.line(depth+1, "With")
			d.dumpSubPipeline(s.Join.Right, depth+2)
		}
	case s.Union != nil:
		for _, src := range s.Union.Sources {
			d.dumpSubPipeline(&src, depth+1)
		}
	case s.Explode != nil:
		d.dumpExpr(s.Explode.Array, depth+1)
		if s.Explode.As != "" {
			d.line(depth+1, "As %s", s.Explode.As)
		}
	case s.Describe != nil:
		// empty
	case s.Parse != nil:
		d.dumpParsePayload(s.Parse, depth+1)
	case s.Top != nil:
		d.dumpTopRare(s.Top, depth+1)
	case s.Rare != nil:
		d.dumpTopRare(s.Rare, depth+1)
	case s.Every != nil:
		d.line(depth+1, "Span")
		d.dumpExpr(s.Every.Span, depth+2)
		if len(s.Every.By) > 0 {
			d.line(depth+1, "By")
			for _, k := range s.Every.By {
				d.dumpExpr(k, depth+2)
			}
		}
		d.line(depth+1, "Aggs")
		for _, agg := range s.Every.Aggs {
			d.dumpAggExpr(&agg, depth+2)
		}
	case s.Rate != nil:
		if s.Rate.Per != nil {
			d.line(depth+1, "Per")
			d.dumpExpr(s.Rate.Per, depth+2)
		}
		if len(s.Rate.By) > 0 {
			d.line(depth+1, "By")
			for _, k := range s.Rate.By {
				d.dumpExpr(k, depth+2)
			}
		}
	case s.Latency != nil:
		d.line(depth+1, "Field")
		d.dumpExpr(s.Latency.Field, depth+2)
		if s.Latency.Every != nil {
			d.line(depth+1, "Every")
			d.dumpExpr(s.Latency.Every, depth+2)
		}
		if len(s.Latency.By) > 0 {
			d.line(depth+1, "By")
			for _, k := range s.Latency.By {
				d.dumpExpr(k, depth+2)
			}
		}
	case s.Percentiles != nil:
		d.line(depth+1, "Field")
		d.dumpExpr(s.Percentiles.Field, depth+2)
		if len(s.Percentiles.By) > 0 {
			d.line(depth+1, "By")
			for _, k := range s.Percentiles.By {
				d.dumpExpr(k, depth+2)
			}
		}
	case s.Proportion != nil:
		d.line(depth+1, "Predicate")
		d.dumpExpr(s.Proportion.Predicate, depth+2)
		d.line(depth+1, "Alias %s", s.Proportion.Alias)
		if s.Proportion.Every != nil {
			d.line(depth+1, "Every")
			d.dumpExpr(s.Proportion.Every, depth+2)
		}
		if len(s.Proportion.By) > 0 {
			d.line(depth+1, "By")
			for _, k := range s.Proportion.By {
				d.dumpExpr(k, depth+2)
			}
		}
	case s.Facets != nil:
		for _, f := range s.Facets.Fields {
			d.dumpExpr(f, depth+1)
		}
		if s.Facets.Limit != nil {
			d.line(depth+1, "Limit %d", *s.Facets.Limit)
		}
	case s.Impact != nil:
		if s.Impact.Agg != nil {
			d.dumpAggExpr(s.Impact.Agg, depth+1)
		}
		if len(s.Impact.By) > 0 {
			d.line(depth+1, "By")
			for _, k := range s.Impact.By {
				d.dumpExpr(k, depth+2)
			}
		}
	case s.Baseline != nil:
		d.line(depth+1, "Field")
		d.dumpExpr(s.Baseline.Field, depth+2)
		d.line(depth+1, "Window %d", s.Baseline.Window)
		if len(s.Baseline.By) > 0 {
			d.line(depth+1, "By")
			for _, k := range s.Baseline.By {
				d.dumpExpr(k, depth+2)
			}
		}
	case s.Changes != nil:
		d.line(depth+1, "Field")
		d.dumpExpr(s.Changes.Field, depth+2)
		if len(s.Changes.By) > 0 {
			d.line(depth+1, "By")
			for _, k := range s.Changes.By {
				d.dumpExpr(k, depth+2)
			}
		}
	case s.Exemplars != nil:
		if s.Exemplars.N != nil {
			d.line(depth+1, "N %d", *s.Exemplars.N)
		}
		if len(s.Exemplars.By) > 0 {
			d.line(depth+1, "By")
			for _, k := range s.Exemplars.By {
				d.dumpExpr(k, depth+2)
			}
		}
	case s.Materialize != nil:
		d.line(depth+1, "Name %q", s.Materialize.Name)
		if s.Materialize.Retention != nil {
			d.line(depth+1, "Retention")
			d.dumpExpr(s.Materialize.Retention, depth+2)
		}
	case s.Tee != nil:
		d.line(depth+1, "Sink %q", s.Tee.Sink)
	case s.Use != nil:
		d.line(depth+1, "Fragment %q", s.Use.Fragment)
	case s.Compare != nil:
		if s.Compare.Previous {
			d.line(depth+1, "Previous")
		}
		if s.Compare.Shift != nil {
			d.line(depth+1, "Shift")
			d.dumpExpr(s.Compare.Shift, depth+2)
		}
	case s.Transaction != nil:
		if len(s.Transaction.Fields) > 0 {
			d.line(depth+1, "Fields")
			for _, f := range s.Transaction.Fields {
				d.dumpExpr(f, depth+2)
			}
		}
		if s.Transaction.MaxSpan != nil {
			d.line(depth+1, "MaxSpan")
			d.dumpExpr(s.Transaction.MaxSpan, depth+2)
		}
		if s.Transaction.StartsWith != nil {
			d.line(depth+1, "StartsWith")
			d.dumpExpr(s.Transaction.StartsWith, depth+2)
		}
		if s.Transaction.EndsWith != nil {
			d.line(depth+1, "EndsWith")
			d.dumpExpr(s.Transaction.EndsWith, depth+2)
		}
	case s.Correlate != nil:
		d.dumpExpr(s.Correlate.Field1, depth+1)
		d.dumpExpr(s.Correlate.Field2, depth+1)
		if s.Correlate.Method != "" {
			d.line(depth+1, "Method %s", s.Correlate.Method)
		}
	case s.Rollup != nil:
		d.line(depth+1, "Resolutions")
		for _, r := range s.Rollup.Resolutions {
			d.dumpExpr(r, depth+2)
		}
		if len(s.Rollup.By) > 0 {
			d.line(depth+1, "By")
			for _, k := range s.Rollup.By {
				d.dumpExpr(k, depth+2)
			}
		}
	case s.Xyseries != nil:
		d.line(depth+1, "X")
		d.dumpExpr(s.Xyseries.X, depth+2)
		d.line(depth+1, "Y")
		d.dumpExpr(s.Xyseries.Y, depth+2)
		d.line(depth+1, "Value")
		d.dumpExpr(s.Xyseries.Value, depth+2)
	case s.Patterns != nil:
		d.dumpGenericPayload(s.Patterns, depth+1)
	case s.Outliers != nil:
		d.dumpGenericPayload(s.Outliers, depth+1)
	case s.Sessionize != nil:
		d.dumpGenericPayload(s.Sessionize, depth+1)
	case s.Trace != nil:
		d.dumpGenericPayload(s.Trace, depth+1)
	case s.Topology != nil:
		d.dumpGenericPayload(s.Topology, depth+1)
	case s.Generic != nil:
		d.dumpGenericPayload(s.Generic, depth+1)
	}
}

func (d *dumper) dumpStatsPayload(sp *StatsPayload, depth int) {
	d.line(depth, "Aggs")
	for _, agg := range sp.Aggs {
		d.dumpAggExpr(&agg, depth+1)
	}
	if len(sp.By) > 0 {
		d.line(depth, "By")
		for _, k := range sp.By {
			d.dumpExpr(k, depth+1)
		}
	}
}

func (d *dumper) dumpAggExpr(agg *AggExpr, depth int) {
	d.line(depth, "Agg")
	d.dumpExpr(agg.Func, depth+1)
	if agg.WhereCond != nil {
		d.line(depth+1, "WhereCond")
		d.dumpExpr(agg.WhereCond, depth+2)
	}
	if agg.Alias != "" {
		d.line(depth+1, "Alias %s", agg.Alias)
	}
}

func (d *dumper) dumpFieldPatterns(fp *FieldPatternsPayload, depth int) {
	if fp.StarExcept {
		d.line(depth, "StarExcept")
	}
	for _, p := range fp.Patterns {
		if p.Glob {
			d.line(depth, "Pattern glob(%s)", p.Name)
		} else {
			d.line(depth, "Pattern %s", p.Name)
		}
	}
}

func (d *dumper) dumpSubPipeline(sp *SubPipeline, depth int) {
	if sp.CTERef != "" {
		d.line(depth, "CTERef $%s", sp.CTERef)
		return
	}
	if sp.Pipeline != nil {
		d.line(depth, "SubPipeline")
		d.dumpPipeline(sp.Pipeline, depth+1)
		return
	}
	d.line(depth, "<error>")
}

func (d *dumper) dumpParsePayload(p *ParsePayload, depth int) {
	if len(p.FirstOf) > 0 {
		d.line(depth, "FirstOf %s", strings.Join(p.FirstOf, ", "))
	} else {
		d.line(depth, "Format %s", p.Format)
	}
	for _, a := range p.FormatArgs {
		d.dumpExpr(a, depth+1)
	}
	if p.From != nil {
		d.line(depth, "From")
		d.dumpExpr(p.From, depth+1)
	}
	if len(p.Into) > 0 {
		d.line(depth, "Into")
		for _, c := range p.Into {
			if c.Type != "" {
				d.line(depth+1, "%s as %s", c.Name, c.Type)
			} else {
				d.line(depth+1, "%s", c.Name)
			}
		}
	}
	if p.Prefix != "" {
		d.line(depth, "Prefix %s", p.Prefix)
	}
	if p.OnError != "" {
		d.line(depth, "OnError %s", p.OnError)
	}
}

func (d *dumper) dumpTopRare(p *TopRarePayload, depth int) {
	if p.N != nil {
		d.line(depth, "N %d", *p.N)
	}
	d.dumpExpr(p.Field, depth)
}

func (d *dumper) dumpGenericPayload(p *GenericOptionsPayload, depth int) {
	for _, pos := range p.Positionals {
		d.dumpExpr(pos, depth)
	}
	for _, opt := range p.Options {
		d.line(depth, "Option %s", opt.Name)
		d.dumpExpr(opt.Value, depth+1)
	}
}

func (d *dumper) dumpExpr(e Expr, depth int) {
	if e == nil {
		d.line(depth, "<nil>")
		return
	}
	switch n := e.(type) {
	case *Ident:
		if n.Quoted {
			d.line(depth, "Ident `%s`", n.Name)
		} else {
			d.line(depth, "Ident %s", n.Name)
		}
	case *Literal:
		d.dumpLiteral(n, depth)
	case *Array:
		d.line(depth, "Array")
		for _, el := range n.Elems {
			d.dumpExpr(el, depth+1)
		}
	case *Object:
		d.line(depth, "Object")
		for _, ent := range n.Entries {
			d.line(depth+1, "Entry %s", ent.Key)
			d.dumpExpr(ent.Value, depth+2)
		}
	case *Binary:
		d.line(depth, "Binary %s", binaryOpName(n.Op))
		d.dumpExpr(n.Left, depth+1)
		d.dumpExpr(n.Right, depth+1)
	case *Unary:
		d.line(depth, "Unary %s", unaryOpName(n.Op))
		d.dumpExpr(n.Operand, depth+1)
	case *In:
		d.line(depth, "In")
		d.dumpExpr(n.LHS, depth+1)
		d.dumpExpr(n.RHS, depth+1)
	case *Between:
		d.line(depth, "Between")
		d.dumpExpr(n.X, depth+1)
		d.dumpExpr(n.Lo, depth+1)
		d.dumpExpr(n.Hi, depth+1)
	case *Call:
		label := "Call " + n.Callee
		if n.Bang {
			label += "!"
		}
		if n.Receiver != nil {
			if n.SafeNav {
				label += " (safeNav)"
			} else {
				label += " (method)"
			}
		}
		d.line(depth, "%s", label)
		if n.Receiver != nil {
			d.line(depth+1, "Receiver")
			d.dumpExpr(n.Receiver, depth+2)
		}
		for _, a := range n.Args {
			d.dumpExpr(a, depth+1)
		}
	case *Member:
		d.line(depth, "Member .%s", n.Field)
		d.dumpExpr(n.Object, depth+1)
	case *SafeMember:
		d.line(depth, "SafeMember ?.%s", n.Field)
		d.dumpExpr(n.Object, depth+1)
	case *Index:
		d.line(depth, "Index")
		d.dumpExpr(n.Object, depth+1)
		d.dumpExpr(n.Idx, depth+1)
	case *Lambda:
		d.line(depth, "Lambda %s", n.Param)
		d.dumpExpr(n.Body, depth+1)
	case *Paren:
		d.line(depth, "Paren")
		d.dumpExpr(n.Inner, depth+1)
	case *ErrorExpr:
		d.line(depth, "Error %q", n.Message)
	case *SearchGlobValue:
		d.line(depth, "SearchGlob %s", n.Pattern)
	default:
		d.line(depth, "<unknown>")
	}
}

func (d *dumper) dumpLiteral(n *Literal, depth int) {
	switch n.Kind {
	case LitString:
		d.line(depth, "String %q", n.Value)
	case LitRawString:
		d.line(depth, "RawString %q", n.Value)
	case LitInt:
		d.line(depth, "Int %v", n.Value)
	case LitFloat:
		d.line(depth, "Float %v", n.Value)
	case LitBool:
		d.line(depth, "Bool %v", n.Value)
	case LitNull:
		d.line(depth, "Null")
	case LitDuration:
		d.line(depth, "Duration %s", n.Raw)
	default:
		d.line(depth, "Literal(%d) %s", n.Kind, n.Raw)
	}
}

func binaryOpName(op BinaryOp) string {
	switch op {
	case OpOr:
		return "or"
	case OpAnd:
		return "and"
	case OpEq:
		return "=="
	case OpNotEq:
		return "!="
	case OpLt:
		return "<"
	case OpLtEq:
		return "<="
	case OpGt:
		return ">"
	case OpGtEq:
		return ">="
	case OpAdd:
		return "+"
	case OpSub:
		return "-"
	case OpMul:
		return "*"
	case OpDiv:
		return "/"
	case OpMod:
		return "%"
	case OpCoalesce:
		return "??"
	}
	return "?"
}

func unaryOpName(op UnaryOp) string {
	switch op {
	case OpNot:
		return "not"
	case OpNeg:
		return "neg"
	}
	return "?"
}
