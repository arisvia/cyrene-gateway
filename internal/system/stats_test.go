package system

import (
	"runtime"
	"testing"
)

func TestCollectStats(t *testing.T) {
	stats, err := CollectStats("1.1.0", nil, "")
	if err != nil {
		t.Fatalf("CollectStats error: %v", err)
	}

	if stats.Version != "1.1.0" {
		t.Errorf("expected version 1.1.0, got %s", stats.Version)
	}
	if stats.PID <= 0 {
		t.Errorf("expected PID > 0, got %d", stats.PID)
	}
	if stats.OS != runtime.GOOS {
		t.Errorf("expected OS %s, got %s", runtime.GOOS, stats.OS)
	}
	if stats.Arch != runtime.GOARCH {
		t.Errorf("expected Arch %s, got %s", runtime.GOARCH, stats.Arch)
	}
	if stats.NumCPU <= 0 {
		t.Errorf("expected NumCPU > 0, got %d", stats.NumCPU)
	}
	if stats.Goroutines <= 0 {
		t.Errorf("expected Goroutines > 0, got %d", stats.Goroutines)
	}
	if stats.Memory.SysBytes == 0 {
		t.Errorf("expected SysBytes > 0")
	}
}
