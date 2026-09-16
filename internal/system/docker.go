package system

import "os"

// IsRunningInDocker checks if the current process is running inside a Docker container.
func IsRunningInDocker() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	if os.Getenv("CYRENE_IN_DOCKER") == "1" || os.Getenv("CYRENE_IN_DOCKER") == "true" {
		return true
	}
	return false
}
