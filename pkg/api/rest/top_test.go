package rest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestTopSnapshot_EmptyServer(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()

	data := fetchTopSnapshotData(t, srv.Addr())
	if data["server"] == nil {
		t.Fatal("missing server section")
	}
	if data["events"] == nil {
		t.Fatal("missing events section")
	}
	if data["storage"] == nil {
		t.Fatal("missing storage section")
	}
	if data["queries"] == nil {
		t.Fatal("missing queries section")
	}

	server := data["server"].(map[string]interface{})
	if server["version"] == "" {
		t.Fatal("missing server.version")
	}
	if server["health"] == "" {
		t.Fatal("missing server.health")
	}

	queries := data["queries"].(map[string]interface{})
	if _, ok := queries["rows"].([]interface{}); !ok {
		t.Fatal("missing queries.rows")
	}
}

func TestTopSnapshot_IndexesAndLevels(t *testing.T) {
	srv, cleanup := startTestServer(t)
	defer cleanup()

	ingestIndexedTestEvents(t, srv.Addr(), 3)

	data := fetchTopSnapshotData(t, srv.Addr())
	indexes := data["indexes"].([]interface{})
	if len(indexes) < 3 {
		t.Fatalf("indexes: got %d, want at least 3", len(indexes))
	}

	seen := map[string]bool{}
	for _, raw := range indexes {
		idx := raw.(map[string]interface{})
		name, _ := idx["name"].(string)
		seen[name] = true
		if name == "idx-00" {
			if idx["event_count"].(float64) == 0 {
				t.Fatal("idx-00 event_count = 0")
			}
			levels := idx["segments_by_level"].(map[string]interface{})
			if levels["L0"].(float64) == 0 {
				t.Fatal("idx-00 L0 count = 0")
			}
		}
	}
	for _, name := range []string{"idx-00", "idx-01", "idx-02"} {
		if !seen[name] {
			t.Fatalf("missing index %s", name)
		}
	}

	storage := data["storage"].(map[string]interface{})
	levels := storage["segments_by_level"].(map[string]interface{})
	if levels["L0"].(float64) < 3 {
		t.Fatalf("storage L0 count = %v, want at least 3", levels["L0"])
	}
}

func fetchTopSnapshotData(t *testing.T, addr string) map[string]interface{} {
	t.Helper()

	resp, err := http.Get(fmt.Sprintf("http://%s/api/v1/top/snapshot", addr))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	var env map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	data, ok := env["data"].(map[string]interface{})
	if !ok {
		t.Fatal("missing data envelope")
	}

	return data
}
