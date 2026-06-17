---
title: "unpivot"
sidebar_label: "unpivot"
---

# unpivot

**Class:** `helper` &middot; **Streaming:** row-at-a-time

Convert selected wide fields into name/value rows.

## Signature

```
| unpivot <fields> as <name>, <value>
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `fields` | `field_list` | Yes | - |
| `name` | `field` | Yes | metric-name output field |
| `value` | `field` | Yes | metric-value output field |

## Examples

```
unpivot cpu_ms, db_ms as metric, value
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
