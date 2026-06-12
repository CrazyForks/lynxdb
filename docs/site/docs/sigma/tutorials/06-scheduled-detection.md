# Scheduled detection

[Back to Sigma docs](../index.md)

LynxDB executes LynxFlow queries. Scheduling stays outside LynxDB.

Convert a rule and hand-migrate it once (rsigma v0.9.0 emits legacy SPL2 that
LynxDB does not execute; see the
[legacy SPL2 mapping](../spl2-mapping.md)):

```bash
rsigma convert -t lynxdb whoami.yml
#   FROM main | search CommandLine=*"whoami"*   <- legacy SPL2, reference only

cat > whoami.lynxflow <<'EOF'
from main | where contains(CommandLine, "whoami")
EOF
```

Create a script:

```bash
cat > run-whoami.sh <<'SH'
#!/bin/sh
set -eu

query="$(cat whoami.lynxflow)"
lynxdb query "$query" --since 5m --format ndjson \
  | while IFS= read -r event; do
      printf '%s\n' "$event"
    done
SH
chmod +x run-whoami.sh
```

Run it every five minutes with cron:

```cron
*/5 * * * * /path/to/run-whoami.sh >> /var/log/lynxdb-sigma.log 2>&1
```

For multiple rules, keep one `.lynxflow` file per rule or run a query file
with one migrated query per line:

```bash
lynxdb query --queries-file rules.lynxflow --since 5m --format ndjson
```
