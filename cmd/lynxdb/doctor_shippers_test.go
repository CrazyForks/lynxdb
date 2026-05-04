package main

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestDoctorShippers_AllHealthy(t *testing.T) {
	baseURL := newTestServer(t)
	body := []byte(`{"index":{"_index":"logs"}}
{"message":"doctor shippers hello"}
`)
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/_bulk", baseURL), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/x-ndjson")
	req.Header.Set("User-Agent", "Filebeat/8.15.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bulk status = %d, want 200", resp.StatusCode)
	}

	stdout, _, err := runCmd(t, "--server", baseURL, "doctor", "shippers")
	if err != nil {
		t.Fatalf("runCmd: %v", err)
	}
	for _, want := range []string{"shipper diagnostics", "ES bulk", "bound", "filebeat/8.15.0"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, stdout)
		}
	}
}
