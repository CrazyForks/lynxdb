# Skipped LynxFlow Transcripts

Total: 51 of 187 transcripts not included in the LynxFlow dual suite.

## Translation refused -- unsupported commands (31)

### unsupported command: streamstats (6)

- `backend_streamstats`: order-dependent running aggregate
- `backend_streamstats_by_service`: order-dependent running aggregate
- `backend_streamstats_cumulative`: order-dependent running aggregate
- `backend_streamstats_rolling`: order-dependent running aggregate
- `backend_streamstats_sorted_tail`: order-dependent running aggregate
- `backend_streamstats_window`: order-dependent running aggregate

### unsupported command: multisearch (5)

- `backend_multisearch_mixed`: use union in LynxFlow
- `backend_multisearch_union`: use union in LynxFlow
- `cross_multisearch_summary`: use union in LynxFlow
- `multisearch_cross_agg`: use union in LynxFlow
- `multisearch_cross_index`: use union in LynxFlow

### unsupported command: transaction (3)

- `backend_transaction_maxspan`: cross-event session state
- `backend_transaction_user` (file): cross-event session state
- `backend_transaction_user` (server): cross-event session state

### unsupported command: outliers (3)

- `backend_outliers_iqr` (file): unsupported command
- `backend_outliers_zscore`: unsupported command
- `backend_outliers_iqr` (server): unsupported command

### unsupported command: append (2)

- `backend_append_error_warn`: use union in LynxFlow
- `cross_append_errors`: use union in LynxFlow

### unsupported command: compare (2)

- `backend_compare_shift` (file): unsupported command
- `backend_compare_shift` (server): unsupported command

### unsupported command: correlate (2)

- `backend_correlate_pearson` (file): unsupported command
- `backend_correlate_pearson` (server): unsupported command

### unsupported command: patterns (2)

- `backend_patterns_message` (file): unsupported command
- `backend_patterns_message` (server): unsupported command

### unsupported command: rollup (2)

- `backend_rollup_multi` (file): unsupported command
- `backend_rollup_multi` (server): unsupported command

### unsupported command: sessionize (2)

- `backend_sessionize_user` (file): unsupported command
- `backend_sessionize_user` (server): unsupported command

### unsupported command: select (1)

- `backend_eventstats_pct`: unsupported command

### translation validation failure (1)

- `backend_where_like`: generated `where like(...)` which LynxFlow parser rejects as unknown stage

## Already LynxFlow tests (5)

- `backend_lynxflow_chain`
- `backend_lynxflow_enrich_outlier`
- `backend_lynxflow_group`
- `backend_lynxflow_keep_omit`
- `backend_lynxflow_let`

## Error tests (2)

- `error_bad_query`: tests SPL2 error path
- `error_nonexistent_file`: tests file-not-found path

## Non-deterministic output (2)

- `backend_glimpse` (file): glimpse output is non-deterministic due to sampling
- `backend_glimpse` (server): glimpse output is non-deterministic due to sampling

## Runtime failures -- LynxFlow execution errors (3)

- `nginx_count`: parse combined(_raw) auto-injection fails in LynxFlow parser
- `nginx_search_401`: parse combined(_raw) auto-injection fails in LynxFlow parser
- `nginx_search_500`: parse combined(_raw) auto-injection fails in LynxFlow parser

## Empty-result guard -- bugs (8)

These transcripts translated successfully but produce fewer rows than the SPL2
golden. Excluded to avoid recording incorrect goldens. These are bugs in the
LynxFlow execution engine or translator.

### bin() returns nanoseconds instead of ISO timestamps (7)

Timechart/bin translations produce a single bucket with nanosecond values
instead of multiple ISO-formatted time buckets.

- `backend_timeseries_15m_services` (file): spl2=12 rows, lf=1 row
- `backend_timeseries_30m_avg_duration` (file): spl2=6 rows, lf=1 row
- `backend_timeseries_30m_count` (file+server): spl2=6 rows, lf=1 row
- `backend_timeseries_30m_error_rate` (file): spl2=6 rows, lf=1 row
- `backend_timeseries_30m_pivot` (file+server): spl2=6 rows, lf=1 row

### CTE conditional eval produces empty string instead of expected values (1)

- `access_cte_error_50x` (file): spl2=2 rows with is_50x="yes"/"no", lf=1 row with is_50x=""
