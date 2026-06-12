package rest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
)

// Unit tests for detectQueryLanguage (post-RFC-002: always lynxflow)

func TestDetectQueryLanguage_ExplicitLynxFlow(t *testing.T) {
	r := detectQueryLanguage("from main | stats count()", "lynxflow")
	if r.Language != LangLynxFlow {
		t.Fatalf("language: got %s, want lynxflow", r.Language)
	}
	if !r.Explicit {
		t.Fatal("expected explicit=true")
	}
}

func TestDetectQueryLanguage_ExplicitSPL2_ReturnsLynxFlow(t *testing.T) {
	// Post-RFC-002: langdetect always returns lynxflow. The API layer
	// rejects explicit spl2 separately.
	r := detectQueryLanguage("index=main | stats count", "spl2")
	if r.Language != LangLynxFlow {
		t.Fatalf("language: got %s, want lynxflow", r.Language)
	}
}

func TestDetectQueryLanguage_ExplicitCaseInsensitive(t *testing.T) {
	r := detectQueryLanguage("from main", "LynxFlow")
	if r.Language != LangLynxFlow {
		t.Fatalf("language: got %s, want lynxflow", r.Language)
	}
	if !r.Explicit {
		t.Fatal("expected explicit=true")
	}
}

func TestDetectQueryLanguage_DetectsLynxFlowOnly(t *testing.T) {
	// "let $x = ..." is lynxflow-only syntax.
	r := detectQueryLanguage("let $x = from main | stats count(); from $x", "")
	if r.Language != LangLynxFlow {
		t.Fatalf("language: got %s, want lynxflow", r.Language)
	}
	if r.Explicit {
		t.Fatal("expected explicit=false for auto-detect")
	}
}

func TestDetectQueryLanguage_AmbiguousGoesToLynxFlow(t *testing.T) {
	// "from main | stats count()" parses in both -> lynxflow (post-RFC-002).
	r := detectQueryLanguage("from main | stats count()", "")
	if r.Language != LangLynxFlow {
		t.Fatalf("language: got %s, want lynxflow (parity reached)", r.Language)
	}
	if r.Explicit {
		t.Fatal("expected explicit=false for auto-detect")
	}
}

func TestDetectQueryLanguage_SPL2Syntax_ReturnsLynxFlow(t *testing.T) {
	// Post-RFC-002: even SPL2-style queries route to lynxflow.
	r := detectQueryLanguage("index=main | stats count", "")
	if r.Language != LangLynxFlow {
		t.Fatalf("language: got %s, want lynxflow", r.Language)
	}
}

func TestDetectQueryLanguage_BothFail(t *testing.T) {
	// Garbage that neither parser accepts.
	r := detectQueryLanguage("@@@ totally invalid ###", "")
	if r.Language != LangLynxFlow {
		t.Fatalf("language: got %s, want lynxflow (default)", r.Language)
	}
	if r.Explicit {
		t.Fatal("expected explicit=false")
	}
}

func TestDetectQueryLanguage_LetPrefix(t *testing.T) {
	// "let $x = ..." is a lynxflow prefix hint.
	r := detectQueryLanguage("let $x = from main | stats count(); from $x", "")
	if r.Language != LangLynxFlow {
		t.Fatalf("language: got %s, want lynxflow", r.Language)
	}
}

func TestValidateExplicitLanguage(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"", false},
		{"lynxflow", false},
		{"LynxFlow", false},
		// Post-RFC-002: spl2 returns a migration error.
		{"spl2", true},
		{"SPL2", true},
		{"python", true},
		{"sql", true},
	}
	for _, tt := range tests {
		msg := validateExplicitLanguage(tt.input)
		gotErr := msg != ""
		if gotErr != tt.wantErr {
			t.Errorf("validateExplicitLanguage(%q): got err=%v, want err=%v (msg=%q)", tt.input, gotErr, tt.wantErr, msg)
		}
	}
}

// Integration tests: REST endpoints with language routing

func TestQuery_LynxFlowExplicit_ReturnsLanguageMeta(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()

	// Ingest test data so the query has something to scan.
	ingestTestEvents(t, srv.Addr(), 5, 1)

	body, _ := json.Marshal(map[string]interface{}{
		"q":        `FROM main | stats count()`,
		"language": "lynxflow",
	})
	resp, err := http.Post(fmt.Sprintf("http://%s/api/v1/query", srv.Addr()), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: %d, body: %s", resp.StatusCode, b)
	}

	var envelope map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&envelope)
	meta := envelope["meta"].(map[string]interface{})
	if meta["language"] != "lynxflow" {
		t.Fatalf("meta.language = %v, want lynxflow", meta["language"])
	}
}

func TestQuery_SPL2Explicit_Returns400(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()

	body, _ := json.Marshal(map[string]interface{}{
		"q":        `index=main | stats count`,
		"language": "spl2",
	})
	resp, err := http.Post(fmt.Sprintf("http://%s/api/v1/query", srv.Addr()), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: %d, want 400; body: %s", resp.StatusCode, b)
	}
}

func TestQuery_Detection_DefaultsToLynxFlow(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()

	ingestTestEvents(t, srv.Addr(), 5, 1)

	body, _ := json.Marshal(map[string]interface{}{
		"q": `FROM main | stats count()`,
	})
	resp, err := http.Post(fmt.Sprintf("http://%s/api/v1/query", srv.Addr()), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: %d, body: %s", resp.StatusCode, b)
	}

	var envelope map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&envelope)
	meta := envelope["meta"].(map[string]interface{})
	if meta["language"] != "lynxflow" {
		t.Fatalf("meta.language = %v, want lynxflow", meta["language"])
	}
}

func TestExplain_LynxFlow_ReturnsValidPlan(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()

	u := fmt.Sprintf("http://%s/api/v1/query/explain?q=%s&language=lynxflow", srv.Addr(),
		url.QueryEscape(`FROM main | where status >= 500 | stats count()`))
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: %d, body: %s", resp.StatusCode, b)
	}

	var envelope map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&envelope)
	data := envelope["data"].(map[string]interface{})
	if data["is_valid"] != true {
		t.Fatalf("is_valid = %v, want true", data["is_valid"])
	}
	plan, _ := data["lynxflow_plan"].(string)
	if plan == "" {
		t.Fatal("missing lynxflow_plan")
	}
	// Rich explain: parsed field should contain the structured pipeline.
	parsed, _ := data["parsed"].(map[string]interface{})
	if parsed == nil {
		t.Fatal("missing parsed (rich explain)")
	}
	pipelineStages, _ := parsed["pipeline"].([]interface{})
	if len(pipelineStages) == 0 {
		t.Fatal("parsed.pipeline is empty")
	}
}

func TestExplain_AnalyzeTrue_ReturnsExecution(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()

	ingestTestEvents(t, srv.Addr(), 5, 1)

	u := fmt.Sprintf("http://%s/api/v1/query/explain?q=%s&analyze=true", srv.Addr(),
		url.QueryEscape(`FROM main | stats count()`))
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: %d, body: %s", resp.StatusCode, b)
	}

	var envelope map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&envelope)
	data := envelope["data"].(map[string]interface{})
	if data["is_valid"] != true {
		t.Fatalf("is_valid = %v, want true", data["is_valid"])
	}
	// analyze=true should include an execution stats section.
	if _, ok := data["execution"]; !ok {
		t.Fatal("missing execution stats in analyze=true response")
	}
}

func TestExplain_SPL2Explicit_Returns400(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()

	u := fmt.Sprintf("http://%s/api/v1/query/explain?q=%s&language=spl2", srv.Addr(),
		url.QueryEscape("| stats count"))
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// The explain endpoint validates language and returns 400 for spl2.
	if resp.StatusCode != 400 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: %d, want 400; body: %s", resp.StatusCode, b)
	}
}

func TestLynxFlow_MeanRoutesToLynxFlow(t *testing.T) {
	r := detectQueryLanguage("stats avg(duration)", "")
	if r.Language != LangLynxFlow {
		t.Fatalf("language: got %s, want lynxflow", r.Language)
	}
}
