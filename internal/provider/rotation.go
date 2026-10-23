package provider

import "sync"

// credentialRotator spreads traffic across accounts that share the same priority
// tier. Without it every request would resolve to the first eligible connection,
// which turns an "equal priority = load balancing" pool into a single hot account.
type credentialRotator struct {
	mu       sync.Mutex
	position map[string]uint64
}

var credentialRoundRobin = &credentialRotator{position: make(map[string]uint64)}

// next returns a round-robin slot in [0, n) for a tier key.
func (r *credentialRotator) next(key string, n int) int {
	if n <= 1 {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	slot := int(r.position[key] % uint64(n))
	r.position[key]++
	return slot
}

// reset drops all rotation state. Test helper only.
func (r *credentialRotator) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.position = make(map[string]uint64)
}
