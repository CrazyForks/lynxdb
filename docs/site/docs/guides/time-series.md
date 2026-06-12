---
title: Time Series Analysis
description: How to analyze log data over time using the every stage and the bin() time-bucketing function in LynxDB.
---

# Time Series Analysis

Log analytics often requires understanding trends over time: when do errors spike? Is latency increasing? How does traffic change throughout the day? LynxDB provides the [`every`](/docs/lynxflow/operators/every) stage and the `bin()` function for time-based aggregation (formerly `TIMECHART`, `BIN`, and `time_bucket()` in SPL2).

## Time bucketing with `every`

The [`every`](/docs/lynxflow/operators/every) stage is the primary tool for time series analysis. It buckets events into time intervals and computes aggregations for each bucket.

### Basic time series

Count events per 5-minute interval:

```bash
lynxdb query 'from main level=error | every 5m stats count() as count'
```

`every` is pure sugar: it desugars to `stats count() as count by bin(_time, 5m)` (visible with `--show-rewritten`), and the bucket timestamp emits as `_time`.

### Choose the time span

The span argument controls the bucket size. Combine it with a bracket time range on `from`:

```bash
# 1-minute granularity (high detail)
lynxdb query 'from main[-1h] level=error | every 1m stats count() as count'

# 1-hour granularity (overview)
lynxdb query 'from main[-7d] level=error | every 1h stats count() as count'

# 1-day granularity (trend)
lynxdb query 'from main[-30d] level=error | every 1d stats count() as count'
```

Common span values: `1m`, `5m`, `10m`, `15m`, `30m`, `1h`, `6h`, `12h`, `1d`.

### Aggregation functions in `every`

Use any aggregate function, not just count:

```bash
# Average latency over time
lynxdb query 'from nginx | every 5m stats avg(duration_ms) as avg_lat'

# P99 latency over time
lynxdb query 'from nginx | every 5m stats p99(duration_ms) as p99_lat'

# Multiple aggregations
lynxdb query 'from nginx | every 5m stats count() as count, avg(duration_ms) as avg_lat, p99(duration_ms) as p99_lat'

# Sum of bytes transferred
lynxdb query 'from nginx | every 1h stats sum(bytes) as total_bytes'
```

### Split by a field

Use `by <field>` to produce separate series for each value:

```bash
# Error count over time, split by source
lynxdb query 'from main level=error | every 5m by source stats count() as count'

# Latency by endpoint
lynxdb query 'from nginx | every 5m by uri stats avg(duration_ms) as avg_lat'

# Status code distribution over time
lynxdb query 'from nginx | every 5m by status stats count() as count'
```

:::warning
In `every`, the `by` clause comes **before** the `stats` keyword: `every 5m by source stats count()`. Writing `every 5m stats count() by source` is a parse error.
:::

### The `rate` shortcut

For plain event counts per bucket, the [`rate`](/docs/lynxflow/operators/rate) sugar stage is even shorter — it desugars to `every <per> [by <keys>] stats count() as rate`:

```bash
lynxdb query 'from nginx | rate per 5m by service'
```

---

## `bin()` -- explicit time buckets in `stats`

`bin(_time, <span>)` snaps a timestamp to a bucket boundary. Used as a group key in [`stats`](/docs/lynxflow/operators/stats), it is the explicit form of `every` — useful when you want to combine time bucketing with other groupings or computations.

:::info
`_time` is the canonical timestamp field in LynxFlow queries and API output. In a `stats by` list, the binned key emits as `_time`.
:::

### Basic binning

```bash
lynxdb query 'from nginx
  | stats count() as count, avg(duration_ms) as avg_lat by bin(_time, 5m)'
```

### Correlate metrics with binned timestamps

```bash
lynxdb query 'from postgres duration_ms>1000
  | stats count() as slow_queries, avg(duration_ms) as avg_latency by bin(_time, 5m)
  | where slow_queries > 10'
```

### `bin()` in `extend`

You can also materialize the bucket as a named column:

```bash
lynxdb query 'from nginx
  | extend hour = bin(_time, 1h)
  | stats avg(duration_ms) as avg_lat, count() as count by hour
  | sort hour'
```

### `every` vs explicit `bin()`

| Feature | `every` | `stats ... by bin(_time, ...)` |
|---------|---------|--------------------------------|
| Convenience | One keyword, reads naturally | Explicit group key |
| Split by | `by` clause before `stats` | Any mix of keys in the `by` list |
| Flexibility | Fixed to time grouping | Combine time with other groupings, name the bucket via `extend` |
| Typical use | Quick time series charts | Complex multi-dimensional analysis |

Both compile to the same plan — `every` is pure sugar.

---

## `bin()` in materialized views

`bin()`-based bucketing (formerly `time_bucket()`) is essential for defining [materialized views](/docs/guides/materialized-views):

```bash
lynxdb mv create mv_errors_5m \
  'from main level=error | every 5m by source stats count() as count, avg(duration) as avg_duration' \
  --retention 90d
```

---

## Practical examples

### Find error spikes

Identify 5-minute windows with abnormally high error counts:

```bash
lynxdb query 'from main[-24h] level=error
  | every 5m stats count() as count
  | where count > 100'
```

### Compare latency across endpoints

```bash
lynxdb query 'from nginx[-6h] | every 10m by uri stats avg(duration_ms) as avg_lat'
```

### Traffic pattern analysis

See how request volume changes hour by hour:

```bash
lynxdb query 'from nginx[-7d] | every 1h stats count() as count'
```

### Correlate errors with slow queries

```bash
lynxdb query 'from main[-6h] level=error or (source=postgres duration_ms>1000)
  | extend event_type = if(source == "postgres", "slow_query", "error")
  | every 5m by event_type stats count() as count'
```

### Compute moving averages with streamstats

Smooth out spiky time series with a running average:

```bash
lynxdb query 'from nginx
  | every 5m stats avg(duration_ms) as avg_lat
  | sort +_time
  | streamstats window=6 avg(avg_lat) as moving_avg
  | keep _time, avg_lat, moving_avg'
```

The `window=6` option computes the average over the trailing 6 rows (30 minutes at 5-minute intervals).

---

## Time series on local files

Time analysis works in pipe mode and file mode:

```bash
# Analyze a local access log
lynxdb query --file access.log '| every 1h stats count() as count'

# Analyze kubectl output over time
kubectl logs deploy/api --since=6h | lynxdb query '
  | stats count() as count, avg(duration_ms) as avg_dur by bin(_time, 5m)'
```

---

## Output format for time series

Time series data often needs to be consumed by other tools. Use `--format csv` for spreadsheet compatibility or `--format json` for programmatic use:

```bash
# CSV for spreadsheets or graphing tools
lynxdb query 'from main[-7d] level=error | every 1h stats count() as count' --format csv > errors_by_hour.csv

# JSON for programmatic use
lynxdb query 'from main[-7d] level=error | every 1h stats count() as count' --format json
```

---

## Next steps

- [Run aggregations](/docs/guides/aggregations) -- general aggregation patterns beyond time series
- [Materialized views](/docs/guides/materialized-views) -- precompute time-bucketed aggregations
- [every reference](/docs/lynxflow/operators/every) -- full every syntax
- [stats reference](/docs/lynxflow/operators/stats) -- stats with bin() group keys
- [from reference](/docs/lynxflow/operators/from) -- bracket time ranges (`from main[-1h]`, `[@d]`, absolute ranges)
