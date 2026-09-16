package system

import (
	"os"
	"runtime"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/db"
)

var processStartTime = time.Now()

// MemoryStats holds Go runtime memory allocation metrics.
type MemoryStats struct {
	AllocBytes      uint64 `json:"allocBytes"`
	TotalAllocBytes uint64 `json:"totalAllocBytes"`
	SysBytes        uint64 `json:"sysBytes"`
	HeapAllocBytes  uint64 `json:"heapAllocBytes"`
	HeapInuseBytes  uint64 `json:"heapInuseBytes"`
	NumGC           uint32 `json:"numGC"`
}

// SystemStats represents the complete runtime health, resources, and storage metrics.
type SystemStats struct {
	Version       string           `json:"version"`
	UptimeSeconds int64            `json:"uptimeSeconds"`
	StartTime     string           `json:"startTime"`
	PID           int              `json:"pid"`
	OS            string           `json:"os"`
	Arch          string           `json:"arch"`
	NumCPU        int              `json:"numCPU"`
	GoVersion     string           `json:"goVersion"`
	Goroutines    int              `json:"goroutines"`
	Memory        MemoryStats      `json:"memory"`
	Storage       *db.StorageStats `json:"storage,omitempty"`
	InDocker      bool             `json:"inDocker"`
}

// CollectStats gathers runtime metrics and SQLite storage statistics.
func CollectStats(version string, database *db.DB, dbPath string) (*SystemStats, error) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	stats := &SystemStats{
		Version:       version,
		UptimeSeconds: int64(time.Since(processStartTime).Seconds()),
		StartTime:     processStartTime.UTC().Format(time.RFC3339),
		PID:           os.Getpid(),
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		NumCPU:        runtime.NumCPU(),
		GoVersion:     runtime.Version(),
		Goroutines:    runtime.NumGoroutine(),
		InDocker:      IsRunningInDocker(),
		Memory: MemoryStats{
			AllocBytes:      m.Alloc,
			TotalAllocBytes: m.TotalAlloc,
			SysBytes:        m.Sys,
			HeapAllocBytes:  m.HeapAlloc,
			HeapInuseBytes:  m.HeapInuse,
			NumGC:           m.NumGC,
		},
	}

	if database != nil && dbPath != "" {
		storage, err := database.GetStorageStats(dbPath)
		if err == nil {
			stats.Storage = storage
		}
	}

	return stats, nil
}
