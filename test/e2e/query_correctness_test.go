//go:build e2e

package e2e

import (
	"fmt"
	"strings"
	"testing"
)

// TestE2E_QueryCorrectness runs LynxFlow query correctness tests against SSH and
// OpenStack log datasets ingested via the typed client. This is the bulk
// migration of the original e2e Categories 1-14 and 16.
//
// A single harness (server) is shared across all subtests — data is ingested
// once, then queried many times.
func TestE2E_QueryCorrectness(t *testing.T) {
	h := NewHarness(t)
	h.IngestFile("idx_ssh", "testdata/logs/OpenSSH_2k.log")
	h.IngestFile("idx_openstack", "testdata/logs/OpenStack_2k.log")

	// Category 1: Data Ingestion & Basic Count
	t.Run("Ingestion", func(t *testing.T) {
		t.Run("SSH_TotalCount_2000", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | stats count() as count`), "count", 2000)
		})
		t.Run("OpenStack_TotalCount_2000", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_openstack | stats count() as count`), "count", 2000)
		})
		t.Run("HEAD_10", func(t *testing.T) {
			requireEventCount(t, h.MustQuery(`from idx_ssh | head 10`), 10)
		})
		t.Run("HEAD_1", func(t *testing.T) {
			requireEventCount(t, h.MustQuery(`from idx_ssh | head 1`), 1)
		})
		t.Run("HEAD_LargeN_CappedByData", func(t *testing.T) {
			requireEventCount(t, h.MustQuery(`from idx_ssh | head 5000`), 2000)
		})
	})

	// Category 2: search Command with Keywords
	// In LynxFlow, mid-pipeline "search" is killed (D15). Use where has()/contains()/glob().
	// Quoted phrases (multi-token) use contains(); bare single tokens use has();
	// wildcard patterns use glob().
	t.Run("SearchKeywords", func(t *testing.T) {
		t.Run("SimpleKeyword_FailedPassword_520", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | where contains(_raw, "Failed password") | stats count() as count`), "count", 520)
		})
		t.Run("CaseInsensitive_520", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | where contains(_raw, "failed password") | stats count() as count`), "count", 520)
		})
		t.Run("PhraseSearch_BREAKIN_85", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | where contains(_raw, "BREAK-IN ATTEMPT") | stats count() as count`), "count", 85)
		})
		t.Run("ImplicitAND_FailedPassword_Root_370", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | where contains(_raw, "Failed password") and has(_raw, "root") | stats count() as count`), "count", 370)
		})
		t.Run("OR_SessionOpenedClosed_2", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | where contains(_raw, "session opened") or contains(_raw, "session closed") | stats count() as count`), "count", 2)
		})
		t.Run("NOT_FailedPassword_NotRoot_150", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | where contains(_raw, "Failed password") and not has(_raw, "root") | stats count() as count`), "count", 150)
		})
		t.Run("Wildcard_InvalidUserFrom_252", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | where matches(_raw, r"(?i)Invalid user.*from") | stats count() as count`), "count", 252)
		})
		t.Run("OpenStack_VMStarted_22", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_openstack | where contains(_raw, "VM Started") | stats count() as count`), "count", 22)
		})
		t.Run("OpenStack_WARNING_31", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_openstack | where has(_raw, "WARNING") | stats count() as count`), "count", 31)
		})
		t.Run("OpenStack_Lifecycle_109", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_openstack | where contains(_raw, "Lifecycle Event") | stats count() as count`), "count", 109)
		})
		t.Run("ComplexBoolean_PositiveResult", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh | where (contains(_raw, "Failed password") or contains(_raw, "Invalid user")) and has(_raw, "173.234.31.186") | stats count() as count`)
			total := GetInt(r, "count")
			if total <= 0 {
				t.Errorf("expected > 0 results for complex boolean, got %d", total)
			}
		})
		t.Run("Nonexistent_Returns0", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | where has(_raw, "NONEXISTENT_STRING_12345") | stats count() as count`), "count", 0)
		})
	})

	// Category 3: WHERE Command
	t.Run("WHERE", func(t *testing.T) {
		t.Run("StringComparison_PID24200", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh | parse regex r"sshd\[(?<pid>\d+)\]" | where pid == "24200" | stats count() as count`)
			total := GetInt(r, "count")
			if total <= 0 {
				t.Errorf("expected > 0 events with PID 24200, got %d", total)
			}
		})
		t.Run("IsNotNull_TargetUser_520", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | parse regex r"Failed password for (?<target_user>\w+)" | where exists(target_user) | stats count() as count`), "count", 520)
		})
		t.Run("IsNull_TargetUser_1480", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | parse regex r"Failed password for (?<target_user>\w+)" | where is_null(target_user) | stats count() as count`), "count", 1480)
		})
		t.Run("Match_IP", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh | where matches(_raw, r"173\.234\.31\.186") | stats count() as count`)
			total := GetInt(r, "count")
			if total <= 0 {
				t.Errorf("expected > 0 events matching IP, got %d", total)
			}
		})
		t.Run("AND_PortAndRoot", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh | parse regex r"port (?<port>\d+)" | where exists(port) and matches(_raw, r"root") | stats count() as count`)
			total := GetInt(r, "count")
			if total <= 0 {
				t.Errorf("expected > 0, got %d", total)
			}
		})
		t.Run("OR_VMStartedOrStopped_43", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_openstack | where matches(_raw, r"VM Started") or matches(_raw, r"VM Stopped") | stats count() as count`), "count", 43)
		})
		t.Run("NumericGTE_Status400_41", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_openstack | parse regex r"status: (?<status>\d+)" | where exists(status) | where int(status) >= 400 | stats count() as count`), "count", 41)
		})
		t.Run("RegexStartsWith_Dec10_09_676", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | where matches(_raw, r"^Dec 10 09:") | stats count() as count`), "count", 676)
		})
		t.Run("WhereTrue_1Eq1_2000", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | where 1 == 1 | stats count() as count`), "count", 2000)
		})
	})

	// Category 4: REX (Regular Expression Extraction) -> parse regex
	t.Run("REX", func(t *testing.T) {
		t.Run("ExtractIP_UniqueIPs_30", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | parse regex r"(?<ip_addr>\d+\.\d+\.\d+\.\d+)" | where exists(ip_addr) | stats dc(ip_addr) as unique_ips`), "unique_ips", 30)
		})
		t.Run("ExtractPID_Positive", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh | parse regex r"sshd\[(?<pid>\d+)\]" | where exists(pid) | stats dc(pid) as unique_pids`)
			pids := GetInt(r, "unique_pids")
			if pids <= 0 {
				t.Errorf("expected unique_pids > 0, got %d", pids)
			}
		})
		t.Run("ExtractUsername_TopAdmin_21", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh | parse regex r"Invalid user (?<username>\w+) from" | where exists(username) | stats count() as count by username | sort -count | head 3`)
			rows := EventRows(r)
			if len(rows) < 3 {
				t.Fatalf("expected at least 3 rows, got %d", len(rows))
			}
			top := fmt.Sprint(rows[0]["username"])
			topCount := toInt(rows[0]["count"])
			if top != "admin" {
				t.Errorf("expected top username=admin, got %s", top)
			}
			if topCount != 21 {
				t.Errorf("expected admin count=21, got %d", topCount)
			}
		})
		t.Run("ExtractPort_525", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | parse regex r"port (?<port>\d+)" | where exists(port) | stats count() as count`), "count", 525)
		})
		t.Run("ChainedREX_PositiveTargetsAndIPs", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh | parse regex r"Failed password for (?:invalid user )?(?<target>\w+) from (?<src_ip>\d+\.\d+\.\d+\.\d+)" | where exists(target) and exists(src_ip) | stats dc(target) as unique_targets, dc(src_ip) as unique_ips`)
			targets := GetInt(r, "unique_targets")
			ips := GetInt(r, "unique_ips")
			if targets <= 0 || ips <= 0 {
				t.Errorf("expected unique_targets > 0 and unique_ips > 0, got targets=%d ips=%d", targets, ips)
			}
		})
		t.Run("ExtractLogLevel_INFO1969_WARNING31", func(t *testing.T) {
			r := h.MustQuery(`from idx_openstack | parse regex r"\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d+ \d+ (?<log_level>\w+)" | stats count() as count by log_level | sort log_level`)
			rows := EventRows(r)
			found := map[string]int{}
			for _, row := range rows {
				level := fmt.Sprint(row["log_level"])
				count := toInt(row["count"])
				found[level] = count
			}
			if found["INFO"] != 1969 {
				t.Errorf("expected INFO=1969, got %d", found["INFO"])
			}
			if found["WARNING"] != 31 {
				t.Errorf("expected WARNING=31, got %d", found["WARNING"])
			}
		})
		t.Run("ExtractHTTPStatus_200_933_404_41", func(t *testing.T) {
			r := h.MustQuery(`from idx_openstack | parse regex r"status: (?<http_status>\d+)" | where exists(http_status) | stats count() as count by http_status | sort -count`)
			rows := EventRows(r)
			statusCounts := map[string]int{}
			for _, row := range rows {
				s := fmt.Sprint(row["http_status"])
				statusCounts[s] = toInt(row["count"])
			}
			expected := map[string]int{"200": 933, "404": 41, "204": 22, "202": 21}
			for status, count := range expected {
				if statusCounts[status] != count {
					t.Errorf("status %s: expected %d, got %d", status, count, statusCounts[status])
				}
			}
		})
		t.Run("ExtractHTTPMethod_GET931_POST64_DELETE22", func(t *testing.T) {
			r := h.MustQuery(`from idx_openstack | parse regex r"(?<http_method>GET|POST|PUT|DELETE|PATCH) /" | where exists(http_method) | stats count() as count by http_method | sort -count`)
			rows := EventRows(r)
			methods := map[string]int{}
			for _, row := range rows {
				m := fmt.Sprint(row["http_method"])
				methods[m] = toInt(row["count"])
			}
			if methods["GET"] != 931 {
				t.Errorf("GET: expected 931, got %d", methods["GET"])
			}
			if methods["POST"] != 64 {
				t.Errorf("POST: expected 64, got %d", methods["POST"])
			}
			if methods["DELETE"] != 22 {
				t.Errorf("DELETE: expected 22, got %d", methods["DELETE"])
			}
		})
		t.Run("ExtractInstanceUUID_22", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_openstack | parse regex r"\[instance: (?<instance_id>[a-f0-9-]+)\]" | where exists(instance_id) | stats dc(instance_id) as unique_instances`), "unique_instances", 22)
		})
		t.Run("ExtractResponseTime_PositiveAvg", func(t *testing.T) {
			r := h.MustQuery(`from idx_openstack | parse regex r"time: (?<resp_time>[0-9.]+)" | where exists(resp_time) | extend resp_ms = float(resp_time) * 1000 | stats count() as count, avg(resp_ms) as avg_ms`)
			total := GetInt(r, "count")
			avgMs := GetFloat(r, "avg_ms")
			if total <= 0 {
				t.Errorf("expected total > 0, got %d", total)
			}
			if avgMs <= 0 {
				t.Errorf("expected avg_ms > 0, got %f", avgMs)
			}
		})
		t.Run("NoMatch_Returns0", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | parse regex r"NONEXISTENT_PATTERN_(?<captured>\w+)" | stats count(captured) as matched`), "matched", 0)
		})
		t.Run("FieldParam_NovaSubsystems", func(t *testing.T) {
			r := h.MustQuery(`from idx_openstack | parse regex r"nova\.(?<nova_subsystem>[\w.]+)" from _raw | where exists(nova_subsystem) | stats dc(nova_subsystem) as unique_subsystems`)
			subs := GetInt(r, "unique_subsystems")
			if subs < 3 {
				t.Errorf("expected at least 3 unique subsystems, got %d", subs)
			}
		})
	})

	// Category 5: EVAL (Expression Evaluation) -> extend
	t.Run("EVAL", func(t *testing.T) {
		t.Run("StringAssignment", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh | extend source_type = "ssh_log" | head 1 | keep source_type`)
			rows := EventRows(r)
			if len(rows) != 1 {
				t.Fatalf("expected 1 row, got %d", len(rows))
			}
			st := fmt.Sprint(rows[0]["source_type"])
			if st != "ssh_log" {
				t.Errorf("expected source_type=ssh_log, got %s", st)
			}
		})
		t.Run("IF_AllPublicIPs", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh | parse regex r"(?<ip_addr>\d+\.\d+\.\d+\.\d+)" | extend ip_class = if(matches(ip_addr, r"^10\."), "private", "public") | where exists(ip_addr) | stats count() as count by ip_class`)
			rows := EventRows(r)
			for _, row := range rows {
				cls := fmt.Sprint(row["ip_class"])
				if cls == "private" {
					t.Error("unexpected private IP in SSH logs")
				}
			}
		})
		t.Run("CASE_FailedAuth520", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh
    | extend event_type = case(
          matches(_raw, r"Failed password"), "failed_auth",
          matches(_raw, r"Invalid user"), "invalid_user",
          matches(_raw, r"Accepted"), "success",
          matches(_raw, r"Connection closed"), "conn_closed",
          matches(_raw, r"Received disconnect"), "disconnect",
          matches(_raw, r"BREAK-IN"), "breakin_attempt",
          "other"
      )
    | stats count() as count by event_type
    | sort -count`)
			rows := EventRows(r)
			types := map[string]int{}
			for _, row := range rows {
				et := fmt.Sprint(row["event_type"])
				types[et] = toInt(row["count"])
			}
			if types["failed_auth"] != 520 {
				t.Errorf("expected failed_auth=520, got %d", types["failed_auth"])
			}
		})
		t.Run("Arithmetic_SlowRequests_81", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_openstack
    | parse regex r"time: (?<resp_time>[0-9.]+)"
    | where exists(resp_time)
    | extend resp_ms = round(float(resp_time) * 1000, 2)
    | where resp_ms > 300
    | stats count() as count`), "count", 81)
		})
		t.Run("Len_PositiveAvgMaxMin", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh | extend raw_len = len(_raw) | stats avg(raw_len) as avg_length, max(raw_len) as max_length, min(raw_len) as min_length`)
			avg := GetFloat(r, "avg_length")
			maxLen := GetFloat(r, "max_length")
			minLen := GetFloat(r, "min_length")
			if avg <= 0 || maxLen <= 0 || minLen <= 0 {
				t.Errorf("expected positive lengths: avg=%f max=%f min=%f", avg, maxLen, minLen)
			}
			if maxLen < avg || minLen > avg {
				t.Errorf("inconsistent: min=%f avg=%f max=%f", minLen, avg, maxLen)
			}
		})
		t.Run("Coalesce_NA_1480", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh
    | parse regex r"Failed password for (?<target>\w+)"
    | extend user_or_unknown = target ?? "N/A"
    | stats count() as count by user_or_unknown
    | where user_or_unknown == "N/A"`), "count", 1480)
		})
		t.Run("Lower_INFO1969_WARNING31", func(t *testing.T) {
			r := h.MustQuery(`from idx_openstack | parse regex r"\d+ (?<level>[A-Z]+) " | extend level_lower = lower(level) | stats count() as count by level_lower`)
			rows := EventRows(r)
			levels := map[string]int{}
			for _, row := range rows {
				l := fmt.Sprint(row["level_lower"])
				levels[l] = toInt(row["count"])
			}
			if levels["info"] != 1969 {
				t.Errorf("expected info=1969, got %d", levels["info"])
			}
			if levels["warning"] != 31 {
				t.Errorf("expected warning=31, got %d", levels["warning"])
			}
		})
		t.Run("MultipleAssignments_WithPort525", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh
    | parse regex r"(?<ip>\d+\.\d+\.\d+\.\d+)"
    | parse regex r"port (?<port>\d+)"
    | extend has_ip = if(exists(ip), 1, 0),
           has_port = if(exists(port), 1, 0),
           connection_info = has_ip + has_port
    | stats sum(has_ip) as with_ip, sum(has_port) as with_port`)
			withPort := GetInt(r, "with_port")
			if withPort != 525 {
				t.Errorf("expected with_port=525, got %d", withPort)
			}
		})
		t.Run("NullPropagation_1475", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh
    | parse regex r"port (?<port>\d+)"
    | extend port_plus_one = int(port) + 1
    | where is_null(port_plus_one)
    | stats count() as count`), "count", 1475)
		})
		t.Run("ToString_404_41", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_openstack
    | parse regex r"status: (?<status>\d+)"
    | where exists(status)
    | extend status_str = string(int(status))
    | where status_str == "404"
    | stats count() as count`), "count", 41)
		})
		t.Run("Substr_Positive", func(t *testing.T) {
			r := h.MustQuery(`from idx_openstack | extend log_source = substr(_raw, 0, 10) | stats dc(log_source) as unique_prefixes`)
			prefixes := GetInt(r, "unique_prefixes")
			if prefixes < 3 {
				t.Errorf("expected at least 3 unique prefixes, got %d", prefixes)
			}
		})
		t.Run("Replace_PositiveSubnets", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh
    | parse regex r"(?<ip>\d+\.\d+\.\d+\.\d+)"
    | where exists(ip)
    | extend ip_masked = replace(ip, r"\.\d+$", ".xxx")
    | stats dc(ip_masked) as unique_subnets`)
			subnets := GetInt(r, "unique_subnets")
			if subnets <= 0 {
				t.Errorf("expected unique_subnets > 0, got %d", subnets)
			}
		})
		t.Run("SplitMvcount_5Parts", func(t *testing.T) {
			r := h.MustQuery(`from idx_openstack
    | parse regex r"\[req-(?<req_id>[a-f0-9-]+)"
    | where exists(req_id)
    | extend req_parts = split(req_id, "-")
    | extend part_count = len(req_parts)
    | head 1
    | keep req_id, part_count`)
			rows := EventRows(r)
			if len(rows) > 0 {
				pc := toInt(rows[0]["part_count"])
				if pc != 5 {
					t.Errorf("expected part_count=5, got %d", pc)
				}
			}
		})
		t.Run("NestedFunctions_AvgLastOctet", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh
    | parse regex r"(?<ip>\d+\.\d+\.\d+\.\d+)"
    | where exists(ip)
    | extend ip_last_octet = int(replace(ip, r".*\.", ""))
    | stats avg(ip_last_octet) as avg_last_octet`)
			avg := GetFloat(r, "avg_last_octet")
			if avg <= 0 || avg > 255 {
				t.Errorf("expected avg_last_octet in (0, 255], got %f", avg)
			}
		})
	})

	// Category 6: STATS (Aggregation)
	t.Run("STATS", func(t *testing.T) {
		t.Run("Count_2000", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | stats count() as count`), "count", 2000)
		})
		t.Run("CountBY_Status200_933", func(t *testing.T) {
			r := h.MustQuery(`from idx_openstack | parse regex r"status: (?<status>\d+)" | where exists(status) | stats count() as count by status | sort -count`)
			rows := EventRows(r)
			if len(rows) < 4 {
				t.Errorf("expected at least 4 rows, got %d", len(rows))
			}
			statusCounts := map[string]int{}
			for _, row := range rows {
				s := fmt.Sprint(row["status"])
				statusCounts[s] = toInt(row["count"])
			}
			if statusCounts["200"] != 933 {
				t.Errorf("status 200: expected 933, got %d", statusCounts["200"])
			}
			if statusCounts["404"] != 41 {
				t.Errorf("status 404: expected 41, got %d", statusCounts["404"])
			}
		})
		t.Run("DC_UniqueIPs_30", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | parse regex r"(?<ip>\d+\.\d+\.\d+\.\d+)" | stats dc(ip) as unique_ips`), "unique_ips", 30)
		})
		t.Run("Values_ContainsMethods", func(t *testing.T) {
			r := h.MustQuery(`from idx_openstack | parse regex r"(?<method>GET|POST|PUT|DELETE) /" | where exists(method) | stats values(method) as methods`)
			methods := GetStr(r, "methods")
			for _, m := range []string{"GET", "POST", "DELETE"} {
				if !strings.Contains(methods, m) {
					t.Errorf("expected methods to contain %s, got: %s", m, methods)
				}
			}
		})
		t.Run("Sum_PositiveTotalBytes", func(t *testing.T) {
			r := h.MustQuery(`from idx_openstack | parse regex r"len: (?<resp_len>\d+)" | where exists(resp_len) | extend resp_len_num = float(resp_len) | stats sum(resp_len_num) as total_bytes, avg(resp_len_num) as avg_bytes`)
			total := GetFloat(r, "total_bytes")
			avg := GetFloat(r, "avg_bytes")
			if total <= 0 || avg <= 0 {
				t.Errorf("expected positive values: total=%f avg=%f", total, avg)
			}
		})
		t.Run("MinMax_ResponseTime", func(t *testing.T) {
			r := h.MustQuery(`from idx_openstack | parse regex r"time: (?<resp_time>[0-9.]+)" | where exists(resp_time) | extend rt = float(resp_time) | stats min(rt) as min_time, max(rt) as max_time`)
			minT := GetFloat(r, "min_time")
			maxT := GetFloat(r, "max_time")
			if minT >= maxT {
				t.Errorf("expected min < max: min=%f max=%f", minT, maxT)
			}
			if minT < 0 {
				t.Errorf("expected min >= 0, got %f", minT)
			}
		})
		t.Run("MultipleBY_MethodStatus", func(t *testing.T) {
			r := h.MustQuery(`from idx_openstack
    | parse regex r"(?<method>GET|POST|DELETE) /"
    | parse regex r"status: (?<status>\d+)"
    | where exists(method) and exists(status)
    | stats count() as count by method, status
    | sort method, status`)
			if EventCount(r) < 3 {
				t.Errorf("expected at least 3 method x status combos, got %d", EventCount(r))
			}
		})
		t.Run("NestedEval_RequestsPerIP", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh
    | parse regex r"(?<ip>\d+\.\d+\.\d+\.\d+)"
    | where exists(ip)
    | stats count() as requests, dc(ip) as unique_ips
    | extend requests_per_ip = round(requests / unique_ips, 2)`)
			rpi := GetFloat(r, "requests_per_ip")
			if rpi <= 0 {
				t.Errorf("expected requests_per_ip > 0, got %f", rpi)
			}
		})
		t.Run("Percentile_P95GEMedian", func(t *testing.T) {
			r := h.MustQuery(`from idx_openstack
    | parse regex r"time: (?<resp_time>[0-9.]+)"
    | where exists(resp_time)
    | extend rt = float(resp_time)
    | stats p95(rt) as p95, p50(rt) as median`)
			p95 := GetFloat(r, "p95")
			median := GetFloat(r, "median")
			if p95 < median {
				t.Errorf("expected p95 >= median: p95=%f median=%f", p95, median)
			}
			if p95 <= 0 || median <= 0 {
				t.Errorf("expected positive: p95=%f median=%f", p95, median)
			}
		})
		t.Run("EarliestLatest_NonEmpty", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh | stats earliest(_time) as first_event, latest(_time) as last_event`)
			first := GetStr(r, "first_event")
			last := GetStr(r, "last_event")
			if first == "" || last == "" {
				t.Errorf("expected non-empty timestamps: first=%q last=%q", first, last)
			}
		})
		t.Run("MaxFromSingleIP_867", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh
    | parse regex r"(?<ip>\d+\.\d+\.\d+\.\d+)"
    | where exists(ip)
    | stats count() as count by ip
    | stats max(count) as max_from_single_ip, avg(count) as avg_per_ip`), "max_from_single_ip", 867)
		})
		t.Run("TopUsernames_Admin21", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh | parse regex r"Invalid user (?<username>\w+)" | where exists(username) | stats count() as count by username | sort -count | head 5`)
			rows := EventRows(r)
			if len(rows) != 5 {
				t.Errorf("expected 5 rows, got %d", len(rows))
			}
			if len(rows) > 0 {
				name := fmt.Sprint(rows[0]["username"])
				cnt := toInt(rows[0]["count"])
				if name != "admin" || cnt != 21 {
					t.Errorf("expected admin(21), got %s(%d)", name, cnt)
				}
			}
		})
	})

	// Category 7: BIN (Time Bucketing) -> extend + bin() function
	t.Run("BIN", func(t *testing.T) {
		t.Run("Span1h_SumsTo2000", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh | extend hour_bucket = bin(_time, 1h) | stats count() as count by hour_bucket | sort hour_bucket`)
			rows := EventRows(r)
			if len(rows) < 5 {
				t.Errorf("expected at least 5 hour buckets, got %d", len(rows))
			}
			total := 0
			for _, row := range rows {
				total += toInt(row["count"])
			}
			if total != 2000 {
				t.Errorf("expected bucket totals to sum to 2000, got %d", total)
			}
		})
		t.Run("Span5m_SumsTo2000", func(t *testing.T) {
			r := h.MustQuery(`from idx_openstack | extend time_bucket = bin(_time, 5m) | stats count() as count by time_bucket | sort time_bucket`)
			rows := EventRows(r)
			total := 0
			for _, row := range rows {
				total += toInt(row["count"])
			}
			if total != 2000 {
				t.Errorf("expected 2000 total, got %d", total)
			}
		})
		t.Run("Span1m_Top5", func(t *testing.T) {
			requireEventCount(t, h.MustQuery(`from idx_ssh | extend minute_bucket = bin(_time, 1m) | stats count() as count by minute_bucket | sort -count | head 5`), 5)
		})
		t.Run("BucketCount_About15", func(t *testing.T) {
			r := h.MustQuery(`from idx_openstack | extend bucket = bin(_time, 1m) | stats dc(bucket) as num_buckets`)
			buckets := GetInt(r, "num_buckets")
			if buckets < 10 || buckets > 20 {
				t.Errorf("expected ~15 buckets, got %d", buckets)
			}
		})
		t.Run("BINWithStats_AvgResponse", func(t *testing.T) {
			r := h.MustQuery(`from idx_openstack
    | parse regex r"time: (?<resp_time>[0-9.]+)"
    | where exists(resp_time)
    | extend rt = float(resp_time)
    | extend bucket = bin(_time, 5m)
    | stats avg(rt) as avg_response, count() as requests by bucket
    | sort bucket`)
			if EventCount(r) < 2 {
				t.Errorf("expected at least 2 time buckets, got %d", EventCount(r))
			}
		})
	})

	// Category 8: SORT
	t.Run("SORT", func(t *testing.T) {
		t.Run("Ascending_Order", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh | parse regex r"(?<ip>\d+\.\d+\.\d+\.\d+)" | where exists(ip) | stats count() as count by ip | sort count | head 3`)
			rows := EventRows(r)
			if len(rows) != 3 {
				t.Fatalf("expected 3 rows, got %d", len(rows))
			}
			for i := 1; i < len(rows); i++ {
				prev := toInt(rows[i-1]["count"])
				curr := toInt(rows[i]["count"])
				if curr < prev {
					t.Errorf("not ascending: row[%d]=%d < row[%d]=%d", i, curr, i-1, prev)
				}
			}
		})
		t.Run("Descending_TopIP_867", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh | parse regex r"(?<ip>\d+\.\d+\.\d+\.\d+)" | where exists(ip) | stats count() as count by ip | sort -count | head 1`)
			rows := EventRows(r)
			if len(rows) == 0 {
				t.Fatal("expected at least 1 row, got 0 (parse regex extraction may be broken)")
			}
			ip := fmt.Sprint(rows[0]["ip"])
			count := toInt(rows[0]["count"])
			if ip != "183.62.140.253" {
				t.Errorf("expected top IP=183.62.140.253, got %s", ip)
			}
			if count != 867 {
				t.Errorf("expected count=867, got %d", count)
			}
		})
		t.Run("MultipleFields", func(t *testing.T) {
			r := h.MustQuery(`from idx_openstack
    | parse regex r"(?<method>GET|POST|DELETE) /"
    | parse regex r"status: (?<status>\d+)"
    | where exists(method) and exists(status)
    | stats count() as count by method, status
    | sort method, -count`)
			if EventCount(r) < 3 {
				t.Errorf("expected at least 3 rows, got %d", EventCount(r))
			}
		})
		t.Run("PreservesAllRows_30IPs", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | parse regex r"(?<ip>\d+\.\d+\.\d+\.\d+)" | where exists(ip) | stats count() as count by ip | sort count | stats count() as count`), "count", 30)
		})
		t.Run("StringField_Alphabetical", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh | parse regex r"Invalid user (?<username>\w+)" | where exists(username) | stats count() as count by username | sort username | head 3`)
			rows := EventRows(r)
			if len(rows) != 3 {
				t.Fatalf("expected 3 rows, got %d", len(rows))
			}
			for i := 1; i < len(rows); i++ {
				prev := fmt.Sprint(rows[i-1]["username"])
				curr := fmt.Sprint(rows[i]["username"])
				if curr < prev {
					t.Errorf("not alphabetical: %s < %s", curr, prev)
				}
			}
		})
	})

	// Category 9: RENAME and TABLE -> rename and keep
	t.Run("RenameTable", func(t *testing.T) {
		t.Run("Rename_IP_30", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh
    | parse regex r"(?<ip>\d+\.\d+\.\d+\.\d+)"
    | rename ip as source_ip
    | where exists(source_ip)
    | stats dc(source_ip) as unique_sources`), "unique_sources", 30)
		})
		t.Run("Table_Raw_3Rows", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh | head 3 | keep _raw`)
			rows := EventRows(r)
			if len(rows) != 3 {
				t.Errorf("expected 3 rows, got %d", len(rows))
			}
			for i, row := range rows {
				if _, ok := row["_raw"]; !ok {
					t.Errorf("row %d missing _raw field", i)
				}
			}
		})
		t.Run("RenameThenStats_200_933", func(t *testing.T) {
			r := h.MustQuery(`from idx_openstack
    | parse regex r"status: (?<status>\d+)"
    | rename status as http_code
    | where exists(http_code)
    | stats count() as count by http_code`)
			rows := EventRows(r)
			codes := map[string]int{}
			for _, row := range rows {
				code := fmt.Sprint(row["http_code"])
				codes[code] = toInt(row["count"])
			}
			if codes["200"] != 933 {
				t.Errorf("expected 200=933, got %d", codes["200"])
			}
		})
		t.Run("TableMultipleFields", func(t *testing.T) {
			requireEventCount(t, h.MustQuery(`from idx_ssh
    | parse regex r"sshd\[(?<pid>\d+)\]"
    | parse regex r"(?<ip>\d+\.\d+\.\d+\.\d+)"
    | head 5
    | keep _time, pid, ip`), 5)
		})
	})

	// Category 10: DEDUP
	t.Run("DEDUP", func(t *testing.T) {
		t.Run("DedupField_UniqueIPs_30", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh
    | parse regex r"(?<ip>\d+\.\d+\.\d+\.\d+)"
    | where exists(ip)
    | dedup ip
    | stats count() as count`), "count", 30)
		})
		t.Run("KeepsFirst_5Rows", func(t *testing.T) {
			requireEventCount(t, h.MustQuery(`from idx_ssh
    | parse regex r"(?<ip>\d+\.\d+\.\d+\.\d+)"
    | where exists(ip)
    | dedup ip
    | sort ip
    | head 5`), 5)
		})
		t.Run("MultipleFields_AtLeast3", func(t *testing.T) {
			r := h.MustQuery(`from idx_openstack
    | parse regex r"(?<method>GET|POST|DELETE) /"
    | parse regex r"status: (?<status>\d+)"
    | where exists(method) and exists(status)
    | dedup method, status
    | stats count() as count`)
			combos := GetInt(r, "count")
			if combos < 3 {
				t.Errorf("expected at least 3 unique combos, got %d", combos)
			}
		})
		t.Run("WithLimit_NoneOver3", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh
    | parse regex r"(?<ip>\d+\.\d+\.\d+\.\d+)"
    | where exists(ip)
    | dedup 3 ip
    | stats count() as count by ip
    | where count > 3
    | stats count() as count`), "count", 0)
		})
	})

	// Category 11: EVENTSTATS
	t.Run("EVENTSTATS", func(t *testing.T) {
		t.Run("GlobalAggregation_1017", func(t *testing.T) {
			r := h.MustQuery(`from idx_openstack
    | parse regex r"status: (?<status>\d+)"
    | where exists(status)
    | eventstats count() as total_requests
    | head 1
    | keep status, total_requests`)
			totalReq := GetInt(r, "total_requests")
			if totalReq != 1017 {
				t.Errorf("expected total_requests=1017, got %d", totalReq)
			}
		})
		t.Run("WithBY_TopIP_867", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh
    | parse regex r"(?<ip>\d+\.\d+\.\d+\.\d+)"
    | where exists(ip)
    | eventstats count() as ip_count by ip
    | where ip == "183.62.140.253"
    | head 1
    | keep ip, ip_count`)
			ipCount := GetInt(r, "ip_count")
			if ipCount != 867 {
				t.Errorf("expected ip_count=867, got %d", ipCount)
			}
		})
		t.Run("Percentage_Status200_About92", func(t *testing.T) {
			r := h.MustQuery(`from idx_openstack
    | parse regex r"status: (?<status>\d+)"
    | where exists(status)
    | stats count() as count by status
    | eventstats sum(count) as total
    | extend pct = round(count * 100.0 / total, 2)
    | sort -pct`)
			rows := EventRows(r)
			if len(rows) == 0 {
				t.Fatal("expected at least 1 row, got 0")
			}
			topStatus := fmt.Sprint(rows[0]["status"])
			topPct := toFloat(rows[0]["pct"])
			if topStatus != "200" {
				t.Errorf("expected top status=200, got %s", topStatus)
			}
			if topPct < 90 || topPct > 93 {
				t.Errorf("expected pct ~91.7%%, got %f", topPct)
			}
		})
		t.Run("DoesNotReduce_2000", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | eventstats count() as total | stats count() as count`), "count", 2000)
		})
	})

	// Category 12: STREAMSTATS
	t.Run("STREAMSTATS", func(t *testing.T) {
		t.Run("RunningCount_10Rows", func(t *testing.T) {
			requireEventCount(t, h.MustQuery(`from idx_ssh | streamstats count() as row_num | where row_num <= 10 | keep row_num`), 10)
		})
		t.Run("Window_10Rows", func(t *testing.T) {
			requireEventCount(t, h.MustQuery(`from idx_openstack
    | parse regex r"time: (?<resp_time>[0-9.]+)"
    | where exists(resp_time)
    | extend rt = float(resp_time)
    | streamstats window=5 avg(rt) as rolling_avg
    | head 10
    | keep rt, rolling_avg`), 10)
		})
		t.Run("CurrentTrue_LastRow2000", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh
    | streamstats current=true count() as running_total
    | where running_total == 2000
    | stats count() as count`), "count", 1)
		})
		t.Run("WithBY_FirstOccurrence", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh
    | parse regex r"(?<ip>\d+\.\d+\.\d+\.\d+)"
    | where exists(ip)
    | streamstats count() as ip_running_count by ip
    | where ip == "183.62.140.253" and ip_running_count == 1
    | stats count() as count`), "count", 1)
		})
	})

	// Category 13: TRANSACTION
	t.Run("TRANSACTION", func(t *testing.T) {
		t.Run("ByIP_30Transactions", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh
    | parse regex r"(?<ip>\d+\.\d+\.\d+\.\d+)"
    | where exists(ip)
    | transaction ip
    | stats count() as count`), "count", 30)
		})
		t.Run("WithMaxspan_PositiveSessions", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh
    | parse regex r"(?<ip>\d+\.\d+\.\d+\.\d+)"
    | where exists(ip)
    | transaction ip maxspan=5m
    | stats count() as count`)
			sessions := GetInt(r, "count")
			if sessions <= 0 {
				t.Errorf("expected sessions > 0, got %d", sessions)
			}
		})
		t.Run("Duration_PositiveMax", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh
    | parse regex r"(?<ip>\d+\.\d+\.\d+\.\d+)"
    | where exists(ip)
    | transaction ip
    | extend duration_sec = duration
    | stats max(duration_sec) as max_duration`)
			maxD := GetFloat(r, "max_duration")
			if maxD <= 0 {
				t.Errorf("expected max_duration > 0, got %f", maxD)
			}
		})
	})

	// Category 14: Complex Multi-Stage Pipelines
	t.Run("ComplexPipelines", func(t *testing.T) {
		t.Run("BruteForceDetection_Has183", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh
    | parse regex r"Failed password for (?:invalid user )?(?<target>\w+) from (?<src_ip>\d+\.\d+\.\d+\.\d+) port (?<port>\d+)"
    | where exists(src_ip)
    | stats count() as attempts, dc(target) as unique_targets by src_ip
    | where attempts > 50
    | sort -attempts`)
			rows := EventRows(r)
			if len(rows) < 1 {
				t.Errorf("expected at least 1 brute force IP, got %d", len(rows))
			}
			found := false
			for _, row := range rows {
				if fmt.Sprint(row["src_ip"]) == "183.62.140.253" {
					found = true
				}
			}
			if !found {
				t.Error("expected 183.62.140.253 in brute force results")
			}
		})
		t.Run("APILatencyAnalysis_AtLeast2Methods", func(t *testing.T) {
			r := h.MustQuery(`from idx_openstack
    | parse regex r"(?<method>GET|POST|DELETE) (?<url_path>/[^\s]+) HTTP"
    | parse regex r"status: (?<status>\d+) len: (?<resp_len>\d+) time: (?<resp_time>[0-9.]+)"
    | where exists(method)
    | extend resp_ms = round(float(resp_time) * 1000, 2)
    | stats count() as requests, avg(resp_ms) as avg_latency, max(resp_ms) as max_latency by method
    | sort method`)
			if EventCount(r) < 2 {
				t.Errorf("expected at least 2 methods, got %d", EventCount(r))
			}
		})
		t.Run("EventClassification_PercentagesSumTo100", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh
    | extend category = case(
          matches(_raw, r"Failed password"), "auth_failure",
          matches(_raw, r"Accepted"), "auth_success",
          matches(_raw, r"Invalid user"), "invalid_user",
          matches(_raw, r"BREAK-IN"), "breakin",
          matches(_raw, r"Connection closed"), "conn_closed",
          matches(_raw, r"Received disconnect"), "disconnect",
          matches(_raw, r"pam_unix"), "pam",
          "other"
      )
    | stats count() as count by category
    | eventstats sum(count) as total
    | extend percentage = round(count * 100.0 / total, 1)
    | sort -count`)
			rows := EventRows(r)
			totalPct := 0.0
			for _, row := range rows {
				totalPct += toFloat(row["percentage"])
			}
			if totalPct < 99 || totalPct > 101 {
				t.Errorf("expected percentages to sum to ~100, got %f", totalPct)
			}
		})
		t.Run("InstanceLifecycle_AtLeast3Types", func(t *testing.T) {
			r := h.MustQuery(`from idx_openstack
    | parse regex r"\[instance: (?<instance_id>[a-f0-9-]+)\]"
    | where exists(instance_id)
    | extend lifecycle_event = case(
          matches(_raw, r"VM Started"), "started",
          matches(_raw, r"VM Stopped"), "stopped",
          matches(_raw, r"VM Paused"), "paused",
          matches(_raw, r"VM Resumed"), "resumed",
          matches(_raw, r"spawned successfully"), "spawned",
          matches(_raw, r"Deleting instance"), "deleting",
          matches(_raw, r"Terminating"), "terminating",
          "other"
      )
    | stats count() as count by lifecycle_event
    | sort -count`)
			if EventCount(r) < 3 {
				t.Errorf("expected at least 3 lifecycle event types, got %d", EventCount(r))
			}
		})
		t.Run("TwoLevelAggregation_AtLeast2ThreatLevels", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh
    | parse regex r"(?<ip>\d+\.\d+\.\d+\.\d+)"
    | where exists(ip)
    | stats count() as count by ip
    | extend threat_level = case(
          count > 500, "critical",
          count > 100, "high",
          count > 50, "medium",
          "low"
      )
    | stats count() as count by threat_level
    | sort -count`)
			if EventCount(r) < 2 {
				t.Errorf("expected at least 2 threat levels, got %d", EventCount(r))
			}
		})
		t.Run("TimeBucketedRate_AtLeast2Windows", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh
    | parse regex r"(?<ip>\d+\.\d+\.\d+\.\d+)"
    | where exists(ip)
    | extend time_window = bin(_time, 10m)
    | stats count() as events, dc(ip) as unique_ips by time_window
    | extend events_per_ip = round(events / unique_ips, 2)
    | sort time_window`)
			if EventCount(r) < 2 {
				t.Errorf("expected at least 2 time windows, got %d", EventCount(r))
			}
		})
		t.Run("RequestAnalysisChain_AtLeast2Rows", func(t *testing.T) {
			r := h.MustQuery(`from idx_openstack
    | parse regex r"(?<method>GET|POST|DELETE) (?<url_path>/[^\s]+)"
    | parse regex r"status: (?<status>\d+)"
    | where exists(method) and exists(url_path)
    | extend endpoint = case(
          matches(url_path, r"servers/detail"), "servers_detail",
          matches(url_path, r"os-server-external-events"), "external_events",
          matches(url_path, r"metadata"), "metadata",
          "other"
      )
    | stats count() as count by endpoint, method
    | sort -count`)
			if EventCount(r) < 2 {
				t.Errorf("expected at least 2 rows, got %d", EventCount(r))
			}
		})
		t.Run("SubnetAnalysis_AtLeast3Subnets", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh
    | parse regex r"(?<ip>\d+\.\d+\.\d+\.\d+)"
    | where exists(ip)
    | parse regex r"(?<subnet>\d+\.\d+\.\d+)\." from ip
    | stats count() as requests, dc(ip) as unique_hosts by subnet
    | sort -requests
    | head 5`)
			if EventCount(r) < 3 {
				t.Errorf("expected at least 3 subnets, got %d", EventCount(r))
			}
		})
	})

	// Category 16: Wildcard Search
	// In LynxFlow, mid-pipeline search is killed. Wildcard patterns use glob(),
	// substring patterns use contains(), and boolean ops use where + and/or/not.
	t.Run("WildcardSearch", func(t *testing.T) {
		// A) Prefix wildcards
		t.Run("Prefix_Failed_610", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | where contains(_raw, "Failed") | stats count() as count`), "count", 610)
		})
		t.Run("Prefix_ReceivedDisconnect_468", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | where contains(_raw, "Received disconnect") | stats count() as count`), "count", 468)
		})
		// B) Suffix wildcards
		t.Run("Suffix_Preauth_618", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | where contains(_raw, "preauth") | stats count() as count`), "count", 618)
		})
		t.Run("Suffix_AuthFailure_496", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | where contains(_raw, "authentication failure") | stats count() as count`), "count", 496)
		})
		// C) Contains wildcards
		t.Run("Contains_Password_521", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | where contains(_raw, "password") | stats count() as count`), "count", 521)
		})
		t.Run("Contains_Preauth_618", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | where contains(_raw, "preauth") | stats count() as count`), "count", 618)
		})
		t.Run("Contains_IP_10", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | where contains(_raw, "173.234.31.186") | stats count() as count`), "count", 10)
		})
		// D) Multi-wildcard
		t.Run("Multi_FailedFromPort_524", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | where matches(_raw, r"Failed.*from.*port") | stats count() as count`), "count", 524)
		})
		t.Run("Multi_FailedInvalidFrom_139", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | where matches(_raw, r"Failed.*invalid.*from") | stats count() as count`), "count", 139)
		})
		t.Run("Multi_PasswordRootPortSsh2_370", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | where matches(_raw, r"password.*root.*port.*ssh2") | stats count() as count`), "count", 370)
		})
		// E) Wildcard-only: match all
		t.Run("WildcardOnly_All_2000", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | where exists(_raw) | stats count() as count`), "count", 2000)
		})
		// F) Wildcard with boolean operators
		t.Run("WildcardAND_Root_370", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | where contains(_raw, "Failed") and has(_raw, "root") | stats count() as count`), "count", 370)
		})
		t.Run("WildcardOR_FailedOrInvalid_836", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | where contains(_raw, "Failed") or contains(_raw, "Invalid") | stats count() as count`), "count", 836)
		})
		t.Run("WildcardNOT_NotRoot_1257", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | where not contains(_raw, "root") | stats count() as count`), "count", 1257)
		})
		// G) Field comparison wildcards
		t.Run("FieldPrefix_IP173_10", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_ssh | parse regex r"(?<ip>\d+\.\d+\.\d+\.\d+)" | where exists(ip) | where glob(ip, "173.234.*") | stats count() as count`), "count", 10)
		})
		t.Run("FieldSuffix_IP253_Positive", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh | parse regex r"(?<ip>\d+\.\d+\.\d+\.\d+)" | where exists(ip) | where glob(ip, "*253") | stats count() as count`)
			total := GetInt(r, "count")
			if total <= 0 {
				t.Errorf("expected > 0 events with IP ending in 253, got %d", total)
			}
		})
		t.Run("FieldExistence_Port_525", func(t *testing.T) {
			r := h.MustQuery(`from idx_ssh | parse regex r"port (?<port>\d+)" | where exists(port) | stats count() as count`)
			total := GetInt(r, "count")
			if total != 525 {
				t.Errorf("expected 525 events with port field, got %d", total)
			}
		})
		// H) IN with wildcards -> use OR of globs
		t.Run("INWithWildcards_GET_POST_GT990", func(t *testing.T) {
			r := h.MustQuery(`from idx_openstack | parse regex r"(?<method>GET|POST|PUT|DELETE|PATCH) /" | where exists(method) | where glob(method, "G*") or glob(method, "P*") | stats count() as count`)
			total := GetInt(r, "count")
			if total <= 990 {
				t.Errorf("expected > 990 events matching GET/POST/PUT/PATCH, got %d", total)
			}
		})
		// I) OpenStack wildcards
		t.Run("LifecycleEvent_109", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_openstack | where contains(_raw, "Lifecycle Event") | stats count() as count`), "count", 109)
		})
		t.Run("VMEvent_109", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_openstack | where matches(_raw, r"VM.*Event") | stats count() as count`), "count", 109)
		})
		t.Run("NovaInstance_646", func(t *testing.T) {
			requireAggValue(t, h.MustQuery(`from idx_openstack | where matches(_raw, r"(?i)nova.*instance") | stats count() as count`), "count", 646)
		})
	})
}
