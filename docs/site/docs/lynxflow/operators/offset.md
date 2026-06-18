---
title: "offset"
sidebar_label: "offset"
---

# offset

**Class:** `core` &middot; **Streaming:** row-at-a-time

Skip the first N rows. Pair with head/limit for pagination after a sort.

## Signature

```
| offset <n>
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `n` | `int` | Yes | - |

## Examples

```
sort -count | offset 40 | limit 20
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
