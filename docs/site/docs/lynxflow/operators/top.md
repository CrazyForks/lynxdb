---
title: "top"
sidebar_label: "top"
---

# top

**Class:** `sugar` &middot; **Streaming:** accumulating

Top-N frequent values.

## Signature

```
| top [n] <field> [by <fields>]
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `n` | `int` | No | default 10 |
| `field` | `field` | Yes | - |

## Desugars To

```
stats count() as count by <field>[, <fields>] | sort <fields>, -count | head <n> or per-group row_number filter
```

## Examples

```
top 10 uri
```

```
top 3 uri by service
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
