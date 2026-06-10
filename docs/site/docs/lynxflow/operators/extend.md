---
title: "extend"
sidebar_label: "extend"
---

# extend

**Class:** `core` &middot; **Streaming:** row-at-a-time

Add or replace computed columns, evaluated left to right.

## Signature

```
| extend <assignments>
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `assignments` | `assign_list` | Yes | - |

## Examples

```
extend is_err = status >= 500, amount = amount ?? 0
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
