package server

import (
	"sort"
	"time"

	"github.com/lynxbase/lynxdb/pkg/bufmgr"
	"github.com/lynxbase/lynxdb/pkg/memgov"
	"github.com/lynxbase/lynxdb/pkg/spl2"
)

// TopSnapshot is the engine-owned portion of the live "top" dashboard state.
type TopSnapshot struct {
	Events  TopEventsSnapshot  `json:"events"`
	Storage TopStorageSnapshot `json:"storage"`
	Indexes []TopIndexSnapshot `json:"indexes"`
	Queries TopQueriesSnapshot `json:"queries"`
	Memory  TopMemorySnapshot  `json:"memory"`
	Cluster ClusterStatusInfo  `json:"cluster"`
}

type TopEventsSnapshot struct {
	Total         int64   `json:"total"`
	Today         int64   `json:"today"`
	Buffered      int64   `json:"buffered"`
	IngestRateEPS float64 `json:"ingest_rate_eps"`
}

type TopStorageSnapshot struct {
	UsedBytes       int64          `json:"used_bytes"`
	SegmentCount    int            `json:"segment_count"`
	SegmentBytes    int64          `json:"segment_bytes"`
	SegmentsByLevel map[string]int `json:"segments_by_level"`
	OldestEvent     string         `json:"oldest_event,omitempty"`
}

type TopIndexSnapshot struct {
	Name            string         `json:"name"`
	EventCount      int64          `json:"event_count"`
	SegmentCount    int            `json:"segment_count"`
	SizeBytes       int64          `json:"size_bytes"`
	SegmentsByLevel map[string]int `json:"segments_by_level"`
	ActiveQueries   int            `json:"active_queries"`
	LoadScore       float64        `json:"load_score"`
}

type TopQueriesSnapshot struct {
	Active       int           `json:"active"`
	Recent       int           `json:"recent"`
	CacheHitRate float64       `json:"cache_hit_rate"`
	Rows         []TopQueryRow `json:"rows"`
}

type TopQueryRow struct {
	JobID              string    `json:"job_id"`
	Query              string    `json:"query"`
	Status             string    `json:"status"`
	CreatedAt          time.Time `json:"created_at"`
	ElapsedMS          float64   `json:"elapsed_ms"`
	Phase              string    `json:"phase,omitempty"`
	Percent            float64   `json:"percent"`
	RowsReadSoFar      int64     `json:"rows_read_so_far,omitempty"`
	SegmentsTotal      int       `json:"segments_total,omitempty"`
	SegmentsScanned    int       `json:"segments_scanned,omitempty"`
	SegmentsDispatched int       `json:"segments_dispatched,omitempty"`
	SegmentsSkipped    int       `json:"segments_skipped,omitempty"`
	CurrentMemoryBytes int64     `json:"current_memory_bytes,omitempty"`
	PeakMemoryBytes    int64     `json:"peak_memory_bytes,omitempty"`
	SpillBytes         int64     `json:"spill_bytes,omitempty"`
	SpillFiles         int       `json:"spill_files,omitempty"`
	ProcessedBytes     int64     `json:"processed_bytes,omitempty"`
	Indexes            []string  `json:"indexes,omitempty"`
}

type TopMemorySnapshot struct {
	Governor      *memgov.TotalStats   `json:"governor,omitempty"`
	BufferManager *bufmgr.ManagerStats `json:"buffer_manager,omitempty"`
	SpillFiles    int                  `json:"spill_files"`
	SpillBytes    int64                `json:"spill_bytes"`
}

// TopSnapshot returns one consistent-enough monitoring snapshot for the TUI.
func (e *Engine) TopSnapshot() TopSnapshot {
	stats := e.Stats()
	cluster := e.ClusterStatus()

	snap := TopSnapshot{
		Events: TopEventsSnapshot{
			Total:    stats.TotalEvents,
			Today:    stats.EventsToday,
			Buffered: stats.BufferedEvents,
		},
		Storage: TopStorageSnapshot{
			UsedBytes:       stats.StorageBytes,
			SegmentCount:    stats.SegmentCount,
			SegmentBytes:    stats.StorageBytes,
			SegmentsByLevel: map[string]int{"L0": 0, "L1": 0, "L2": 0, "L3": 0},
			OldestEvent:     stats.OldestEvent,
		},
		Cluster: cluster,
	}

	indexes := e.topIndexStats()
	jobs := e.topQueryRows(indexes)
	for _, row := range jobs {
		for _, name := range row.Indexes {
			if idx, ok := indexes[name]; ok && row.Status == JobStatusRunning {
				idx.ActiveQueries++
				idx.LoadScore += 1
			}
		}
	}

	snap.Indexes = make([]TopIndexSnapshot, 0, len(indexes))
	for _, idx := range indexes {
		if idx.SegmentCount > 0 {
			idx.LoadScore += float64(idx.SegmentCount) / 100
		}
		snap.Indexes = append(snap.Indexes, *idx)
		for level, count := range idx.SegmentsByLevel {
			snap.Storage.SegmentsByLevel[level] += count
		}
	}
	sort.Slice(snap.Indexes, func(i, j int) bool {
		a, b := snap.Indexes[i], snap.Indexes[j]
		if a.ActiveQueries != b.ActiveQueries {
			return a.ActiveQueries > b.ActiveQueries
		}
		if a.EventCount != b.EventCount {
			return a.EventCount > b.EventCount
		}
		if a.SizeBytes != b.SizeBytes {
			return a.SizeBytes > b.SizeBytes
		}
		return a.Name < b.Name
	})

	cacheStats := e.CacheStats()
	snap.Queries = TopQueriesSnapshot{
		Active:       int(e.ActiveJobCount()),
		Recent:       len(jobs),
		CacheHitRate: cacheStats.HitRate,
		Rows:         jobs,
	}

	spillFiles, spillBytes := e.SpillStats()
	snap.Memory.SpillFiles = spillFiles
	snap.Memory.SpillBytes = spillBytes
	snap.Memory.Governor = e.GovernorStats()
	snap.Memory.BufferManager = e.BufMgrStats()

	return snap
}

func (e *Engine) topIndexStats() map[string]*TopIndexSnapshot {
	e.mu.RLock()
	defer e.mu.RUnlock()

	out := make(map[string]*TopIndexSnapshot, len(e.indexes))
	for name := range e.indexes {
		out[name] = &TopIndexSnapshot{
			Name:            name,
			SegmentsByLevel: map[string]int{"L0": 0, "L1": 0, "L2": 0, "L3": 0},
		}
	}
	for _, sh := range e.currentEpoch.Load().segments {
		name := sh.meta.Index
		if name == "" {
			name = DefaultIndexName
		}
		idx := out[name]
		if idx == nil {
			idx = &TopIndexSnapshot{
				Name:            name,
				SegmentsByLevel: map[string]int{"L0": 0, "L1": 0, "L2": 0, "L3": 0},
			}
			out[name] = idx
		}
		idx.EventCount += sh.meta.EventCount
		idx.SegmentCount++
		idx.SizeBytes += sh.meta.SizeBytes
		idx.SegmentsByLevel[levelName(sh.meta.Level)]++
	}

	return out
}

func (e *Engine) topQueryRows(indexes map[string]*TopIndexSnapshot) []TopQueryRow {
	rows := make([]TopQueryRow, 0)
	e.jobs.Range(func(_, value interface{}) bool {
		job := value.(*SearchJob)
		snap := job.Snapshot()
		progress := job.Progress.Load()

		elapsed := time.Since(snap.CreatedAt).Seconds() * 1000
		if !snap.DoneAt.IsZero() {
			elapsed = snap.DoneAt.Sub(snap.CreatedAt).Seconds() * 1000
		}

		row := TopQueryRow{
			JobID:              snap.ID,
			Query:              snap.Query,
			Status:             snap.Status,
			CreatedAt:          snap.CreatedAt,
			ElapsedMS:          elapsed,
			CurrentMemoryBytes: snap.Stats.MemAllocBytes,
			PeakMemoryBytes:    snap.Stats.PeakMemoryBytes,
			SpillBytes:         snap.Stats.SpillBytes,
			SpillFiles:         snap.Stats.SpillFiles,
			ProcessedBytes:     snap.Stats.ProcessedBytes,
			Indexes:            append([]string(nil), snap.Stats.IndexesUsed...),
		}
		if progress != nil {
			row.Phase = string(progress.Phase)
			row.ElapsedMS = progress.ElapsedMS
			row.RowsReadSoFar = progress.RowsReadSoFar
			row.SegmentsTotal = progress.SegmentsTotal
			row.SegmentsScanned = progress.SegmentsScanned
			row.SegmentsDispatched = progress.SegmentsDispatched
			row.SegmentsSkipped = progress.SegmentsSkippedIdx + progress.SegmentsSkippedTime +
				progress.SegmentsSkippedStat + progress.SegmentsSkippedBF + progress.SegmentsSkippedRange
			if progress.SegmentsTotal > 0 {
				done := progress.SegmentsScanned + row.SegmentsSkipped
				row.Percent = clampPercent(float64(done) / float64(progress.SegmentsTotal) * 100)
			}
		} else if snap.Status != JobStatusRunning {
			row.Percent = 100
		}
		if len(row.Indexes) == 0 {
			row.Indexes = inferQueryIndexes(snap.Query, indexes)
		}
		rows = append(rows, row)

		return true
	})
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Status != rows[j].Status {
			return rows[i].Status == JobStatusRunning
		}
		return rows[i].CreatedAt.After(rows[j].CreatedAt)
	})

	return rows
}

func inferQueryIndexes(query string, indexes map[string]*TopIndexSnapshot) []string {
	prog, err := spl2.ParseProgram(spl2.NormalizeQuery(query))
	if err != nil {
		if _, ok := indexes[DefaultIndexName]; ok {
			return []string{DefaultIndexName}
		}
		return nil
	}
	hints := spl2.ExtractQueryHints(prog)
	set := hints.SourceIndexSet()
	out := make([]string, 0, len(set))
	for name := range set {
		if _, ok := indexes[name]; ok {
			out = append(out, name)
		}
	}
	if len(out) == 0 && hints.IndexName != "" {
		out = append(out, hints.IndexName)
	}
	if len(out) == 0 {
		if _, ok := indexes[DefaultIndexName]; ok {
			out = append(out, DefaultIndexName)
		}
	}
	sort.Strings(out)
	return out
}

func levelName(level int) string {
	switch level {
	case 0:
		return "L0"
	case 1:
		return "L1"
	case 2:
		return "L2"
	case 3:
		return "L3"
	default:
		return "L?"
	}
}

func clampPercent(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}
