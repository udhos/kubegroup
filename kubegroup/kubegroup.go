// Package kubegroup provides autodiscovery for groupcache.
package kubegroup

import (
	"context"
	"fmt"
	"log"
	"maps"
	"net"
	"os"

	"github.com/groupcache/groupcache-go/v3/transport/peer"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/udhos/aws-emf/emf"
	"github.com/udhos/cloudwatchlog/cwlog"
	"github.com/udhos/kubepodinformer/podinformer"
	"k8s.io/client-go/kubernetes"
)

// FindMyURL returns my URL for groupcache pool.
// groupcachePort example: ":5000".
// Sample resulting URL: "http://10.0.0.1:5000"
func FindMyURL(groupcachePort string) (string, error) {
	addr, errAddr := findMyAddr()
	if errAddr != nil {
		return "", errAddr
	}
	url := buildURL(addr, groupcachePort)
	return url, nil
}

// FindMyAddress returns my address.
func FindMyAddress() (string, error) {
	return findMyAddr()
}

func findMyAddr() (string, error) {
	host, errHost := os.Hostname()
	if errHost != nil {
		return "", errHost
	}
	addrs, errAddr := net.LookupHost(host)
	if errAddr != nil {
		return "", errAddr
	}
	if len(addrs) < 1 {
		return "", fmt.Errorf("findMyAddr: hostname '%s': no addr found", host)
	}
	addr := addrs[0]
	if len(addrs) > 1 {
		return addr, fmt.Errorf("findMyAddr: hostname '%s': found multiple addresses: %v", host, addrs)
	}
	return addr, nil
}

func findMyNamespace() (string, error) {
	buf, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	return string(buf), err
}

func buildURL(addr, groupcachePort string) string {
	return "http://" + addr + groupcachePort
}

// PeerGroup is an interface to plug in a target for delivering peering
// updates. *groupcache.HTTPPool, created with
// groupcache.NewHTTPPoolOpts(), implements this interface.
type PeerGroup interface {
	Set(peers ...string)
}

// PeerSet is an interface to plug in a target for delivering peering
// updates. *groupcache.daemon, created with
// groupcache.ListenAndServe(), implements this interface.
type PeerSet interface {
	SetPeers(ctx context.Context, peers []peer.Info) error
}

// Options specifies options for UpdatePeers.
type Options struct {
	// Pool is an interface to plug in a target for delivering peering
	// updates. *groupcache.HTTPPool, created with
	// groupcache.NewHTTPPoolOpts(), implements this interface.
	// Pool supports groupcache2.
	Pool PeerGroup

	// Peers is an interface to plug in a target for delivering peering
	// updates. *groupcache.Daemon, created with
	// groupcache.ListenAndServe(), implements this interface.
	// Peers supports groupcache3.
	Peers PeerSet

	// Client provides kubernetes client.
	Client *kubernetes.Clientset

	// GroupCachePort is the listening port used by groupcache peering http
	// server. For instance, ":5000".
	GroupCachePort string

	// LabelSelector is required. Example: "key1=value1,key2=value2"
	LabelSelector string

	// Debug enables non-error logging. Errors are always logged.
	Debug bool

	// Logf optionally sets custom logging.
	Logf func(format string, v ...any)

	// MetricsNamespace provides optional namespace for prometheus metrics.
	MetricsNamespace string

	// MetricsRegisterer is registerer for prometheus metrics.
	MetricsRegisterer prometheus.Registerer

	// DogstatsdClient optionally sends metrics to Datadog Dogstatsd.
	DogstatsdClient DogstatsdClient

	// DogstatsdExtraTags optionally adds tags do Dogstatsd metrics.
	DogstatsdExtraTags []string

	// DogstatsdTagHosnameKey defaults to "pod_name".
	DogstatsdTagHosnameKey string

	// DogstatsdDisableTagHostname prevents adding tag $DogstatsdTagHosnameKey:$hostname
	DogstatsdDisableTagHostname bool

	// EmfEnable optionally enables AWS CloudWatch EMF metrics.
	EmfEnable bool

	// EmfDimensions optionally adds dimensions to AWS CloudWatch EMF metrics.
	EmfDimensions map[string]string

	// EmfDimensionHosnameKey defaults to "pod_name".
	EmfDimensionHosnameKey string

	// EmfDisableDimensionHostname prevents adding tag $EmfDimensionHosnameKey:$hostname
	EmfDisableDimensionHostname bool

	// EmfCloudWatchLogsClient optionally sends AWS CloudWatch EMF metrics directly to CloudWatch logs.
	// If EmfCloudWatchLogsClient is left undefined, AWS CloudWatch EMF metrics are issued to standard output.
	// EmfCloudWatchLogsClient can be created by cwlog.New().
	EmfCloudWatchLogsClient *cwlog.Log

	// ForceNamespaceDefault is used only for testing.
	ForceNamespaceDefault bool
}

// DogstatsdClient is implemented by *statsd.Client.
// Simplified version of statsd.ClientInterface.
type DogstatsdClient interface {
	// Gauge measures the value of a metric at a particular time.
	Gauge(name string, value float64, tags []string, rate float64) error

	// Count tracks how many times something happened per second.
	Count(name string, value int64, tags []string, rate float64) error

	// Close the client connection.
	Close() error
}

// Group holds context for kubegroup.
type Group struct {
	options  Options
	informer *podinformer.PodInformer
	m        *metrics
	myAddr   string
}

func (g *Group) debugf(format string, v ...any) {
	if g.options.Debug {
		g.options.Logf("DEBUG kubegroup: "+format, v...)
	}
}

func (g *Group) errorf(format string, v ...any) {
	g.options.Logf("ERROR kubegroup: "+format, v...)
}

// Close terminates kubegroup goroutines to release resources.
func (g *Group) Close() {
	g.debugf("Close called to release resources")
	g.informer.Stop()
}

// UpdatePeers continuously updates groupcache peers.
func UpdatePeers(options Options) (*Group, error) {

	//
	// Required fields.
	//
	if options.Pool == nil && options.Peers == nil {
		panic("Pool and Peers are both nil")
	}
	if options.Client == nil {
		panic("Client is nil")
	}
	if options.GroupCachePort == "" {
		panic("GroupCachePort is empty")
	}
	if options.LabelSelector == "" {
		panic("LabelSelector is empty")
	}

	if !options.DogstatsdDisableTagHostname {
		if options.DogstatsdTagHosnameKey == "" {
			options.DogstatsdTagHosnameKey = "pod_name"
		}
		hostname, err := os.Hostname()
		if err != nil {
			return nil, err
		}
		options.DogstatsdExtraTags = append(options.DogstatsdExtraTags,
			fmt.Sprintf("%s:%s", options.DogstatsdTagHosnameKey, hostname))
	}

	if options.Logf == nil {
		options.Logf = log.Printf
	}

	var namespace string
	if options.ForceNamespaceDefault {
		namespace = "default"
	} else {
		ns, errNs := findMyNamespace()
		if errNs != nil {
			return nil, errNs
		}
		namespace = ns
	}

	myAddr, errAddr := findMyAddr()
	if errAddr != nil {
		return nil, errAddr
	}

	//
	// enable AWS CloudWatch EMF metrics
	//
	var emfMetric *emf.Metric
	var emfDimensions map[string]string
	if options.EmfEnable {
		emfMetric = emf.New(emf.Options{})
		emfDimensions = map[string]string{}
		maps.Copy(emfDimensions, options.EmfDimensions)

		if !options.EmfDisableDimensionHostname {
			if options.EmfDimensionHosnameKey == "" {
				options.EmfDimensionHosnameKey = "pod_name"
			}
			hostname, err := os.Hostname()
			if err != nil {
				return nil, err
			}
			emfDimensions[options.EmfDimensionHosnameKey] = hostname
		}
	}

	group := &Group{
		options: options,
		m: newMetrics(options.MetricsNamespace,
			options.MetricsRegisterer, options.DogstatsdClient,
			options.DogstatsdExtraTags, emfMetric, emfDimensions,
			options.EmfCloudWatchLogsClient),
		myAddr: myAddr,
	}

	optionsInformer := podinformer.Options{
		Client:        options.Client,
		Namespace:     namespace,
		LabelSelector: options.LabelSelector,
		OnUpdate:      group.onUpdate,
		DebugLog:      options.Debug,
	}

	group.informer = podinformer.New(optionsInformer)

	go func() {
		errInformer := group.informer.Run()
		group.errorf("informer exited, error: %v", errInformer)
	}()

	return group, nil
}

func (g *Group) onUpdate(pods []podinformer.Pod) {
	const me = "onUpdate"

	size := len(pods)
	g.debugf("%s: %d", me, size)

	if g.options.Peers != nil {

		//
		// groupcache3
		//

		peers := make([]peer.Info, 0, size)

		for i, p := range pods {
			hostPort := p.IP + g.options.GroupCachePort
			isSelf := g.myAddr == p.IP

			g.debugf("%s: %d/%d: namespace=%s pod=%s ip=%s ready=%t host_port=%s is_self=%t",
				me, i+1, size, p.Namespace, p.Name, p.IP, p.Ready, hostPort, isSelf)

			if p.Ready {
				peers = append(peers, peer.Info{
					Address: hostPort,
					IsSelf:  isSelf,
				})
			}
		}

		err := g.options.Peers.SetPeers(context.TODO(), peers)
		if err != nil {
			g.errorf("set peers: error: %v", err)
		}

	} else {

		//
		// groupcache2
		//

		peers := make([]string, 0, size)

		for i, p := range pods {
			g.debugf("%s: %d/%d: namespace=%s pod=%s ip=%s ready=%t",
				me, i+1, size, p.Namespace, p.Name, p.IP, p.Ready)

			if p.Ready {
				peers = append(peers, buildURL(p.IP, g.options.GroupCachePort))
			}
		}

		g.options.Pool.Set(peers...)
	}

	g.m.update(size)
}

// DogstatsdClientMock mocks the interface DogstatsdClient.
type DogstatsdClientMock struct{}

// Gauge measures the value of a metric at a particular time.
func (m *DogstatsdClientMock) Gauge(name string, value float64, tags []string, rate float64) error {
	log.Printf("DogstatsdClientMock.Gauge: name=%s value=%f tags=%v rate=%f",
		name, value, tags, rate)
	return nil
}

// Count tracks how many times something happened per second.
func (m *DogstatsdClientMock) Count(name string, value int64, tags []string, rate float64) error {
	log.Printf("DogstatsdClientMock.Count: name=%s value=%d tags=%v rate=%f",
		name, value, tags, rate)
	return nil
}

// Close the client connection.
func (m *DogstatsdClientMock) Close() error {
	return nil
}
