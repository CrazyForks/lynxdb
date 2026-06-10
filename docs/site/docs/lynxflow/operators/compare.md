---
title: "compare"
sidebar_label: "compare"
---

# compare

**Class:** `helper` &middot; **Streaming:** accumulating

Re-run the pipeline prefix over the previous window; adds previous_*/change_* columns. Logical-IR node.

## Signature

```
| compare <shift>
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `shift` | `duration` | Yes | optionally preceded by the keyword previous |

## Examples

```
compare previous 1h
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
