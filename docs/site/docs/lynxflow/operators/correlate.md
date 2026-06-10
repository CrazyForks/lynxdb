---
title: "correlate"
sidebar_label: "correlate"
---

# correlate

**Class:** `helper` &middot; **Streaming:** accumulating

Correlation between two numeric fields.

## Signature

```
| correlate <field1> <field2> [method=<enum>]
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `field1` | `field` | Yes | - |
| `field2` | `field` | Yes | - |

## Options

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `method` | `enum` | `pearson` | - Values: `pearson`, `spearman`. |

## Examples

```
correlate duration_ms cpu_pct method=pearson
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
