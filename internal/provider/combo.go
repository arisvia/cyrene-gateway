package provider

import (
	"sort"
	"sync"
	"time"
)

// ComboStrategy defines the rotation strategy for combo models.
type ComboStrategy string

const (
	StrategyFallback     ComboStrategy = "fallback"
	StrategyRoundRobin   ComboStrategy = "round-robin"
	StrategyCostFirst    ComboStrategy = "cost-first"
	StrategyLatencyFirst ComboStrategy = "latency-first"
)

// rotationState tracks round-robin position per combo.
type rotationState struct {
	index               int
	consecutiveUseCount int
}

// modelMetrics tracks running EWMA latency and failures for latency-first routing.
type modelMetrics struct {
	ewmaLatencyMs float64
	failures      int
}

// ComboManager handles combo model rotation and fallback logic.
type ComboManager struct {
	rotation map[string]*rotationState
	metrics  map[string]*modelMetrics
	mu       sync.Mutex
}

// NewComboManager creates a new ComboManager.
func NewComboManager() *ComboManager {
	return &ComboManager{
		rotation: make(map[string]*rotationState),
		metrics:  make(map[string]*modelMetrics),
	}
}

// RecordModelLatency records an observed round-trip duration for a model.
func (cm *ComboManager) RecordModelLatency(modelStr string, latencyMs int64) {
	if modelStr == "" || latencyMs <= 0 {
		return
	}
	cm.mu.Lock()
	defer cm.mu.Unlock()

	m, ok := cm.metrics[modelStr]
	if !ok {
		cm.metrics[modelStr] = &modelMetrics{ewmaLatencyMs: float64(latencyMs)}
		return
	}
	// EWMA: 70% historical, 30% recent observation
	m.ewmaLatencyMs = 0.7*m.ewmaLatencyMs + 0.3*float64(latencyMs)
	if m.failures > 0 {
		m.failures--
	}
}

// RecordModelFailure notes an error for a model, applying a temporary latency penalty.
func (cm *ComboManager) RecordModelFailure(modelStr string) {
	if modelStr == "" {
		return
	}
	cm.mu.Lock()
	defer cm.mu.Unlock()

	m, ok := cm.metrics[modelStr]
	if !ok {
		cm.metrics[modelStr] = &modelMetrics{ewmaLatencyMs: 3000.0, failures: 1}
		return
	}
	m.failures++
	m.ewmaLatencyMs += 2000.0 // +2s penalty per failure
}

// GetModelLatency returns the effective latency score for a model.
func (cm *ComboManager) GetModelLatency(modelStr string) float64 {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if m, ok := cm.metrics[modelStr]; ok {
		return m.ewmaLatencyMs
	}
	return 500.0 // neutral optimistic default for untried models
}

// GetRotatedModels returns the model list rotated according to strategy.
// For "fallback", models are returned as-is.
// For "round-robin", models are rotated based on sticky limit.
// GetRotatedModels returns the model list ordered according to the chosen strategy:
// - "fallback": preserved in original order.
// - "round-robin": rotated based on sticky limit.
// - "cost-first": sorted by costFunc ascending (cheapest first).
// - "latency-first": sorted by observed latency ascending (fastest first).
func (cm *ComboManager) GetRotatedModels(models []string, comboName string, strategy ComboStrategy, stickyLimit int, costFn ...func(string) float64) []string {
	if len(models) <= 1 {
		return models
	}

	switch strategy {
	case StrategyRoundRobin:
		if stickyLimit <= 0 {
			stickyLimit = 1
		}
		rotationKey := comboName
		if rotationKey == "" {
			rotationKey = "__default__"
		}

		cm.mu.Lock()
		state, ok := cm.rotation[rotationKey]
		if !ok {
			state = &rotationState{}
			cm.rotation[rotationKey] = state
		}
		currentIndex := state.index % len(models)
		state.index = currentIndex
		state.consecutiveUseCount++
		if state.consecutiveUseCount >= stickyLimit {
			state.index = (currentIndex + 1) % len(models)
			state.consecutiveUseCount = 0
		}
		cm.mu.Unlock()
		return rotateFromIndex(models, currentIndex)

	case StrategyCostFirst:
		if len(costFn) == 0 || costFn[0] == nil {
			return models
		}
		fn := costFn[0]
		sorted := make([]string, len(models))
		copy(sorted, models)
		sort.SliceStable(sorted, func(i, j int) bool {
			return fn(sorted[i]) < fn(sorted[j])
		})
		return sorted

	case StrategyLatencyFirst:
		sorted := make([]string, len(models))
		copy(sorted, models)
		sort.SliceStable(sorted, func(i, j int) bool {
			return cm.GetModelLatency(sorted[i]) < cm.GetModelLatency(sorted[j])
		})
		return sorted

	default:
		return models
	}
}

// ResetRotation clears rotation state for a specific combo or all combos.
func (cm *ComboManager) ResetRotation(comboName string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if comboName == "" {
		cm.rotation = make(map[string]*rotationState)
	} else {
		delete(cm.rotation, comboName)
	}
}

// rotateFromIndex rotates a slice so that element at index becomes first.
func rotateFromIndex(models []string, index int) []string {
	if index <= 0 || index >= len(models) {
		return models
	}
	rotated := make([]string, len(models))
	copy(rotated, models[index:])
	copy(rotated[len(models)-index:], models[:index])
	return rotated
}

// ComboResult represents the outcome of a combo attempt.
type ComboResult struct {
	ErrorText  string
	RetryAfter string
	Status     int
	Success    bool
}

// HandleComboFallback iterates through models, calling tryFunc for each.
// Returns on first success. On failure, applies cooldown logic and tries next.
func HandleComboFallback(models []string, tryFunc func(modelStr string) ComboResult, logFunc func(format string, args ...any)) ComboResult {
	var lastError string
	var lastStatus int
	var earliestRetryAfter string

	for i, modelStr := range models {
		if logFunc != nil {
			logFunc("combo: trying model %d/%d: %s", i+1, len(models), modelStr)
		}

		result := tryFunc(modelStr)

		if result.Success {
			if logFunc != nil {
				logFunc("combo: model %s succeeded", modelStr)
			}
			return result
		}

		// Check if we should fallback
		fallbackResult := CheckFallbackError(result.Status, result.ErrorText, 0)
		if !fallbackResult.ShouldFallback {
			if logFunc != nil {
				logFunc("combo: model %s failed (no fallback), status=%d", modelStr, result.Status)
			}
			return result
		}

		// Track earliest retryAfter
		if result.RetryAfter != "" {
			if earliestRetryAfter == "" || isEarlierRetryAfter(result.RetryAfter, earliestRetryAfter) {
				earliestRetryAfter = result.RetryAfter
			}
		}

		// For transient errors with short cooldown, wait before trying next
		if fallbackResult.Cooldown > 0 && fallbackResult.Cooldown <= 5*time.Second &&
			(result.Status == 502 || result.Status == 503 || result.Status == 504) {
			if logFunc != nil {
				logFunc("combo: model %s transient %d, waiting %v before next", modelStr, result.Status, fallbackResult.Cooldown)
			}
			time.Sleep(fallbackResult.Cooldown)
		}

		lastError = result.ErrorText
		if lastStatus == 0 {
			lastStatus = result.Status
		}

		if logFunc != nil {
			logFunc("combo: model %s failed, trying next (status=%d)", modelStr, result.Status)
		}
	}

	// All models failed
	status := lastStatus
	if status == 0 {
		status = 503
	}
	msg := lastError
	if msg == "" {
		msg = "all combo models unavailable"
	}

	return ComboResult{
		Success:    false,
		Status:     status,
		ErrorText:  msg,
		RetryAfter: earliestRetryAfter,
	}
}

func isEarlierRetryAfter(a, b string) bool {
	ta, errA := time.Parse(time.RFC3339, a)
	tb, errB := time.Parse(time.RFC3339, b)
	if errA != nil || errB != nil {
		return false
	}
	return ta.Before(tb)
}
