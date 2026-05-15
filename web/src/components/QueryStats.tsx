import { useState, useEffect } from "react";
import type {
  QueryStats as QueryStatsType,
  DetailedStats,
} from "../api/client";
import { formatCount, formatMs, formatBytes } from "../utils/format";
import { formatElapsed } from "../utils/format";
import styles from "./QueryStats.module.css";

interface QueryStatsProps {
  stats: QueryStatsType | null;
  loading: boolean;
  error: string | null;
  resultCount: number;
  tailActive?: boolean;
  tailEventCount?: number;
  tailCatchupDone?: boolean;

  // Streaming & Progress
  /** True while NDJSON search stream is active */
  streaming?: boolean;
  /** Row count ticking up during streaming */
  streamingCount?: number;
  /** Aggregation progress data from SSE */
  progress?: {
    percent: number;
    scanned: number;
    total: number;
    elapsedMs: number;
  } | null;
  /** True when query was canceled by user */
  canceled?: boolean;
  /** Elapsed milliseconds since query started (ticking live) */
  elapsedMs?: number;
  /** True when result is showing preview rows (not final) */
  isPreview?: boolean;

  // Detailed stats & explain
  /** Callback when user clicks the Explain button */
  onExplainToggle?: () => void;
  /** Whether explain data is available */
  explainAvailable?: boolean;
  /** Whether the SSE tail connection is reconnecting */
  tailReconnecting?: boolean;
}

/**
 * Produce the compact stats summary line.
 * Format: "142 results in 4.2ms -- 12/48 segments, 36 skipped (bloom: 24, time: 12)"
 */
function formatCompactStats(
  stats: QueryStatsType,
  resultCount: number,
): string {
  const ds = stats.stats as DetailedStats | undefined;

  const parts: string[] = [];
  parts.push(
    `${formatCount(resultCount)} ${resultCount === 1 ? "result" : "results"}`,
  );
  parts.push(`in ${formatMs(stats.took_ms)}`);

  if (ds?.segments_total != null && ds.segments_scanned != null) {
    const skipped = ds.segments_total - ds.segments_scanned;
    let segPart = `${ds.segments_scanned}/${ds.segments_total} segments`;
    if (skipped > 0) {
      const skipDetails: string[] = [];
      if (ds.segments_skipped_bloom && ds.segments_skipped_bloom > 0) {
        skipDetails.push(`bloom: ${ds.segments_skipped_bloom}`);
      }
      if (ds.segments_skipped_time && ds.segments_skipped_time > 0) {
        skipDetails.push(`time: ${ds.segments_skipped_time}`);
      }
      if (ds.segments_skipped_index && ds.segments_skipped_index > 0) {
        skipDetails.push(`index: ${ds.segments_skipped_index}`);
      }
      if (ds.segments_skipped_range && ds.segments_skipped_range > 0) {
        skipDetails.push(`range: ${ds.segments_skipped_range}`);
      }
      segPart += `, ${skipped} skipped`;
      if (skipDetails.length > 0) {
        segPart += ` (${skipDetails.join(", ")})`;
      }
    }
    parts.push(`\u2014 ${segPart}`);
  } else if (stats.scanned > 0) {
    parts.push(`(scanned ${formatCount(stats.scanned)})`);
  }

  return parts.join(" ");
}

/**
 * Return string array of active optimization badge names from detailed stats.
 */
function getOptimizationBadges(ds: DetailedStats): string[] {
  const badges: string[] = [];
  if (ds.cache_hit) badges.push("cache");
  if (ds.segments_skipped_bloom && ds.segments_skipped_bloom > 0)
    badges.push("bloom");
  if (ds.partial_agg_used) badges.push("partial-agg");
  if (ds.topk_used) badges.push("TopK");
  if (ds.vectorized_filter_used) badges.push("vectorized");
  if (ds.dict_filter_used) badges.push("dict-filter");
  if (ds.count_star_optimized) badges.push("count(*)");
  if (ds.inverted_index_hits && ds.inverted_index_hits > 0)
    badges.push("inverted-idx");
  return badges;
}

export function QueryStatsBar({
  stats,
  loading,
  error,
  resultCount,
  tailActive,
  tailEventCount,
  tailCatchupDone,
  streaming,
  streamingCount,
  progress,
  canceled,
  elapsedMs,
  isPreview,
  onExplainToggle,
  explainAvailable,
  tailReconnecting,
}: QueryStatsProps) {
  // Expand/collapse state for detailed stats row. Resets on new stats (Pitfall 3).
  const [expanded, setExpanded] = useState(false);
  useEffect(() => {
    setExpanded(false);
  }, [stats]);

  /* --- Live Tail mode --- */
  if (tailActive) {
    const count = tailEventCount ?? 0;

    // Show error/warning inline even in tail mode
    if (error) {
      return (
        <div className={styles.bar} role="alert">
          <span className={styles.tailDot} aria-hidden="true" />
          <span className={styles.tailLabel}>Live Tail</span>
          <span className={styles.errorMsg}>{error}</span>
        </div>
      );
    }

    // Reconnecting state: amber dot and "Reconnecting..." label
    if (tailReconnecting) {
      return (
        <div className={styles.bar} role="status" aria-live="polite">
          <span className={styles.reconnectingDot} aria-hidden="true" />
          <span className={styles.reconnectingLabel}>Reconnecting...</span>
          <span className={styles.tailSep} aria-hidden="true">
            &mdash;
          </span>
          <span>
            {formatCount(count)} {count === 1 ? "event" : "events"}
          </span>
        </div>
      );
    }

    const statusText = tailCatchupDone
      ? `${formatCount(count)} ${count === 1 ? "event" : "events"}`
      : `Catching up\u2026 ${formatCount(count)} ${count === 1 ? "event" : "events"}`;

    return (
      <div className={styles.bar} role="status" aria-live="polite">
        <span className={styles.tailDot} aria-hidden="true" />
        <span className={styles.tailLabel}>Live Tail</span>
        <span className={styles.tailSep} aria-hidden="true">
          &mdash;
        </span>
        <span>{statusText}</span>
      </div>
    );
  }

  /* --- Canceled state --- */
  if (canceled) {
    const elapsed = formatElapsed(elapsedMs ?? 0);
    const hasPartialResults =
      streamingCount !== undefined && streamingCount > 0;

    return (
      <div className={styles.bar} role="status" aria-live="polite">
        <span className={styles.canceledIcon} aria-hidden="true">
          &#9888;
        </span>
        {hasPartialResults
          ? `Canceled \u2014 ${formatCount(streamingCount!)} partial results in ${elapsed}`
          : `Canceled \u2014 ${elapsed}`}
      </div>
    );
  }

  /* --- Streaming state (NDJSON search in progress) --- */
  if (streaming) {
    return (
      <div className={styles.bar} role="status" aria-live="polite">
        <span className={styles.streamingDot} aria-hidden="true" />
        {`${formatCount(streamingCount ?? 0)} results (streaming...) \u2014 ${formatElapsed(elapsedMs ?? 0)}`}
      </div>
    );
  }

  /* --- Progress state (aggregation with progress bar) --- */
  if (progress) {
    return (
      <div className={styles.bar} role="status" aria-live="polite">
        <div className={styles.progressTrack}>
          <div
            className={styles.progressFill}
            style={{ width: `${progress.percent}%` }}
          />
        </div>
        {`${formatCount(progress.scanned)}/${formatCount(progress.total)} segments (${Math.round(progress.percent)}%) \u2014 ${formatElapsed(elapsedMs ?? progress.elapsedMs)}`}
        {isPreview && (
          <span className={styles.previewHint}>Showing partial results\u2026</span>
        )}
      </div>
    );
  }

  /* --- Standard query mode --- */
  if (loading) {
    return (
      <div className={styles.bar} role="status" aria-live="polite">
        <span className={styles.spinner} aria-hidden="true" />
        Running query...
      </div>
    );
  }

  if (error) {
    return (
      <div className={styles.bar} role="alert">
        <span className={styles.errorIcon} aria-hidden="true">
          &#9888;
        </span>
        <span className={styles.errorMsg}>{error}</span>
      </div>
    );
  }

  if (!stats) {
    return <div className={styles.bar}>Ready</div>;
  }

  // --- Completed query with compact/expanded stats ---
  const compactText = formatCompactStats(stats, resultCount);
  const ds = stats.stats as DetailedStats | undefined;
  const badges = ds ? getOptimizationBadges(ds) : [];

  // MV acceleration info from query response meta
  const acceleratedBy = ds?.accelerated_by;
  const mvSpeedup = ds?.mv_speedup;

  // Determine if we have detail data to expand
  const hasDetail =
    ds &&
    (ds.scan_ms != null ||
      ds.pipeline_ms != null ||
      badges.length > 0 ||
      ds.processed_bytes != null);

  return (
    <div
      className={hasDetail ? styles.barColumn : styles.bar}
      role="status"
      aria-live="polite"
    >
      <div className={styles.compactLine}>
        <span className={styles.success} aria-hidden="true">
          &#10003;
        </span>
        <span>{compactText}</span>
        {acceleratedBy && (
          <span className={styles.mvBadge}>
            <span className={styles.mvIcon} aria-hidden="true">
              &#9889;
            </span>
            MV: {acceleratedBy}
            {mvSpeedup && ` (~${mvSpeedup})`}
          </span>
        )}
        {hasDetail && (
          <button
            type="button"
            className={styles.expandToggle}
            onClick={() => setExpanded(!expanded)}
            aria-label={expanded ? "Collapse details" : "Expand details"}
            aria-expanded={expanded}
          >
            {expanded ? "\u25B2" : "\u25BC"}
          </button>
        )}
        {explainAvailable && onExplainToggle && (
          <button
            type="button"
            className={styles.explainBtn}
            onClick={onExplainToggle}
          >
            Explain
          </button>
        )}
      </div>
      {expanded && ds && (
        <div className={styles.expandedRow}>
          {ds.scan_ms != null && (
            <span className={styles.latencyDetail}>
              Scan: {formatMs(ds.scan_ms)}
            </span>
          )}
          {ds.pipeline_ms != null && (
            <span className={styles.latencyDetail}>
              Pipeline: {formatMs(ds.pipeline_ms)}
            </span>
          )}
          {ds.parse_ms != null && (
            <span className={styles.latencyDetail}>
              Parse: {formatMs(ds.parse_ms)}
            </span>
          )}
          {ds.optimize_ms != null && (
            <span className={styles.latencyDetail}>
              Optimize: {formatMs(ds.optimize_ms)}
            </span>
          )}
          {badges.map((b) => (
            <span key={b} className={styles.badge}>
              {b}
            </span>
          ))}
          {ds.processed_bytes != null && ds.processed_bytes > 0 && (
            <span className={styles.latencyDetail}>
              {formatBytes(ds.processed_bytes)} processed
            </span>
          )}
        </div>
      )}
    </div>
  );
}
