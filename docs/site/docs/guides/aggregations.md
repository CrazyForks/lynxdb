---
title: Run Aggregations
description: How to aggregate log data in LynxDB using stats, count, avg, percentiles, conditional counting, group by, and multi-level aggregations.
---

# Run Aggregations

The [`stats`](/docs/lynxflow/operators/stats) stage is the core of log analytics in LynxDB. It computes aggregate functions over events, optionally grouped by one or more fields. This guide covers all the aggregation patterns you need for day-to-day log analysis.

## Basic counting

### Count all events

```bash
lynxdb query '| stats count()'
```

:::note
`count()` always takes parentheses in LynxFlow — `stats count` is a parse error. An unaliased `count()` produces a column literally named `count()`, so alias it (`stats count() as count`) whenever a later stage or output references the column.
:::

### Count with a filter

Search-sugar terms like `level=error` are accepted directly after the source in a [`from`](/docs/lynxflow/operators/from) stage:

```bash
lynxdb query 'from main level=error | stats count()'
```

In a bare pipeline (no `from`), use `where` with the `==` comparison operator:

```bash
lynxdb query 'where level == "error" | stats count()'
```

### Quick count shortcut

The [`lynxdb count`](/docs/cli/shortcuts) command is a faster way to get a simple count. It wraps your filter stage in `from main | <filter> | stats count()`:

```bash
lynxdb count 'where level == "error"' --since 1h
```

---

## Group by a field

Add `by <field>` to break results down by category:

```bash
lynxdb query 'from main level=error | stats count() as count by source'
```

Result:

| source | count |
|--------|-------|
| nginx | 847 |
| api-gateway | 523 |
| postgres | 211 |

### Group by multiple fields

```bash
lynxdb query 'from nginx | stats count() as count by status, uri'
```

### Name your aggregation

Use `as` to give the result column a meaningful name:

```bash
lynxdb query 'from main level=error | stats count() as error_count by source'
```

---

## Aggregation functions

LynxDB supports 26 aggregate functions. Here are the most common ones.

### Count, sum, avg

```bash
lynxdb query 'from nginx | stats count() as count, sum(bytes) as total_bytes, avg(duration_ms) as avg_lat by uri'
```

Result:

| uri | count | total_bytes | avg_lat |
|-----|-------|-------------|---------|
| /api/v2/users | 1423 | 4892100 | 45.2 |
| /api/v1/health | 891 | 124500 | 12.1 |
| /api/v1/login | 456 | 342000 | 89.7 |

:::note
SPL2's `mean` is gone — use `avg`.
:::

### Min and max

```bash
lynxdb query 'from nginx | stats min(duration_ms) as fastest, max(duration_ms) as slowest by uri'
```

### Distinct count

Count unique values with `dc()`:

```bash
lynxdb query 'from nginx | stats dc(client_ip) as unique_visitors by uri'
```

`dc()` is exact below 10K distinct values and switches to HyperLogLog above; `estdc()` always uses HyperLogLog.

### Percentiles

Compute latency percentiles:

```bash
lynxdb query 'from nginx | stats avg(duration_ms) as avg_lat, p50(duration_ms) as p50_lat, p95(duration_ms) as p95_lat, p99(duration_ms) as p99_lat by uri'
```

Available percentile functions: `p50`, `p75`, `p90`, `p95`, `p99` (the SPL2 `perc50`...`perc99` zoo is gone). For arbitrary percentiles the registry defines `perc(x, p)` with `p` in [0, 100] — see the [aggregate functions reference](/docs/lynxflow/aggregates).

There is also a [`percentiles`](/docs/lynxflow/operators/percentiles) sugar stage that expands to the full p50/p75/p90/p95/p99 set:

```bash
lynxdb query 'from nginx | percentiles duration_ms by uri'
```

### Standard deviation and variance

```bash
lynxdb query 'from nginx | stats avg(duration_ms) as avg_lat, stdev(duration_ms) as stdev_lat by uri'
```

`stdev()` is the sample standard deviation; `var()` is the sample variance.

### Collect values

The `values()` function collects all distinct non-null values of a field into an array:

```bash
lynxdb query 'from main level=error | stats count() as count, values(source) as sources by host'
```

`list()` collects all non-null values in row order (duplicates included).

### Earliest and latest

Get the first and last value seen (by `_time`):

```bash
lynxdb query 'from main | stats earliest(message) as first_msg, latest(message) as last_msg by source'
```

`first()` and `last()` are the row-order equivalents.

See the [aggregate functions reference](/docs/lynxflow/aggregates) for the complete list.

---

## Conditional counting

Count events that match a condition with a `where` clause inside the aggregate (formerly `count(eval(...))` in SPL2):

```bash
lynxdb query 'from nginx | stats count() as total, count(where status >= 500) as errors by uri'
```

All standard aggregates support `where` clauses, e.g. `sum(bytes, where status == 200)`:

```bash
lynxdb query 'from nginx | stats sum(bytes, where status == 200) as ok_bytes by uri'
```

### Compute error rates

Combine conditional counting with [`extend`](/docs/lynxflow/operators/extend) (formerly `eval`) to calculate ratios:

```bash
lynxdb query 'from nginx
  | stats count() as total, count(where status >= 500) as errors by uri
  | extend error_rate = round(errors / total * 100, 1)
  | where error_rate > 5
  | sort -error_rate
  | keep uri, total, errors, error_rate'
```

---

## Sorting results

Pipe aggregation results into [`sort`](/docs/lynxflow/operators/sort) to order them:

```bash
# Sort descending by count (prefix with -)
lynxdb query 'from main level=error | stats count() as count by source | sort -count'

# Sort ascending
lynxdb query 'from main level=error | stats count() as count by source | sort count'

# Sort by multiple fields
lynxdb query 'from nginx | stats count() as count by status, uri | sort status, -count'
```

---

## Top and rare

The [`top`](/docs/lynxflow/operators/top) and [`rare`](/docs/lynxflow/operators/rare) stages are shortcuts for the most and least common values:

```bash
# Top 10 URIs by request count
lynxdb query 'from nginx | top 10 uri'

# Rarest error messages
lynxdb query 'from main level=error | rare 10 message'
```

These desugar to `stats count() as count by <field> | sort -count | head N` (and `sort +count` for `rare`) — visible with `--show-rewritten`.

---

## Multi-level aggregation

You can chain multiple `stats` stages in a pipeline. Each one aggregates the output of the previous step:

```bash
# First: count errors per host per source
# Then: find hosts with more than 100 total errors
lynxdb query 'from main level=error
  | stats count() as count by host, source
  | stats sum(count) as total_errors by host
  | where total_errors > 100
  | sort -total_errors'
```

---

## Streaming aggregations

### streamstats -- running aggregations

[`streamstats`](/docs/lynxflow/operators/streamstats) computes running (cumulative) aggregations without collapsing events:

```bash
lynxdb query 'from nginx
  | sort +_time
  | streamstats count() as request_num, avg(duration_ms) as running_avg_latency'
```

`streamstats` also supports window-only functions such as `lag`, `lead`, and `row_number` — see the [aggregate functions reference](/docs/lynxflow/aggregates).

### eventstats -- enrich events with aggregates

[`eventstats`](/docs/lynxflow/operators/eventstats) adds aggregation values to each event without collapsing:

```bash
lynxdb query 'from nginx
  | eventstats avg(duration_ms) as global_avg by uri
  | where duration_ms > global_avg * 3
  | keep _time, uri, duration_ms, global_avg'
```

This is useful for finding outliers: events where the latency is more than 3x the average.

---

## Aggregations on local files

All aggregation stages work in pipe mode and file mode:

```bash
# Aggregate a local file
lynxdb query --file access.log '| stats count() as count by status'

# Aggregate piped input
kubectl logs deploy/api | lynxdb query '| stats avg(duration_ms) as avg_dur, p99(duration_ms) as p99_dur by endpoint'

# Combine with Unix tools
lynxdb query --file access.log '| stats count() as count by status' --format csv | sort -t, -k2 -rn
```

---

## Performance tips

- **Time range first**: Always narrow the scan window with a bracket range on the source: `from main[-1h] level=error | stats count()` is much faster than scanning all data. The `--since` CLI flag works too.
- **Filter before aggregating**: Place `where` before `stats` to reduce the number of events processed.
- **Use materialized views**: For queries you run repeatedly, create a [materialized view](/docs/guides/materialized-views) to precompute the aggregation and get results up to 400x faster.
- **Partial aggregation**: LynxDB automatically uses two-phase partial aggregation (per-segment, then global merge), so `stats` scales linearly with data size.

---

## Next steps

- [Time series analysis](/docs/guides/time-series) -- aggregate over time windows with `every` and `bin()`
- [Materialized views](/docs/guides/materialized-views) -- precompute aggregations for repeated queries
- [stats reference](/docs/lynxflow/operators/stats) -- full syntax and all options
- [Aggregate functions reference](/docs/lynxflow/aggregates) -- complete list of aggregate functions
