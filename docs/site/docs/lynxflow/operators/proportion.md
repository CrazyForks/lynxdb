---
title: "proportion"
sidebar_label: "proportion"
---

# proportion

**Class:** `sugar` &middot; **Streaming:** accumulating

Matching events divided by all events, denominator visible.

## Signature

```
| proportion <predicate> <as> [every=<duration>] [by=<field_list>]
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `predicate` | `predicate` | Yes | - |
| `as` | `field` | Yes | alias is mandatory |

## Options

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `every` | `duration` | `-` | - |
| `by` | `field_list` | `-` | - |

## Desugars To

```
stats count(where <pred>) as <name>_num, count() as <name>_den [by ...] | extend <name> = <name>_num / <name>_den
```

## Examples

```
proportion status >= 500 as error_rate by service
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
