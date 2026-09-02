// Package metrics provides Prometheus instrumentation for the gateway.
package metrics

import (
	"net/http"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// M holds the gateway's Prometheus collectors.
type M struct {
	reg *prometheus.Registry

	// RequestsTotal counts proxied requests by provider/model/endpoint/status.
	RequestsTotal *prometheus.CounterVec
	// RequestDuration observes upstream latency by provider/endpoint.
	RequestDuration *prometheus.HistogramVec
	// TokensTotal counts tokens by provider and type.
	TokensTotal *prometheus.CounterVec
	// CooldownGauge tracks connections currently in quota cooldown.
	CooldownGauge *prometheus.GaugeVec
	// BuildInfo exports the binary version.
	BuildInfo *prometheus.GaugeVec
}

// New creates the collectors on a dedicated registry (plus Go runtime).
func New(version string) *M {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	m := &M{
		reg: reg,
		RequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "cyrene_requests_total",
			Help: "Proxied LLM requests by provider, model, endpoint and status.",
		}, []string{"provider", "model", "endpoint", "status"}),
		RequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "cyrene_request_duration_seconds",
			Help:    "Upstream request duration by provider and endpoint.",
			Buckets: []float64{.1, .25, .5, 1, 2.5, 5, 10, 30, 60, 120, 300},
		}, []string{"provider", "endpoint"}),
		TokensTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "cyrene_tokens_total",
			Help: "Token consumption by provider and type (prompt|completion|cached|reasoning).",
		}, []string{"provider", "type"}),
		CooldownGauge: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "cyrene_credentials_in_cooldown",
			Help: "Provider connections currently in quota cooldown.",
		}, []string{"provider"}),
		BuildInfo: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "cyrene_build_info",
			Help: "Gateway build version.",
		}, []string{"version"}),
	}
	reg.MustRegister(m.RequestsTotal, m.RequestDuration, m.TokensTotal, m.CooldownGauge, m.BuildInfo)
	m.BuildInfo.WithLabelValues(version).Set(1)
	return m
}

// Handler serves the /metrics endpoint.
func (m *M) Handler() http.Handler {
	return promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{})
}

// Usage is the token accounting snapshot passed to ObserveRequest.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	CachedTokens     int
	ReasoningTokens  int
}

// ObserveRequest records one completed proxied request.
func (m *M) ObserveRequest(provider, modelName, endpoint string, status int, seconds float64, u *Usage) {
	m.RequestsTotal.WithLabelValues(provider, modelName, endpoint, strconv.Itoa(status)).Inc()
	m.RequestDuration.WithLabelValues(provider, endpoint).Observe(seconds)
	if u == nil {
		return
	}
	if u.PromptTokens > 0 {
		m.TokensTotal.WithLabelValues(provider, "prompt").Add(float64(u.PromptTokens))
	}
	if u.CompletionTokens > 0 {
		m.TokensTotal.WithLabelValues(provider, "completion").Add(float64(u.CompletionTokens))
	}
	if u.CachedTokens > 0 {
		m.TokensTotal.WithLabelValues(provider, "cached").Add(float64(u.CachedTokens))
	}
	if u.ReasoningTokens > 0 {
		m.TokensTotal.WithLabelValues(provider, "reasoning").Add(float64(u.ReasoningTokens))
	}
}

// SetCooldown updates the cooldown gauge for a provider.
func (m *M) SetCooldown(provider string, count int) {
	m.CooldownGauge.WithLabelValues(provider).Set(float64(count))
}
