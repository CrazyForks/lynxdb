---
title: "where"
sidebar_label: "where"
---

# where

**Class:** `core` &middot; **Streaming:** row-at-a-time

Typed predicate filter.

## Signature

```
| where <predicate>
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `predicate` | `predicate` | Yes | - |

## Examples

```
where status >= 500 and has(_raw, "timeout")
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
