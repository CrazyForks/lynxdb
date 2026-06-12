# rsigma pipelines for LynxDB

[Back to Sigma docs](index.md)

rsigma pipelines adapt a Sigma rule before conversion. For LynxDB, the common
uses are selecting an index and renaming fields so the rule matches the events
you ingest.

Remember that rsigma v0.9.0 emits the legacy SPL2 dialect, so the converted
output still needs the hand-migration step to LynxFlow described in the
[legacy SPL2 mapping](spl2-mapping.md). Pipelines change *what* rsigma emits
(index, field names); they do not change the dialect.

## Select an index

Without pipeline state, rsigma emits queries against `main`:

```spl
FROM main | search CommandLine="whoami"     <- legacy SPL2, not executable
```

A pipeline with `set_state index=security` changes the generated query to:

```spl
FROM security | search CommandLine="whoami" <- legacy SPL2, not executable
```

which you then migrate to the LynxFlow query you actually run:

```
from security | where CommandLine == "whoami"
```

Use this when your events are stored in a LynxDB index other than `main`. The
curated corpus covers this shape with the `simple_eq_index` golden
(`from security_logs | where CommandLine == "whoami"`).

## Rename fields

Sigma rules often use ECS, OCSF, or Windows event field names. Your LynxDB
events only match if those names exist at query time.

For example, if a rule tests `process.command_line` but the ingested event
uses `CommandLine`, define an rsigma field-mapping pipeline so conversion
emits the legacy shape:

```spl
FROM security | search CommandLine=*"whoami"*           <- legacy SPL2
```

which migrates to:

```
from security | where contains(CommandLine, "whoami")
```

instead of the unmapped:

```spl
FROM security | search process.command_line=*"whoami"*  <- legacy SPL2
```

which would migrate to:

```
from security | where contains(process.command_line, "whoami")
```

Keep the mapping in rsigma. LynxDB receives the final, hand-migrated LynxFlow
string and does not need to know the original Sigma field name.

See [tutorial 05](tutorials/05-pipelines.md) for a copy-pasteable ECS example.
