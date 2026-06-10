---
title: "head"
sidebar_label: "head"
---

# head

**Class:** `core` &middot; **Streaming:** row-at-a-time

First N rows (TopK pushdown after sort).

## Signature

```
| head <n>
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `n` | `int` | Yes | - |

## Examples

```
head 10
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
