---
title: makeresults
description: Generate temporary result rows.
---

# makeresults

Generate temporary rows for examples, tests, and ad hoc query construction. Each generated row includes `_time`.

## Syntax

```spl
| makeresults
| makeresults count=<N>
| makeresults <N>
```

Omitting `count` creates one row. `count=0` creates no rows.

## Examples

```spl
-- Generate one row
| makeresults

-- Generate three rows
| makeresults count=3

-- SPL2 positional count spelling
| makeresults 3
```

## Notes

- LynxDB currently supports row generation and `_time` annotation.
- Splunk `annotate`, `format`, and `data` options are not implemented yet.

## See Also

- [eval](/docs/lynx-flow/commands/eval) -- Add fields to generated rows
- [stats](/docs/lynx-flow/commands/stats) -- Aggregate generated rows
