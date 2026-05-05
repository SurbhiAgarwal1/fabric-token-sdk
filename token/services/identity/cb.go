/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package identity

import (
	"sync"
	"sync/atomic"
	"time"
)

// CircuitBreaker implements a lightweight in-memory circuit breaker.
type CircuitBreaker struct {
	sync.RWMutex
	failureCount int64
	lastFailure  time.Time
	open         bool
	threshold    int64
	cooldown     time.Duration
}

// CircuitBreakerConfig contains the configuration for the CircuitBreaker.
type CircuitBreakerConfig struct {
	// Threshold is the number of failures after which the circuit opens.
	Threshold int64
	// Cooldown is the duration after which the circuit closes automatically.
	Cooldown time.Duration
}

// NewCircuitBreaker returns a new instance of CircuitBreaker with the provided configuration.
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	if config.Threshold <= 0 {
		config.Threshold = 5
	}
	if config.Cooldown <= 0 {
		config.Cooldown = 30 * time.Second
	}
	return &CircuitBreaker{
		threshold: config.Threshold,
		cooldown:  config.Cooldown,
	}
}

// Allow returns true if the circuit breaker allows the request to proceed.
func (cb *CircuitBreaker) Allow() bool {
	cb.RLock()
	if !cb.open {
		cb.RUnlock()
		return true
	}
	cb.RUnlock()

	cb.Lock()
	defer cb.Unlock()
	if time.Since(cb.lastFailure) > cb.cooldown {
		cb.open = false
		atomic.StoreInt64(&cb.failureCount, 0)
		return true
	}
	return false
}

// RecordFailure increments the failure count and opens the circuit if the threshold is reached.
func (cb *CircuitBreaker) RecordFailure() {
	count := atomic.AddInt64(&cb.failureCount, 1)
	if count >= cb.threshold {
		cb.Lock()
		cb.open = true
		cb.lastFailure = time.Now()
		cb.Unlock()
	}
}

// RecordSuccess resets the failure count and closes the circuit.
func (cb *CircuitBreaker) RecordSuccess() {
	atomic.StoreInt64(&cb.failureCount, 0)
	cb.Lock()
	cb.open = false
	cb.Unlock()
}
