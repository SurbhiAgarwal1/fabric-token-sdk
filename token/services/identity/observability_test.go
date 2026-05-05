/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package identity_test

import (
	"context"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-token-sdk/token/driver"
	"github.com/hyperledger-labs/fabric-token-sdk/token/services/identity"
	"github.com/hyperledger-labs/fabric-token-sdk/token/services/identity/mock"
	"github.com/hyperledger-labs/fabric-token-sdk/token/services/logging"
	"github.com/stretchr/testify/assert"
)

type MockCounter struct {
	value float64
}

func (c *MockCounter) Add(delta float64) {
	c.value += delta
}

func (c *MockCounter) With(labelValues ...string) metrics.Counter {
	return c
}

type MockGauge struct {
	value float64
}

func (g *MockGauge) Add(delta float64) {
	g.value += delta
}

func (g *MockGauge) Set(value float64) {
	g.value = value
}

func (g *MockGauge) With(labelValues ...string) metrics.Gauge {
	return g
}

type MockHistogram struct {
	observations []float64
}

func (h *MockHistogram) Observe(value float64) {
	h.observations = append(h.observations, value)
}

func (h *MockHistogram) With(labelValues ...string) metrics.Histogram {
	return h
}

type MockMetricsProvider struct {
	Counters   map[string]*MockCounter
	Gauges     map[string]*MockGauge
	Histograms map[string]*MockHistogram
}

func NewMockMetricsProvider() *MockMetricsProvider {
	return &MockMetricsProvider{
		Counters:   make(map[string]*MockCounter),
		Gauges:     make(map[string]*MockGauge),
		Histograms: make(map[string]*MockHistogram),
	}
}

func (m *MockMetricsProvider) NewCounter(opts metrics.CounterOpts) metrics.Counter {
	c := &MockCounter{}
	m.Counters[opts.Name] = c
	return c
}

func (m *MockMetricsProvider) NewGauge(opts metrics.GaugeOpts) metrics.Gauge {
	g := &MockGauge{}
	m.Gauges[opts.Name] = g
	return g
}

func (m *MockMetricsProvider) NewHistogram(opts metrics.HistogramOpts) metrics.Histogram {
	h := &MockHistogram{}
	m.Histograms[opts.Name] = h
	return h
}

func TestCircuitBreaker(t *testing.T) {
	cb := identity.NewCircuitBreaker(identity.CircuitBreakerConfig{
		Threshold: 2,
		Cooldown:  100 * time.Millisecond,
	})

	// Initial state: closed
	assert.True(t, cb.Allow())

	// Record one failure
	cb.RecordFailure()
	assert.True(t, cb.Allow())

	// Record second failure -> opens
	cb.RecordFailure()
	assert.False(t, cb.Allow())

	// Wait for cooldown
	time.Sleep(150 * time.Millisecond)
	assert.True(t, cb.Allow())

	// Record success -> resets
	cb.RecordFailure()
	cb.RecordSuccess()
	cb.RecordFailure()
	assert.True(t, cb.Allow())
}

func TestProviderObservability(t *testing.T) {
	storage := &mock.Storage{}
	metricsProvider := NewMockMetricsProvider()
	p := identity.NewProvider(logging.MustGetLogger(), storage, nil, nil, nil,
		identity.WithMetrics(metricsProvider),
		identity.WithCircuitBreaker(identity.CircuitBreakerConfig{Threshold: 1, Cooldown: time.Hour}),
	)

	// Configure storage to return an error
	storage.StoreIdentityDataReturns(errors.New("storage error"))

	ctx := context.Background()
	data := &driver.RecipientData{Identity: driver.Identity("id")}

	// First call fails
	err := p.RegisterRecipientData(ctx, data)
	assert.Error(t, err)
	assert.Equal(t, "storage error", err.Error())

	// Verify metrics
	assert.Equal(t, 1.0, metricsProvider.Counters["identity_requests_total"].value)
	assert.Equal(t, 1.0, metricsProvider.Counters["identity_errors_total"].value)
	assert.Equal(t, 0.0, metricsProvider.Gauges["identity_inflight_requests"].value)
	assert.Len(t, metricsProvider.Histograms["identity_request_latency_ms"].observations, 1)

	// Circuit should be open now
	err = p.RegisterRecipientData(ctx, data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "back-pressure")

	// Verify metrics for the second call
	assert.Equal(t, 2.0, metricsProvider.Counters["identity_requests_total"].value)
	assert.Equal(t, 2.0, metricsProvider.Counters["identity_errors_total"].value)
}
