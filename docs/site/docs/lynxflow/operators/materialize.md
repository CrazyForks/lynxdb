---
title: "materialize"
sidebar_label: "materialize"
---

# materialize

**Class:** `management` &middot; **Streaming:** accumulating

Terminal stage: create a materialized view from the current pipeline.

## Signature

```
| materialize <name> [retention=<duration>] [partition_by=<field_list>]
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `name` | `string` | Yes | - |

## Options

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `retention` | `duration` | `-` | - |
| `partition_by` | `field_list` | `-` | - |

## Examples

```
stats count() by service, bin(_time, 5m) | materialize "mv_errors_5m" retention=90d
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
