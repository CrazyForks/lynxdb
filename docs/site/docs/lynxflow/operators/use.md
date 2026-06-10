---
title: "use"
sidebar_label: "use"
---

# use

**Class:** `management` &middot; **Streaming:** row-at-a-time

Expand a named pipeline fragment at parse time. Missing fragments are explicit errors.

## Signature

```
| use <fragment>
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `fragment` | `string` | Yes | - |

## Examples

```
use @ops/error-filter
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
