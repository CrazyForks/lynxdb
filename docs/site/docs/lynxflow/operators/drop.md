---
title: "drop"
sidebar_label: "drop"
---

# drop

**Class:** `core` &middot; **Streaming:** row-at-a-time

Remove columns. Supports globs.

## Signature

```
| drop <fields>
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `fields` | `field_patterns` | Yes | - |

## Examples

```
drop _raw, trace_*
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
