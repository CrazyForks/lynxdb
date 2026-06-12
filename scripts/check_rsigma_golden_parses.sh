#!/usr/bin/env bash
set -euo pipefail

# Parse-check every rsigma golden .lynxflow fixture with the LynxFlow parser
# (pkg/lynxflow/parser). Defaults to the committed corpus; pass a directory to
# check a freshly generated one (see scripts/sync_rsigma_golden.sh).
#
# The committed corpus is also covered by TestParseLynxFlowGoldens in
# pkg/sigmaqueries; this script exists so generated corpora outside the repo
# tree can be checked too.

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
golden_dir="${1:-$repo_root/pkg/sigmaqueries/testdata/golden}"

if ! command -v go >/dev/null 2>&1; then
  echo "go is required to parse-check rsigma golden fixtures" >&2
  exit 1
fi

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

cat >"$tmpdir/check_rsigma_golden_parses.go" <<'GO'
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/lynxbase/lynxdb/pkg/lynxflow/parser"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "no .lynxflow files provided")
		os.Exit(1)
	}

	var failures int
	for _, path := range os.Args[1:] {
		if err := checkFile(path); err != nil {
			fmt.Fprintln(os.Stderr, err)
			failures++
		}
	}
	if failures > 0 {
		os.Exit(1)
	}
}

func checkFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	var failures int
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		q, diags := parser.Parse(line)
		if q == nil {
			fmt.Fprintf(os.Stderr, "%s:%d: parse returned nil AST\n", path, lineNo)
			failures++
			continue
		}
		for _, d := range diags {
			if d.Severity == parser.SeverityError {
				fmt.Fprintf(os.Stderr, "%s:%d: %s\n", path, lineNo, d.Message)
				failures++
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if failures > 0 {
		return fmt.Errorf("%s: %d parse error(s)", path, failures)
	}
	return nil
}
GO

files=()
while IFS= read -r file; do
  files+=("$file")
done < <(find "$golden_dir" -maxdepth 1 -name '*.lynxflow' -type f | sort)
if ((${#files[@]} == 0)); then
  echo "no .lynxflow files found under $golden_dir" >&2
  exit 1
fi

(cd "$repo_root" && go run "$tmpdir/check_rsigma_golden_parses.go" "${files[@]}")
echo "parsed ${#files[@]} rsigma golden .lynxflow fixture(s)"
