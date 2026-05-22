package rest

import (
	"net/http"
	"time"

	"github.com/lynxbase/lynxdb/internal/buildinfo"
	"github.com/lynxbase/lynxdb/pkg/storage"
)

type topSnapshotResponse struct {
	Server  topServerSnapshot `json:"server"`
	Events  interface{}       `json:"events"`
	Storage interface{}       `json:"storage"`
	Indexes interface{}       `json:"indexes"`
	Queries interface{}       `json:"queries"`
	Memory  interface{}       `json:"memory"`
	Cluster interface{}       `json:"cluster"`
}

type topServerSnapshot struct {
	Version       string `json:"version"`
	Health        string `json:"health"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	DataDir       string `json:"data_dir"`
	Profile       string `json:"profile"`
}

func (s *Server) handleTopSnapshot(w http.ResponseWriter, r *http.Request) {
	snap := s.engine.TopSnapshot()
	stats := s.engine.Stats()
	snap.Events.IngestRateEPS = s.topIngestRate(stats.TotalEvents, time.Now())

	respondData(w, http.StatusOK, topSnapshotResponse{
		Server: topServerSnapshot{
			Version:       buildinfo.Version,
			Health:        snap.Cluster.Status,
			UptimeSeconds: stats.UptimeSeconds,
			DataDir:       snap.Cluster.DataDir,
			Profile:       storage.ResolveProfile(s.runtimeCfg.DataDir, s.runtimeCfg.Storage.S3Bucket).String(),
		},
		Events:  snap.Events,
		Storage: snap.Storage,
		Indexes: snap.Indexes,
		Queries: snap.Queries,
		Memory:  snap.Memory,
		Cluster: snap.Cluster,
	})
}

func (s *Server) topIngestRate(total int64, now time.Time) float64 {
	s.topMu.Lock()
	defer s.topMu.Unlock()

	if !s.topLastAt.IsZero() {
		elapsed := now.Sub(s.topLastAt).Seconds()
		delta := total - s.topLastTotal
		if elapsed > 0 && delta >= 0 {
			s.topRateEPS = float64(delta) / elapsed
		}
	}
	s.topLastTotal = total
	s.topLastAt = now

	return s.topRateEPS
}
