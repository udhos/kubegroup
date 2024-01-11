// Package kubegroup provides autodiscovery for groupcache.
package kubegroup

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	// "github.com/golang/groupcache"
	"golang.org/x/exp/maps"
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

func buildURL(addr, groupcachePort string) string {
	return "http://" + addr + groupcachePort
}

// PeerGroup is an interface to plug in groupcache peering updates.
// *groupcache.HTTPPool, created with groupcache.NewHTTPPoolOpts(), implements this interface.
type PeerGroup interface {
	Set(peers ...string)
}

// Options specifies options for UpdatePeers.
type Options struct {
	// PeerGroup is an interface to plug in groupcache peering updates.
	// *groupcache.HTTPPool, created with groupcache.NewHTTPPoolOpts(), implements this interface.
	Pool PeerGroup

	// GroupCachePort is the listening port used by groupcache peering http server. For instance, ":5000".
	GroupCachePort string

	// PodLabelKey is label key to match peer PODs, if unspecified defaults to "app".
	// Example: If PODs are labeled as app=my-app-name, you could either set PodLabelKey
	// to "app" or leave it empty (since "app" is the default value).
	PodLabelKey string

	// PodLabelValue is label value to match peer PODs, if unspecified defaults to
	// current POD label value for PodLabelKey.
	// Example: If PODs are labeled as app=my-app-name, you could either set PodLabelValue
	// to "my-app-name" or leave it empty (since by default PodLabelValue takes its value
	// from the PodLabelKey key).
	PodLabelValue string

	// KubeEngine sets a plugable kube client. If unspecified, defaults to DefaultEngine.
	// You can plug in a mocked client like KubeBogus for testing.
	Engine KubeEngine

	// Cooldown sets interval between retries. If unspecified defaults to 5 seconds.
	Cooldown time.Duration

	// Debug enables non-error logging. Errors are always logged.
	Debug bool

	// Debugf optionally sets custom logging stream for debug messages.
	Debugf func(format string, v ...any)

	// Errorf optionally sets custom logging stream for error messages.
	Errorf func(format string, v ...any)

	// Fatalf optionally sets custom logging stream for fatal messages. It must terminate/abort the program.
	Fatalf func(format string, v ...any)
}

func debugf(format string, v ...any) {
	log.Printf("DEBUG: "+format, v...)
}

func errorf(format string, v ...any) {
	log.Printf("ERROR: "+format, v...)
}

func fatalf(format string, v ...any) {
	log.Fatalf("FATAL: "+format, v...)
}

// DefaultEngine defines default kube client engine.
var DefaultEngine = NewKubeReal()

func defaultOptions(options Options) Options {
	if options.Cooldown == 0 {
		options.Cooldown = 5 * time.Second
	}
	if options.Engine == nil {
		options.Engine = DefaultEngine
	}
	if options.Debugf == nil {
		options.Debugf = func(format string, v ...any) {
			if options.Debug {
				debugf(format, v...)
			}
		}
	}
	if options.Errorf == nil {
		options.Errorf = errorf
	}
	if options.Fatalf == nil {
		options.Fatalf = fatalf
	}
	return options
}

// Group holds context for kubegroup.
type Group struct {
	options Options
	client  kubeClient
	peers   map[string]bool
	done    chan struct{}
	closed  bool
	mutex   sync.Mutex
}

// Close terminates kubegroup goroutines to release resources.
func (g *Group) Close() {
	g.mutex.Lock()
	if !g.closed {
		close(g.done)
		g.closed = true
	}
	g.mutex.Unlock()
}

// UpdatePeers continuously updates groupcache peers.
// groupcachePort example: ":5000".
func UpdatePeers(options Options) (*Group, error) {

	const me = "UpdatePeers"

	options = defaultOptions(options)

	group := &Group{
		options: options,
		done:    make(chan struct{}),
	}

	kc, errClient := newKubeClient(options)
	if errClient != nil {
		return nil, errClient
	}

	group.client = kc

	addresses, errList := kc.listPodsAddresses()
	if errList != nil {
		options.Fatalf("%s: list addresses: %v", me, errList)
		return nil, errList
	}

	var myAddr string

	{
		var errAddr error
		myAddr, errAddr = options.Engine.findMyAddress()
		if errAddr != nil {
			options.Errorf("%s: %v", me, errAddr)
			return nil, errAddr
		}
	}

	if myAddr == "" {
		return nil, errors.New("could not find my address")
	}

	addresses = append(addresses, myAddr) // force my own addr

	group.peers = map[string]bool{}

	for _, addr := range addresses {
		url := buildURL(addr, options.GroupCachePort)
		group.peers[url] = true
	}

	keys := maps.Keys(group.peers)
	options.Debugf("%s: initial peers: %v", me, keys)
	options.Pool.Set(keys...)

	go updateLoop(group)

	return group, nil
}

func updateLoop(group *Group) {
	ch := make(chan podAddress)

	const me = "updateLoop"

	go watchPeers(group, ch)

	for {
		select {
		case <-group.done:
			group.options.Debugf("%s: done channel closed, goroutine exiting", me)
			return
		case n, ok := <-ch:
			if !ok {
				group.options.Errorf("%s: channel has been closed, nothing to do, exiting goroutine", me)
				return
			}
			url := buildURL(n.address, group.options.GroupCachePort)
			group.options.Debugf("%s: peer=%s added=%t current peers: %v",
				me, url, n.added, maps.Keys(group.peers))
			count := len(group.peers)
			if n.added {
				group.peers[url] = true
			} else {
				delete(group.peers, url)
			}
			if len(group.peers) == count {
				continue
			}
			keys := maps.Keys(group.peers)
			group.options.Debugf("%s: updating peers: %v", me, keys)
			group.options.Pool.Set(keys...)
		}
	}
}

func watchPeers(group *Group, ch chan<- podAddress) {
	const me = "watchPeers"
	errWatch := group.client.watchPodsAddresses(ch, group.done)
	if errWatch != nil {
		group.client.options.Fatalf("%s: %v", me, errWatch)
	}
	group.client.options.Errorf("%s: nothing to do, exiting goroutine", me)
}
