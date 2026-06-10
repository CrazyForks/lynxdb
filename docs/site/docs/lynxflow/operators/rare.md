---
title: "rare"
sidebar_label: "rare"
---

# rare

**Class:** `sugar` &middot; **Streaming:** accumulating

Bottom-N frequent values.

## Signature

```
| rare [n] <field>
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `n` | `int` | No | default 10 |
| `field` | `field` | Yes | - |

## Desugars To

```
stats count() as count by <field> | sort +count | head <n>
```

## Examples

```
rare 3 service
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
