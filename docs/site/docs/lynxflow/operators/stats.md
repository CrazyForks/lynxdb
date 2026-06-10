---
title: "stats"
sidebar_label: "stats"
---

# stats

**Class:** `core` &middot; **Streaming:** accumulating

Grouped aggregation. count() requires parens; conditional aggregates via count(where p) / sum(x, where p).

## Signature

```
| stats <aggs> [by=<field_list>]
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `aggs` | `agg_list` | Yes | - |

## Options

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `by` | `field_list` | `-` | group keys; bin(_time, dur) allowed and emits as _time |

## Examples

```
stats count(), avg(dur) by service
```

```
stats count(where status >= 500) as errors, count() as total by service
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
