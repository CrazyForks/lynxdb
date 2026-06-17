---
title: "gapfill"
sidebar_label: "gapfill"
---

# gapfill

**Class:** `core` &middot; **Streaming:** accumulating

Insert missing _time buckets per group across the query range when available, else the observed range.

## Signature

```
| gapfill span=<duration> fill=<expr> [by=<field_list>]
```

## Options

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `span` | `duration` | `-` | - |
| `fill` | `expr` | `-` | - |
| `by` | `field_list` | `-` | - |

## Examples

```
gapfill span=5m fill=0 by service
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
