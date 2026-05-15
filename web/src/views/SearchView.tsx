import { useCallback, useEffect, useRef } from "react";
import { QueryEditor } from "../editor/QueryEditor";
import type { QueryEditorHandle } from "../editor/QueryEditor";
import { TimeRangePicker } from "../components/TimeRangePicker";
import { ResultsTable } from "../components/ResultsTable";
import { QueryStatsBar } from "../components/QueryStats";
import { FlowSidebar } from "../components/FlowSidebar";
import { Timeline } from "../components/Timeline";
import { LiveTailButton } from "../components/LiveTailButton";
import { ExplainInspector } from "../components/ExplainInspector";
import { TableToolbar } from "../components/TableToolbar";
import { PaginationBar } from "../components/PaginationBar";
import { ListView } from "../components/ListView";
import { CopyTooltip } from "../components/CopyTooltip";
import { useKeyboardShortcuts } from "../hooks/useKeyboardShortcuts";
import {
  fetchHistogram,
  fetchHistogramGrouped,
  fetchIndexes,
  fetchViews,
  fetchExplain,
  fetchFields,
} from "../api/client";
import { submitHybridQuery, subscribeJobProgress } from "../api/streaming";
import { authHeaders } from "../api/auth";
import { startTail } from "../api/sse";
import { pushHistory } from "../stores/queryHistory";
import { useSearchStore } from "../stores/search";
import {
  useOverlayStore,
  setPaletteOpen,
  setHelpOverlayOpen,
  setPaletteQuery,
  formatShortcut,
  SHORTCUTS,
} from "../utils/keyboard";
import { writeQueryToHash, readQueryFromHash } from "../stores/queryUrl";
import {
  dispatchDiagnostics,
  clearEditorDiagnostics,
} from "../editor/diagnostics";
import {
  generateCSV,
  generateJSON,
  downloadFile,
  generateFilename,
} from "../utils/export";
import { appendFilter } from "../utils/filterQuery";
import type {
  QueryResult,
  EventsResult,
  AggregateResult,
  HistogramBucketGrouped,
} from "../api/client";
import type { TailEvent } from "../api/sse";
import styles from "./SearchView.module.css";

// Known log level keys for histogram grouping detection
const KNOWN_LEVELS = new Set(["debug", "info", "warn", "error"]);

/** Returns true if any bucket in a grouped histogram response contains a known level key. */
function hasKnownLevels(buckets: HistogramBucketGrouped[]): boolean {
  for (const b of buckets) {
    for (const key of Object.keys(b.counts)) {
      if (KNOWN_LEVELS.has(key.toLowerCase())) return true;
    }
  }
  return false;
}

interface Props {
  path?: string;
}

/** Maximum events to keep in the live tail buffer */
const TAIL_BUFFER_CAP = 10_000;

/** Module-level getter for the current EditorView -- set by the component */
let getEditorView: (() => import("@codemirror/view").EditorView | null) | null =
  null;

/** Debounce timer for live explain diagnostics */
let explainDebounceTimer: ReturnType<typeof setTimeout> | undefined;

/** Debounce timer for post-query side effects (histogram, explain, fields) */
let postQueryEffectsTimer: ReturnType<typeof setTimeout> | undefined;

/** Timer for copy tooltip auto-hide */
let copyTooltipTimer: ReturnType<typeof setTimeout> | undefined;

/** Current AbortController for the active query -- null when idle */
let activeAbortController: AbortController | null = null;
/** SSE cleanup function for aggregation job progress */
let jobProgressCleanup: (() => void) | null = null;
/** Elapsed timer interval ID */
let elapsedTimerId: ReturnType<typeof setInterval> | null = null;
/** Monotonic query counter to discard stale responses (Pitfall 3) */
let queryGeneration = 0;

// --- Helpers that read/write the store imperatively (outside React render) ---

const ss = useSearchStore;

function startElapsedTimer() {
  const startTime = performance.now();
  ss.setState({ elapsedMs: 0 });
  elapsedTimerId = setInterval(() => {
    ss.setState({ elapsedMs: performance.now() - startTime });
  }, 100);
}

function stopElapsedTimer() {
  if (elapsedTimerId !== null) {
    clearInterval(elapsedTimerId);
    elapsedTimerId = null;
  }
}

function cleanupActiveQuery() {
  if (jobProgressCleanup) {
    jobProgressCleanup();
    jobProgressCleanup = null;
  }
  stopElapsedTimer();
  activeAbortController = null;
}

function resultCount(r: QueryResult | null): number {
  if (!r) return 0;
  if (r.type === "events") return r.events.length;
  return r.rows.length;
}

/** Derive columns from a QueryResult (used by export) */
function deriveColumns(r: QueryResult): string[] {
  if (r.type === "events") {
    const evts = (r as EventsResult).events;
    const keySet = new Set<string>();
    const limit = Math.min(evts.length, 100);
    for (let i = 0; i < limit; i++) {
      for (const key of Object.keys(evts[i])) {
        keySet.add(key);
      }
    }
    const priority = ["_time", "_raw", "_source", "source"];
    const ordered: string[] = [];
    for (const p of priority) {
      if (keySet.has(p)) {
        ordered.push(p);
        keySet.delete(p);
      }
    }
    return ordered.concat(Array.from(keySet).sort());
  }
  return (r as AggregateResult).columns;
}

/** Get rows as Record<string, unknown>[] from a QueryResult */
function getResultRows(r: QueryResult): Record<string, unknown>[] {
  if (r.type === "events") return (r as EventsResult).events;
  const agg = r as AggregateResult;
  return agg.rows.map((data) => {
    const row: Record<string, unknown> = {};
    for (let c = 0; c < agg.columns.length; c++) {
      row[agg.columns[c]] = data[c];
    }
    return row;
  });
}

/**
 * Post-query side effects: push history, update URL hash, clear diagnostics,
 * fetch histogram/explain/fields. Extracted so both sync and streaming paths
 * can call it after query completion.
 */
function runPostQueryEffects(
  q: string,
  fromVal: string,
  toVal: string | undefined,
  pg: number,
  sz: number,
): void {
  ss.setState({ hasQueried: true });

  pushHistory(q);
  writeQueryToHash(q, fromVal, toVal, pg, sz);

  const view = getEditorView?.();
  if (view) clearEditorDiagnostics(view);

  // Fetch grouped histogram (with ungrouped fallback) and explain in
  // parallel after query succeeds. Non-blocking -- failures ignored.
  // If the grouped response contains no known level keys, fall through
  // to the ungrouped single-color display so the legend stays clean.
  fetchHistogramGrouped(fromVal, toVal, 60, "level")
    .then((histResult) => {
      if (histResult.buckets.length > 0 && hasKnownLevels(histResult.buckets)) {
        ss.setState({ groupedBuckets: histResult.buckets, timelineBuckets: [] });
      } else {
        // No known level keys — fall through to ungrouped display
        ss.setState({ groupedBuckets: [] });
        return fetchHistogram(fromVal, toVal, 60).then((h) => {
          ss.setState({ timelineBuckets: h.buckets });
        });
      }
    })
    .catch(() => {
      fetchHistogram(fromVal, toVal, 60)
        .then((histResult) => {
          ss.setState({
            timelineBuckets: histResult.buckets,
            groupedBuckets: [],
          });
        })
        .catch(() => {
          /* non-critical */
        });
    });

  fetchExplain(q, fromVal, toVal)
    .then((explain) => {
      ss.setState({ explainResult: explain });
    })
    .catch(() => {
      /* non-critical */
    });

  fetchFields()
    .then((fields) => {
      const m = new Map<string, string>();
      for (const f of fields) m.set(f.name, f.type);
      ss.setState({ catalogFields: fields, fieldTypeMap: m });
    })
    .catch(() => {
      /* non-critical */
    });
}

/**
 * Debounced wrapper for runPostQueryEffects. Fires 300ms after the last
 * call so rapid successive queries coalesce histogram/explain/fields requests.
 */
function runPostQueryEffectsDebounced(
  q: string,
  fromVal: string,
  toVal: string | undefined,
  pg: number,
  sz: number,
): void {
  clearTimeout(postQueryEffectsTimer);
  postQueryEffectsTimer = setTimeout(() => {
    runPostQueryEffects(q, fromVal, toVal, pg, sz);
  }, 300);
}

/**
 * Run a query with adaptive sync/streaming execution.
 *
 * Flow: submit hybrid query (200ms sync wait). If fast, instant swap. If slow,
 * switch to NDJSON streaming (search) or SSE progress tracking (aggregation).
 * Previous results stay visible during the initial 200ms wait period.
 *
 * Accepts optional pg/sz params for pagination.
 */
function runQueryAndRefresh(
  q: string,
  fromVal: string,
  toVal: string | undefined,
  pg?: number,
  sz?: number,
): void {
  const state = ss.getState();
  if (!q || state.queryActive) return;

  const currentPage = pg ?? state.page;
  const currentSize = sz ?? state.pageSize;
  const currentOffset = (currentPage - 1) * currentSize;

  // Increment generation counter to detect stale responses
  queryGeneration++;
  const gen = queryGeneration;

  // Cancel any previous query
  if (activeAbortController) activeAbortController.abort();
  cleanupActiveQuery();

  const controller = new AbortController();
  activeAbortController = controller;

  // Reset state -- do NOT clear result yet (previous results stay during 200ms wait)
  ss.setState({
    queryActive: true,
    canceled: false,
    streaming: false,
    streamingCount: 0,
    progressData: null,
    error: null,
    explainOpen: false,
  });

  // Start elapsed timer
  startElapsedTimer();

  submitHybridQuery(
    q,
    fromVal,
    toVal,
    currentSize,
    currentOffset,
    controller.signal,
  )
    .then((hybrid) => {
      // Discard stale responses
      if (gen !== queryGeneration) return;

      if (hybrid.status === "sync") {
        // FAST PATH: query completed within 200ms -- instant swap
        ss.setState({
          result: hybrid.syncResult!.result,
          stats: hybrid.syncResult!.stats,
          loading: false,
          queryActive: false,
        });
        stopElapsedTimer();
        ss.setState({ elapsedMs: hybrid.syncResult!.stats.took_ms });
        runPostQueryEffects(q, fromVal, toVal, currentPage, currentSize);
        cleanupActiveQuery();
        return;
      }

      // SLOW PATH: query is async — clear stats immediately; results cleared lazily on first row
      ss.setState({ stats: null });

      // --- SSE progress for both event and aggregate queries ---
      // Reuses the existing async job (same pipeline as the hybrid query)
      // instead of starting a second independent scan via streamQuery().
      ss.setState({ loading: true });
      startElapsedTimer();
      const jobId = hybrid.jobId;
      if (!jobId) {
        ss.setState({
          error: "No job ID returned for async query",
          loading: false,
          queryActive: false,
        });
        stopElapsedTimer();
        cleanupActiveQuery();
        return;
      }

      const unsubscribe = subscribeJobProgress(
        jobId,
        (p) => {
          // onProgress
          if (gen !== queryGeneration) return;
          const updates: Partial<ReturnType<typeof ss.getState>> = {
            progressData: {
              percent: p.percent,
              scanned: p.scanned,
              total: p.segments_total ?? 0,
              elapsedMs: p.elapsed_ms,
            },
          };

          // Render preview rows while query is running
          if (p.preview && p.preview.length > 0) {
            updates.result = {
              type: "events",
              events: p.preview,
              total: p.preview.length,
              has_more: true,
            } satisfies EventsResult;
            updates.isPreview = true;
          }
          ss.setState(updates);
        },
        (data: unknown) => {
          // onComplete — SSE complete event is { data: QueryResult, meta: { took_ms, scanned, stats } }
          if (gen !== queryGeneration) return;
          const payload = data as
            | { data: QueryResult; meta?: Record<string, unknown> }
            | QueryResult;
          const queryResult: QueryResult =
            payload &&
            typeof payload === "object" &&
            "data" in payload &&
            "meta" in payload
              ? (payload as { data: QueryResult }).data
              : (payload as QueryResult);
          const metaStats =
            payload && typeof payload === "object" && "meta" in payload
              ? (payload as { meta: Record<string, unknown> }).meta
              : undefined;
          const detailedStats = metaStats?.stats as
            | Record<string, unknown>
            | undefined;

          ss.setState({
            result: queryResult ?? null,
            stats: {
              took_ms: (metaStats?.took_ms as number) ?? ss.getState().elapsedMs,
              scanned: (metaStats?.scanned as number) ?? 0,
              query_id: jobId,
              stats: detailedStats
                ? {
                    segments_total:
                      (detailedStats.segments_total as number) ?? 0,
                    segments_scanned:
                      (detailedStats.segments_scanned as number) ?? 0,
                    segments_skipped_bf:
                      (detailedStats.segments_skipped_bloom as number) ?? 0,
                    rows_scanned: (detailedStats.rows_scanned as number) ?? 0,
                    took_ms: (metaStats?.took_ms as number) ?? ss.getState().elapsedMs,
                  }
                : undefined,
            },
            progressData: null,
            queryActive: false,
            loading: false,
            isPreview: false,
          });
          stopElapsedTimer();
          runPostQueryEffectsDebounced(
            q,
            fromVal,
            toVal,
            currentPage,
            currentSize,
          );
          cleanupActiveQuery();
        },
        (message: string) => {
          // onFailed
          if (gen !== queryGeneration) return;
          ss.setState({
            error: message,
            progressData: null,
            queryActive: false,
            loading: false,
            isPreview: false,
          });
          stopElapsedTimer();
          cleanupActiveQuery();
        },
        () => {
          // onCanceled
          if (gen !== queryGeneration) return;
          ss.setState({
            canceled: true,
            result: null,
            progressData: null,
            queryActive: false,
            loading: false,
            isPreview: false,
          });
          stopElapsedTimer();
          cleanupActiveQuery();
        },
      );

      jobProgressCleanup = unsubscribe;
    })
    .catch((err: unknown) => {
      if (gen !== queryGeneration) return;
      if (err instanceof DOMException && err.name === "AbortError") {
        // Cancel during hybrid submit phase
        ss.setState({
          canceled: true,
          queryActive: false,
          loading: false,
        });
        stopElapsedTimer();
        cleanupActiveQuery();
        return;
      }
      const message = err instanceof Error ? err.message : "Unknown error";
      ss.setState({
        error: message,
        queryActive: false,
        loading: false,
      });
      stopElapsedTimer();

      // On failure, fetch explain to show diagnostics in the editor
      const view = getEditorView?.();
      if (view) {
        fetchExplain(q, fromVal, toVal)
          .then((explain) => {
            if (!explain.is_valid) {
              dispatchDiagnostics(view, q, explain);
            }
          })
          .catch(() => {
            /* non-critical */
          });
      }

      cleanupActiveQuery();
    });
}

/** Cancel the currently running query. */
function handleCancelQuery() {
  if (!activeAbortController) return;
  activeAbortController.abort();
  // For aggregation jobs, fire-and-forget the server-side cancel
  if (jobProgressCleanup) {
    // The abort will trigger onCanceled via SSE or the catch block
  }
  cleanupActiveQuery();
}

// Empty state components

function EmptyStateInitial() {
  return (
    <div className={styles.emptyState}>
      <div className={styles.emptyTitle}>No events yet</div>
      <div className={styles.emptyHint}>
        Run a query to explore your data, or try:
      </div>
      <code className={styles.emptyCode}>lynxdb demo</code>
      <div className={styles.emptySubHint}>to generate sample log data</div>
    </div>
  );
}

function EmptyStateNoResults() {
  return (
    <div className={styles.emptyState}>
      <div className={styles.emptyTitle}>No matching events</div>
      <div className={styles.emptyHint}>
        Try adjusting your query or expanding the time range
      </div>
    </div>
  );
}

// Main component

export function SearchView(_props: Props) {
  const tailCleanupRef = useRef<(() => void) | null>(null);
  const resultsAreaRef = useRef<HTMLDivElement>(null);
  const editorHandleRef = useRef<QueryEditorHandle | null>(null);
  /** Tracks whether auto-scroll is paused (user scrolled away from top) */
  const autoScrollPaused = useRef(false);

  // Subscribe to search store slices
  const query = useSearchStore((s) => s.query);
  const from = useSearchStore((s) => s.from);
  const to = useSearchStore((s) => s.to);
  const result = useSearchStore((s) => s.result);
  const stats = useSearchStore((s) => s.stats);
  const loading = useSearchStore((s) => s.loading);
  const error = useSearchStore((s) => s.error);
  const sidebarVisible = useSearchStore((s) => s.sidebarVisible);
  const timelineBuckets = useSearchStore((s) => s.timelineBuckets);
  const groupedBuckets = useSearchStore((s) => s.groupedBuckets);
  const histogramBrushed = useSearchStore((s) => s.histogramBrushed);
  const hasQueried = useSearchStore((s) => s.hasQueried);
  const sidebarIndexes = useSearchStore((s) => s.sidebarIndexes);
  const sidebarViews = useSearchStore((s) => s.sidebarViews);
  const explainResult = useSearchStore((s) => s.explainResult);
  const fieldTypeMap = useSearchStore((s) => s.fieldTypeMap);
  const catalogFields = useSearchStore((s) => s.catalogFields);
  const tailActive = useSearchStore((s) => s.tailActive);
  const tailEvents = useSearchStore((s) => s.tailEvents);
  const tailNewCount = useSearchStore((s) => s.tailNewCount);
  const tailCatchupDone = useSearchStore((s) => s.tailCatchupDone);
  const tailReconnecting = useSearchStore((s) => s.tailReconnecting);
  const explainOpen = useSearchStore((s) => s.explainOpen);
  const queryActive = useSearchStore((s) => s.queryActive);
  const streaming = useSearchStore((s) => s.streaming);
  const streamingCount = useSearchStore((s) => s.streamingCount);
  const progressData = useSearchStore((s) => s.progressData);
  const canceled = useSearchStore((s) => s.canceled);
  const elapsedMs = useSearchStore((s) => s.elapsedMs);
  const isPreview = useSearchStore((s) => s.isPreview);
  const page = useSearchStore((s) => s.page);
  const pageSize = useSearchStore((s) => s.pageSize);
  const viewMode = useSearchStore((s) => s.viewMode);
  const copyTooltip = useSearchStore((s) => s.copyTooltip);

  // Set up module-level editor view getter so runQueryAndRefresh can access it
  getEditorView = () => editorHandleRef.current?.getView() ?? null;

  const handleQueryChange = useCallback((value: string) => {
    ss.setState({ query: value });

    // Debounced explain for live inline diagnostics (500ms after typing stops)
    clearTimeout(explainDebounceTimer);
    if (value.trim()) {
      explainDebounceTimer = setTimeout(() => {
        const view = getEditorView?.();
        if (!view) return;
        const { from: f, to: t } = ss.getState();
        fetchExplain(value, f, t)
          .then((explain) => {
            if (!explain.is_valid) {
              dispatchDiagnostics(view, value, explain);
            } else {
              clearEditorDiagnostics(view);
            }
          })
          .catch(() => {
            /* non-critical */
          });
      }, 500);
    } else {
      // Clear diagnostics when query is empty
      const view = getEditorView?.();
      if (view) clearEditorDiagnostics(view);
    }
  }, []);

  const handleExecute = useCallback(() => {
    const state = ss.getState();
    if (state.tailActive) return; // block while tailing
    // Ctrl+Enter while running -> cancel (dual behavior)
    if (state.queryActive) {
      handleCancelQuery();
      return;
    }
    // Reset to page 1 on new query execution (Pitfall 5)
    ss.setState({ page: 1 });
    runQueryAndRefresh(
      state.query.trim(),
      state.from,
      state.to,
      1,
      state.pageSize,
    );
  }, []);

  const handleSidebarToggle = useCallback(() => {
    ss.setState((s) => ({ sidebarVisible: !s.sidebarVisible }));
  }, []);

  const handleInsertCommand = useCallback((template: string) => {
    const current = ss.getState().query.trim();
    ss.setState({ query: current ? `${current} ${template}` : template });
    setTimeout(() => {
      editorHandleRef.current?.focus();
    }, 0);
  }, []);

  const handleSetSource = useCallback((name: string) => {
    ss.setState({ query: `from ${name} ` });
    // Focus the editor so the user can continue typing
    setTimeout(() => {
      editorHandleRef.current?.focus();
    }, 0);
  }, []);

  const handleTimelineBrush = useCallback((fromTs: number, toTs: number) => {
    // Convert epoch seconds to ISO strings for the time range
    const newFrom = new Date(fromTs * 1000).toISOString();
    const newTo = new Date(toTs * 1000).toISOString();
    ss.setState({ from: newFrom, to: newTo, histogramBrushed: true, page: 1 });

    const state = ss.getState();
    runQueryAndRefresh(
      state.query.trim(),
      state.from,
      state.to,
      1,
      state.pageSize,
    );
  }, []);

  const handleHistogramReset = useCallback(() => {
    ss.setState({ from: "-1h", to: undefined, histogramBrushed: false, page: 1 });
    const state = ss.getState();
    runQueryAndRefresh(
      state.query.trim(),
      state.from,
      state.to,
      1,
      state.pageSize,
    );
  }, []);

  /* --- Sort handler --- */
  const handleSort = useCallback((newQuery: string) => {
    ss.setState({ query: newQuery, page: 1 }); // Reset to page 1 on sort change
    const state = ss.getState();
    runQueryAndRefresh(newQuery, state.from, state.to, 1, state.pageSize);

    const view = getEditorView?.();
    if (view) {
      view.dispatch({
        changes: { from: 0, to: view.state.doc.length, insert: newQuery },
      });
    }
  }, []);

  /* --- Filter handler (from EventDetail [+]/[-] buttons) --- */
  const handleFilter = useCallback(
    (field: string, value: string, exclude: boolean) => {
      const state = ss.getState();
      const newQuery = appendFilter(state.query, field, value, exclude);
      ss.setState({ query: newQuery, page: 1 }); // Reset to page 1 on filter change (Pitfall 6)

      // Update editor content to show the new query
      const view = getEditorView?.();
      if (view) {
        view.dispatch({
          changes: { from: 0, to: view.state.doc.length, insert: newQuery },
        });
      }

      const updated = ss.getState();
      runQueryAndRefresh(newQuery, updated.from, updated.to, 1, updated.pageSize);
    },
    [],
  );

  /* --- Pagination handlers --- */
  const handlePageChange = useCallback((newPage: number) => {
    ss.setState({ page: newPage });
    const state = ss.getState();
    runQueryAndRefresh(
      state.query.trim(),
      state.from,
      state.to,
      newPage,
      state.pageSize,
    );
  }, []);

  const handlePageSizeChange = useCallback((newSize: number) => {
    ss.setState({ pageSize: newSize, page: 1 }); // Reset to first page
    const state = ss.getState();
    runQueryAndRefresh(state.query.trim(), state.from, state.to, 1, newSize);
  }, []);

  /* --- View mode and wrap handlers --- */
  const handleViewModeChange = useCallback((mode: "table" | "list") => {
    ss.setState({ viewMode: mode });
  }, []);

  /* --- Cell copy handler --- */
  const handleCellCopy = useCallback((value: string, x: number, y: number) => {
    navigator.clipboard.writeText(value).then(() => {
      clearTimeout(copyTooltipTimer);
      ss.setState({ copyTooltip: { visible: true, x, y } });
      copyTooltipTimer = setTimeout(() => {
        ss.setState({ copyTooltip: { visible: false, x: 0, y: 0 } });
      }, 1500);
    });
  }, []);

  /* --- Export handler --- */
  const handleExport = useCallback(
    async (format: "csv" | "json", scope: "page" | "all") => {
      let rows: Record<string, unknown>[];
      let columns: string[];

      if (scope === "page") {
        // Use current result data
        const r = ss.getState().result;
        if (!r) return;
        columns = deriveColumns(r);
        rows = getResultRows(r);
      } else {
        // Fetch all results via streaming endpoint
        const state = ss.getState();
        try {
          const resp = await fetch("/api/v1/query/stream", {
            method: "POST",
            headers: { "Content-Type": "application/json", ...authHeaders() },
            body: JSON.stringify({
              q: state.query,
              from: state.from,
              to: state.to,
            }),
          });
          if (!resp.ok) {
            // Fallback to current page data
            const r = state.result;
            if (!r) return;
            columns = deriveColumns(r);
            rows = getResultRows(r);
          } else {
            const text = await resp.text();
            rows = text
              .trim()
              .split("\n")
              .filter(Boolean)
              .map((line) => JSON.parse(line));
            if (rows.length > 0) {
              const keySet = new Set<string>();
              for (const row of rows.slice(0, 100)) {
                for (const key of Object.keys(row)) keySet.add(key);
              }
              const priority = ["_time", "_raw", "_source", "source"];
              const ordered: string[] = [];
              for (const p of priority) {
                if (keySet.has(p)) {
                  ordered.push(p);
                  keySet.delete(p);
                }
              }
              columns = ordered.concat(Array.from(keySet).sort());
            } else {
              return;
            }
          }
        } catch {
          // On network error, fallback to current page
          const r = state.result;
          if (!r) return;
          columns = deriveColumns(r);
          rows = getResultRows(r);
        }
      }

      if (format === "csv") {
        const csv = generateCSV(columns, rows);
        downloadFile(csv, generateFilename("csv"), "text/csv");
      } else {
        const json = generateJSON(rows);
        downloadFile(json, generateFilename("json"), "application/json");
      }
    },
    [],
  );

  /* --- Live Tail toggle --- */
  const handleTailToggle = useCallback(() => {
    const state = ss.getState();
    if (state.tailActive) {
      // Stop tailing
      if (tailCleanupRef.current) {
        tailCleanupRef.current();
        tailCleanupRef.current = null;
      }
      ss.setState({
        tailActive: false,
        tailEvents: [],
        tailNewCount: 0,
        tailCatchupDone: false,
        tailReconnecting: false,
      });
      autoScrollPaused.current = false;
      return;
    }

    // Start tailing
    const q = state.query.trim();
    ss.setState({
      tailActive: true,
      tailEvents: [],
      tailNewCount: 0,
      tailCatchupDone: false,
      result: null,
      stats: null,
      error: null,
    });
    autoScrollPaused.current = false;

    const cleanup = startTail(q, state.from, 100, {
      onEvent(event: TailEvent) {
        const prev = ss.getState().tailEvents;
        const next = [event, ...prev];
        ss.setState({
          tailEvents:
            next.length > TAIL_BUFFER_CAP ? next.slice(0, TAIL_BUFFER_CAP) : next,
        });

        if (autoScrollPaused.current) {
          ss.setState((s) => ({ tailNewCount: s.tailNewCount + 1 }));
        }
      },
      onCatchupDone(_count: number) {
        ss.setState({ tailCatchupDone: true });
      },
      onError(message: string) {
        ss.setState({ error: message });
      },
      onWarning(message: string) {
        // Show warning briefly in the error slot, then clear
        ss.setState({ error: message });
        setTimeout(() => {
          if (ss.getState().error === message) {
            ss.setState({ error: null });
          }
        }, 3000);
      },
      onReconnecting(isReconnecting: boolean) {
        ss.setState({ tailReconnecting: isReconnecting });
      },
    });

    tailCleanupRef.current = cleanup;
  }, []);

  /** Toggle the explain inspector panel */
  const handleExplainToggle = useCallback(() => {
    ss.setState((s) => ({ explainOpen: !s.explainOpen }));
  }, []);

  /** Click handler for the "new events" badge -- scroll back to top */
  const handleNewEventsBadgeClick = useCallback(() => {
    if (!resultsAreaRef.current) return;
    const viewport = resultsAreaRef.current.querySelector(
      "[class*='viewport']",
    );
    if (viewport) {
      viewport.scrollTop = 0;
    }
    autoScrollPaused.current = false;
    ss.setState({ tailNewCount: 0 });
  }, []);

  // Editor ref callback
  const handleEditorRef = useCallback((handle: QueryEditorHandle | null) => {
    editorHandleRef.current = handle;
  }, []);

  // --- Keyboard shortcuts ---
  useKeyboardShortcuts({
    onFocusEditor: () => editorHandleRef.current?.focus(),
    onToggleTail: handleTailToggle,
    onToggleSidebar: () => {
      ss.setState((s) => ({ sidebarVisible: !s.sidebarVisible }));
    },
    onClosePanel: () => {
      // Layered close: explain inspector > blur editor
      if (ss.getState().explainOpen) {
        ss.setState({ explainOpen: false });
        return;
      }
      editorHandleRef.current?.getView()?.contentDOM.blur();
    },
    onOpenPalette: () => {
      setHelpOverlayOpen(false); // Close help if open (Pitfall 7)
      const current = useOverlayStore.getState().paletteOpen;
      setPaletteOpen(!current);
    },
    onOpenHelp: () => {
      setPaletteOpen(false); // Close palette if open (Pitfall 7)
      const current = useOverlayStore.getState().helpOverlayOpen;
      setHelpOverlayOpen(!current);
    },
  });

  // Watch for queries loaded from the command palette
  useEffect(() => {
    const unsubscribe = useOverlayStore.subscribe((state, prevState) => {
      const q = state.paletteQuery;
      if (!q || q === prevState.paletteQuery) return;
      setPaletteQuery(null);
      ss.setState({ query: q });
      const view = getEditorView?.();
      if (view) {
        view.dispatch({
          changes: { from: 0, to: view.state.doc.length, insert: q },
        });
      }
      ss.setState({ page: 1 });
      const s = ss.getState();
      runQueryAndRefresh(q, s.from, s.to, 1, s.pageSize);
    });
    return unsubscribe;
  }, []);

  // Capture-phase scroll listener for auto-scroll pause detection.
  // Scroll events do not bubble, so we must capture them on the
  // results area container to intercept scrolls from the nested
  // ResultsTable viewport.
  useEffect(() => {
    const el = resultsAreaRef.current;
    if (!el) return;

    function onScroll(e: Event) {
      if (!ss.getState().tailActive) return;
      const target = e.target;
      if (!(target instanceof HTMLElement)) return;
      const scrolledFromTop = target.scrollTop;
      autoScrollPaused.current = scrolledFromTop > 10;
      if (!autoScrollPaused.current) {
        ss.setState({ tailNewCount: 0 });
      }
    }

    el.addEventListener("scroll", onScroll, true);
    return () => el.removeEventListener("scroll", onScroll, true);
  }, []);

  // Cleanup SSE and streaming on unmount
  useEffect(() => {
    return () => {
      if (tailCleanupRef.current) {
        tailCleanupRef.current();
        tailCleanupRef.current = null;
      }
      // Streaming/progress cleanup
      if (activeAbortController) activeAbortController.abort();
      cleanupActiveQuery();
    };
  }, []);

  // Fetch indexes, views, and field catalog on mount for the flow sidebar
  useEffect(() => {
    Promise.allSettled([fetchIndexes(), fetchViews(), fetchFields()]).then(
      ([idx, views, fields]) => {
        if (idx.status === "fulfilled") ss.setState({ sidebarIndexes: idx.value });
        if (views.status === "fulfilled") ss.setState({ sidebarViews: views.value });
        if (fields.status === "fulfilled") {
          const m = new Map<string, string>();
          for (const f of fields.value) {
            m.set(f.name, f.type);
          }
          ss.setState({ catalogFields: fields.value, fieldTypeMap: m });
        }
      },
    );
  }, []);

  // Restore query, time range, and pagination from URL hash on mount (Pitfall 4: defer execution)
  useEffect(() => {
    const hashData = readQueryFromHash();
    if (hashData) {
      const updates: Record<string, unknown> = {
        query: hashData.q,
        from: hashData.from || "-1h",
        to: hashData.to,
      };
      if (hashData.page) updates.page = hashData.page;
      if (hashData.size) updates.pageSize = hashData.size;
      ss.setState(updates);
      // Defer execution to ensure editor has rendered
      setTimeout(() => {
        const s = ss.getState();
        runQueryAndRefresh(
          hashData.q,
          s.from,
          s.to,
          s.page,
          s.pageSize,
        );
      }, 0);
    }
  }, []);

  // Build an EventsResult from live tail events for ResultsTable
  const activeResult: QueryResult | null = tailActive
    ? ({
        type: "events",
        events: tailEvents as unknown as Record<string, unknown>[],
        total: tailEvents.length,
        has_more: false,
      } satisfies EventsResult)
    : result;

  // Determine which content to show in the results area
  const showInitialEmpty =
    !tailActive &&
    !hasQueried &&
    !loading &&
    !queryActive &&
    !error;
  const showNoResults =
    !tailActive &&
    hasQueried &&
    !loading &&
    !queryActive &&
    !error &&
    !canceled &&
    resultCount(result) === 0;

  // Compute total count for pagination and toolbar
  const totalCount = activeResult
    ? activeResult.type === "events"
      ? activeResult.total
      : activeResult.rows.length
    : 0;
  const pageCount = resultCount(activeResult);
  const hasResults = activeResult && pageCount > 0 && !tailActive;

  return (
    <div className={styles.view}>
      <div className={styles.queryBar}>
        <QueryEditor
          value={query}
          onChange={handleQueryChange}
          onExecute={handleExecute}
          editorRef={handleEditorRef}
        />
        <button
          type="button"
          className={`${styles.runBtn}${queryActive ? ` ${styles.cancelBtn}` : ""}`}
          onClick={handleExecute}
          disabled={tailActive}
          aria-label={queryActive ? "Cancel query" : "Run query"}
          title={
            queryActive
              ? `Cancel query (${formatShortcut(SHORTCUTS.runQuery)})`
              : `Run query (${formatShortcut(SHORTCUTS.runQuery)})`
          }
        >
          {queryActive ? "■" : "▶"}
        </button>
        <LiveTailButton active={tailActive} onToggle={handleTailToggle} />
        <TimeRangePicker
          from={from}
          to={to}
          onFromChange={(v) => ss.setState({ from: v })}
          onToChange={(v) => ss.setState({ to: v })}
          onApply={() => {
            if (!ss.getState().tailActive) {
              ss.setState({ histogramBrushed: false, page: 1 }); // Reset brush state on manual time change
              const s = ss.getState();
              runQueryAndRefresh(
                s.query.trim(),
                s.from,
                s.to,
                1,
                s.pageSize,
              );
            }
          }}
        />
      </div>

      <div className={styles.body}>
        <FlowSidebar
          visible={sidebarVisible}
          indexes={sidebarIndexes}
          views={sidebarViews}
          explainResult={explainResult}
          fieldTypes={fieldTypeMap}
          selectedFields={activeResult ? deriveColumns(activeResult) : []}
          catalogFields={catalogFields}
          onFilter={handleFilter}
          onToggle={handleSidebarToggle}
          onSelectSource={handleSetSource}
          onInsertCommand={handleInsertCommand}
        />

        <div className={styles.mainContent}>
          <Timeline
            from={from}
            to={to}
            buckets={timelineBuckets}
            groupedBuckets={groupedBuckets}
            visible={hasQueried && !tailActive}
            onBrush={handleTimelineBrush}
            onReset={handleHistogramReset}
            showReset={histogramBrushed}
          />

          <QueryStatsBar
            stats={stats}
            loading={loading}
            error={error}
            resultCount={
              tailActive
                ? tailEvents.length
                : resultCount(result)
            }
            tailActive={tailActive}
            tailEventCount={tailEvents.length}
            tailCatchupDone={tailCatchupDone}
            streaming={streaming}
            streamingCount={streamingCount}
            progress={progressData}
            canceled={canceled}
            elapsedMs={elapsedMs}
            isPreview={isPreview}
            onExplainToggle={handleExplainToggle}
            explainAvailable={
              !!(explainResult?.is_valid && explainResult?.parsed)
            }
            tailReconnecting={tailReconnecting}
          />

          {explainOpen &&
            explainResult?.is_valid &&
            explainResult?.parsed && (
              <ExplainInspector
                explain={explainResult}
                stats={stats}
              />
            )}

          {/* Table toolbar -- only show when results exist */}
          {hasResults && (
            <TableToolbar
              viewMode={viewMode}
              onViewModeChange={handleViewModeChange}
              onExport={handleExport}
              totalCount={totalCount}
              pageCount={pageCount}
            />
          )}

          <div className={styles.resultsArea} ref={resultsAreaRef}>
            {tailActive && tailNewCount > 0 && (
              <button
                type="button"
                className={styles.newEventsBadge}
                onClick={handleNewEventsBadgeClick}
                aria-label={`${tailNewCount} new events, click to scroll to top`}
              >
                &#8593; {tailNewCount} new{" "}
                {tailNewCount === 1 ? "event" : "events"}
              </button>
            )}
            {showInitialEmpty && <EmptyStateInitial />}
            {showNoResults && <EmptyStateNoResults />}
            {!showInitialEmpty &&
              !showNoResults &&
              (viewMode === "table" ? (
                <ResultsTable
                  result={activeResult}
                  onSort={handleSort}
                  currentQuery={query}
                  onFilter={handleFilter}
                />
              ) : (
                <ListView
                  result={activeResult}
                  onCellCopy={handleCellCopy}
                  onFilter={handleFilter}
                />
              ))}
          </div>

          {/* Pagination bar -- only show for non-tail, non-empty results */}
          {hasResults && (
            <PaginationBar
              page={page}
              pageSize={pageSize}
              total={totalCount}
              onPageChange={handlePageChange}
              onPageSizeChange={handlePageSizeChange}
            />
          )}
        </div>
      </div>

      {/* Copy tooltip */}
      <CopyTooltip
        visible={copyTooltip.visible}
        x={copyTooltip.x}
        y={copyTooltip.y}
      />
    </div>
  );
}
