---
title: "exemplars"
sidebar_label: "exemplars"
---

# exemplars

**Class:** `sugar` &middot; **Streaming:** accumulating

Newest representative rows, globally or per group.

## Signature

```
| exemplars [n] [by=<field_list>]
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `n` | `int` | No | default 3 |

## Options

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `by` | `field_list` | `-` | - |

## Desugars To

```
sort -_time | dedup <n> <keys>  (global: sort -_time | head <n>)
```

## Examples

```
exemplars 5 by endpoint
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
