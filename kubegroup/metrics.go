package kubegroup

import (
	"fmt"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type metrics struct {
	peers           prometheus.Gauge
	events          prometheus.Counter
	dogstatsdClient DogstatsdClient
	sampleRate      float64
	tags            []string
}

func (m *metrics) update(peers int) {
	if m.events != nil {
		m.events.Inc()
	}
	if m.peers != nil {
		m.peers.Set(float64(peers))
	}

	if m.dogstatsdClient != nil {
		if err := m.dogstatsdClient.Count("events", 1, m.tags, m.sampleRate); err != nil {
			slog.Error(fmt.Sprintf("exportCount: error: %v", err))
		}
		if err := m.dogstatsdClient.Gauge("peers", float64(peers), m.tags, m.sampleRate); err != nil {
			slog.Error(fmt.Sprintf("exportGauge: error: %v", err))
		}
	}
}

func newMetrics(namespace string, registerer prometheus.Registerer,
	client DogstatsdClient, dogstatsdExtraTags []string) *metrics {

	m := &metrics{
		dogstatsdClient: client,
		tags:            dogstatsdExtraTags,
		sampleRate:      1,
	}

	if registerer == nil {
		return m
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
