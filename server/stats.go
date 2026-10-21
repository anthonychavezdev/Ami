package main

import (
	"context"
	"net/http"
	"os"
	"runtime"
	"runtime/metrics"
	"sync"
	"time"
)

// Stats is what GET /api/stats answers: the resources behind this server, so
// people can see what their pictures and videos add up to. Host figures the
// platform cannot provide are 0 (see stats_other.go).
type Stats struct {
	Storage StorageStats `json:"storage"`
	CPU     CPUStats     `json:"cpu"`
	Memory  MemoryStats  `json:"memory"`
}

type StorageStats struct {
	DiskTotal   int64 `json:"disk_total"` // the volume holding the uploads
	DiskFree    int64 `json:"disk_free"`
	Uploads     int64 `json:"uploads"`      // everyone's pictures and videos
	Database    int64 `json:"database"`     // the tables and indexes, as MariaDB keeps them
	Yours       int64 `json:"yours"`        // what you have uploaded so far
	YourLimit   int64 `json:"your_limit"`   // the most you may upload in total (-user-storage-gb); 0 is no limit
	UploadLimit int64 `json:"upload_limit"` // the most one upload may be
}

type CPUStats struct {
	Cores         int     `json:"cores"`
	Load1         float64 `json:"load_1"` // runnable processes, averaged over the last 1, 5 and 15 minutes
	Load5         float64 `json:"load_5"`
	Load15        float64 `json:"load_15"`
	ServerPercent float64 `json:"server_percent"` // this process, as a share of one core, over the last few seconds
}

type MemoryStats struct {
	Total     int64 `json:"total"`
	Available int64 `json:"available"` // free, plus what the OS would reclaim from caches
	Server    int64 `json:"server"`    // this process: the memory the Go runtime holds
}

// host is what the platform files report; see hostStats.
type host struct {
	memTotal, memAvailable int64
	load                   [3]float64
}

func (s *server) stats(w http.ResponseWriter, r *http.Request, me *User) error {
	yours, err := s.userUploads(me.ID)
	if err != nil {
		return err
	}
	h := hostStats()
	diskTotal, diskFree := diskStats(s.mediaDir)
	return ok(w, Stats{
		Storage: StorageStats{
			DiskTotal: diskTotal, DiskFree: diskFree,
			Uploads:     dirSize(s.mediaDir),
			Database:    databaseSize(s.db),
			Yours:       yours,
			YourLimit:   s.storageCap,
			UploadLimit: maxUploadBytes,
		},
		CPU:    CPUStats{Cores: runtime.NumCPU(), Load1: h.load[0], Load5: h.load[1], Load15: h.load[2], ServerPercent: s.cpu.get()},
		Memory: MemoryStats{Total: h.memTotal, Available: h.memAvailable, Server: processMemory()},
	})
}

// dirSize is the bytes of the files in dir (uploads are one flat directory).
func dirSize(dir string) int64 {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	var total int64
	for _, e := range entries {
		if info, err := e.Info(); err == nil && info.Mode().IsRegular() {
			total += info.Size()
		}
	}
	return total
}

// processMemory is the memory the Go runtime holds for this process, close to
// what the OS shows as resident.
func processMemory() int64 {
	sample := []metrics.Sample{{Name: "/memory/classes/total:bytes"}}
	metrics.Read(sample)
	if sample[0].Value.Kind() != metrics.KindUint64 {
		return 0
	}
	return int64(sample[0].Value.Uint64())
}

// cpuMeter keeps this process's recent CPU use, sampled by meterCPU.
type cpuMeter struct {
	mu      sync.Mutex
	percent float64
}

func (m *cpuMeter) get() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.percent
}

func (m *cpuMeter) set(percent float64) {
	m.mu.Lock()
	m.percent = percent
	m.mu.Unlock()
}

// meterCPU samples the process's CPU time every few seconds: the share of one
// core it used between two samples is the stats' server_percent.
func (s *server) meterCPU(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	last, lastAt := processCPUTime(), time.Now()
	for {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
		now, at := processCPUTime(), time.Now()
		if wall := at.Sub(lastAt); wall > 0 {
			s.cpu.set(100 * float64(now-last) / float64(wall))
		}
		last, lastAt = now, at
	}
}
