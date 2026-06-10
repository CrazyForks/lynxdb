---
title: "sessionize"
sidebar_label: "sessionize"
---

# sessionize

**Class:** `helper` &middot; **Streaming:** accumulating

Add session id/start/end fields based on time gaps within each group.

## Signature

```
| sessionize [maxpause=<duration>] [by=<field_list>]
```

## Options

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `maxpause` | `duration` | `30m` | - |
| `by` | `field_list` | `-` | - |

## Examples

```
sessionize maxpause=30m by user_id
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
