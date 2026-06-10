---
title: "impact"
sidebar_label: "impact"
---

# impact

**Class:** `sugar` &middot; **Streaming:** accumulating

Contribution percentage per group.

## Signature

```
| impact [agg] by=<field_list>
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `agg` | `agg_list` | No | default count() |

## Options

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `by` | `field_list` | `-` | - |

## Desugars To

```
stats <agg> as v by <keys> | eventstats sum(v) as total_v | extend pct_v = v / total_v | sort -pct_v
```

## Examples

```
impact sum(bytes) by host
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
