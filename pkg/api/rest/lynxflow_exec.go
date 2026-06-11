package rest

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/lynxbase/lynxdb/pkg/engine/pipeline"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/lint"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/parser"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/run"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/sema"
	"github.com/lynxbase/lynxdb/pkg/model"
	"github.com/lynxbase/lynxdb/pkg/planner"
	"github.com/lynxbase/lynxdb/pkg/usecases"
)

// executeLynxFlowQuery runs a LynxFlow query via the QueryService (which
// delegates to the planner and engine for full segment-streaming scan, result
// cache, scan stats, job infrastructure, and sync/hybrid/async dispatch).
func (s *Server) executeLynxFlowQuery(w http.ResponseWriter, r *http.Request, req QueryRequest, lang langDetectResult) {
	query := req.effectiveQuery()
	query = substituteVariables(query, req.Variables)

	skipResultCache := planner.DynamicTimeBounds(req.effectiveFrom(), req.effectiveTo()) ||
		planner.QueryUsesDynamicTimeSyntax(query)

	mode, wait := mapQueryMode(req.Wait)
	queryCfg := s.currentQueryConfig()
	limit := clampLimit(req.Limit, queryCfg)

	result, err := s.queryService.Submit(r.Context(), usecases.SubmitRequest{
		Query:           query,
		From:            req.effectiveFrom(),
		To:              req.effectiveTo(),
		Limit:           limit,
		Offset:          req.Offset,
		Mode:            mode,
		Wait:            wait,
		Profile:         req.Profile,
		NoLint:          req.Lint != nil && !*req.Lint,
		NoSuggestions:   req.Suggestions != nil && !*req.Suggestions,
		LintLimit:       req.LintLimit,
		LintFull:        req.LintFull,
		SkipResultCache: skipResultCache,
	})
	if err != nil {
		// Check for planner.ParseError with Diag for the structured error envelope.
		var pe *planner.ParseError
		if errors.As(err, &pe) && pe.Diag != nil {
			respondLynxFlowParseError(w, *pe.Diag)
			return
		}
		handlePlanError(w, err)
		return
	}

	langOpt := WithLanguage(string(lang.Language))

	if result.Done {
		if result.Error != "" {
			respondQueryError(w, result.Error, result.ErrorCode)
			return
		}

		// Append LF_DETECT detection-notice lint when language was auto-detected.
		lints := result.Lints
		if !lang.Explicit && lang.DetectNotice != "" {
			detectLint := model.QueryLint{
				Code:    "LF_DETECT",
				Message: lang.DetectNotice,
			}
			lints = appendLintIfAbsent(lints, detectLint)
		}

		lintsEnabled := req.Lint == nil || *req.Lint
		writeSyncResultFromUsecase(w, result, limit, req.Offset, query, queryCfg,
			lintsEnabled, req.LintLimit, req.LintFull,
			langOpt, WithLints(lints))
	} else {
		// Append LF_DETECT to the job handle lints.
		if !lang.Explicit && lang.DetectNotice != "" {
			detectLint := model.QueryLint{
				Code:    "LF_DETECT",
				Message: lang.DetectNotice,
			}
			result.Lints = appendLintIfAbsent(result.Lints, detectLint)
		}
		writeJobHandleFromUsecase(w, result, langOpt)
	}
}

// appendLintIfAbsent appends a lint to the slice if its code is not already present.
func appendLintIfAbsent(lints []model.QueryLint, lint model.QueryLint) []model.QueryLint {
	for _, l := range lints {
		if l.Code == lint.Code {
			return lints
		}
	}
	return append(lints, lint)
}

// executeLynxFlowStream runs a LynxFlow query as a streaming NDJSON response
// via queryService.Stream (which delegates to the planner and engine for
// segment-streaming scan with hints).
func (s *Server) executeLynxFlowStream(w http.ResponseWriter, r *http.Request, query string, req QueryRequest, lang langDetectResult) {
	start := time.Now()

	iter, stats, err := s.queryService.Stream(r.Context(), usecases.StreamRequest{
		Query: query,
		From:  req.effectiveFrom(),
		To:    req.effectiveTo(),
	})
	if err != nil {
		// Check for planner.ParseError with Diag for the structured error envelope.
		var pe *planner.ParseError
		if errors.As(err, &pe) && pe.Diag != nil {
			respondLynxFlowParseError(w, *pe.Diag)
			return
		}
		handlePlanError(w, err)
		return
	}
	defer iter.Close()

	// Set streaming headers before first write.
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
			if encErr := enc.Encode(map[string]interface{}{
				"__error": map[string]interface{}{
					"code":    "STREAM_ERROR",
					"message": err.Error(),
				},
			}); encErr != nil {
				slog.Warn("rest: stream json encode failed", "error", encErr)
			}
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
			if encErr := enc.Encode(out); encErr != nil {
				slog.Warn("rest: stream json encode failed", "error", encErr)
			}
			total++
		}
		if flusher != nil {
			flusher.Flush()
		}
	}

	// Attempt to extract post-drain scan stats from the iterator.
	scanned := stats.RowsScanned
	if ss, ok := iter.(interface {
		ScanStats() *pipeline.SegmentStreamStats
	}); ok {
		if segStats := ss.ScanStats(); segStats != nil {
			scanned = segStats.EventsScanned
		}
	}

	elapsed := time.Since(start)
	if encErr := enc.Encode(map[string]interface{}{
		"__meta": map[string]interface{}{
			"total":   total,
			"scanned": scanned,
			"took_ms": elapsed.Milliseconds(),
		},
	}); encErr != nil {
		slog.Warn("rest: stream json encode failed", "error", encErr)
	}
	if flusher != nil {
		flusher.Flush()
	}
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

// convertLynxFlowLints converts LF lints and sema warnings to the model.QueryLint
// type for the shared response envelope. Retained for the LF_DETECT detection-
// notice logic; the primary lint path now runs inside the planner/queryService.
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
