---
title: "streamstats"
sidebar_label: "streamstats"
---

# streamstats

**Class:** `core` &middot; **Streaming:** row-at-a-time

Running/windowed values in row order.

## Signature

```
| streamstats <aggs> [window=<int>] [current=<bool>] [by=<field_list>]
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `aggs` | `agg_list` | Yes | aggregates or window functions (lag, lead, row_number, ...) |

## Options

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `window` | `int` | `-` | sliding window size in rows; 0 = all preceding |
| `current` | `bool` | `true` | include the current row |
| `by` | `field_list` | `-` | - |

## Examples

```
streamstats window=3 avg(duration_ms) as rolling_avg
```

```
streamstats row_number() as rk by host
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
