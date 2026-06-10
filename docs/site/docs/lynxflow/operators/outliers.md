---
title: "outliers"
sidebar_label: "outliers"
---

# outliers

**Class:** `helper` &middot; **Streaming:** accumulating

Mark statistical outliers using the selected method.

## Signature

```
| outliers field=<field> [method=<enum>] [threshold=<string>]
```

## Options

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `field` | `field` | `-` | - |
| `method` | `enum` | `iqr` | - Values: `iqr`, `zscore`, `mad`. |
| `threshold` | `string` | `-` | - |

## Examples

```
outliers field=duration_ms method=zscore threshold=2.0
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
