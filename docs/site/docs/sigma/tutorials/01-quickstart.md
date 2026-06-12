# Detect Whoami in 60 seconds

[Back to Sigma docs](../index.md)

This tutorial uses rsigma as an external converter and LynxDB as the LynxFlow
execution target. One honest caveat up front: rsigma v0.9.0 emits the legacy
SPL2 dialect, which LynxDB no longer executes, so there is a hand-migration
step between converting and running. It is a one-liner for this rule.

Install rsigma:

```bash
cargo install rsigma
```

Create a small Sigma rule:

```bash
cat > whoami.yml <<'YAML'
title: Whoami Process
logsource:
  product: windows
detection:
  selection:
    CommandLine|contains: whoami
  condition: selection
YAML
```

Create one matching event:

```bash
printf '%s\n' '{"CommandLine":"cmd.exe /c whoami","Image":"C:\\Windows\\System32\\cmd.exe"}' > events.ndjson
```

Convert the rule to see what rsigma produces:

```bash
rsigma convert -t lynxdb whoami.yml
```

Expected output shape — this is legacy SPL2, which LynxDB cannot execute:

```spl
FROM main | search CommandLine=*"whoami"*
```

Hand-migrate it to LynxFlow: `FROM main | search` becomes `from main | where`,
and the `*"whoami"*` contains-glob becomes `contains(CommandLine, "whoami")`
(full table in the [legacy SPL2 mapping](../spl2-mapping.md)):

```bash
cat > whoami.lynxflow <<'EOF'
from main | where contains(CommandLine, "whoami")
EOF
```

Run the migrated query against the event file:

```bash
lynxdb query --file events.ndjson "$(cat whoami.lynxflow)" --format ndjson
```

The output should contain the event with `cmd.exe /c whoami`.

The same query can be sent to a running server without helper commands:

```bash
lynxdb server
```

In another terminal:

```bash
lynxdb ingest events.ndjson --source windows --sourcetype json
QUERY="$(cat whoami.lynxflow)"
curl -sS http://localhost:3100/api/v1/query \
  -H 'content-type: application/json' \
  -d "{\"query\":$(printf '%s' "$QUERY" | jq -Rs .)}"
```
