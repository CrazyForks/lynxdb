package segment

import (
	"fmt"
	"sync"
	"testing"
)

func TestConcurrent_Reader_LazyCaches_NoRace(t *testing.T) {
	events := makeRangeBSIEvents(t, 2048)
	data := writeRangeBSISegment(t, events, nil)

	r, err := OpenSegment(data)
	if err != nil {
		t.Fatalf("OpenSegment: %v", err)
	}

	const goroutines = 16
	const iterations = 128
	var wg sync.WaitGroup
	errCh := make(chan error, goroutines*iterations)

	for worker := 0; worker < goroutines; worker++ {
		worker := worker
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				if stats := r.StatsByName("level"); stats == nil {
					errCh <- fmt.Errorf("missing level stats")
					return
				}
				rgIdx := (worker + i) % r.RowGroupCount()
				_, err := r.CheckColumnBloom(rgIdx, "_raw", "duration_ms")
				if err != nil {
					errCh <- err
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
}
