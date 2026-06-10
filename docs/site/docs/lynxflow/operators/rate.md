---
title: "rate"
sidebar_label: "rate"
---

# rate

**Class:** `sugar` &middot; **Streaming:** accumulating

Event count per time bucket.

## Signature

```
| rate [per=<duration>] [by=<field_list>]
```

## Options

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `per` | `duration` | `1m` | - |
| `by` | `field_list` | `-` | - |

## Desugars To

```
every <per> [by <keys>] stats count() as rate
```

## Examples

```
rate per 5m by service
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
