---
title: "explode"
sidebar_label: "explode"
---

# explode

**Class:** `core` &middot; **Streaming:** row-at-a-time

One row per array element; rows with missing/empty arrays are dropped.

## Signature

```
| explode <array> [as]
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `array` | `field` | Yes | - |
| `as` | `field` | No | element output field (default: the array field name) |

## Examples

```
explode tags as tag
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
