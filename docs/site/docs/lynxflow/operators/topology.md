---
title: "topology"
sidebar_label: "topology"
---

# topology

**Class:** `helper` &middot; **Streaming:** accumulating

Build edge/node summaries from source/destination fields.

## Signature

```
| topology [source_field=<field>] [dest_field=<field>] [weight_field=<field>] [max_nodes=<int>]
```

## Options

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `source_field` | `field` | `service` | - |
| `dest_field` | `field` | `downstream` | - |
| `weight_field` | `field` | `-` | - |
| `max_nodes` | `int` | `-` | - |

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
