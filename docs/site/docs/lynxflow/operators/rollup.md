---
title: "rollup"
sidebar_label: "rollup"
---

# rollup

**Class:** `helper` &middot; **Streaming:** accumulating

Multiple time resolutions in one stream; adds _resolution.

## Signature

```
| rollup <resolutions>... [by=<field_list>]
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `resolutions...` | `duration` | Yes | - |

## Options

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `by` | `field_list` | `-` | - |

## Examples

```
rollup 1m, 1h by service
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
