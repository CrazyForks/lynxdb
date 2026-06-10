---
title: "latency"
sidebar_label: "latency"
---

# latency

**Class:** `sugar` &middot; **Streaming:** accumulating

Latency percentile summary. The metric field is required (no guessed defaults).

## Signature

```
| latency <field> [every=<duration>] [by=<field_list>]
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `field` | `field` | Yes | - |

## Options

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `every` | `duration` | `-` | - |
| `by` | `field_list` | `-` | - |

## Desugars To

```
stats p50(<f>), p95(<f>), p99(<f>), count() [by <keys>, bin(_time, <every>)]
```

## Examples

```
latency duration_ms every 5m by endpoint
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
