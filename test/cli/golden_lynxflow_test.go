//go:build clitest

package cli_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/lynxbase/lynxdb/pkg/lynxflow/translate"
)

// getRegenFlag returns true when the caller wants to regenerate the
// lynxflow golden transcripts. Triggered by env var because adding
// custom flags alongside the existing -update flag is fragile under
// the clitest build tag.
func getRegenFlag(t *testing.T) bool {
	t.Helper()
	return os.Getenv("LYNXDB_REGEN_LYNXFLOW") == "1"
}

type skipRecord struct {
	Name   string
	Reason string
}

// regenFileMode translates and re-records file-mode transcripts.
// Returns translated names, skipped records, and bug reports.
func regenFileMode(t *testing.T, outRoot string) (translated []string, skipped []skipRecord, bugs []string) {
	t.Helper()

	fileDir := filepath.Join(projectRoot, "testdata", "cli", "file")
	fileTests := discoverTests(t, fileDir)

	for _, tc := range fileTests {
		if tc.ExitCode != "0" {
			skipped = append(skipped, skipRecord{tc.Name, "error test (non-zero exit code)"})
			continue
		}
		if strings.Contains(tc.Name, "lynxflow") {
			skipped = append(skipped, skipRecord{tc.Name, "already a lynxflow test"})
			continue
		}
		if tc.Skip != "" {
			skipped = append(skipped, skipRecord{tc.Name, "skip: " + tc.Skip})
			continue
		}
		if tc.File == "" {
			skipped = append(skipped, skipRecord{tc.Name, "file mode test without # file: header"})
			continue
		}

		lfQuery, _, err := translate.SPL2ToLynxFlow(tc.Query)
		if err != nil {
			skipped = append(skipped, skipRecord{tc.Name, err.Error()})
			continue
		}

		var testContent strings.Builder
		fmt.Fprintf(&testContent, "# file: %s\n", tc.File)
		fmt.Fprintf(&testContent, "# format: %s\n", tc.Format)
		testContent.WriteString("# language: lynxflow\n")
		testContent.WriteString(lfQuery)

		outTestPath := filepath.Join(outRoot, "file", tc.Name+".test")
		if err := os.WriteFile(outTestPath, []byte(testContent.String()), 0o644); err != nil {
			t.Fatalf("write test file %s: %v", outTestPath, err)
		}

		logPath := testdataLog(tc.File)
		args := []string{"query", "--file", logPath, "--format", tc.Format, "--language", "lynxflow", lfQuery}
		result := runLynxDB(t, args...)

		if result.ExitCode != 0 {
			skipped = append(skipped, skipRecord{tc.Name, fmt.Sprintf("lynxflow query failed (exit %d): %s", result.ExitCode, strings.TrimSpace(result.Stderr))})
			os.Remove(outTestPath)
			continue
		}

		var actual string
		switch tc.Format {
		case "json":
			actual = normalizeNDJSON(result.Stdout)
		default:
			actual = normalizeText(result.Stdout)
		}

		// Empty-result guard.
		if _, err := os.Stat(tc.ExpectedPath); err == nil {
			spl2Data, _ := os.ReadFile(tc.ExpectedPath)
			spl2Rows := countNDJSONRows(string(spl2Data))
			lfRows := countNDJSONRows(actual)
			if spl2Rows > 0 && lfRows == 0 {
				bugs = append(bugs, fmt.Sprintf("BUG: %s -- spl2 had %d rows, lynxflow has 0 (empty result)", tc.Name, spl2Rows))
				os.Remove(outTestPath)
				continue
			}
		}

		outExpectedPath := filepath.Join(outRoot, "file", tc.Name+".expected")
		if err := os.WriteFile(outExpectedPath, []byte(actual), 0o644); err != nil {
			t.Fatalf("write expected file %s: %v", outExpectedPath, err)
		}
		translated = append(translated, tc.Name)
	}
	return
}

// regenServerMode translates and re-records server-mode transcripts.
func regenServerMode(t *testing.T, outRoot string) (translated []string, skipped []skipRecord, bugs []string) {
	t.Helper()

	serverDir := filepath.Join(projectRoot, "testdata", "cli", "server")
	serverTests := discoverTests(t, serverDir)

	srv := startServer(t)
	logIndexes := map[string]string{
		"backend_server.log":     "backend",
		"nginx_access.log":       "nginx",
		"frontend_console.log":   "frontend",
		"audit_security.log":     "audit_security",
		"audit_transactions.log": "audit_transactions",
	}
	for logFile, index := range logIndexes {
		ingestFileWithIndex(t, srv, testdataLog(logFile), index)
	}

	for _, tc := range serverTests {
		if tc.ExitCode != "0" {
			skipped = append(skipped, skipRecord{tc.Name, "error test (non-zero exit code)"})
			continue
		}
		if strings.Contains(tc.Name, "lynxflow") {
			skipped = append(skipped, skipRecord{tc.Name, "already a lynxflow test"})
			continue
		}
		if tc.Skip != "" {
			skipped = append(skipped, skipRecord{tc.Name, "skip: " + tc.Skip})
			continue
		}

		lfQuery, _, err := translate.SPL2ToLynxFlow(tc.Query)
		if err != nil {
			skipped = append(skipped, skipRecord{tc.Name, err.Error()})
			continue
		}

		var testContent strings.Builder
		fmt.Fprintf(&testContent, "# format: %s\n", tc.Format)
		testContent.WriteString("# language: lynxflow\n")
		testContent.WriteString(lfQuery)

		outTestPath := filepath.Join(outRoot, "server", tc.Name+".test")
		if err := os.WriteFile(outTestPath, []byte(testContent.String()), 0o644); err != nil {
			t.Fatalf("write test file %s: %v", outTestPath, err)
		}

		args := []string{"--server", srv.BaseURL, "query", "--format", tc.Format, "--language", "lynxflow", lfQuery}
		result := runLynxDB(t, args...)

		if result.ExitCode != 0 {
			skipped = append(skipped, skipRecord{tc.Name, fmt.Sprintf("lynxflow query failed (exit %d): %s", result.ExitCode, strings.TrimSpace(result.Stderr))})
			os.Remove(outTestPath)
			continue
		}

		var actual string
		switch tc.Format {
		case "json":
			actual = normalizeNDJSON(result.Stdout)
		default:
			actual = normalizeText(result.Stdout)
		}

		// Empty-result guard.
		if _, err := os.Stat(tc.ExpectedPath); err == nil {
			spl2Data, _ := os.ReadFile(tc.ExpectedPath)
			spl2Rows := countNDJSONRows(string(spl2Data))
			lfRows := countNDJSONRows(actual)
			if spl2Rows > 0 && lfRows == 0 {
				bugs = append(bugs, fmt.Sprintf("BUG: %s -- spl2 had %d rows, lynxflow has 0 (empty result)", tc.Name, spl2Rows))
				os.Remove(outTestPath)
				continue
			}
		}

		outExpectedPath := filepath.Join(outRoot, "server", tc.Name+".expected")
		if err := os.WriteFile(outExpectedPath, []byte(actual), 0o644); err != nil {
			t.Fatalf("write expected file %s: %v", outExpectedPath, err)
		}
		translated = append(translated, tc.Name)
	}
	return
}

// TestRegenLynxFlow_File generates file-mode lynxflow golden transcripts.
// Run with: LYNXDB_REGEN_LYNXFLOW=1 go test -tags clitest -run TestRegenLynxFlow_File -timeout 10m ./test/cli/
func TestRegenLynxFlow_File(t *testing.T) {
	if !getRegenFlag(t) {
		t.Skip("set LYNXDB_REGEN_LYNXFLOW=1 to regenerate lynxflow transcripts")
	}

	outRoot := filepath.Join(projectRoot, "testdata", "cli-lynxflow")
	os.MkdirAll(filepath.Join(outRoot, "file"), 0o755)

	translated, skipped, bugs := regenFileMode(t, outRoot)
	logRegenReport(t, "file", translated, skipped, bugs, outRoot)
}

// TestRegenLynxFlow_Server generates server-mode lynxflow golden transcripts.
// Run with: LYNXDB_REGEN_LYNXFLOW=1 go test -tags clitest -run TestRegenLynxFlow_Server -timeout 10m ./test/cli/
func TestRegenLynxFlow_Server(t *testing.T) {
	if !getRegenFlag(t) {
		t.Skip("set LYNXDB_REGEN_LYNXFLOW=1 to regenerate lynxflow transcripts")
	}

	outRoot := filepath.Join(projectRoot, "testdata", "cli-lynxflow")
	os.MkdirAll(filepath.Join(outRoot, "server"), 0o755)

	translated, skipped, bugs := regenServerMode(t, outRoot)
	logRegenReport(t, "server", translated, skipped, bugs, outRoot)
}

// TestRegenLynxFlow generates ALL lynxflow golden transcripts (file + server).
// Run with: LYNXDB_REGEN_LYNXFLOW=1 go test -tags clitest -run TestRegenLynxFlow$ -timeout 15m ./test/cli/
func TestRegenLynxFlow(t *testing.T) {
	if !getRegenFlag(t) {
		t.Skip("set LYNXDB_REGEN_LYNXFLOW=1 to regenerate lynxflow transcripts")
	}

	outRoot := filepath.Join(projectRoot, "testdata", "cli-lynxflow")
	os.RemoveAll(outRoot)
	os.MkdirAll(filepath.Join(outRoot, "file"), 0o755)
	os.MkdirAll(filepath.Join(outRoot, "server"), 0o755)

	ft, fs, fb := regenFileMode(t, outRoot)
	st, ss, sb := regenServerMode(t, outRoot)

	allTranslated := append(ft, st...)
	allSkipped := append(fs, ss...)
	allBugs := append(fb, sb...)

	writeSkippedMDFromRecords(t, outRoot, allSkipped)
	logRegenReport(t, "all", allTranslated, allSkipped, allBugs, outRoot)

	// Diff sampling.
	t.Logf("=== Diff Sampling (first 10 translated with spl2 golden) ===")
	sampled := 0
	for _, name := range allTranslated {
		if sampled >= 10 {
			break
		}
		for _, sub := range []string{"file", "server"} {
			spl2Path := filepath.Join(projectRoot, "testdata", "cli", sub, name+".expected")
			lfPath := filepath.Join(outRoot, sub, name+".expected")
			spl2Data, err1 := os.ReadFile(spl2Path)
			lfData, err2 := os.ReadFile(lfPath)
			if err1 != nil || err2 != nil {
				continue
			}
			if string(spl2Data) != string(lfData) {
				spl2Lines := strings.Split(string(spl2Data), "\n")
				lfLines := strings.Split(string(lfData), "\n")
				t.Logf("DIFF %s/%s: spl2=%d lines, lf=%d lines", sub, name, len(spl2Lines), len(lfLines))
			} else {
				t.Logf("IDENTICAL %s/%s", sub, name)
			}
			sampled++
			break
		}
	}
}

func logRegenReport(t *testing.T, mode string, translated []string, skipped []skipRecord, bugs []string, outRoot string) {
	t.Helper()
	t.Logf("=== LynxFlow Golden Transcript Generation (%s) ===", mode)
	t.Logf("Translated+recorded: %d", len(translated))
	t.Logf("Skipped: %d", len(skipped))

	reasonGroups := map[string]int{}
	for _, s := range skipped {
		reason := s.Reason
		if idx := strings.Index(reason, ":"); idx > 0 && idx < 60 {
			reason = strings.TrimSpace(reason[:idx])
		}
		reasonGroups[reason]++
	}
	for reason, count := range reasonGroups {
		t.Logf("  - %s: %d", reason, count)
	}
	if len(bugs) > 0 {
		t.Logf("BUGS (empty-result guard):")
		for _, b := range bugs {
			t.Logf("  %s", b)
		}
	}
}

// ---------- Dual-suite tests ----------

// TestGoldenLynxFlow_File runs the lynxflow file-mode golden suite.
func TestGoldenLynxFlow_File(t *testing.T) {
	testDir := filepath.Join(projectRoot, "testdata", "cli-lynxflow", "file")
	if _, err := os.Stat(testDir); os.IsNotExist(err) {
		t.Skip("testdata/cli-lynxflow/file/ not generated yet; run with LYNXDB_REGEN_LYNXFLOW=1")
	}

	tests := discoverTests(t, testDir)
	if len(tests) == 0 {
		t.Skip("no .test files found in testdata/cli-lynxflow/file/")
	}

	for _, tc := range tests {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			if tc.Skip != "" {
				t.Skip(tc.Skip)
			}
			if tc.File == "" {
				t.Fatalf("test %s: file mode requires '# file:' header", tc.Name)
			}
			logPath := testdataLog(tc.File)
			args := []string{"query", "--file", logPath, "--format", tc.Format}
			if tc.Language != "" {
				args = append(args, "--language", tc.Language)
			}
			args = append(args, tc.Query)
			result := runLynxDB(t, args...)
			assertGolden(t, tc, result)
		})
	}
}

// TestGoldenLynxFlow_Server runs the lynxflow server-mode golden suite.
func TestGoldenLynxFlow_Server(t *testing.T) {
	testDir := filepath.Join(projectRoot, "testdata", "cli-lynxflow", "server")
	if _, err := os.Stat(testDir); os.IsNotExist(err) {
		t.Skip("testdata/cli-lynxflow/server/ not generated yet; run with LYNXDB_REGEN_LYNXFLOW=1")
	}

	tests := discoverTests(t, testDir)
	if len(tests) == 0 {
		t.Skip("no .test files found in testdata/cli-lynxflow/server/")
	}

	srv := startServer(t)
	logIndexes := map[string]string{
		"backend_server.log":     "backend",
		"nginx_access.log":       "nginx",
		"frontend_console.log":   "frontend",
		"audit_security.log":     "audit_security",
		"audit_transactions.log": "audit_transactions",
	}
	for logFile, index := range logIndexes {
		ingestFileWithIndex(t, srv, testdataLog(logFile), index)
	}

	querySlots := make(chan struct{}, 8)
	for _, tc := range tests {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			if tc.Skip != "" {
				t.Skip(tc.Skip)
			}
			args := []string{"--server", srv.BaseURL, "query", "--format", tc.Format}
			if tc.Language != "" {
				args = append(args, "--language", tc.Language)
			}
			args = append(args, tc.Query)
			querySlots <- struct{}{}
			defer func() { <-querySlots }()
			result := runLynxDB(t, args...)
			assertGolden(t, tc, result)
		})
	}
}

// ---------- Helpers ----------

func countNDJSONRows(s string) int {
	n := 0
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var m map[string]interface{}
		if json.Unmarshal([]byte(line), &m) == nil {
			n++
		}
	}
	return n
}

func writeSkippedMDFromRecords(t *testing.T, outRoot string, skipped []skipRecord) {
	t.Helper()
	if len(skipped) == 0 {
		return
	}

	groups := map[string][]skipRecord{}
	for _, s := range skipped {
		groupKey := s.Reason
		if strings.HasPrefix(groupKey, "translate.SPL2ToLynxFlow") {
			// Extract the inner reason.
			if idx := strings.LastIndex(groupKey, ": unsupported command"); idx > 0 {
				groupKey = strings.TrimSpace(groupKey[idx+2:])
			} else {
				groupKey = "translation error"
			}
		} else if strings.Contains(groupKey, "lynxflow query failed") {
			groupKey = "lynxflow runtime error"
		}
		// Shorten long reasons for grouping.
		if len(groupKey) > 80 {
			if idx := strings.Index(groupKey, "("); idx > 0 && idx < 60 {
				groupKey = strings.TrimSpace(groupKey[:idx])
			}
		}
		groups[groupKey] = append(groups[groupKey], s)
	}

	var keys []string
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString("# Skipped Transcripts\n\n")
	sb.WriteString(fmt.Sprintf("Total skipped: %d\n\n", len(skipped)))
	for _, k := range keys {
		entries := groups[k]
		sb.WriteString(fmt.Sprintf("## %s (%d)\n\n", k, len(entries)))
		for _, e := range entries {
			sb.WriteString(fmt.Sprintf("- `%s`: %s\n", e.Name, e.Reason))
		}
		sb.WriteByte('\n')
	}

	path := filepath.Join(outRoot, "SKIPPED.md")
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		t.Fatalf("write SKIPPED.md: %v", err)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
