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

type CircuitBreaker struct {
	sync.RWMutex
	failureCount int64
	lastFailure  time.Time
	open         bool
	threshold    int64
	cooldown     time.Duration
}

type CircuitBreakerConfig struct {
	Threshold int64
	Cooldown  time.Duration
}

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

func (cb *CircuitBreaker) RecordFailure() {
	count := atomic.AddInt64(&cb.failureCount, 1)
	if count >= cb.threshold {
		cb.Lock()
		cb.open = true
		cb.lastFailure = time.Now()
		cb.Unlock()
	}
}

func (cb *CircuitBreaker) RecordSuccess() {
	atomic.StoreInt64(&cb.failureCount, 0)
	cb.Lock()
	cb.open = false
	cb.Unlock()
}
