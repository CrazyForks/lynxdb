//go:build e2e

package e2e

import (
	"testing"

	"github.com/lynxbase/lynxdb/pkg/client"
)

// TestE2E_Persistence_DataSurvivesRestart ingests data to disk, restarts the
// server, and verifies that all queries return identical results.
//
// Regression test for persistence bug (fixed). Verifies that batcher flush
// and atomic part writes preserve all events across restart for multiple indexes.
func TestE2E_Persistence_DataSurvivesRestart(t *testing.T) {
	h := NewHarness(t, WithDisk())
	h.IngestFile("idx_ssh", "testdata/logs/OpenSSH_2k.log")
	h.IngestFile("idx_openstack", "testdata/logs/OpenStack_2k.log")
	h.FlushBatcher()

	// Collect pre-restart reference results.
	type queryCase struct {
		name  string
		query string
	}
	queries := []queryCase{
		{"SSH_Count", `from idx_ssh | stats count() as count`},
		{"OpenStack_Count", `from idx_openstack | stats count() as count`},
		{"SSH_TopIPs", `from idx_ssh | parse regex r"(?<ip>\d+\.\d+\.\d+\.\d+)" | where exists(ip) | stats count() as count by ip | sort -count | head 3`},
		{"OpenStack_StatusCounts", `from idx_openstack | parse regex r"status: (?<status>\d+)" | where exists(status) | stats count() as count by status | sort status`},
		{"SSH_HourlyBuckets", `from idx_ssh | extend hour = bin(_time, 1h) | stats count() as count by hour | sort hour`},
		{"SSH_FailedCount", `from idx_ssh | extend has_failed = if(matches(_raw, r"Failed password"), 1, 0) | stats sum(has_failed) as failed_count`},
		// count AS attempts in compound STATS (with BY) uses single-agg path where alias is broken.
		// We use count BY user directly.
		{"SSH_TopAttackers", `from idx_ssh | parse regex r"Failed password for (?:invalid user )?(?<user>\w+) from (?<ip>\d+\.\d+\.\d+\.\d+)" | where exists(user) | stats count() as count by user | sort -count, user | head 3`},
		{"SSH_UniqueIPs", `from idx_ssh | parse regex r"(?<ip>\d+\.\d+\.\d+\.\d+)" | where exists(ip) | dedup ip | stats count() as count`},
		{"OpenStack_APILatency", `from idx_openstack | parse regex r"(?<method>GET|POST|DELETE) (?<url_path>/[^\s]+) HTTP" | parse regex r"status: (?<status>\d+) len: (?<resp_len>\d+) time: (?<resp_time>[0-9.]+)" | where exists(method) | extend resp_ms = round(float(resp_time) * 1000, 2) | stats count() as requests, avg(resp_ms) as avg_latency by method | sort method`},
		// Wildcard persistence — validates bloom filter tokenization after flush.
		{"Wildcard_InvalidUserFrom", `from idx_ssh | where matches(_raw, r"Invalid user.*from") | stats count() as count`},
		{"Wildcard_Password", `from idx_ssh | where contains(_raw, "password") | stats count() as count`},
		{"Wildcard_FailedFromPort", `from idx_ssh | where matches(_raw, r"Failed.*from.*port") | stats count() as count`},
	}

	preResults := make(map[string]*client.QueryResult, len(queries))
	for _, q := range queries {
		preResults[q.name] = h.MustQuery(q.query)
	}

	// Restart.
	h.RestartServer()

	// Verify post-restart results match.
	for _, q := range queries {
		t.Run(q.name, func(t *testing.T) {
			post := h.MustQuery(q.query)
			assertQueryResultsEqual(t, q.name, preResults[q.name], post)
		})
	}

	// Additional: verify both indexes are still queryable.
	t.Run("BothIndexes_Queryable", func(t *testing.T) {
		requireAggValue(t, h.MustQuery(`from idx_ssh | stats count() as count`), "count", 2000)
		requireAggValue(t, h.MustQuery(`from idx_openstack | stats count() as count`), "count", 2000)
	})
}
