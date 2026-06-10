---
title: "union"
sidebar_label: "union"
---

# union

**Class:** `core` &middot; **Streaming:** row-at-a-time

Append rows from sub-pipelines; schemas merge by name with null-padding.

## Signature

```
| union <pipelines>...
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `pipelines...` | `sub_pipeline` | Yes | - |

## Examples

```
union [from audit[-1h] | where res == "failed"]
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
