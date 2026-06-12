---
title: "count"
sidebar_label: "count"
---

# count

**Class:** `sugar` &middot; **Streaming:** accumulating

Row count, optionally per group.

## Signature

```
| count [by=<field_list>]
```

## Options

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `by` | `field_list` | `-` | - |

## Desugars To

```
stats count() as count [by <fields>]
```

## Examples

```
count
```

```
count by host
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
