---
title: appendpipe
description: Append the result of a subpipe run over the current result set.
---

# appendpipe

Append rows produced by running a subpipe over the current result set.

## Syntax

```spl
| appendpipe [run_in_preview=<bool>] [<subpipe>]
```

## Examples

```spl
-- Add a total row after grouped results
| stats count as user_count by action, user
| appendpipe [stats sum(user_count) as total by action]
```

## Notes

- The subpipe runs when the pipeline reaches `appendpipe`, not before the main search.
- LynxDB materializes current rows, emits the original rows, then emits the subpipe output.
- `run_in_preview` parses for Splunk compatibility but has no effect because LynxDB does not expose Splunk preview-mode execution.

## See Also

- [append](/docs/lynx-flow/commands/append) -- Append a separate subsearch
- [union](/docs/lynx-flow/commands/union) -- Merge result sets
