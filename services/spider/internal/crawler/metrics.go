package crawler

import (
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type SpiderMetrics struct {
	fetchTotal          *prometheus.CounterVec
	fetchDuration       *prometheus.HistogramVec
	enqueueTotal        *prometheus.CounterVec
	policyDecisionTotal *prometheus.CounterVec
	bypassTotal         *prometheus.CounterVec
	budgetDeniedTotal   *prometheus.CounterVec
	budgetRemaining     *prometheus.GaugeVec
}

func NewSpiderMetrics(registerer prometheus.Registerer) *SpiderMetrics {
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}

	m := &SpiderMetrics{
		fetchTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "moogle_spider_fetch_total",
				Help: "Total number of fetch attempts grouped by result and status class.",
			},
			[]string{"result", "status_class"},
		),
		fetchDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "moogle_spider_fetch_duration_seconds",
				Help:    "Fetch duration by result type.",
				Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 5, 10, 20},
			},
			[]string{"result"},
		),
		enqueueTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "moogle_spider_enqueue_total",
				Help: "Queue enqueue attempts grouped by result and reason.",
			},
			[]string{"result", "reason"},
		),
		policyDecisionTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "moogle_spider_policy_decision_total",
				Help: "Policy decisions grouped by action and reason.",
			},
			[]string{"decision", "reason"},
		),
		bypassTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "moogle_spider_bypass_total",
				Help: "Robots bypass decisions grouped by result and reason.",
			},
			[]string{"result", "reason"},
		),
		budgetDeniedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "moogle_spider_budget_denied_total",
				Help: "Budget gate deny events grouped by reason.",
			},
			[]string{"reason"},
		),
		budgetRemaining: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "moogle_spider_budget_remaining",
				Help: "Remaining crawl budget grouped by scope.",
			},
			[]string{"scope"},
		),
	}

	registerer.MustRegister(
		m.fetchTotal,
		m.fetchDuration,
		m.enqueueTotal,
		m.policyDecisionTotal,
		m.bypassTotal,
		m.budgetDeniedTotal,
		m.budgetRemaining,
	)

	return m
}

func (m *SpiderMetrics) ObserveFetch(result string, statusCode int, duration time.Duration) {
	if m == nil {
		return
	}

	normalizedResult := normalizeMetricLabel(result, "unknown")
	statusClass := classifyStatusClass(statusCode)
	m.fetchTotal.WithLabelValues(normalizedResult, statusClass).Inc()
	m.fetchDuration.WithLabelValues(normalizedResult).Observe(duration.Seconds())
}

func (m *SpiderMetrics) ObserveEnqueue(result string, reason string) {
	if m == nil {
		return
	}

	m.enqueueTotal.WithLabelValues(normalizeMetricLabel(result, "unknown"), normalizeMetricLabel(reason, "unknown")).Inc()
}

func (m *SpiderMetrics) ObservePolicyDecision(allowed bool, reason string) {
	if m == nil {
		return
	}

	decision := "deny"
	if allowed {
		decision = "allow"
	}

	m.policyDecisionTotal.WithLabelValues(decision, normalizeMetricLabel(reason, "unknown")).Inc()
}

func (m *SpiderMetrics) ObserveBypass(granted bool, reason string) {
	if m == nil {
		return
	}

	result := "denied"
	if granted {
		result = "granted"
	}

	m.bypassTotal.WithLabelValues(result, normalizeMetricLabel(reason, "unknown")).Inc()
}

func (m *SpiderMetrics) ObserveBudgetDenied(reason string) {
	if m == nil {
		return
	}

	m.budgetDeniedTotal.WithLabelValues(normalizeMetricLabel(reason, "unknown")).Inc()
}

func (m *SpiderMetrics) SetBudgetRemaining(scope string, value int) {
	if m == nil {
		return
	}

	if value < 0 {
		return
	}

	m.budgetRemaining.WithLabelValues(normalizeMetricLabel(scope, "unknown")).Set(float64(value))
}

func normalizeMetricLabel(value string, fallback string) string {
	normalized := strings.TrimSpace(strings.ToLower(value))
	if normalized == "" {
		return fallback
	}

	builder := strings.Builder{}
	for _, r := range normalized {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			builder.WriteRune(r)
			continue
		}
		builder.WriteByte('_')
	}

	cleaned := strings.Trim(builder.String(), "_")
	if cleaned == "" {
		return fallback
	}

	return cleaned
}

func classifyStatusClass(statusCode int) string {
	if statusCode < 100 || statusCode > 599 {
		return "none"
	}

	return fmt.Sprintf("%dxx", statusCode/100)
}
