package logical

import (
	"strings"

	"github.com/lynxbase/lynxdb/pkg/lynxflow/ast"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/parser"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/registry"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/sema"
)

// Options controls lowering behavior.
type Options struct {
	// DefaultSource is the source name used when constructing an initial
	// schema (e.g. "main"). Only affects schema lookup, not plan structure.
	DefaultSource string
}

// Diag is a diagnostic produced during lowering.
type Diag = parser.Diag

// Lower converts a DESUGARED LynxFlow AST into a typed logical plan.
//
// The input must be desugared (no sugar-class stages). If a sugar stage is
// encountered, an internal-error diagnostic is emitted (defensive). The
// returned Plan's Root is the terminal operator of the main pipeline. CTE
// bindings are in Plan.Lets, and CTE references share pointers into that map.
func Lower(q *ast.Query, opts Options) (*Plan, []Diag) {
	if q == nil {
		return &Plan{}, nil
	}
	l := &lowerer{
		opts:       opts,
		lets:       make(map[string]*Plan),
		initSchema: builtinSchema(),
	}

	// Lower CTEs first.
	for _, let := range q.Lets {
		letPlan, _ := l.lowerPipeline(let.Pipeline)
		l.lets[let.Name] = &Plan{Root: letPlan}
	}

	// Lower main pipeline.
	root, _ := l.lowerPipeline(q.Pipeline)

	return &Plan{Root: root, Lets: l.lets}, l.diags
}

type lowerer struct {
	opts       Options
	diags      []Diag
	lets       map[string]*Plan
	initSchema []sema.Field
}

func (l *lowerer) addDiag(code parser.DiagCode, sev parser.Severity, span ast.Span, msg string) {
	l.diags = append(l.diags, Diag{
		Code:     code,
		Severity: sev,
		Message:  msg,
		Span:     span,
	})
}

func builtinSchema() []sema.Field {
	return []sema.Field{
		{Name: "_time", Type: sema.TypeTimestamp},
		{Name: "_raw", Type: sema.TypeString},
		{Name: "_source", Type: sema.TypeString},
		{Name: "_sourcetype", Type: sema.TypeString},
		{Name: "host", Type: sema.TypeString},
		{Name: "index", Type: sema.TypeString},
	}
}

// lowerPipeline converts a Pipeline into a chain of Nodes. Returns the
// terminal (root) node.
func (l *lowerer) lowerPipeline(p ast.Pipeline) (Node, []sema.Field) {
	var current Node

	// 1. From stage -> Scan (or CTE reference)
	if p.Source != nil {
		current = l.lowerFrom(p.Source)
	} else {
		// No source: synthetic scan from default
		current = &Scan{OutputSchema: copySchema(l.initSchema)}
	}

	// 2. Lower each stage, applying fusion rules.
	stages := p.Stages
	for i := 0; i < len(stages); i++ {
		s := stages[i]

		// Check for sugar stages (defensive).
		if op, found := registry.LookupOperator(s.Name); found && op.Class == registry.ClassSugar {
			l.addDiag("L001", parser.SeverityError, s.Pos,
				"internal error: sugar stage '"+s.Name+"' in input to Lower (expected desugared AST)")
			continue
		}

		// Fusion: consecutive keep/drop/rename -> single Project.
		if isProjection(s.Name) {
			var projStages []ast.Stage
			projStages = append(projStages, s)
			for i+1 < len(stages) && isProjection(stages[i+1].Name) {
				i++
				projStages = append(projStages, stages[i])
			}
			current = l.lowerProjectFused(current, projStages)
			continue
		}

		// Fusion: sort immediately followed by head -> TopK.
		if s.Name == "sort" && s.Sort != nil && i+1 < len(stages) && stages[i+1].Name == "head" && stages[i+1].Head != nil {
			current = l.lowerTopK(current, s, stages[i+1])
			i++ // skip the head
			continue
		}

		current = l.lowerStage(current, s)
	}

	schema := current.Schema()
	return current, schema
}

func isProjection(name string) bool {
	return name == "keep" || name == "drop" || name == "rename"
}

func (l *lowerer) lowerFrom(f *ast.FromStage) Node {
	// Check for single-CTE source.
	if len(f.Sources) == 1 && f.Sources[0].Kind == ast.SourceCTE {
		cteName := f.Sources[0].Name
		if letPlan, ok := l.lets[cteName]; ok {
			// Return a shared pointer to the CTE's root node.
			return letPlan.Root
		}
		// CTE not found: emit a scan with the name (will be caught later).
	}

	sources := make([]SourcePattern, len(f.Sources))
	for i, s := range f.Sources {
		sources[i] = SourcePattern{
			Kind:    s.Kind,
			Name:    s.Name,
			Pattern: s.Pattern,
		}
	}

	var tb *TimeBounds
	if len(f.TimeRanges) > 0 {
		tr := f.TimeRanges[0]
		tb = &TimeBounds{
			Start: tr.Start,
			End:   tr.End,
			Snap:  tr.Snap,
		}
	}

	return &Scan{
		Sources:      sources,
		TimeRange:    tb,
		OutputSchema: copySchema(l.initSchema),
	}
}

func (l *lowerer) lowerStage(input Node, s ast.Stage) Node {
	switch s.Name {
	case "where":
		return l.lowerWhere(input, s)
	case "extend":
		return l.lowerExtend(input, s)
	case "stats":
		return l.lowerStats(input, s.Stats, nil)
	case "eventstats":
		ws := &WindowSpec{Variant: WindowEventstats}
		return l.lowerStats(input, s.Eventstats, ws)
	case "streamstats":
		ws := &WindowSpec{Variant: WindowStreamstats}
		if s.Streamstats != nil {
			ws.Window = s.Streamstats.Window
			ws.Current = s.Streamstats.Current
		}
		var sp *ast.StatsPayload
		if s.Streamstats != nil {
			sp = &s.Streamstats.StatsPayload
		}
		return l.lowerStats(input, sp, ws)
	case "sort":
		return l.lowerSort(input, s)
	case "head":
		return l.lowerHead(input, s)
	case "tail":
		return l.lowerTail(input, s)
	case "dedup":
		return l.lowerDedup(input, s)
	case "join":
		return l.lowerJoin(input, s)
	case "union":
		return l.lowerUnion(input, s)
	case "explode":
		return l.lowerExplode(input, s)
	case "describe":
		return &Describe{unaryNode: unaryNode{Input: input}}
	case "parse":
		return l.lowerParse(input, s)
	case "materialize":
		return l.lowerMaterialize(input, s)
	case "tee":
		return l.lowerTee(input, s)
	// Helpers
	case "compare":
		return l.lowerCompare(input, s)
	case "patterns":
		return l.lowerHelperGeneric(input, s, s.Patterns, helperExtraFields(s.Name))
	case "outliers":
		return l.lowerHelperGeneric(input, s, s.Outliers, helperExtraFields(s.Name))
	case "sessionize":
		return l.lowerHelperGeneric(input, s, s.Sessionize, helperExtraFields(s.Name))
	case "transaction":
		return l.lowerHelper(input, s, helperExtraFields(s.Name))
	case "trace":
		return l.lowerHelperGeneric(input, s, s.Trace, helperExtraFields(s.Name))
	case "topology":
		return l.lowerHelperGeneric(input, s, s.Topology, helperExtraFields(s.Name))
	case "correlate":
		return l.lowerHelper(input, s, helperExtraFields(s.Name))
	case "rollup":
		return l.lowerHelper(input, s, helperExtraFields(s.Name))
	case "xyseries":
		return l.lowerHelper(input, s, helperExtraFields(s.Name))
	case "keep", "drop", "rename":
		// Should have been fused; handle single occurrence.
		return l.lowerProjectFused(input, []ast.Stage{s})
	default:
		// Unknown stage: pass through as Helper.
		return &Helper{
			unaryNode: unaryNode{Input: input},
			Name:      s.Name,
		}
	}
}

func (l *lowerer) lowerWhere(input Node, s ast.Stage) Node {
	if s.Where == nil {
		return input
	}
	return &Filter{
		unaryNode: unaryNode{Input: input},
		Expr:      s.Where.Expr,
	}
}

func (l *lowerer) lowerExtend(input Node, s ast.Stage) Node {
	if s.Extend == nil {
		return input
	}
	assigns := make([]Assignment, len(s.Extend.Assignments))
	for i, a := range s.Extend.Assignments {
		assigns[i] = Assignment{Name: a.Name, Value: a.Value}
	}
	return &Extend{
		unaryNode:   unaryNode{Input: input},
		Assignments: assigns,
	}
}

func (l *lowerer) lowerStats(input Node, sp *ast.StatsPayload, window *WindowSpec) Node {
	if sp == nil {
		return input
	}

	var aggs []Agg
	for _, a := range sp.Aggs {
		aggs = append(aggs, Agg{
			Func:      a.Func,
			WhereCond: a.WhereCond,
			Alias:     a.Alias,
		})
	}

	var keys []Key
	var timeBin *TimeBin

	for _, byExpr := range sp.By {
		// Check for bin(_time, d) -> extract TimeBin.
		if call, ok := byExpr.(*ast.Call); ok && call.Callee == "bin" && len(call.Args) >= 2 {
			if ident, ok := call.Args[0].(*ast.Ident); ok && ident.Name == "_time" {
				timeBin = &TimeBin{Duration: call.Args[1]}
				continue // extracted, not added to Keys
			}
		}
		name := exprFieldName(byExpr)
		keys = append(keys, Key{Expr: byExpr, Name: name})
	}

	return &Aggregate{
		unaryNode: unaryNode{Input: input},
		Aggs:      aggs,
		Keys:      keys,
		TimeBin:   timeBin,
		Window:    window,
	}
}

func (l *lowerer) lowerSort(input Node, s ast.Stage) Node {
	if s.Sort == nil {
		return input
	}
	keys := make([]SortKey, len(s.Sort.Keys))
	for i, k := range s.Sort.Keys {
		keys[i] = SortKey{Expr: k.Field, Desc: k.Desc}
	}
	return &Sort{
		unaryNode: unaryNode{Input: input},
		Keys:      keys,
	}
}

func (l *lowerer) lowerHead(input Node, s ast.Stage) Node {
	if s.Head == nil {
		return input
	}
	return &Limit{
		unaryNode: unaryNode{Input: input},
		N:         s.Head.N,
		Tail:      false,
	}
}

func (l *lowerer) lowerTail(input Node, s ast.Stage) Node {
	if s.Tail == nil {
		return input
	}
	return &Limit{
		unaryNode: unaryNode{Input: input},
		N:         s.Tail.N,
		Tail:      true,
	}
}

func (l *lowerer) lowerTopK(input Node, sortStage, headStage ast.Stage) Node {
	keys := make([]SortKey, len(sortStage.Sort.Keys))
	for i, k := range sortStage.Sort.Keys {
		keys[i] = SortKey{Expr: k.Field, Desc: k.Desc}
	}
	return &TopK{
		unaryNode: unaryNode{Input: input},
		K:         headStage.Head.N,
		SortKeys:  keys,
	}
}

func (l *lowerer) lowerDedup(input Node, s ast.Stage) Node {
	if s.Dedup == nil {
		return input
	}
	fields := make([]string, len(s.Dedup.Fields))
	for i, f := range s.Dedup.Fields {
		fields[i] = exprFieldName(f)
	}
	n := s.Dedup.N
	if n == 0 {
		n = 1
	}
	return &Dedup{
		unaryNode: unaryNode{Input: input},
		N:         n,
		Fields:    fields,
	}
}

func (l *lowerer) lowerJoin(input Node, s ast.Stage) Node {
	if s.Join == nil {
		return input
	}
	onFields := make([]string, len(s.Join.On))
	for i, f := range s.Join.On {
		onFields[i] = exprFieldName(f)
	}
	joinType := s.Join.Type
	if joinType == "" {
		joinType = "inner"
	}

	var rightNode Node
	if s.Join.Right != nil {
		if s.Join.Right.CTERef != "" {
			// CTE reference: resolve from lets.
			if letPlan, ok := l.lets[s.Join.Right.CTERef]; ok {
				rightNode = letPlan.Root
			} else {
				// Unresolved CTE: synthesize a scan.
				rightNode = &Scan{
					Sources:      []SourcePattern{{Kind: ast.SourceCTE, Name: s.Join.Right.CTERef}},
					OutputSchema: copySchema(l.initSchema),
				}
			}
		} else if s.Join.Right.Pipeline != nil {
			rightNode, _ = l.lowerPipeline(*s.Join.Right.Pipeline)
		}
	}

	return &Join{
		unaryNode: unaryNode{Input: input},
		Type:      joinType,
		On:        onFields,
		Right:     rightNode,
	}
}

func (l *lowerer) lowerUnion(input Node, s ast.Stage) Node {
	if s.Union == nil {
		return input
	}
	inputs := []Node{input}
	for _, src := range s.Union.Sources {
		if src.CTERef != "" {
			if letPlan, ok := l.lets[src.CTERef]; ok {
				inputs = append(inputs, letPlan.Root)
			} else {
				inputs = append(inputs, &Scan{
					Sources:      []SourcePattern{{Kind: ast.SourceCTE, Name: src.CTERef}},
					OutputSchema: copySchema(l.initSchema),
				})
			}
		} else if src.Pipeline != nil {
			subNode, _ := l.lowerPipeline(*src.Pipeline)
			inputs = append(inputs, subNode)
		}
	}
	return &Union{Inputs: inputs}
}

func (l *lowerer) lowerExplode(input Node, s ast.Stage) Node {
	if s.Explode == nil {
		return input
	}
	field := exprFieldName(s.Explode.Array)
	return &Explode{
		unaryNode: unaryNode{Input: input},
		Field:     field,
		As:        s.Explode.As,
	}
}

func (l *lowerer) lowerParse(input Node, s ast.Stage) Node {
	if s.Parse == nil {
		return input
	}
	pp := s.Parse
	var captures []Capture
	for _, c := range pp.Into {
		captures = append(captures, Capture{Name: c.Name, Type: c.Type})
	}
	from := ""
	if pp.From != nil {
		from = exprFieldName(pp.From)
	}
	// Extract the regex/pattern string from FormatArgs[0] when present.
	pattern := ""
	if (pp.Format == "regex" || pp.Format == "pattern") && len(pp.FormatArgs) > 0 {
		pattern = exprStringValue(pp.FormatArgs[0])
	}

	return &Parse{
		unaryNode: unaryNode{Input: input},
		Format:    pp.Format,
		Pattern:   pattern,
		FirstOf:   pp.FirstOf,
		From:      from,
		Captures:  captures,
		Prefix:    pp.Prefix,
		OnError:   pp.OnError,
	}
}

func (l *lowerer) lowerMaterialize(input Node, s ast.Stage) Node {
	if s.Materialize == nil {
		return input
	}
	retention := ""
	if s.Materialize.Retention != nil {
		retention = s.Materialize.Retention.String()
	}
	return &Materialize{
		unaryNode: unaryNode{Input: input},
		Name:      s.Materialize.Name,
		Retention: retention,
	}
}

func (l *lowerer) lowerTee(input Node, s ast.Stage) Node {
	if s.Tee == nil {
		return input
	}
	return &Tee{
		unaryNode: unaryNode{Input: input},
		Sink:      s.Tee.Sink,
	}
}

func (l *lowerer) lowerProjectFused(input Node, stages []ast.Stage) Node {
	var cols []ProjectCol
	for _, s := range stages {
		switch s.Name {
		case "keep":
			if s.Keep != nil {
				if s.Keep.StarExcept {
					// keep * except f1, f2 -> drops
					for _, p := range s.Keep.Patterns {
						cols = append(cols, ProjectCol{
							Action: ProjectDrop,
							Name:   p.Name,
							Glob:   p.Glob,
						})
					}
				} else {
					for _, p := range s.Keep.Patterns {
						cols = append(cols, ProjectCol{
							Action: ProjectKeep,
							Name:   p.Name,
							Glob:   p.Glob,
						})
					}
				}
			}
		case "drop":
			if s.Drop != nil {
				for _, p := range s.Drop.Patterns {
					cols = append(cols, ProjectCol{
						Action: ProjectDrop,
						Name:   p.Name,
						Glob:   p.Glob,
					})
				}
			}
		case "rename":
			if s.Rename != nil {
				for _, r := range s.Rename.Renames {
					cols = append(cols, ProjectCol{
						Action: ProjectRename,
						Name:   r.New,
						From:   r.Old,
					})
				}
			}
		}
	}
	return &Project{
		unaryNode: unaryNode{Input: input},
		Cols:      cols,
	}
}

// lowerCompare expands `| compare previous <dur>` into a logical IR subtree
// that replays the pipeline prefix over a time-shifted window and joins the
// results with the current window. The expansion requires the input to be an
// Aggregate (plain stats, not eventstats/streamstats).
//
// Expansion shape (for `<prefix ending in agg> | compare previous <dur>`):
//
//	Join(type=left, on=[group-by-keys])
//	  LEFT:  <prefix as-is>
//	  RIGHT: deep-copy of prefix with Scan time bounds shifted back by <dur>
//	         + if TimeBin present: Extend(_time = _time + <dur>) for alignment
//	         + Project(rename agg cols: c -> previous_c)
//	POST-JOIN:
//	  Extend(change_c = c - previous_c for each numeric agg output)
func (l *lowerer) lowerCompare(input Node, s ast.Stage) Node {
	// 1. Validate: input must be an Aggregate (plain stats).
	agg, ok := input.(*Aggregate)
	if !ok {
		l.addDiag("L002", parser.SeverityError, s.Pos,
			"compare requires an aggregated pipeline (| stats ... | compare); "+
				"the preceding stage is not a stats aggregation")
		return input
	}
	if agg.Window != nil {
		l.addDiag("L002", parser.SeverityError, s.Pos,
			"compare is not supported with eventstats or streamstats; use plain stats")
		return input
	}

	// 2. Extract the shift duration.
	if s.Compare == nil || s.Compare.Shift == nil {
		l.addDiag("L002", parser.SeverityError, s.Pos,
			"compare requires a duration (e.g. compare previous 1h)")
		return input
	}
	shiftExpr := s.Compare.Shift

	// 3. Identify aggregate output columns and group-by keys.
	aggSchema := agg.Schema()
	var groupByKeys []string
	if agg.TimeBin != nil {
		groupByKeys = append(groupByKeys, "_time")
	}
	for _, k := range agg.Keys {
		groupByKeys = append(groupByKeys, k.Name)
	}

	// Aggregate-output column names (the non-group-by columns).
	groupKeySet := make(map[string]bool, len(groupByKeys))
	for _, k := range groupByKeys {
		groupKeySet[k] = true
	}
	var aggOutputCols []string
	for _, f := range aggSchema {
		if !groupKeySet[f.Name] {
			aggOutputCols = append(aggOutputCols, f.Name)
		}
	}

	// 4. Deep-copy the entire prefix chain (Scan ... Aggregate).
	prevRoot := deepCopyChain(input)

	// 5. Shift time bounds on the copy's Scan.
	prevScan := findScan(prevRoot)
	if prevScan != nil {
		shiftScanTimeBounds(prevScan, shiftExpr)
	}

	// 6. If TimeBin is present, add an Extend to the copy that aligns the
	// time bucket: _time = _time + <dur> so the shifted window's buckets
	// match the current window's bucket boundaries.
	var prevPipeline Node = prevRoot
	if agg.TimeBin != nil {
		alignExpr := &ast.Binary{
			Op:    ast.OpAdd,
			Left:  &ast.Ident{Name: "_time"},
			Right: shiftExpr,
		}
		prevPipeline = &Extend{
			unaryNode:   unaryNode{Input: prevPipeline},
			Assignments: []Assignment{{Name: "_time", Value: alignExpr}},
		}
	}

	// 7. Rename aggregate output columns to previous_*.
	var renameCols []ProjectCol
	for _, col := range aggOutputCols {
		renameCols = append(renameCols, ProjectCol{
			Action: ProjectRename,
			Name:   "previous_" + col,
			From:   col,
		})
	}
	prevPipeline = &Project{
		unaryNode: unaryNode{Input: prevPipeline},
		Cols:      renameCols,
	}

	// 8. Join current with previous on group-by keys.
	joinOn := groupByKeys
	if len(joinOn) == 0 {
		// No group-by: cross-join on no keys (single row on each side).
		joinOn = nil
	}

	joined := &Join{
		unaryNode: unaryNode{Input: input},
		Type:      "left",
		On:        joinOn,
		Right:     prevPipeline,
	}

	// 9. Extend change_* columns: change_c = c - previous_c for each agg output.
	var changeAssigns []Assignment
	for _, col := range aggOutputCols {
		changeAssigns = append(changeAssigns, Assignment{
			Name: "change_" + col,
			Value: &ast.Binary{
				Op:    ast.OpSub,
				Left:  &ast.Ident{Name: col},
				Right: &ast.Ident{Name: "previous_" + col},
			},
		})
	}

	if len(changeAssigns) == 0 {
		return joined
	}

	return &Extend{
		unaryNode:   unaryNode{Input: joined},
		Assignments: changeAssigns,
	}
}

// deepCopyChain creates a deep copy of a node chain (linear pipeline).
// Each node is cloned and re-linked. For Join, the Right sub-plan is also
// deep-copied. AST expression pointers are SHARED (not deep-copied) because
// the optimizer never mutates existing AST expressions (it creates new ones).
func deepCopyChain(root Node) Node {
	if root == nil {
		return nil
	}

	// Linearize: walk from root to leaf.
	var chain []Node
	cur := root
	for cur != nil {
		chain = append(chain, cur)
		children := cur.Children()
		if len(children) == 0 {
			break
		}
		cur = children[0]
	}

	// Clone from leaf to root, re-linking children.
	var prev Node
	for i := len(chain) - 1; i >= 0; i-- {
		cloned := shallowClone(chain[i])
		if prev != nil {
			cloned.SetChildren([]Node{prev})
		}
		// For Join, deep-copy the Right sub-plan too.
		if j, ok := cloned.(*Join); ok {
			if origJ, ok := chain[i].(*Join); ok && origJ.Right != nil {
				j.Right = deepCopyChain(origJ.Right)
			}
		}
		prev = cloned
	}
	return prev
}

// shallowClone creates a shallow copy of a logical node.
func shallowClone(n Node) Node {
	switch x := n.(type) {
	case *Scan:
		c := *x
		// Deep-copy mutable slices.
		c.Sources = append([]SourcePattern(nil), x.Sources...)
		c.OutputSchema = append([]sema.Field(nil), x.OutputSchema...)
		c.Pushdown = Pushdown{
			FieldPredicates: append([]ast.Expr(nil), x.Pushdown.FieldPredicates...),
			BloomTerms:      append([]string(nil), x.Pushdown.BloomTerms...),
			RawTerms:        append([]string(nil), x.Pushdown.RawTerms...),
			TokenGlobs:      append([]string(nil), x.Pushdown.TokenGlobs...),
			Columns:         append([]string(nil), x.Pushdown.Columns...),
		}
		if x.Pushdown.TimeBounds != nil {
			tb := *x.Pushdown.TimeBounds
			c.Pushdown.TimeBounds = &tb
		}
		if x.TimeRange != nil {
			tr := *x.TimeRange
			c.TimeRange = &tr
		}
		return &c
	case *Filter:
		c := *x
		return &c
	case *Extend:
		c := *x
		c.Assignments = append([]Assignment(nil), x.Assignments...)
		c.cachedSchema = nil
		return &c
	case *Project:
		c := *x
		c.Cols = append([]ProjectCol(nil), x.Cols...)
		c.cachedSchema = nil
		return &c
	case *Aggregate:
		c := *x
		c.Aggs = append([]Agg(nil), x.Aggs...)
		c.Keys = append([]Key(nil), x.Keys...)
		c.cachedSchema = nil
		if x.TimeBin != nil {
			tb := *x.TimeBin
			c.TimeBin = &tb
		}
		if x.Window != nil {
			w := *x.Window
			c.Window = &w
		}
		return &c
	case *Sort:
		c := *x
		c.Keys = append([]SortKey(nil), x.Keys...)
		return &c
	case *Limit:
		c := *x
		return &c
	case *Dedup:
		c := *x
		c.Fields = append([]string(nil), x.Fields...)
		return &c
	case *TopK:
		c := *x
		c.SortKeys = append([]SortKey(nil), x.SortKeys...)
		return &c
	case *Join:
		c := *x
		c.On = append([]string(nil), x.On...)
		c.cachedSchema = nil
		return &c
	case *Union:
		c := *x
		c.Inputs = append([]Node(nil), x.Inputs...)
		c.cachedSchema = nil
		return &c
	case *Parse:
		c := *x
		c.Captures = append([]Capture(nil), x.Captures...)
		c.FirstOf = append([]string(nil), x.FirstOf...)
		c.cachedSchema = nil
		return &c
	case *Explode:
		c := *x
		c.cachedSchema = nil
		return &c
	case *Describe:
		c := *x
		return &c
	case *Helper:
		c := *x
		opts := make(map[string]ast.Expr, len(x.Options))
		for k, v := range x.Options {
			opts[k] = v
		}
		c.Options = opts
		c.Positional = append([]ast.Expr(nil), x.Positional...)
		c.extraFields = append([]sema.Field(nil), x.extraFields...)
		c.cachedSchema = nil
		return &c
	case *Empty:
		c := *x
		return &c
	case *Materialize:
		c := *x
		return &c
	case *Tee:
		c := *x
		return &c
	}
	// Fallback: return the original (should not happen for known types).
	return n
}

// findScan walks down the chain and returns the first Scan node, or nil.
func findScan(root Node) *Scan {
	cur := root
	for cur != nil {
		if s, ok := cur.(*Scan); ok {
			return s
		}
		children := cur.Children()
		if len(children) == 0 {
			return nil
		}
		cur = children[0]
	}
	return nil
}

// shiftScanTimeBounds shifts a Scan's time range back by the given duration
// expression. If the Scan already has bracket time bounds, the start and end
// are adjusted by subtracting the shift. If there are no explicit bounds, a
// pushdown hint is set instead.
func shiftScanTimeBounds(scan *Scan, shiftExpr ast.Expr) {
	if scan.TimeRange != nil {
		// Shift both start and end back by the duration.
		if scan.TimeRange.Start != nil {
			scan.TimeRange.Start = &ast.Binary{
				Op:    ast.OpSub,
				Left:  scan.TimeRange.Start,
				Right: shiftExpr,
			}
		}
		if scan.TimeRange.End != nil {
			scan.TimeRange.End = &ast.Binary{
				Op:    ast.OpSub,
				Left:  scan.TimeRange.End,
				Right: shiftExpr,
			}
		}
	}
	// Also shift pushdown time bounds if present.
	if scan.Pushdown.TimeBounds != nil {
		if scan.Pushdown.TimeBounds.Start != nil {
			scan.Pushdown.TimeBounds.Start = &ast.Binary{
				Op:    ast.OpSub,
				Left:  scan.Pushdown.TimeBounds.Start,
				Right: shiftExpr,
			}
		}
		if scan.Pushdown.TimeBounds.End != nil {
			scan.Pushdown.TimeBounds.End = &ast.Binary{
				Op:    ast.OpSub,
				Left:  scan.Pushdown.TimeBounds.End,
				Right: shiftExpr,
			}
		}
	}
}

func (l *lowerer) lowerHelper(input Node, s ast.Stage, extra []sema.Field) Node {
	opts := make(map[string]ast.Expr)
	var positional []ast.Expr

	// Extract what we can from known payloads.
	switch s.Name {
	case "transaction":
		if s.Transaction != nil {
			positional = s.Transaction.Fields
			if s.Transaction.MaxSpan != nil {
				opts["maxspan"] = s.Transaction.MaxSpan
			}
			if s.Transaction.StartsWith != nil {
				opts["startswith"] = s.Transaction.StartsWith
			}
			if s.Transaction.EndsWith != nil {
				opts["endswith"] = s.Transaction.EndsWith
			}
		}
	case "correlate":
		if s.Correlate != nil {
			positional = []ast.Expr{s.Correlate.Field1, s.Correlate.Field2}
		}
	case "rollup":
		if s.Rollup != nil {
			positional = s.Rollup.Resolutions
		}
	case "xyseries":
		if s.Xyseries != nil {
			positional = []ast.Expr{s.Xyseries.X, s.Xyseries.Y, s.Xyseries.Value}
		}
	}

	return &Helper{
		unaryNode:   unaryNode{Input: input},
		Name:        s.Name,
		Options:     opts,
		Positional:  positional,
		extraFields: extra,
	}
}

func (l *lowerer) lowerHelperGeneric(input Node, s ast.Stage, gp *ast.GenericOptionsPayload, extra []sema.Field) Node {
	opts := make(map[string]ast.Expr)
	var positional []ast.Expr

	if gp != nil {
		positional = gp.Positionals
		for _, o := range gp.Options {
			opts[o.Name] = o.Value
		}
	}

	return &Helper{
		unaryNode:   unaryNode{Input: input},
		Name:        s.Name,
		Options:     opts,
		Positional:  positional,
		extraFields: extra,
	}
}

// helperExtraFields returns the fields a helper stage adds to the schema.
// Matches the sema analyzer's knowledge.
func helperExtraFields(name string) []sema.Field {
	switch name {
	case "patterns":
		return []sema.Field{
			{Name: "_pattern", Type: sema.TypeString},
			{Name: "_pattern_count", Type: sema.TypeInt},
		}
	case "outliers":
		return []sema.Field{
			{Name: "_outlier", Type: sema.TypeBool},
			{Name: "_outlier_score", Type: sema.TypeFloat},
		}
	case "sessionize":
		return []sema.Field{
			{Name: "_session_id", Type: sema.TypeString},
			{Name: "_session_start", Type: sema.TypeTimestamp},
			{Name: "_session_end", Type: sema.TypeTimestamp},
		}
	case "transaction":
		return []sema.Field{
			{Name: "duration", Type: sema.TypeDuration},
			{Name: "eventcount", Type: sema.TypeInt},
		}
	case "trace":
		return []sema.Field{
			{Name: "_depth", Type: sema.TypeInt},
			{Name: "_tree", Type: sema.TypeString},
		}
	case "topology":
		return []sema.Field{
			{Name: "_source_node", Type: sema.TypeString},
			{Name: "_dest_node", Type: sema.TypeString},
			{Name: "_edge_weight", Type: sema.TypeFloat},
		}
	case "correlate":
		return []sema.Field{
			{Name: "_correlation", Type: sema.TypeFloat},
		}
	case "rollup":
		return []sema.Field{
			{Name: "_resolution", Type: sema.TypeDuration},
		}
	case "xyseries":
		// xyseries pivots — output is dynamic.
		return nil
	default:
		return nil
	}
}

// exprStringValue extracts a string literal value from an expression.
// For raw string literals (r"..."), it returns the inner string.
func exprStringValue(e ast.Expr) string {
	if lit, ok := e.(*ast.Literal); ok {
		if s, ok := lit.Value.(string); ok {
			return s
		}
	}
	return e.String()
}

// exprFieldName extracts a simple field name from an expression.
func exprFieldName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.Member:
		return exprFieldName(x.Object) + "." + x.Field
	case *ast.Call:
		if len(x.Args) > 0 {
			return x.Callee + "(" + exprFieldName(x.Args[0]) + ")"
		}
		return x.Callee + "()"
	}
	return strings.ReplaceAll(e.String(), " ", "")
}
