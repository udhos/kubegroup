package kubegroup

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type metrics struct {
	//registerer prometheus.Registerer
	//gatherer   prometheus.Gatherer
	peers  prometheus.Gauge
	events prometheus.Counter
}

func newMetrics(namespace string, registerer prometheus.Registerer) *metrics {
	m := &metrics{
		//registerer: registerer,
		//gatherer:   options.MetricsGatherer,
	}

	const subsystem = "kubegroup"

	m.peers = newGauge(
		registerer,
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "peers",
			Help:      "Number of peer PODs discovered.",
		},
	)

	m.events = newCounter(
		registerer,
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "events",
			Help:      "Number of events received.",
		},
	)

	return m
}

func newGauge(registerer prometheus.Registerer,
	opts prometheus.GaugeOpts) prometheus.Gauge {
	return promauto.With(registerer).NewGauge(opts)
}

func newCounter(registerer prometheus.Registerer,
	opts prometheus.CounterOpts) prometheus.Counter {
	return promauto.With(registerer).NewCounter(opts)
}
