---
title: "limit"
sidebar_label: "limit"
---

# limit

**Class:** `core` &middot; **Streaming:** row-at-a-time

First N rows. SQL-style alias for head.

## Signature

```
| limit <n>
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `n` | `int` | Yes | - |

## Examples

```
limit 20
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
