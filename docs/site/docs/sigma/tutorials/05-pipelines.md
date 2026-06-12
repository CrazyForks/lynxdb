# Pipelines

[Back to Sigma docs](../index.md)

Use rsigma pipelines to select a LynxDB index and map rule fields to ingested
event fields. The pipeline shapes what rsigma emits; the emitted query is
still legacy SPL2 and needs the usual hand-migration to LynxFlow before it
runs.

Create a concrete event:

```bash
printf '%s\n' '{"process.command_line":"cmd.exe /c whoami","user.name":"alice"}' > ecs.ndjson
lynxdb ingest ecs.ndjson --source ecs --sourcetype json --index security
```

Create a rule that uses ECS field names:

```bash
cat > ecs-whoami.yml <<'YAML'
title: ECS Whoami
logsource:
  product: windows
detection:
  selection:
    process.command_line|contains: whoami
  condition: selection
YAML
```

Create a pipeline that targets the `security` index:

```bash
cat > ecs-lynxdb.yml <<'YAML'
transformations:
  - type: set_state
    key: index
    value: security
YAML
```

Convert:

```bash
rsigma convert -t lynxdb -p ecs-lynxdb.yml ecs-whoami.yml
```

The legacy SPL2 output (reference only):

```spl
FROM security | search process.command_line=*"whoami"*
```

Hand-migrate to LynxFlow and run:

```bash
cat > ecs-whoami.lynxflow <<'EOF'
from security | where contains(process.command_line, "whoami")
EOF

lynxdb query "$(cat ecs-whoami.lynxflow)" --since 24h
```

If your ingested fields differ from the rule fields, add rsigma field-mapping
transformations to the same pipeline so the emitted (and therefore migrated)
query uses your real field names. Keep those mappings near the rule pack, and
commit the migrated `.lynxflow` file next to the rule so the runnable query is
reproducible.
