package provider

import (
	"sync"
	"time"
)

// ComboStrategy defines the rotation strategy for combo models.
type ComboStrategy string

const (
	StrategyFallback   ComboStrategy = "fallback"
	StrategyRoundRobin ComboStrategy = "round-robin"
)

// rotationState tracks round-robin position per combo.
type rotationState struct {
	index               int
	consecutiveUseCount int
}

// ComboManager handles combo model rotation and fallback logic.
type ComboManager struct {
	mu       sync.Mutex
	rotation map[string]*rotationState
}

// NewComboManager creates a new ComboManager.
func NewComboManager() *ComboManager {
	return &ComboManager{
		rotation: make(map[string]*rotationState),
	}
}

// GetRotatedModels returns the model list rotated according to strategy.
// For "fallback", models are returned as-is.
// For "round-robin", models are rotated based on sticky limit.
func (cm *ComboManager) GetRotatedModels(models []string, comboName string, strategy ComboStrategy, stickyLimit int) []string {
	if len(models) <= 1 || strategy != StrategyRoundRobin {
		return models
	}

	if stickyLimit <= 0 {
		stickyLimit = 1
	}

	rotationKey := comboName
	if rotationKey == "" {
		rotationKey = "__default__"
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	state, ok := cm.rotation[rotationKey]
	if !ok {
		state = &rotationState{index: 0, consecutiveUseCount: 0}
	}

	currentIndex := state.index % len(models)
	rotated := rotateFromIndex(models, currentIndex)

	nextUseCount := state.consecutiveUseCount + 1
	if nextUseCount >= stickyLimit {
		cm.rotation[rotationKey] = &rotationState{
			index:               (currentIndex + 1) % len(models),
			consecutiveUseCount: 0,
		}
	} else {
		cm.rotation[rotationKey] = &rotationState{
			index:               currentIndex,
			consecutiveUseCount: nextUseCount,
		}
	}

	return rotated
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
	Success    bool
	Status     int
	ErrorText  string
	RetryAfter string
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
