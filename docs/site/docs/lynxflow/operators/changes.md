---
title: "changes"
sidebar_label: "changes"
---

# changes

**Class:** `sugar` &middot; **Streaming:** accumulating

Rows where a field changed relative to the previous row in the same group.

## Signature

```
| changes <field> [by=<field_list>]
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `field` | `field` | Yes | - |

## Options

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `by` | `field_list` | `-` | - |

## Desugars To

```
sort +_time | streamstats current=false last(<f>) as previous_<f> [by <keys>] | where exists(previous_<f>) and <f> != previous_<f>
```

## Examples

```
changes version by service
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
