package rest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Unit tests for detectQueryLanguage
// ---------------------------------------------------------------------------

func TestDetectQueryLanguage_ExplicitLynxFlow(t *testing.T) {
	r := detectQueryLanguage("from main | stats count()", "lynxflow")
	if r.Language != LangLynxFlow {
		t.Fatalf("language: got %s, want lynxflow", r.Language)
	}
	if !r.Explicit {
		t.Fatal("expected explicit=true")
	}
	if r.DetectNotice != "" {
		t.Fatalf("unexpected notice: %s", r.DetectNotice)
	}
}

func TestDetectQueryLanguage_ExplicitSPL2(t *testing.T) {
	r := detectQueryLanguage("index=main | stats count", "spl2")
	if r.Language != LangSPL2 {
		t.Fatalf("language: got %s, want spl2", r.Language)
	}
	if !r.Explicit {
		t.Fatal("expected explicit=true")
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
	// "let $x = ..." is lynxflow-only syntax (SPL2 cannot parse it).
	r := detectQueryLanguage("let $x = from main | stats count(); from $x", "")
	if r.Language != LangLynxFlow {
		t.Fatalf("language: got %s, want lynxflow", r.Language)
	}
	if r.Explicit {
		t.Fatal("expected explicit=false for auto-detect")
	}
	if r.DetectNotice == "" {
		t.Fatal("expected non-empty detect notice")
	}
}

func TestDetectQueryLanguage_AmbiguousGoesToSPL2(t *testing.T) {
	// "from main | stats count()" parses in both -> conservatively SPL2.
	r := detectQueryLanguage("from main | stats count()", "")
	if r.Language != LangSPL2 {
		t.Fatalf("language: got %s, want spl2 (conservative for ambiguous queries)", r.Language)
	}
	if r.Explicit {
		t.Fatal("expected explicit=false for auto-detect")
	}
	if r.DetectNotice == "" {
		t.Fatal("expected non-empty detect notice")
	}
}

func TestDetectQueryLanguage_DetectsSPL2(t *testing.T) {
	// "index=main | stats count" is SPL2 syntax (index= is not valid lynxflow).
	r := detectQueryLanguage("index=main | stats count", "")
	if r.Language != LangSPL2 {
		t.Fatalf("language: got %s, want spl2", r.Language)
	}
	if r.Explicit {
		t.Fatal("expected explicit=false for auto-detect")
	}
	if r.DetectNotice == "" {
		t.Fatal("expected non-empty detect notice")
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
		{"spl2", false},
		{"LynxFlow", false},
		{"SPL2", false},
		{"python", true},
		{"sql", true},
	}
	for _, tt := range tests {
		msg := validateExplicitLanguage(tt.input)
		gotErr := msg != ""
		if gotErr != tt.wantErr {
			t.Errorf("validateExplicitLanguage(%q): got err=%v, want err=%v", tt.input, gotErr, tt.wantErr)
		}
	}
}

// ---------------------------------------------------------------------------
// Integration tests: REST endpoints with language routing
// ---------------------------------------------------------------------------

func TestQuery_LynxFlowExplicit_ReturnsLanguageMeta(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()

	// Ingest test data so the query has something to scan.
	ingestTestEvents(t, srv.Addr(), 5, 1)
	time.Sleep(200 * time.Millisecond) // allow async batcher to flush

	body := `{"q": "from main | stats count()", "language": "lynxflow"}`
	resp, err := http.Post(
		fmt.Sprintf("http://%s/api/v1/query", srv.Addr()),
		"application/json",
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: %d, body: %s", resp.StatusCode, respBody)
	}

	var envelope map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&envelope)
	meta, _ := envelope["meta"].(map[string]interface{})
	if meta == nil {
		t.Fatal("missing meta in response")
	}
	if meta["language"] != "lynxflow" {
		t.Fatalf("meta.language: got %v, want lynxflow", meta["language"])
	}
}

func TestQuery_SPL2Explicit_ReturnsLanguageMeta(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()

	ingestTestEvents(t, srv.Addr(), 5, 1)
	time.Sleep(200 * time.Millisecond)

	body := `{"q": "FROM main | stats count", "language": "spl2"}`
	resp, err := http.Post(
		fmt.Sprintf("http://%s/api/v1/query", srv.Addr()),
		"application/json",
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: %d, body: %s", resp.StatusCode, respBody)
	}

	var envelope map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&envelope)
	meta, _ := envelope["meta"].(map[string]interface{})
	if meta == nil {
		t.Fatal("missing meta in response")
	}
	if meta["language"] != "spl2" {
		t.Fatalf("meta.language: got %v, want spl2", meta["language"])
	}
}

func TestQuery_Detection_AmbiguousGoesToSPL2(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()

	ingestTestEvents(t, srv.Addr(), 5, 1)
	time.Sleep(200 * time.Millisecond)

	// "from main | stats count()" parses in both languages -> conservative SPL2.
	body := `{"q": "from main | stats count()"}`
	resp, err := http.Post(
		fmt.Sprintf("http://%s/api/v1/query", srv.Addr()),
		"application/json",
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: %d, body: %s", resp.StatusCode, respBody)
	}

	var envelope map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&envelope)
	meta, _ := envelope["meta"].(map[string]interface{})
	if meta == nil {
		t.Fatal("missing meta in response")
	}
	if meta["language"] != "spl2" {
		t.Fatalf("meta.language: got %v, want spl2 (conservative for ambiguous queries)", meta["language"])
	}
}

func TestQuery_Detection_SPL2ForIndexEquals(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()

	ingestTestEvents(t, srv.Addr(), 5, 1)
	time.Sleep(200 * time.Millisecond)

	// "index=main | stats count" should auto-detect as spl2.
	body := `{"q": "index=main | stats count"}`
	resp, err := http.Post(
		fmt.Sprintf("http://%s/api/v1/query", srv.Addr()),
		"application/json",
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: %d, body: %s", resp.StatusCode, respBody)
	}

	var envelope map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&envelope)
	meta, _ := envelope["meta"].(map[string]interface{})
	if meta == nil {
		t.Fatal("missing meta in response")
	}
	if meta["language"] != "spl2" {
		t.Fatalf("meta.language: got %v, want spl2", meta["language"])
	}
}

func TestQuery_LynxFlowParseError_StructuredError(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()

	// Invalid lynxflow query — explicit language forces lynxflow parse.
	body := `{"q": "from main | where @@@", "language": "lynxflow"}`
	resp, err := http.Post(
		fmt.Sprintf("http://%s/api/v1/query", srv.Addr()),
		"application/json",
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: %d, want 400, body: %s", resp.StatusCode, respBody)
	}

	var envelope map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&envelope)
	errObj, _ := envelope["error"].(map[string]interface{})
	if errObj == nil {
		t.Fatal("missing error in response")
	}
	// Should have a code and message.
	if errObj["code"] == nil || errObj["code"] == "" {
		t.Fatal("expected non-empty error code")
	}
	if errObj["message"] == nil || errObj["message"] == "" {
		t.Fatal("expected non-empty error message")
	}
	// Should have a position with start/end for span-carrying errors.
	if pos, ok := errObj["position"].(map[string]interface{}); ok {
		if _, hasStart := pos["start"]; !hasStart {
			t.Fatal("position missing start")
		}
		if _, hasEnd := pos["end"]; !hasEnd {
			t.Fatal("position missing end")
		}
	}
}

func TestQuery_LynxFlowRewrites_MetaPresent(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()

	ingestTestEvents(t, srv.Addr(), 5, 1)
	time.Sleep(200 * time.Millisecond)

	// "top 10 host" is a sugar form that desugars — should produce meta.rewrites.
	body := `{"q": "from main | top 10 host", "language": "lynxflow"}`
	resp, err := http.Post(
		fmt.Sprintf("http://%s/api/v1/query", srv.Addr()),
		"application/json",
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: %d, body: %s", resp.StatusCode, respBody)
	}

	var envelope map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&envelope)
	meta, _ := envelope["meta"].(map[string]interface{})
	if meta == nil {
		t.Fatal("missing meta in response")
	}

	rewrites, _ := meta["rewrites"].([]interface{})
	if len(rewrites) == 0 {
		t.Fatal("expected non-empty meta.rewrites for sugar query")
	}
}

func TestQuery_InvalidLanguage_Returns400(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()

	body := `{"q": "from main", "language": "python"}`
	resp, err := http.Post(
		fmt.Sprintf("http://%s/api/v1/query", srv.Addr()),
		"application/json",
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Fatalf("status: %d, want 400", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Explain endpoint with lynxflow
// ---------------------------------------------------------------------------

func TestExplain_LynxFlow_ReturnsIRRender(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()

	u := fmt.Sprintf("http://%s/api/v1/query/explain?q=%s&language=lynxflow",
		srv.Addr(), url.QueryEscape("from main | stats count()"))
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: %d, body: %s", resp.StatusCode, respBody)
	}

	var envelope map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&envelope)
	data, _ := envelope["data"].(map[string]interface{})
	if data == nil {
		t.Fatal("missing data in response")
	}
	if data["is_valid"] != true {
		t.Fatal("expected is_valid=true")
	}
	if data["lynxflow_plan"] == nil || data["lynxflow_plan"] == "" {
		t.Fatal("expected non-empty lynxflow_plan")
	}

	meta, _ := envelope["meta"].(map[string]interface{})
	if meta == nil {
		t.Fatal("missing meta")
	}
	if meta["language"] != "lynxflow" {
		t.Fatalf("meta.language: got %v, want lynxflow", meta["language"])
	}
}

func TestExplain_SPL2_UnchangedBehavior(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()

	u := fmt.Sprintf("http://%s/api/v1/query/explain?q=%s&language=spl2",
		srv.Addr(), url.QueryEscape("FROM main | stats count"))
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: %d, body: %s", resp.StatusCode, respBody)
	}

	var envelope map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&envelope)
	data, _ := envelope["data"].(map[string]interface{})
	if data == nil {
		t.Fatal("missing data in response")
	}
	// SPL2 explain should have parsed.pipeline, not lynxflow_plan.
	if data["lynxflow_plan"] != nil {
		t.Fatal("SPL2 explain should not have lynxflow_plan")
	}
	if data["parsed"] == nil {
		t.Fatal("SPL2 explain should have parsed")
	}
}

// ---------------------------------------------------------------------------
// Catalog endpoint
// ---------------------------------------------------------------------------

func TestCatalog_Shape(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()

	resp, err := http.Get(fmt.Sprintf("http://%s/api/v1/catalog", srv.Addr()))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}

	// Check content-type and ETag.
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type: %s", ct)
	}
	if etag := resp.Header.Get("ETag"); etag == "" {
		t.Fatal("missing ETag header")
	}

	var catalog map[string]interface{}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &catalog); err != nil {
		t.Fatalf("json decode: %v", err)
	}

	// Check required top-level keys.
	for _, key := range []string{"operators", "functions", "aggregates", "parse_formats"} {
		if catalog[key] == nil {
			t.Fatalf("missing catalog key: %s", key)
		}
	}

	// Check that known operators are present.
	operators, _ := catalog["operators"].([]interface{})
	opNames := make(map[string]bool)
	for _, op := range operators {
		o, _ := op.(map[string]interface{})
		if name, ok := o["name"].(string); ok {
			opNames[name] = true
		}
	}
	for _, want := range []string{"from", "stats", "where"} {
		if !opNames[want] {
			t.Fatalf("operator %q not found in catalog", want)
		}
	}

	// Check that known functions are present.
	functions, _ := catalog["functions"].([]interface{})
	fnNames := make(map[string]bool)
	for _, fn := range functions {
		f, _ := fn.(map[string]interface{})
		if name, ok := f["name"].(string); ok {
			fnNames[name] = true
		}
	}
	if !fnNames["has"] {
		t.Fatal("function 'has' not found in catalog")
	}

	// Check parse_formats contains json.
	formats, _ := catalog["parse_formats"].([]interface{})
	foundJSON := false
	for _, f := range formats {
		if f == "json" {
			foundJSON = true
			break
		}
	}
	if !foundJSON {
		t.Fatal("parse_formats missing 'json'")
	}
}

func TestCatalog_ETagConditionalGet(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()

	// First request to get ETag.
	resp1, err := http.Get(fmt.Sprintf("http://%s/api/v1/catalog", srv.Addr()))
	if err != nil {
		t.Fatal(err)
	}
	resp1.Body.Close()
	etag := resp1.Header.Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag")
	}

	// Second request with If-None-Match.
	req, _ := http.NewRequest("GET", fmt.Sprintf("http://%s/api/v1/catalog", srv.Addr()), nil)
	req.Header.Set("If-None-Match", etag)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()

	if resp2.StatusCode != 304 {
		t.Fatalf("status: %d, want 304", resp2.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Stream endpoint with language routing
// ---------------------------------------------------------------------------

func TestQueryStream_LynxFlow(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()

	ingestTestEvents(t, srv.Addr(), 5, 1)
	time.Sleep(200 * time.Millisecond)

	body := `{"q": "from main | head 3", "language": "lynxflow"}`
	resp, err := http.Post(
		fmt.Sprintf("http://%s/api/v1/query/stream", srv.Addr()),
		"application/json",
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: %d, body: %s", resp.StatusCode, respBody)
	}

	// Should be NDJSON.
	if ct := resp.Header.Get("Content-Type"); ct != "application/x-ndjson" {
		t.Fatalf("content-type: %s, want application/x-ndjson", ct)
	}

	// Read all lines; last should be __meta.
	respBody, _ := io.ReadAll(resp.Body)
	lines := bytes.Split(bytes.TrimSpace(respBody), []byte("\n"))
	if len(lines) == 0 {
		t.Fatal("empty stream response")
	}

	// Last line should be __meta.
	var lastLine map[string]interface{}
	if err := json.Unmarshal(lines[len(lines)-1], &lastLine); err != nil {
		t.Fatalf("last line json: %v", err)
	}
	if lastLine["__meta"] == nil {
		t.Fatal("last line missing __meta")
	}
}
