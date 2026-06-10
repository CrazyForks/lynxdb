---
title: "trace"
sidebar_label: "trace"
---

# trace

**Class:** `helper` &middot; **Streaming:** accumulating

Build a span tree from trace/span fields; adds depth/tree fields.

## Signature

```
| trace [trace_id=<field>] [span_id=<field>] [parent_id=<field>]
```

## Options

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `trace_id` | `field` | `trace_id` | - |
| `span_id` | `field` | `span_id` | - |
| `parent_id` | `field` | `parent_id` | - |

## Examples

```
where trace_id == "req-abc-123" | trace
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
