---
title: "dedup"
sidebar_label: "dedup"
---

# dedup

**Class:** `core` &middot; **Streaming:** row-at-a-time

Keep first N (default 1) rows per key.

## Signature

```
| dedup [n] <fields>
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `n` | `int` | No | rows kept per key (default 1) |
| `fields` | `field_list` | Yes | - |

## Examples

```
dedup service
```

```
dedup 3 service, host
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
