package kubegroup

import (
	"fmt"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/udhos/aws-emf/emf"
	"github.com/udhos/cloudwatchlog/cwlog"
)

type metrics struct {
	// prometheus
	peers  prometheus.Gauge
	events prometheus.Counter

	// dogstatsd
	dogstatsdClient DogstatsdClient
	sampleRate      float64
	tags            []string

	// aws cloudwatch emf
	emfMetric     *emf.Metric
	emfNamespace  string
	emfDimensions map[string]string
	cwlogClient   *cwlog.Log
}

var (
	metricEvents = emf.MetricDefinition{Name: "events", Unit: "Count"}
	metricPeers  = emf.MetricDefinition{Name: "peers", Unit: "Count"}
)

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

	if m.emfMetric != nil {

		m.emfMetric.Record(m.emfNamespace, metricEvents, m.emfDimensions, 1)
		m.emfMetric.Record(m.emfNamespace, metricPeers, m.emfDimensions, peers)

		if m.cwlogClient == nil {
			// send metrics to stdout
			m.emfMetric.Println()
		} else {
			// send metrics to cloudwatch logs
			events := m.emfMetric.CloudWatchLogEvents()
			if err := m.cwlogClient.PutLogEvents(events); err != nil {
				slog.Error(fmt.Sprintf("kubegroup metrics.update() error: %v", err))
			}
		}
	}
}

func newMetrics(namespace string, registerer prometheus.Registerer,
	client DogstatsdClient, dogstatsdExtraTags []string,
	emfMetric *emf.Metric, emfDimensions map[string]string,
	cwlogClient *cwlog.Log) *metrics {

	m := &metrics{
		// dogstatsd
		dogstatsdClient: client,
		tags:            dogstatsdExtraTags,
		sampleRate:      1,

		// aws cloudwatch emf
		emfMetric:     emfMetric,
		emfNamespace:  "kubegroup",
		emfDimensions: emfDimensions,
		cwlogClient:   cwlogClient,
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
