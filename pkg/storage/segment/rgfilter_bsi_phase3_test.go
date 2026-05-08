package segment

import (
	"bytes"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/RoaringBitmap/roaring"

	"github.com/lynxbase/lynxdb/pkg/event"
)

func TestIntegration_RGFilter_BSIProvesRangeHasNoRows_ReturnsRGSkip(t *testing.T) {
	events := makeRGFilterBSIEvents(t, 100, func(i int, e *event.Event) {
		e.SetField("duration_ms", event.IntValue(int64(i+1)))
	})
	reader := openRGFilterBSISegment(t, events, 100, []string{"duration_ms"})
	widenZoneMapMaxForTest(t, reader, 0, "duration_ms", "200")

	node := &RGFilterNode{Op: RGFilterFieldRange, Field: "duration_ms", RangeOp: ">=", RangeVal: "200"}
	eval := NewRGFilterEvaluator(node, reader)
	var stats RGFilterStats

	if verdict := eval.EvaluateRowGroup(0, &stats); verdict != RGSkip {
		t.Fatalf("EvaluateRowGroup = %d, want RGSkip", verdict)
	}
	if stats.RangeBSIChecks != 1 {
		t.Fatalf("RangeBSIChecks = %d, want 1", stats.RangeBSIChecks)
	}
	if stats.RangeBSISkips != 1 {
		t.Fatalf("RangeBSISkips = %d, want 1", stats.RangeBSISkips)
	}
	if mask := eval.RowMaskFor(0); mask != nil {
		t.Fatalf("RowMaskFor(0) = cardinality %d, want nil", mask.GetCardinality())
	}
}

func TestIntegration_RGFilter_BSIProducesRowMask_MatchesSourceRows(t *testing.T) {
	events := makeRGFilterBSIEvents(t, 1024, func(i int, e *event.Event) {
		e.SetField("status", event.IntValue(int64(200+i%400)))
	})
	reader := openRGFilterBSISegment(t, events, 1024, []string{"status"})

	node := &RGFilterNode{Op: RGFilterFieldRange, Field: "status", RangeOp: ">=", RangeVal: "500"}
	eval := NewRGFilterEvaluator(node, reader)
	var stats RGFilterStats

	if verdict := eval.EvaluateRowGroup(0, &stats); verdict != RGMaybe {
		t.Fatalf("EvaluateRowGroup = %d, want RGMaybe", verdict)
	}
	mask := eval.RowMaskFor(0)
	if mask == nil {
		t.Fatal("RowMaskFor(0) = nil, want BSI row mask")
	}
	want := expectedRGFilterBSIMask(events, 0, len(events), "status", func(v event.Value) bool {
		n, ok := v.TryAsInt()
		return ok && n >= 500
	})
	assertBitmapEqual(t, mask, want)
	assertMaskBitsMatchSource(t, mask, events, "status", 32, func(v event.Value) bool {
		n, ok := v.TryAsInt()
		return ok && n >= 500
	})
	if stats.RangeBSIChecks != 1 {
		t.Fatalf("RangeBSIChecks = %d, want 1", stats.RangeBSIChecks)
	}
	if stats.RangeBSIMaskBytes <= 0 {
		t.Fatalf("RangeBSIMaskBytes = %d, want > 0", stats.RangeBSIMaskBytes)
	}
}

func TestIntegration_RGFilter_BSIMasksFromAndPredicates_AreIntersected(t *testing.T) {
	events := makeRGFilterBSIEvents(t, 1024, func(i int, e *event.Event) {
		e.SetField("status", event.IntValue(int64(200+i%400)))
	})
	reader := openRGFilterBSISegment(t, events, 1024, []string{"status"})

	node := &RGFilterNode{
		Op: RGFilterAnd,
		Children: []RGFilterNode{
			{Op: RGFilterFieldRange, Field: "status", RangeOp: ">=", RangeVal: "500"},
			{Op: RGFilterFieldRange, Field: "status", RangeOp: "<", RangeVal: "503"},
		},
	}
	eval := NewRGFilterEvaluator(node, reader)
	var stats RGFilterStats

	if verdict := eval.EvaluateRowGroup(0, &stats); verdict != RGMaybe {
		t.Fatalf("EvaluateRowGroup = %d, want RGMaybe", verdict)
	}
	mask := eval.RowMaskFor(0)
	if mask == nil {
		t.Fatal("RowMaskFor(0) = nil, want intersected BSI row mask")
	}
	want := expectedRGFilterBSIMask(events, 0, len(events), "status", func(v event.Value) bool {
		n, ok := v.TryAsInt()
		return ok && n >= 500 && n < 503
	})
	assertBitmapEqual(t, mask, want)
	if stats.RangeBSIChecks != 2 {
		t.Fatalf("RangeBSIChecks = %d, want 2", stats.RangeBSIChecks)
	}
}

func TestIntegration_RGFilter_ResetRowMasks_PreventsCrossSegmentLeak(t *testing.T) {
	eventsA := makeRGFilterBSIEvents(t, 256, func(i int, e *event.Event) {
		e.SetField("status", event.IntValue(int64(450+i%100)))
	})
	readerA := openRGFilterBSISegment(t, eventsA, 256, []string{"status"})
	eventsB := makeRGFilterBSIEvents(t, 256, func(i int, e *event.Event) {
		e.SetField("status", event.IntValue(int64(500+i%25)))
	})
	readerB := openRGFilterBSISegment(t, eventsB, 256, []string{"status"})

	node := &RGFilterNode{Op: RGFilterFieldRange, Field: "status", RangeOp: ">=", RangeVal: "500"}
	evalA := NewRGFilterEvaluator(node, readerA)
	var statsA RGFilterStats
	if verdict := evalA.EvaluateRowGroup(0, &statsA); verdict != RGMaybe {
		t.Fatalf("segment A EvaluateRowGroup = %d, want RGMaybe", verdict)
	}
	if mask := evalA.RowMaskFor(0); mask == nil || mask.GetCardinality() == 0 {
		t.Fatalf("segment A mask cardinality = %d, want > 0", bitmapCardinality(mask))
	}
	evalA.ResetRowMasks()
	if mask := evalA.RowMaskFor(0); mask != nil {
		t.Fatalf("RowMaskFor(0) after reset = cardinality %d, want nil", mask.GetCardinality())
	}

	evalB := NewRGFilterEvaluator(node, readerB)
	var statsB RGFilterStats
	if verdict := evalB.EvaluateRowGroup(0, &statsB); verdict != RGMaybe {
		t.Fatalf("segment B EvaluateRowGroup = %d, want RGMaybe", verdict)
	}
	maskB := evalB.RowMaskFor(0)
	wantB := expectedRGFilterBSIMask(eventsB, 0, len(eventsB), "status", func(v event.Value) bool {
		n, ok := v.TryAsInt()
		return ok && n >= 500
	})
	assertBitmapEqual(t, maskB, wantB)
}

func TestIntegration_RGFilter_BSIFloatPredicate_UsesOrderedEncoding(t *testing.T) {
	events := makeRGFilterBSIEvents(t, 512, func(i int, e *event.Event) {
		e.SetField("latency_ms", event.FloatValue(0.5+math.Mod(float64(i)*13.75, 999.4)))
	})
	reader := openRGFilterBSISegment(t, events, 512, []string{"latency_ms"})

	node := &RGFilterNode{Op: RGFilterFieldRange, Field: "latency_ms", RangeOp: "<", RangeVal: "100.0"}
	eval := NewRGFilterEvaluator(node, reader)
	var stats RGFilterStats

	if verdict := eval.EvaluateRowGroup(0, &stats); verdict != RGMaybe {
		t.Fatalf("EvaluateRowGroup = %d, want RGMaybe", verdict)
	}
	mask := eval.RowMaskFor(0)
	if mask == nil {
		t.Fatal("RowMaskFor(0) = nil, want BSI row mask")
	}
	want := expectedRGFilterBSIMask(events, 0, len(events), "latency_ms", func(v event.Value) bool {
		f, ok := v.TryAsFloat()
		return ok && f < 100.0
	})
	assertBitmapEqual(t, mask, want)
}

func TestIntegration_RGFilter_BSIConstantOutsideRange_ReturnsRGSkip(t *testing.T) {
	events := makeRGFilterBSIEvents(t, 128, func(i int, e *event.Event) {
		e.SetField("bytes", event.IntValue(int64(100+i%101)))
	})
	reader := openRGFilterBSISegment(t, events, 128, []string{"bytes"})
	widenZoneMapMaxForTest(t, reader, 0, "bytes", "1001")

	node := &RGFilterNode{Op: RGFilterFieldRange, Field: "bytes", RangeOp: ">", RangeVal: "1000"}
	eval := NewRGFilterEvaluator(node, reader)
	var stats RGFilterStats

	if verdict := eval.EvaluateRowGroup(0, &stats); verdict != RGSkip {
		t.Fatalf("EvaluateRowGroup = %d, want RGSkip", verdict)
	}
	if stats.RangeBSISkips != 1 {
		t.Fatalf("RangeBSISkips = %d, want 1", stats.RangeBSISkips)
	}
	if mask := eval.RowMaskFor(0); mask != nil {
		t.Fatalf("RowMaskFor(0) = cardinality %d, want nil", mask.GetCardinality())
	}
}

func TestIntegration_RGFilter_CorruptBSI_DegradesToRGMaybe(t *testing.T) {
	events := makeRGFilterBSIEvents(t, 512, func(i int, e *event.Event) {
		e.SetField("status", event.IntValue(int64(200+i%400)))
	})
	data := writeRGFilterBSISegment(t, events, 512, []string{"status"})
	mutateFirstRangeBSIPayloadByteForReaderTest(t, data)

	reader, err := OpenSegment(data)
	if err != nil {
		t.Fatalf("OpenSegment: %v", err)
	}
	node := &RGFilterNode{Op: RGFilterFieldRange, Field: "status", RangeOp: ">=", RangeVal: "500"}
	eval := NewRGFilterEvaluator(node, reader)
	var stats RGFilterStats

	if verdict := eval.EvaluateRowGroup(0, &stats); verdict != RGMaybe {
		t.Fatalf("EvaluateRowGroup = %d, want RGMaybe on corrupt BSI", verdict)
	}
	if stats.RangeBSISkips != 0 {
		t.Fatalf("RangeBSISkips = %d, want 0", stats.RangeBSISkips)
	}
	if stats.TotalSkipped != 0 {
		t.Fatalf("TotalSkipped = %d, want 0", stats.TotalSkipped)
	}
	if mask := eval.RowMaskFor(0); mask != nil {
		t.Fatalf("RowMaskFor(0) = cardinality %d, want nil", mask.GetCardinality())
	}
}

func TestIntegration_RGFilter_V1SegmentWithRangePredicate_DoesNotUseBSI(t *testing.T) {
	data := writeSyntheticV1SegmentForRangeBSITest(t)
	reader, err := OpenSegment(data)
	if err != nil {
		t.Fatalf("OpenSegment: %v", err)
	}

	node := &RGFilterNode{Op: RGFilterFieldRange, Field: "status", RangeOp: ">=", RangeVal: "200"}
	eval := NewRGFilterEvaluator(node, reader)
	var stats RGFilterStats

	if verdict := eval.EvaluateRowGroup(0, &stats); verdict != RGMaybe {
		t.Fatalf("EvaluateRowGroup = %d, want RGMaybe", verdict)
	}
	if stats.RangeBSISkips != 0 {
		t.Fatalf("RangeBSISkips = %d, want 0", stats.RangeBSISkips)
	}
	if stats.RangeBSIMaskBytes != 0 {
		t.Fatalf("RangeBSIMaskBytes = %d, want 0", stats.RangeBSIMaskBytes)
	}
	if mask := eval.RowMaskFor(0); mask != nil {
		t.Fatalf("RowMaskFor(0) = cardinality %d, want nil", mask.GetCardinality())
	}
}

func TestConcurrent_RGFilter_BSIMasksWithSharedReader_NoRace(t *testing.T) {
	events := makeRGFilterBSIEvents(t, 4096, func(i int, e *event.Event) {
		e.SetField("status", event.IntValue(int64(200+i%400)))
	})
	reader := openRGFilterBSISegment(t, events, 512, []string{"status"})
	node := &RGFilterNode{Op: RGFilterFieldRange, Field: "status", RangeOp: ">=", RangeVal: "500"}

	var wg sync.WaitGroup
	errCh := make(chan string, reader.RowGroupCount())
	for rgIdx := 0; rgIdx < reader.RowGroupCount(); rgIdx++ {
		rgIdx := rgIdx
		wg.Add(1)
		go func() {
			defer wg.Done()
			eval := NewRGFilterEvaluator(node, reader)
			var stats RGFilterStats
			verdict := eval.EvaluateRowGroup(rgIdx, &stats)
			if verdict != RGMaybe {
				errCh <- fmt.Sprintf("rg %d verdict = %d, want RGMaybe", rgIdx, verdict)
				return
			}
			mask := eval.RowMaskFor(rgIdx)
			if mask == nil {
				errCh <- fmt.Sprintf("rg %d mask = nil, want BSI row mask", rgIdx)
				return
			}
			start := rgIdx * 512
			want := expectedRGFilterBSIMask(events, start, reader.RowGroupRowCount(rgIdx), "status", func(v event.Value) bool {
				n, ok := v.TryAsInt()
				return ok && n >= 500
			})
			if !mask.Equals(want) {
				errCh <- fmt.Sprintf("rg %d mask cardinality = %d, want %d", rgIdx, mask.GetCardinality(), want.GetCardinality())
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for msg := range errCh {
		t.Error(msg)
	}
}

func makeRGFilterBSIEvents(t *testing.T, n int, configure func(int, *event.Event)) []*event.Event {
	t.Helper()
	base := time.Date(2026, 5, 8, 14, 0, 0, 0, time.UTC)
	events := make([]*event.Event, n)
	for i := 0; i < n; i++ {
		e := event.NewEvent(base.Add(time.Duration(i)*time.Millisecond), fmt.Sprintf("row=%d", i))
		e.Host = "bsi-host"
		e.Source = "/var/log/range-bsi-phase3.log"
		e.SourceType = "json"
		e.Index = "main"
		configure(i, e)
		events[i] = e
	}
	return events
}

func openRGFilterBSISegment(t *testing.T, events []*event.Event, rowGroupSize int, columns []string) *Reader {
	t.Helper()
	data := writeRGFilterBSISegment(t, events, rowGroupSize, columns)
	reader, err := OpenSegment(data)
	if err != nil {
		t.Fatalf("OpenSegment: %v", err)
	}
	if !reader.HasRangeBSI() {
		t.Fatal("HasRangeBSI() = false, want true")
	}
	return reader
}

func writeRGFilterBSISegment(t *testing.T, events []*event.Event, rowGroupSize int, columns []string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := NewWriter(&buf)
	w.SetRowGroupSize(rowGroupSize)
	overrides := make(map[string]IndexProfile, len(columns))
	for _, col := range columns {
		overrides[col] = IndexProfileRangeBSI
	}
	w.SetIndexConfig(IndexConfig{ProfileOverrides: overrides, BSIMaxBitCount: 64})
	if _, err := w.Write(events); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return append([]byte(nil), buf.Bytes()...)
}

func expectedRGFilterBSIMask(events []*event.Event, globalStart, rowCount int, field string, match func(event.Value) bool) *roaring.Bitmap {
	out := roaring.New()
	for local := 0; local < rowCount; local++ {
		global := globalStart + local
		if global >= len(events) {
			break
		}
		if match(events[global].GetField(field)) {
			out.Add(uint32(local))
		}
	}
	return out
}

func widenZoneMapMaxForTest(t *testing.T, reader *Reader, rgIdx int, field, maxValue string) {
	t.Helper()
	chunk := reader.ColumnChunkInRowGroup(rgIdx, field)
	if chunk == nil {
		t.Fatalf("ColumnChunkInRowGroup(%d, %q) = nil", rgIdx, field)
	}
	chunk.MaxValue = maxValue
}

func assertBitmapEqual(t *testing.T, got, want *roaring.Bitmap) {
	t.Helper()
	if got == nil {
		t.Fatal("got bitmap = nil")
	}
	if !got.Equals(want) {
		t.Fatalf("bitmap cardinality = %d, want %d; got=%v want=%v",
			got.GetCardinality(), want.GetCardinality(), got.ToArray(), want.ToArray())
	}
}

func assertMaskBitsMatchSource(t *testing.T, mask *roaring.Bitmap, events []*event.Event, field string, samples int, match func(event.Value) bool) {
	t.Helper()
	checked := 0
	it := mask.Iterator()
	for it.HasNext() && checked < samples {
		localRow := it.Next()
		if int(localRow) >= len(events) {
			t.Fatalf("mask contains local row %d outside %d source rows", localRow, len(events))
		}
		if !match(events[localRow].GetField(field)) {
			t.Fatalf("mask contains row %d that does not match predicate", localRow)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("mask had no sampled set bits")
	}
}

func bitmapCardinality(mask *roaring.Bitmap) uint64 {
	if mask == nil {
		return 0
	}
	return mask.GetCardinality()
}
