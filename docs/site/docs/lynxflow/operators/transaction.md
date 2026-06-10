---
title: "transaction"
sidebar_label: "transaction"
---

# transaction

**Class:** `helper` &middot; **Streaming:** accumulating

Group events into transactions keyed by fields.

## Signature

```
| transaction <fields> [maxspan=<duration>] [startswith=<predicate>] [endswith=<predicate>]
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `fields` | `field_list` | Yes | - |

## Options

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `maxspan` | `duration` | `-` | - |
| `startswith` | `predicate` | `-` | - |
| `endswith` | `predicate` | `-` | - |

## Examples

```
transaction user_id maxspan=30m
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
