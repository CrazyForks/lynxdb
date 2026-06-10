---
title: "tee"
sidebar_label: "tee"
---

# tee

**Class:** `management` &middot; **Streaming:** row-at-a-time

Side effect: additionally send JSON rows to a sink without interrupting the stream.

## Signature

```
| tee <sink>
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `sink` | `string` | Yes | - |

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
