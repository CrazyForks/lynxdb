---
title: "hist"
sidebar_label: "hist"
---

# hist

**Class:** `sugar` &middot; **Streaming:** accumulating

Histogram rows with lo, hi, count, and chart columns.

## Signature

```
| hist <field> [bins=<n>]
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `field` | `field` | Yes | - |

## Options

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `bins` | `int` | `20` | - |

## Desugars To

```
stats histogram(<field>, <bins>) as _h | explode _h as bin | extend lo = bin.lo, hi = bin.hi, count = bin.count | eventstats max(count) as _m | extend chart = bar(count, 0, _m, 40) | drop _h, _m, bin | sort lo
```

## Examples

```
hist duration_ms bins=20
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
