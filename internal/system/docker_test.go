package system

import (
	"os"
	"testing"
)

func TestIsRunningInDocker(t *testing.T) {
	os.Unsetenv("CYRENE_IN_DOCKER")
	// Outside container without /.dockerenv
	if _, err := os.Stat("/.dockerenv"); os.IsNotExist(err) {
		if IsRunningInDocker() {
			t.Errorf("expected false when not in docker")
		}
	}

	os.Setenv("CYRENE_IN_DOCKER", "1")
	defer os.Unsetenv("CYRENE_IN_DOCKER")
	if !IsRunningInDocker() {
		t.Errorf("expected true when CYRENE_IN_DOCKER=1")
	}
}
