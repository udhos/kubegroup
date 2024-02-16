package kubegroup

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type metrics struct {
	registerer prometheus.Registerer
	//gatherer   prometheus.Gatherer
	peers   prometheus.Gauge
	events  *prometheus.CounterVec
	changes *prometheus.CounterVec
}

func newMetrics(options Options) *metrics {
	m := &metrics{
		registerer: options.MetricsRegisterer,
		//gatherer:   options.MetricsGatherer,
	}

	const subsystem = "kubegroup"

	m.peers = newGauge(
		m.registerer,
		prometheus.GaugeOpts{
			Namespace: options.MetricsNamespace,
			Subsystem: subsystem,
			Name:      "peers",
			Help:      "Number of peer PODs discovered.",
		},
	)

	m.events = newCounterVec(
		m.registerer,
		prometheus.CounterOpts{
			Namespace: options.MetricsNamespace,
			Subsystem: subsystem,
			Name:      "events",
			Help:      "Number of events received.",
		},
		[]string{"action", "change", "event_type", "event_error"},
	)

	m.events = newCounterVec(
		m.registerer,
		prometheus.CounterOpts{
			Namespace: options.MetricsNamespace,
			Subsystem: subsystem,
			Name:      "changes",
			Help:      "Number of peer changes.",
		},
		[]string{"action", "change"},
	)

	return m
}

func actionChangeString(action, change bool) (string, string) {
	var actionStr, changeStr string

	if action {
		actionStr = "accepted"
	} else {
		actionStr = "ignored"
	}

	if change {
		changeStr = "add"
	} else {
		changeStr = "remove"
	}

	return actionStr, changeStr
}

func (m *metrics) recordEvents(eventType, eventError string, action, change bool) {
	actionStr, changeStr := actionChangeString(action, change)
	m.events.WithLabelValues(actionStr, changeStr, eventType, eventError).Inc()
}

func (m *metrics) recordChanges(action, change bool) {
	actionStr, changeStr := actionChangeString(action, change)
	m.events.WithLabelValues(actionStr, changeStr).Inc()
}

func newGauge(registerer prometheus.Registerer,
	opts prometheus.GaugeOpts) prometheus.Gauge {
	return promauto.With(registerer).NewGauge(opts)
}

func newCounterVec(registerer prometheus.Registerer,
	opts prometheus.CounterOpts,
	labelValues []string) *prometheus.CounterVec {
	return promauto.With(registerer).NewCounterVec(opts, labelValues)
}
