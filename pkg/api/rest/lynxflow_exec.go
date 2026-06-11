package rest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/lynxbase/lynxdb/pkg/engine/pipeline"
	"github.com/lynxbase/lynxdb/pkg/event"
	"github.com/lynxbase/lynxdb/pkg/logical"
	"github.com/lynxbase/lynxdb/pkg/logical/opt"
	"github.com/lynxbase/lynxdb/pkg/logical/physical"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/ast"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/desugar"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/lint"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/parser"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/run"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/sema"
	"github.com/lynxbase/lynxdb/pkg/model"
	"github.com/lynxbase/lynxdb/pkg/server"
)

// executeLynxFlowQuery runs a LynxFlow query against the server engine and
// writes the result using the standard query response envelope.
func (s *Server) executeLynxFlowQuery(w http.ResponseWriter, r *http.Request, req QueryRequest, lang langDetectResult) {
	query := req.effectiveQuery()
	start := time.Now()

	// 1. Parse
	q, diags := parser.Parse(query)
	for _, d := range diags {
		if d.Severity == parser.SeverityError {
			respondLynxFlowParseError(w, d)
			return
		}
	}

	// 2. Desugar
	desugared, rewrites := desugar.Desugar(q, desugar.Options{DefaultSource: "main"})

	// 3. Semantic analysis (advisory; does not block execution).
	semaResult := sema.Analyze(desugared, sema.MapCatalog(nil))

	// 4. Lint
	lints := lint.Run(desugared)

	// 5. Lower to logical plan
	plan, lowerDiags := logical.Lower(desugared, logical.Options{DefaultSource: "main"})
	for _, d := range lowerDiags {
		if d.Severity == parser.SeverityError {
			respondError(w, ErrCodeInvalidQuery, http.StatusBadRequest,
				"lynxflow lower error: "+d.Message)
			return
		}
	}

	// 6. Optimize
	plan, _ = opt.Optimize(plan)

	// 7. Build event store from engine (reuse the SPL2 path's storage access).
	// Extract the source index from the desugared AST so that queries like
	// "FROM logs" correctly read from the "logs" index, not the hardcoded "main".
	// For CTEs, we also scan CTE bindings for their sources to ensure all
	// referenced indexes are loaded.
	defaultSrc := "main"
	indexName, scopeType := extractLynxFlowSourceScope(desugared)
	if indexName != "" {
		defaultSrc = indexName
	}
	hints := &model.QueryHints{IndexName: indexName}
	if scopeType != "" {
		hints.SourceScopeType = scopeType
		if scopeType == "single" && indexName != "" {
			hints.SourceScopeSources = []string{indexName}
		}
	}
	// When the main pipeline references a CTE, defaultSrc may be "main"
	// but the actual data lives in a different index. Override defaultSrc
	// to the CTE's source if the main pipeline is a CTE reference.
	if desugared != nil && len(desugared.Lets) > 0 && desugared.Pipeline.Source != nil {
		if len(desugared.Pipeline.Source.Sources) == 1 && desugared.Pipeline.Source.Sources[0].Kind == ast.SourceCTE {
			// The main pipeline references a CTE; set defaultSrc to first CTE's source.
			for _, let := range desugared.Lets {
				if let.Pipeline.Source != nil {
					for _, s := range let.Pipeline.Source.Sources {
						if s.Kind == ast.SourceName && s.Name != "" {
							defaultSrc = s.Name
							break
						}
					}
					break
				}
			}
		}
	}
	applyTimeRangeToHints(hints, req.effectiveFrom(), req.effectiveTo())

	eventStore := s.engine.BuildEventStoreFromHints(hints)

	// 8. Build physical pipeline with the engine's event store.
	source := physical.NewStorageSourceFromMap(eventStore, defaultSrc)
	iter, err := physical.Build(plan, physical.BuildOptions{
		Source: source,
		Now:    time.Now(),
	})
	if err != nil {
		respondInternalError(w, fmt.Sprintf("lynxflow build: %v", err))
		return
	}

	// 9. Drain
	rows, err := pipeline.CollectAll(r.Context(), iter)
	if err != nil {
		respondInternalError(w, fmt.Sprintf("lynxflow execute: %v", err))
		return
	}

	elapsed := time.Since(start)

	// 10. Build response.
	queryCfg := s.currentQueryConfig()
	limit := clampLimit(req.Limit, queryCfg)

	data := buildLynxFlowResponse(rows, limit, req.Offset, plan)

	// Build meta options.
	metaOpts := []MetaOpt{
		WithTook(elapsed),
		WithLanguage(string(lang.Language)),
	}

	// Convert desugar rewrites to model.QueryRewrite for the existing envelope.
	if len(rewrites) > 0 {
		spl2Rewrites := make([]model.QueryRewrite, len(rewrites))
		for i, rw := range rewrites {
			spl2Rewrites[i] = model.QueryRewrite{
				Before: rw.Before,
				After:  rw.After,
				Reason: rw.Reason,
			}
		}
		metaOpts = append(metaOpts, WithRewrites(spl2Rewrites))
	}

	// Convert LF lints to model.QueryLint for the existing envelope, unless suppressed.
	lintEnabled := req.Lint == nil || *req.Lint
	if lintEnabled {
		allLints := model.PrepareQueryLints(convertLynxFlowLints(lints, semaResult, lang))
		if len(allLints) > 0 {
			metaOpts = append(metaOpts, WithLints(allLints))
		}
	}

	respondData(w, http.StatusOK, data, metaOpts...)
}

// executeLynxFlowStream runs a LynxFlow query as a streaming NDJSON response.
func (s *Server) executeLynxFlowStream(w http.ResponseWriter, r *http.Request, query string, req QueryRequest, lang langDetectResult) {
	start := time.Now()

	// 1. Parse
	q, diags := parser.Parse(query)
	for _, d := range diags {
		if d.Severity == parser.SeverityError {
			respondLynxFlowParseError(w, d)
			return
		}
	}

	// 2. Desugar
	desugared, _ := desugar.Desugar(q, desugar.Options{DefaultSource: "main"})

	// 3. Lower
	plan, lowerDiags := logical.Lower(desugared, logical.Options{DefaultSource: "main"})
	for _, d := range lowerDiags {
		if d.Severity == parser.SeverityError {
			respondError(w, ErrCodeInvalidQuery, http.StatusBadRequest,
				"lynxflow lower error: "+d.Message)
			return
		}
	}

	// 4. Optimize
	plan, _ = opt.Optimize(plan)

	// 5. Build event store.
	defaultSrc := "main"
	indexName, scopeType := extractLynxFlowSourceScope(desugared)
	if indexName != "" {
		defaultSrc = indexName
	}
	hints := &model.QueryHints{IndexName: indexName}
	if scopeType != "" {
		hints.SourceScopeType = scopeType
		if scopeType == "single" && indexName != "" {
			hints.SourceScopeSources = []string{indexName}
		}
	}
	applyTimeRangeToHints(hints, req.effectiveFrom(), req.effectiveTo())
	eventStore := s.engine.BuildEventStoreFromHints(hints)

	// 6. Build physical pipeline.
	source := physical.NewStorageSourceFromMap(eventStore, defaultSrc)
	iter, err := physical.Build(plan, physical.BuildOptions{
		Source: source,
		Now:    time.Now(),
	})
	if err != nil {
		respondInternalError(w, fmt.Sprintf("lynxflow build: %v", err))
		return
	}

	// 7. Stream results as NDJSON.
	streamLynxFlowResults(w, r, iter, start, lang)
}

// executeLynxFlowExplain returns the EXPLAIN output for a LynxFlow query.
func (s *Server) executeLynxFlowExplain(w http.ResponseWriter, query string, lang langDetectResult) {
	explainText, err := run.ExecuteExplain(query, run.Options{DefaultSource: "main"})
	if err != nil {
		// Match the SPL2 explain behavior: return 200 with is_valid=false
		// and the error in the errors array, not a 400 error response.
		resp := map[string]interface{}{
			"is_valid": false,
			"errors": []interface{}{
				map[string]interface{}{
					"message": err.Error(),
				},
			},
		}
		respondData(w, http.StatusOK, resp, WithLanguage(string(lang.Language)))
		return
	}

	// Build a response compatible with the existing explain shape, with an
	// additive lynxflow_plan field.
	resp := map[string]interface{}{
		"is_valid":      true,
		"lynxflow_plan": explainText,
		"errors":        []interface{}{},
	}

	respondData(w, http.StatusOK, resp, WithLanguage(string(lang.Language)))
}

// respondLynxFlowParseError maps a LynxFlow parser.Diag to the structured
// error contract: {code, message, position{start,end}, expected, suggestion}.
//
// Uses the same top-level error code (INVALID_QUERY) as the SPL2 path for
// backward compatibility. The detailed diag code (e.g. E001) is carried in
// the "diag_code" field for callers that want finer-grained classification.
func respondLynxFlowParseError(w http.ResponseWriter, d parser.Diag) {
	errObj := map[string]interface{}{
		"code":      string(ErrCodeInvalidQuery),
		"diag_code": string(d.Code),
		"message":   "parse error: " + d.Message,
	}
	if d.Span.Start != 0 || d.Span.End != 0 {
		errObj["position"] = map[string]interface{}{
			"start": d.Span.Start,
			"end":   d.Span.End,
		}
	}
	if len(d.Expected) > 0 {
		errObj["expected"] = d.Expected
	}
	if d.Suggestion != "" {
		errObj["suggestion"] = d.Suggestion
	}

	respondJSON(w, http.StatusBadRequest, map[string]interface{}{"error": errObj})
}

// applyTimeRangeToHints translates request from/to params into QueryHints
// time bounds, mirroring the SPL2 path's behavior.
func applyTimeRangeToHints(hints *model.QueryHints, from, to string) {
	if from == "" && to == "" {
		return
	}
	tb, err := server.ParseTimeBoundsStrict(from, to)
	if err != nil || tb == nil {
		return
	}
	hints.TimeBounds = tb
}

// extractLynxFlowSourceScope extracts the source index name and scope type
// from a desugared LynxFlow AST. This is used to build QueryHints that match
// the SPL2 path's data access behavior (source routing, buffered events, disk parts).
//
// Returns ("main", "single") for a bare "from main", ("logs", "single") for
// "from logs", ("", "all") for "from *", and ("main", "single") as default
// when no from stage is present (the desugarer inserts from main).
func extractLynxFlowSourceScope(q *ast.Query) (indexName, scopeType string) {
	if q == nil {
		return "main", "single"
	}

	// Collect sources from the main pipeline AND all CTE bindings so that
	// server-mode queries like:
	//   let $errs = from idx_backend | where level == "ERROR"; from $errs | ...
	// correctly load events from idx_backend (not just "main").
	allNames := collectPipelineSources(&q.Pipeline)
	for i := range q.Lets {
		allNames = append(allNames, collectPipelineSources(&q.Lets[i].Pipeline)...)
	}

	// Also scan union/join sub-pipelines in each stage.
	allNames = append(allNames, collectSubPipelineSources(q)...)

	// Deduplicate and resolve.
	unique := deduplicateStrings(allNames)

	// Check for star/glob.
	for _, n := range unique {
		if n == "*" {
			return "*", "all"
		}
	}

	switch len(unique) {
	case 0:
		return "main", "single"
	case 1:
		return unique[0], "single"
	default:
		// Multiple indexes referenced: load all.
		return "", "all"
	}
}

// collectPipelineSources extracts named index sources from a pipeline's FROM clause.
func collectPipelineSources(p *ast.Pipeline) []string {
	if p == nil || p.Source == nil {
		return nil
	}
	var names []string
	for _, s := range p.Source.Sources {
		switch s.Kind {
		case ast.SourceStar:
			names = append(names, "*")
		case ast.SourceName:
			if s.Name != "" {
				names = append(names, s.Name)
			}
		case ast.SourceGlob:
			names = append(names, s.Pattern)
			// SourceCTE: skip; the CTE itself is scanned for sources.
		}
	}
	return names
}

// collectSubPipelineSources scans all stages (including CTE stages) for
// sub-pipelines in union and join stages that may reference additional indexes.
func collectSubPipelineSources(q *ast.Query) []string {
	var names []string

	scanStages := func(stages []ast.Stage) {
		for _, st := range stages {
			if st.Union != nil {
				for i := range st.Union.Sources {
					sp := &st.Union.Sources[i]
					if sp.Pipeline != nil {
						names = append(names, collectPipelineSources(sp.Pipeline)...)
					}
				}
			}
			if st.Join != nil && st.Join.Right != nil {
				if st.Join.Right.Pipeline != nil {
					names = append(names, collectPipelineSources(st.Join.Right.Pipeline)...)
				}
			}
		}
	}

	scanStages(q.Pipeline.Stages)
	for _, let := range q.Lets {
		scanStages(let.Pipeline.Stages)
	}
	return names
}

// deduplicateStrings returns unique strings preserving first-occurrence order.
func deduplicateStrings(ss []string) []string {
	seen := make(map[string]struct{}, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

// buildLynxFlowResponse converts LynxFlow result rows to the correct response
// envelope: "aggregate" (with columns+rows) when the plan root is an Aggregate
// or TopK over Aggregate, or "events" (with events array) otherwise.
// This produces byte-compatible output with the SPL2 path's writeSyncResultFromUsecase.
func buildLynxFlowResponse(rows []map[string]event.Value, limit, offset int, plan *logical.Plan) map[string]interface{} {
	if isAggregatePlan(plan) {
		return buildLynxFlowAggregateResponse(rows, limit, offset)
	}
	return buildLynxFlowEventsResponse(rows, limit, offset)
}

// isAggregatePlan walks the plan root backwards through transparent nodes
// (Limit, Sort, Project, TopK) to find whether the plan is aggregate-rooted.
func isAggregatePlan(plan *logical.Plan) bool {
	if plan == nil || plan.Root == nil {
		return false
	}
	return isAggregateNode(plan.Root)
}

// isAggregateNode checks if a node is or wraps an aggregate.
func isAggregateNode(n logical.Node) bool {
	switch nd := n.(type) {
	case *logical.Aggregate:
		return nd.Window == nil // windowed (eventstats/streamstats) produce events, not aggregate
	case *logical.TopK:
		return true
	case *logical.Describe:
		return true
	// Transparent nodes: check child.
	case *logical.Limit:
		return isAggregateNode(nd.Input)
	case *logical.Sort:
		return isAggregateNode(nd.Input)
	case *logical.Project:
		return isAggregateNode(nd.Input)
	}
	return false
}

// buildLynxFlowAggregateResponse converts LynxFlow result rows to the
// aggregate response shape with columns and row arrays, byte-compatible
// with the SPL2 path's buildAggregateResponse.
func buildLynxFlowAggregateResponse(rows []map[string]event.Value, limit, offset int) map[string]interface{} {
	total := len(rows)
	if offset > 0 && offset < len(rows) {
		rows = rows[offset:]
	} else if offset >= len(rows) {
		rows = nil
	}
	hasMore := limit > 0 && len(rows) > limit
	if limit > 0 && limit < len(rows) {
		rows = rows[:limit]
	}

	if len(rows) == 0 {
		return map[string]interface{}{
			"type": "aggregate", "columns": []string{}, "rows": [][]interface{}{}, "total_rows": total, "has_more": false,
		}
	}

	// Collect column names from all rows.
	seen := map[string]struct{}{}
	for _, row := range rows {
		for k := range row {
			seen[k] = struct{}{}
		}
	}
	cols := orderColumns(seen)

	tableRows := make([][]interface{}, len(rows))
	for i, row := range rows {
		r := make([]interface{}, len(cols))
		for j, col := range cols {
			if v, ok := row[col]; ok {
				r[j] = v.Interface()
			}
		}
		tableRows[i] = r
	}

	return map[string]interface{}{
		"type": "aggregate", "columns": cols, "rows": tableRows, "total_rows": total, "has_more": hasMore,
	}
}

// buildLynxFlowEventsResponse converts LynxFlow result rows to the standard
// events response shape.
func buildLynxFlowEventsResponse(rows []map[string]event.Value, limit, offset int) map[string]interface{} {
	total := len(rows)
	if offset > 0 && offset < len(rows) {
		rows = rows[offset:]
	} else if offset >= len(rows) {
		rows = nil
	}
	hasMore := len(rows) > limit
	if limit > 0 && limit < len(rows) {
		rows = rows[:limit]
	}

	events := make([]map[string]interface{}, len(rows))
	for i, row := range rows {
		m := make(map[string]interface{}, len(row))
		for k, v := range row {
			m[k] = v.Interface()
		}
		events[i] = m
	}

	return map[string]interface{}{
		"type": "events", "events": events,
		"total": total, "has_more": hasMore,
	}
}

// convertLynxFlowLints converts LF lints and sema warnings to the model.QueryLint
// type for the shared response envelope.
func convertLynxFlowLints(lfLints []lint.Lint, semaResult sema.Result, lang langDetectResult) []model.QueryLint {
	var out []model.QueryLint

	// Detection notice as a lint.
	if !lang.Explicit && lang.DetectNotice != "" {
		out = append(out, model.QueryLint{
			Code:    "LF_DETECT",
			Message: lang.DetectNotice,
		})
	}

	// LynxFlow lint rules.
	for _, l := range lfLints {
		out = append(out, model.QueryLint{
			Code:     l.Code,
			Message:  l.Message,
			Reason:   l.Reason,
			Position: l.Span.Start,
		})
	}

	// Sema warnings.
	for _, d := range semaResult.Diags {
		if d.Severity == parser.SeverityWarning {
			out = append(out, model.QueryLint{
				Code:     string(d.Code),
				Message:  d.Message,
				Position: d.Span.Start,
			})
		}
	}

	return out
}

// streamLynxFlowResults writes LynxFlow results as NDJSON, matching the SPL2
// streaming response format.
func streamLynxFlowResults(w http.ResponseWriter, r *http.Request, iter pipeline.Iterator, startTime time.Time, _ langDetectResult) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	enc := json.NewEncoder(w)
	total := 0

	for {
		if err := r.Context().Err(); err != nil {
			return
		}
		batch, err := iter.Next(r.Context())
		if err != nil {
			_ = enc.Encode(map[string]interface{}{
				"__error": map[string]interface{}{
					"code":    "STREAM_ERROR",
					"message": err.Error(),
				},
			})
			if flusher != nil {
				flusher.Flush()
			}
			return
		}
		if batch == nil {
			break
		}
		for i := 0; i < batch.Len; i++ {
			row := batch.Row(i)
			out := rowToInterface(row)
			_ = enc.Encode(out)
			total++
		}
		if flusher != nil {
			flusher.Flush()
		}
	}

	elapsed := time.Since(startTime)
	_ = enc.Encode(map[string]interface{}{
		"__meta": map[string]interface{}{
			"total":   total,
			"took_ms": elapsed.Milliseconds(),
			"scanned": total, // parity with the SPL2 stream path
		},
	})
	if flusher != nil {
		flusher.Flush()
	}
}
