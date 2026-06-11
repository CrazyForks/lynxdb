package rest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestQueryExplain_Valid(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()

	u := fmt.Sprintf("http://%s/api/v1/query/explain?q=%s", srv.Addr(),
		url.QueryEscape(`FROM main | where has("error") | head 10`))
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: %d, body: %s", resp.StatusCode, body)
	}

	var envelope map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&envelope)
	data := envelope["data"].(map[string]interface{})

	if data["is_valid"] != true {
		t.Fatal("expected is_valid=true")
	}
	plan, ok := data["lynxflow_plan"].(string)
	if !ok || plan == "" {
		t.Fatalf("missing lynxflow_plan: %#v", data)
	}
	// Plan should mention Scan and Filter/Limit nodes.
	if !strings.Contains(plan, "Scan") {
		t.Fatalf("plan missing Scan: %s", plan)
	}
}

func TestQueryExplain_Rewrites(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()

	// "stats count()" without FROM should get a default-source rewrite from
	// the LynxFlow desugar pass. The explain endpoint includes the desugared
	// form in lynxflow_plan.
	query := "stats count()"
	u := fmt.Sprintf("http://%s/api/v1/query/explain?q=%s", srv.Addr(), url.QueryEscape(query))
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: %d, body: %s", resp.StatusCode, body)
	}

	var envelope map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&envelope)
	data := envelope["data"].(map[string]interface{})

	if data["is_valid"] != true {
		t.Fatalf("expected is_valid=true, got: %#v", data)
	}
	// The plan should reference the default source "main".
	plan, _ := data["lynxflow_plan"].(string)
	if !strings.Contains(plan, "main") {
		t.Fatalf("plan missing default source 'main': %s", plan)
	}
}

func TestQueryExplain_InvalidQuery(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()

	u := fmt.Sprintf("http://%s/api/v1/query/explain?q=%s", srv.Addr(),
		url.QueryEscape("INVALID @@@"))
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status: %d (explain should return 200 even for invalid queries)", resp.StatusCode)
	}

	var envelope map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&envelope)
	data := envelope["data"].(map[string]interface{})

	if data["is_valid"] != false {
		t.Fatal("expected is_valid=false")
	}
	errs := data["errors"].([]interface{})
	if len(errs) == 0 {
		t.Fatal("expected errors array")
	}
}

func TestQueryExplain_MissingQuery(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()

	resp, err := http.Get(fmt.Sprintf("http://%s/api/v1/query/explain", srv.Addr()))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Fatalf("status: %d, want 400", resp.StatusCode)
	}
}

func TestQueryExplain_StatsQuery(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()

	u := fmt.Sprintf("http://%s/api/v1/query/explain?q=%s", srv.Addr(),
		url.QueryEscape("FROM main | stats count() by host"))
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var envelope map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&envelope)
	data := envelope["data"].(map[string]interface{})

	if data["is_valid"] != true {
		t.Fatalf("expected is_valid=true, got: %#v", data)
	}
	plan, _ := data["lynxflow_plan"].(string)
	// Stats query plan should mention Aggregate.
	if !strings.Contains(plan, "Aggregate") {
		t.Fatalf("plan missing Aggregate node: %s", plan)
	}
}
