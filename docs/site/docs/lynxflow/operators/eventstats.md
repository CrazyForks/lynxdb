---
title: "eventstats"
sidebar_label: "eventstats"
---

# eventstats

**Class:** `core` &middot; **Streaming:** accumulating

Aggregates appended to every row without collapsing.

## Signature

```
| eventstats <aggs> [by=<field_list>]
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `aggs` | `agg_list` | Yes | - |

## Options

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `by` | `field_list` | `-` | - |

## Examples

```
eventstats avg(duration_ms) as global_avg
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
