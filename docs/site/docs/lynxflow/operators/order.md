---
title: "order"
sidebar_label: "order"
---

# order

**Class:** `core` &middot; **Streaming:** accumulating

SQL-style ordering. Alias for sort: `order by f desc` is `sort -f`.

## Signature

```
| order by <field> [asc|desc], …
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `keys` | `sort_list` | Yes | SQL-style keys after by: f1 [asc|desc], f2 [asc|desc] |

## Examples

```
order by count desc
```

```
order by service asc, count desc
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
