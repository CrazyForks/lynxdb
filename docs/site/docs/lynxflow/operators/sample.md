---
title: "sample"
sidebar_label: "sample"
---

# sample

**Class:** `core` &middot; **Streaming:** accumulating

Random sampling. `sample 1%` is streaming Bernoulli sampling; `sample 10000` is reservoir sampling.

## Signature

```
| sample <n-or-percent> [seed=<int>]
```

## Positional Arguments

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `n-or-percent` | `expr` | Yes | row count for reservoir sampling, or percentage with % suffix for Bernoulli sampling |

## Options

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `seed` | `int` | `-` | optional deterministic random seed |

## Examples

```
sample 1% seed=42
```

```
sample 10000
```

---

*Generated from the [LynxFlow registry](https://github.com/lynxbase/lynxdb/blob/main/pkg/lynxflow/registry/operators.go). See [RFC-002](https://github.com/lynxbase/lynxdb/blob/main/docs/grammar/RFC-002.md) for the full language specification.*
