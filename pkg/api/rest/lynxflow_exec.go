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
	"github.com/lynxbase/lynxdb/pkg/server"
	"github.com/lynxbase/lynxdb/pkg/spl2"
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
	defaultSrc := "main"
	indexName, scopeType := extractLynxFlowSourceScope(desugared)
	if indexName != "" {
		defaultSrc = indexName
	}
	hints := &spl2.QueryHints{IndexName: indexName}
	if scopeType != "" {
		hints.SourceScopeType = scopeType
		if scopeType == "single" && indexName != "" {
			hints.SourceScopeSources = []string{indexName}
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

	// Convert desugar rewrites to spl2.QueryRewrite for the existing envelope.
	if len(rewrites) > 0 {
		spl2Rewrites := make([]spl2.QueryRewrite, len(rewrites))
		for i, rw := range rewrites {
			spl2Rewrites[i] = spl2.QueryRewrite{
				Before: rw.Before,
				After:  rw.After,
				Reason: rw.Reason,
			}
		}
		metaOpts = append(metaOpts, WithRewrites(spl2Rewrites))
	}

	// Convert LF lints to spl2.QueryLint for the existing envelope.
	allLints := convertLynxFlowLints(lints, semaResult, lang)
	if len(allLints) > 0 {
		metaOpts = append(metaOpts, WithLints(allLints))
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
	hints := &spl2.QueryHints{IndexName: indexName}
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
func applyTimeRangeToHints(hints *spl2.QueryHints, from, to string) {
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
	src := q.Pipeline.Source
	if src == nil {
		return "main", "single"
	}
	if len(src.Sources) == 0 {
		return "main", "single"
	}
	// Single named source.
	if len(src.Sources) == 1 {
		s := src.Sources[0]
		switch s.Kind {
		case ast.SourceStar:
			return "*", "all"
		case ast.SourceName:
			name := s.Name
			if name == "" {
				name = "main"
			}
			return name, "single"
		case ast.SourceGlob:
			return s.Pattern, "glob"
		case ast.SourceCTE:
			// CTE: scan the default index.
			return "main", "single"
		}
	}
	// Multiple sources: list scope.
	names := make([]string, 0, len(src.Sources))
	for _, s := range src.Sources {
		switch s.Kind {
		case ast.SourceStar:
			return "*", "all"
		case ast.SourceName:
			if s.Name != "" {
				names = append(names, s.Name)
			}
		}
	}
	if len(names) == 1 {
		return names[0], "single"
	}
	// For multi-source, use the first name as indexName (the hints will use
	// SourceScopeSources for multi-source). We set IndexName="" so all indexes
	// are scanned and the physical layer resolves per-source.
	return "", "all"
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

// convertLynxFlowLints converts LF lints and sema warnings to the spl2.QueryLint
// type for the shared response envelope.
func convertLynxFlowLints(lfLints []lint.Lint, semaResult sema.Result, lang langDetectResult) []spl2.QueryLint {
	var out []spl2.QueryLint

	// Detection notice as a lint.
	if !lang.Explicit && lang.DetectNotice != "" {
		out = append(out, spl2.QueryLint{
			Code:    "LF_DETECT",
			Message: lang.DetectNotice,
		})
	}

	// LynxFlow lint rules.
	for _, l := range lfLints {
		out = append(out, spl2.QueryLint{
			Code:     l.Code,
			Message:  l.Message,
			Position: l.Span.Start,
		})
	}

	// Sema warnings.
	for _, d := range semaResult.Diags {
		if d.Severity == parser.SeverityWarning {
			out = append(out, spl2.QueryLint{
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
