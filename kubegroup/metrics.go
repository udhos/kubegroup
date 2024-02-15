package kubegroup

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type metrics struct {
	registerer prometheus.Registerer
	//gatherer   prometheus.Gatherer
	peers  prometheus.Gauge
	events prometheus.CounterVec
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
		[]string{"type", "action", "add", "error"},
	)

	return m
}

func newGauge(registerer prometheus.Registerer,
	opts prometheus.GaugeOpts) prometheus.Gauge {
	g := promauto.With(registerer).NewGauge(opts)
	return g
}

func newCounterVec(registerer prometheus.Registerer,
	opts prometheus.CounterOpts,
	labelValues []string) prometheus.CounterVec {
	return *promauto.With(registerer).NewCounterVec(opts, labelValues)
}
