package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExplain_ValidQuery_JSON(t *testing.T) {
	baseURL := newTestServer(t)

	stdout, _, err := runCmd(t, "--server", baseURL, "explain", "--format", "json",
		`from main | where level == "error" | stats count()`)
	if err != nil {
		t.Fatalf("explain failed: %v", err)
	}

	// Should be valid JSON.
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &result); err != nil {
		t.Fatalf("parse explain JSON: %v\noutput: %q", err, stdout)
	}

	// LynxFlow explain returns is_valid and lynxflow_plan.
	if result["is_valid"] != true {
		t.Errorf("explain JSON: is_valid != true, got %#v", result)
	}
}

func TestExplain_ValidQuery_Table(t *testing.T) {
	baseURL := newTestServer(t)

	stdout, _, err := runCmd(t, "--server", baseURL, "explain", "--format", "table",
		`from main | where level == "error" | stats count()`)
	if err != nil {
		t.Fatalf("explain failed: %v", err)
	}

	// Should contain plan-related output.
	if !strings.Contains(stdout, "Scan") && !strings.Contains(stdout, "Aggregate") && !strings.Contains(stdout, "plan") {
		t.Errorf("expected plan output, got: %q", stdout)
	}
}

func TestExplain_InvalidQuery_ShowsErrors(t *testing.T) {
	baseURL := newTestServer(t)

	// The explain endpoint returns HTTP 200 with is_valid=false for parse errors,
	// so the CLI does not return a Go error. Instead, verify the output contains
	// error information.
	stdout, _, _ := runCmd(t, "--server", baseURL, "explain", "--format", "json", "| where")

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &result); err != nil {
		t.Fatalf("parse explain JSON: %v\noutput: %q", err, stdout)
	}

	isValid, ok := result["is_valid"].(bool)
	if !ok {
		t.Fatalf("explain JSON missing 'is_valid' key, got keys: %v", cliMapKeys(result))
	}

	if isValid {
		t.Errorf("expected is_valid=false for incomplete WHERE, got true")
	}
}

func TestExplain_InvalidQuery_TableShowsDiagnostics(t *testing.T) {
	baseURL := newTestServer(t)

	// Invalid query in LynxFlow should show diagnostics.
	stdout, _, _ := runCmd(t, "--server", baseURL, "explain", "--format", "table", "| where")

	// Should contain error or diagnostic information.
	lower := strings.ToLower(stdout)
	if !strings.Contains(lower, "error") && !strings.Contains(lower, "diagnostic") && !strings.Contains(lower, "invalid") {
		t.Errorf("expected error info in invalid explain output, got: %q", stdout)
	}
}
