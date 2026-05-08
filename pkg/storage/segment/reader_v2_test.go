package segment

import "testing"

func TestIntegration_Reader_V2Segment_PreservesColumnAndBloomContracts(t *testing.T) {
	data := writeTinyV2Segment(t)

	r, err := OpenSegment(data)
	if err != nil {
		t.Fatalf("OpenSegment: %v", err)
	}

	if r.EventCount() != 12 {
		t.Fatalf("EventCount = %d, want 12", r.EventCount())
	}
	if r.RowGroupCount() != 2 {
		t.Fatalf("RowGroupCount = %d, want 2", r.RowGroupCount())
	}
	if !r.HasColumnInRowGroup(0, "_raw") {
		t.Fatal("row group 0 should have _raw")
	}
	if !r.HasColumnInRowGroup(1, "status") {
		t.Fatal("row group 1 should have status")
	}
	if !r.IsConstColumn(0, "_source") {
		t.Fatal("_source should be const in row group 0")
	}
	if got, ok := r.GetConstValue(0, "_source"); !ok || got != "/var/log/app.log" {
		t.Fatalf("GetConstValue(_source) = (%q,%v), want (/var/log/app.log,true)", got, ok)
	}

	bf, err := r.BloomFilterForRowGroup(0)
	if err != nil {
		t.Fatalf("BloomFilterForRowGroup: %v", err)
	}
	if bf == nil {
		t.Fatal("row group 0 should have a _raw bloom")
	}
	if !bf.MayContain("request") {
		t.Fatal("_raw bloom should contain request")
	}
	maybe, err := r.CheckColumnBloom(0, "_raw", "processed")
	if err != nil {
		t.Fatalf("CheckColumnBloom: %v", err)
	}
	if !maybe {
		t.Fatal("_raw column bloom should contain processed")
	}

	statuses, err := r.ReadInt64s("status")
	if err != nil {
		t.Fatalf("ReadInt64s(status): %v", err)
	}
	if len(statuses) != 12 {
		t.Fatalf("statuses len = %d, want 12", len(statuses))
	}
	events, err := r.ReadEvents()
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(events) != 12 {
		t.Fatalf("ReadEvents len = %d, want 12", len(events))
	}
}
