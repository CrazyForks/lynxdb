---
title: "keep"
sidebar_label: "keep"
---

# keep

**Class:** `core` &middot; **Streaming:** row-at-a-time

Projection, order-preserving. Supports globs and `* except f1, f2`.

## Signature

```
| keep <fields>
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `fields` | `field_patterns` | Yes | - |

## Examples

```
keep _time, service, status
```

```
keep * except _raw
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
