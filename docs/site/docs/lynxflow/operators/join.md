---
title: "join"
sidebar_label: "join"
---

# join

**Class:** `core` &middot; **Streaming:** accumulating

Hash join. Default type=inner is a plain inner join (never innerunique). type=asof matches the latest right row at or before the left _time.

## Signature

```
| join <right> [type=<enum>] on=<field_list> [tolerance=<duration>]
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `right` | `sub_pipeline` | Yes | with $cte or with [ &lt;pipeline&gt; ] |

## Options

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `type` | `enum` | `inner` | - Values: `inner`, `left`, `outer`, `semi`, `anti`, `asof`. |
| `on` | `field_list` | `-` | - |
| `tolerance` | `duration` | `-` | maximum lag for type=asof |

## Examples

```
join type=left on user_id with [from users]
```

```
join type=anti on user_id with [from allowlist]
```

```
join type=asof on host tolerance=30s with [from deploys]
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
