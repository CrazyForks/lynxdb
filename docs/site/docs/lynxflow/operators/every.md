---
title: "every"
sidebar_label: "every"
---

# every

**Class:** `sugar` &middot; **Streaming:** accumulating

Time-bucketed aggregation.

## Signature

```
| every <span> <aggs> [by=<field_list>] [fill=<expr>]
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `span` | `duration` | Yes | - |
| `aggs` | `agg_list` | Yes | introduced by the stats keyword |

## Options

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `by` | `field_list` | `-` | - |
| `fill` | `expr` | `-` | - |

## Desugars To

```
stats <aggs> by [<keys>,] bin(_time, <span>) [| gapfill span=<span> fill=<fill> by <keys>]
```

## Examples

```
every 5m by service stats count()
```

```
every 5m by service stats count() fill=0
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
