---
title: "rare"
sidebar_label: "rare"
---

# rare

**Class:** `sugar` &middot; **Streaming:** accumulating

Bottom-N frequent values.

## Signature

```
| rare [n] <field> [by <fields>]
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `n` | `int` | No | default 10 |
| `field` | `field` | Yes | - |

## Desugars To

```
stats count() as count by <field>[, <fields>] | sort <fields>, count | head <n> or per-group row_number filter
```

## Examples

```
rare 3 service
```

```
rare 2 status by host
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
