---
title: "patterns"
sidebar_label: "patterns"
---

# patterns

**Class:** `helper` &middot; **Streaming:** accumulating

Group similar messages into Drain templates. Logical-IR node.

## Signature

```
| patterns [field=<field>] [max_templates=<int>] [similarity=<string>]
```

## Options

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `field` | `field` | `_raw` | - |
| `max_templates` | `int` | `-` | - |
| `similarity` | `string` | `-` | - |

## Examples

```
patterns field=message
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
