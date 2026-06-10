---
title: "sort"
sidebar_label: "sort"
---

# sort

**Class:** `core` &middot; **Streaming:** accumulating

External merge sort, spill-capable. Nulls last ascending, first descending.

## Signature

```
| sort <keys>
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `keys` | `sort_list` | Yes | -f desc, +f or f asc |

## Examples

```
sort -count, service
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
