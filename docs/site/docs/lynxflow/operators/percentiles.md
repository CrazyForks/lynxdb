---
title: "percentiles"
sidebar_label: "percentiles"
---

# percentiles

**Class:** `sugar` &middot; **Streaming:** accumulating

Five-point percentile summary.

## Signature

```
| percentiles <field> [by=<field_list>]
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `field` | `field` | Yes | - |

## Options

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `by` | `field_list` | `-` | - |

## Desugars To

```
stats p50(<f>) as p50_<f>, p75(<f>) as p75_<f>, p90(<f>) as p90_<f>, p95(<f>) as p95_<f>, p99(<f>) as p99_<f> [by <keys>]
```

## Examples

```
percentiles duration_ms by service
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
