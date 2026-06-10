---
title: "from"
sidebar_label: "from"
---

# from

**Class:** `source` &middot; **Streaming:** row-at-a-time

Scan stage. Only valid first in a pipeline. Accepts bracket time ranges and search-sugar terms (RFC-002 §3.1).

## Signature

```
| from <sources>
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `sources` | `field_patterns` | Yes | source names, globs, !-excludes, *, or $cte refs; optional [range] suffix; optional trailing search-sugar terms |

## Examples

```
from nginx[-1h] timeout status>=500
```

```
from logs*,!logs-debug*[-7d..-1d]
```

```
from $errs
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
