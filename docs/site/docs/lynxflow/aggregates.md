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
| `perc_weighted` | (x: number, weight: number, p: number) | `float` | Weighted t-digest percentile; p in [0, 100]. |
| `stdev` | (x: number) | `float` | Sample standard deviation. |
| `var` | (x: number) | `float` | Sample variance. |
| `corr` | (x: number, y: number) | `float` | Pearson correlation over rows where both arguments are numeric. |
| `covar` | (x: number, y: number) | `float` | Sample covariance over rows where both arguments are numeric. |
| `linear_fit` | (x: number, y: number) | `object` | Least-squares fit as &#123;slope, intercept, r2&#125;. |
| `sum_object` | (obj: object) | `object` | Per-key sums for numeric object values. |
| `skewness` | (x: number) | `float` | Bias-corrected sample skewness. |
| `kurtosis` | (x: number) | `float` | Bias-corrected sample excess kurtosis. |
| `mad` | (x: number) | `float` | Median absolute deviation over numeric values. |
| `mode` | (x: any) | `any` | - |
| `arg_max` | (value: any, order: any) | `any` | Value from the row with the greatest order expression. |
| `arg_min` | (value: any, order: any) | `any` | Value from the row with the least order expression. |
| `any_value` | (x: any) | `any` | An arbitrary non-null value from the group. |
| `top_k` | (x: any, k: int) | `array` | Top k non-null values with counts. |
| `top_k_weighted` | (x: any, weight: number, k: int) | `array` | Top k non-null values by summed positive weight. |
| `value_counts` | (x: any, max: int?) | `array` | Non-null values with counts, sorted by frequency and optionally capped. |
| `avg_weighted` | (x: number, weight: number) | `float` | Weighted average as sum(x * weight) / sum(weight). |
| `entropy` | (x: any) | `float` | Shannon entropy of non-null values, in bits. |
| `max_n` | (x: any, n: int) | `array` | Largest n non-null values. |
| `min_n` | (x: any, n: int) | `array` | Smallest n non-null values. |
| `first` | (x: any) | `any` | First non-null in row order. |
| `last` | (x: any) | `any` | Last non-null in row order. |
| `earliest` | (x: any) | `any` | Value from the row with the smallest _time. |
| `latest` | (x: any) | `any` | Value from the row with the largest _time. |
| `values` | (x: any, n: int?) | `array` | Distinct non-null values, optionally capped at n. |
| `list` | (x: any, n: int?) | `array` | All non-null values in row order, optionally capped at n. |
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
| `delta` | (x: number) | `float` | Difference between the current value and previous group value. |
| `ema` | (x: number, n: int) | `float` | Exponential moving average with alpha = 2/(n+1). |

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/aggregates.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full specification.*
