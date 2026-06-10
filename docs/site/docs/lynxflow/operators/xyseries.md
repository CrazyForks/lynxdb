---
title: "xyseries"
sidebar_label: "xyseries"
---

# xyseries

**Class:** `helper` &middot; **Streaming:** accumulating

Pivot rows into a matrix (x rows, y columns).

## Signature

```
| xyseries <x> <y> <value>
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `x` | `field` | Yes | - |
| `y` | `field` | Yes | - |
| `value` | `field` | Yes | - |

## Examples

```
stats count() by service, level | xyseries service level count
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
