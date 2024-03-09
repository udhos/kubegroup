// Package kubegroup provides autodiscovery for groupcache.
package kubegroup

import (
	"fmt"
	"log"
	"net"
	"os"

	"github.com/prometheus/client_golang/prometheus"
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

// Options specifies options for UpdatePeers.
type Options struct {
	// Pool is an interface to plug in a target for delivering peering
	// updates. *groupcache.HTTPPool, created with
	// groupcache.NewHTTPPoolOpts(), implements this interface.
	Pool PeerGroup

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

	// MetricsRegisterer is required registerer for prometheus metrics.
	MetricsRegisterer prometheus.Registerer

	// MetricsRegisterer is required gatherer for prometheus metrics.
	MetricsGatherer prometheus.Gatherer

	// ForceNamespaceDefault is used only for testing.
	ForceNamespaceDefault bool
}

// Group holds context for kubegroup.
type Group struct {
	options  Options
	informer *podinformer.PodInformer
	m        *metrics
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
	if options.Pool == nil {
		panic("Pool is nil")
	}
	if options.Client == nil {
		panic("Client is nil")
	}
	if options.GroupCachePort == "" {
		panic("GroupCachePort is empty")
	}
	if options.MetricsRegisterer == nil {
		panic("MetricsRegisterer is nil")
	}
	if options.MetricsGatherer == nil {
		panic("MetricsGatherer is nil")
	}
	if options.LabelSelector == "" {
		panic("LabelSelector is empty")
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

	group := &Group{
		options: options,
		m:       newMetrics(options.MetricsNamespace, options.MetricsRegisterer),
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

	peers := make([]string, 0, size)

	for i, p := range pods {
		g.debugf("%s: %d/%d: namespace=%s pod=%s ip=%s ready=%t",
			me, i+1, size, p.Namespace, p.Name, p.IP, p.Ready)

		if p.Ready {
			peers = append(peers, buildURL(p.IP, g.options.GroupCachePort))
		}
	}

	g.options.Pool.Set(peers...)

	g.m.events.Inc()
	g.m.peers.Set(float64(size))
}
