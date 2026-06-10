package query

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/lynxbase/lynxdb/pkg/cluster/tracing"
	"github.com/lynxbase/lynxdb/pkg/event"
	"github.com/lynxbase/lynxdb/pkg/logical"
	"github.com/lynxbase/lynxdb/pkg/spl2"
)

// ExecuteQueryDual routes a query to the correct execution path based on
// detected language. SPL2 queries use the existing ExecuteQuery path
// (byte-identical behavior). LynxFlow queries use the new IR-based
// split+render path.
//
// This method exists during the migration window where both languages coexist.
// Once SPL2 is deleted, this collapses to ExecuteQueryIR only.
func (c *Coordinator) ExecuteQueryDual(
	ctx context.Context,
	lang string,
	spl2Prog *spl2.Program,
	spl2Hints *spl2.QueryHints,
	irPlan *logical.Plan,
	irPushdown *logical.Pushdown,
) (*DistributedQueryResult, error) {
	switch lang {
	case "spl2":
		return c.ExecuteQuery(ctx, spl2Prog, spl2Hints)
	case "lynxflow":
		return c.ExecuteQueryIR(ctx, irPlan, irPushdown)
	default:
		// Default to SPL2 if both are available, LynxFlow otherwise.
		if spl2Prog != nil {
			return c.ExecuteQuery(ctx, spl2Prog, spl2Hints)
		}
		return c.ExecuteQueryIR(ctx, irPlan, irPushdown)
	}
}

// ExecuteQueryIR plans, fans out, and merges a distributed query using the
// logical IR. It mirrors ExecuteQuery but operates on logical.Plan.
func (c *Coordinator) ExecuteQueryIR(
	ctx context.Context,
	plan *logical.Plan,
	pushdown *logical.Pushdown,
) (*DistributedQueryResult, error) {
	ctx, span := tracing.Tracer().Start(ctx, "lynxdb.query.distributed.ir")
	defer span.End()

	scanStart := time.Now()

	// 1. Plan the distributed query on the IR.
	irPlan, err := PlanDistributedQueryIR(plan)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "ir plan failed")
		return nil, fmt.Errorf("Coordinator.ExecuteQueryIR: plan: %w", err)
	}

	// 2. Find relevant shards using pushdown hints.
	hints := pushdownToQueryHints(pushdown)
	targets, err := c.pruner.FindRelevantShards(ctx, hints)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "prune shards failed")
		return nil, fmt.Errorf("Coordinator.ExecuteQueryIR: prune shards: %w", err)
	}

	span.SetAttributes(
		attribute.String(tracing.AttrQueryText, irPlan.ShardQuery),
		attribute.String(tracing.AttrMergeStrategy, irPlan.Strategy.String()),
		attribute.Int(tracing.AttrShardsTotal, len(targets)),
	)

	meta := QueryMeta{
		ShardsTotal: len(targets),
	}

	if len(targets) == 0 {
		return &DistributedQueryResult{Meta: meta}, nil
	}

	// 3. Build a DistributedPlan compatible with the existing fan-out methods.
	// This reuses executePartialAgg/executeConcat which only need ShardQuery,
	// Strategy, PartialAggSpec, TopK, and TopKSortFields.
	compatPlan := &DistributedPlan{
		ShardQuery:     irPlan.ShardQuery,
		Strategy:       irPlan.Strategy,
		PartialAggSpec: irPlan.PartialAggSpec,
		TopK:           irPlan.TopK,
		TopKSortFields: irPlan.TopKSortFields,
		SplitIndex:     irPlan.SplitIndex,
	}

	var rows []map[string]event.Value

	switch irPlan.Strategy {
	case MergePartialAgg, MergeTopK:
		rows, err = c.executePartialAgg(ctx, compatPlan, targets, &meta)
	case MergeConcat:
		rows, err = c.executeConcat(ctx, compatPlan, targets, &meta)
	default:
		return nil, fmt.Errorf("Coordinator.ExecuteQueryIR: unknown strategy %v", irPlan.Strategy)
	}

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "fan-out failed")
		return nil, err
	}

	scanMS := float64(time.Since(scanStart).Milliseconds())

	// 4. Apply coordinator commands via the IR coord nodes.
	// For now, if there are coord nodes, render them as LynxFlow text and
	// execute via the SPL2 pipeline (the coord pipeline is small).
	// TODO: When the physical builder is fully wired, build directly from
	// IR nodes. For now, fall back to the SPL2 coord pipeline via text.
	var mergeMS float64
	if len(irPlan.CoordNodes) > 0 && len(rows) > 0 {
		mergeStart := time.Now()
		coordText := logical.RenderPipeline(irPlan.CoordNodes...)
		if coordText != "" {
			rows, err = applyCoordPipelineText(ctx, rows, coordText)
			if err != nil {
				return nil, fmt.Errorf("Coordinator.ExecuteQueryIR: coord pipeline: %w", err)
			}
		}
		mergeMS = float64(time.Since(mergeStart).Milliseconds())
	}

	meta.Partial = meta.ShardsFailed > 0 && meta.ShardsSuccess > 0

	span.SetAttributes(
		attribute.Int(tracing.AttrShardsSuccess, meta.ShardsSuccess),
		attribute.Int(tracing.AttrShardsFailed, meta.ShardsFailed),
		attribute.Bool("lynxdb.query.partial", meta.Partial),
	)

	return &DistributedQueryResult{
		Rows:    rows,
		Meta:    meta,
		ScanMS:  scanMS,
		MergeMS: mergeMS,
	}, nil
}

// pushdownToQueryHints converts a logical.Pushdown to spl2.QueryHints for
// the shard pruner. This bridges the IR and the existing pruner which still
// reads spl2.QueryHints. When the pruner is ported to read logical.Pushdown
// directly, this function is deleted.
func pushdownToQueryHints(pd *logical.Pushdown) *spl2.QueryHints {
	if pd == nil {
		return &spl2.QueryHints{}
	}

	hints := &spl2.QueryHints{}

	if pd.TimeBounds != nil {
		hints.TimeBounds = &spl2.TimeBounds{}
		// Time bounds in the logical IR store AST expressions (relative or
		// absolute). The shard pruner needs resolved times. For now, leave
		// them zero (unbounded) — the shard will re-resolve from the query
		// text. This is correct but not optimal (extra shards scanned).
		// TODO: Resolve time expressions here when the time resolver is
		// available in the logical package.
	}

	hints.SearchTerms = pd.BloomTerms

	return hints
}

// applyCoordPipelineText runs coordinator pipeline stages rendered as LynxFlow
// text against the merged row set. It parses the text as SPL2 (since the coord
// commands are simple stages like sort/head/dedup) and executes via the
// existing pipeline builder.
//
// This is a temporary bridge. When pkg/logical/physical is wired into the
// coordinator, this function is replaced by direct physical.Build from IR nodes.
func applyCoordPipelineText(ctx context.Context, rows []map[string]event.Value, pipelineText string) ([]map[string]event.Value, error) {
	// Try to parse as SPL2 first (coord commands like sort, head, dedup
	// parse identically in both languages).
	prog, err := spl2.ParseProgram("| " + pipelineText)
	if err != nil {
		return rows, nil // If parse fails, return rows as-is.
	}
	if prog.Main == nil || len(prog.Main.Commands) == 0 {
		return rows, nil
	}
	return applyCoordCommands(ctx, rows, prog.Main.Commands)
}
