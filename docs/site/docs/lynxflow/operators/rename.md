---
title: "rename"
sidebar_label: "rename"
---

# rename

**Class:** `core` &middot; **Streaming:** row-at-a-time

Rename columns.

## Signature

```
| rename <renames>
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `renames` | `assign_list` | Yes | old as new, ... |

## Examples

```
rename duration_ms as latency
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
