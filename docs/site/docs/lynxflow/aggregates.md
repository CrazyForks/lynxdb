---
title: "Aggregate Functions"
sidebar_label: "Aggregate Functions"
---

# Aggregate Functions

All aggregate and window functions available in `stats`, `eventstats`, and `streamstats` stages.

## Standard Aggregates

All standard aggregates support `where` clauses for conditional aggregation: `count(where status >= 500)`.

| Function | Params | Result | Description |
|----------|--------|--------|-------------|
| `count` | (x: any?) | `int` | count() counts rows; count(x) counts non-null x; count(where p) counts matching rows. |
| `sum` | (x: number) | `number` | Nulls skipped; all-null group yields null. |
| `avg` | (x: number) | `float` | - |
| `min` | (x: any) | `any` | - |
| `max` | (x: any) | `any` | - |
| `dc` | (x: any) | `int` | Distinct count; exact below 10K, HLL above. |
| `estdc` | (x: any) | `int` | Always-HLL distinct count. |
| `perc` | (x: number, p: number) | `float` | T-digest percentile; p in [0, 100]. |
| `p50` | (x: number) | `float` | Alias for perc(x, 50). |
| `p75` | (x: number) | `float` | Alias for perc(x, 75). |
| `p90` | (x: number) | `float` | Alias for perc(x, 90). |
| `p95` | (x: number) | `float` | Alias for perc(x, 95). |
| `p99` | (x: number) | `float` | Alias for perc(x, 99). |
| `stdev` | (x: number) | `float` | Sample standard deviation. |
| `var` | (x: number) | `float` | Sample variance. |
| `mode` | (x: any) | `any` | - |
| `first` | (x: any) | `any` | First non-null in row order. |
| `last` | (x: any) | `any` | Last non-null in row order. |
| `earliest` | (x: any) | `any` | Value from the row with the smallest _time. |
| `latest` | (x: any) | `any` | Value from the row with the largest _time. |
| `values` | (x: any) | `array` | Distinct non-null values as an array. |
| `list` | (x: any) | `array` | All non-null values as an array, row order. |
| `rate` | () | `float` | Row count divided by the group's time-bucket span. |
| `per_second` | (x: number) | `float` | sum(x) divided by the group's time-bucket span in seconds. |

## Window Functions (streamstats only)

| Function | Params | Result | Description |
|----------|--------|--------|-------------|
| `lag` | (x: any, n: int?) | `any` | Value n rows back (default 1). |
| `lead` | (x: any, n: int?) | `any` | Value n rows ahead (default 1). |
| `row_number` | () | `int` | 1-based row index within the group. |
| `running_sum` | (x: number) | `number` | - |
| `moving_avg` | (x: number, n: int) | `float` | - |

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/aggregates.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full specification.*
