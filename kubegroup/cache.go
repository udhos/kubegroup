// Package kubegroup provides autodiscovery for groupcache.
package kubegroup

import (
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/mailgun/groupcache" // "github.com/golang/groupcache"
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

// Options specifies options for UpdatePeers.
type Options struct {
	Pool           *groupcache.HTTPPool
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

func defaultOptions(options Options) Options {
	if options.Cooldown == 0 {
		options.Cooldown = 5 * time.Second
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

// UpdatePeers continuously updates groupcache peers.
// groupcachePort example: ":5000".
func UpdatePeers(options Options) {

	const me = "UpdatePeers"

	options = defaultOptions(options)

	kc, errClient := newKubeClient(options)
	if errClient != nil {
		options.Fatalf("%s: kube client: %v", me, errClient)
	}

	addresses, errList := kc.listPodsAddresses()
	if errList != nil {
		options.Fatalf("%s: list addresses: %v", me, errList)
	}

	var myAddr string

	for myAddr == "" {
		var errAddr error
		myAddr, errAddr = findMyAddr()
		if errAddr != nil {
			options.Errorf("%s: %v", me, errAddr)
		}
		if myAddr == "" {
			options.Errorf("%s: could not find my address, sleeping %v", me, options.Cooldown)
			time.Sleep(options.Cooldown)
		}
	}

	addresses = append(addresses, myAddr) // force my own addr

	peers := map[string]bool{}

	for _, addr := range addresses {
		url := buildURL(addr, options.GroupCachePort)
		peers[url] = true
	}

	keys := maps.Keys(peers)
	options.Debugf("%s: initial peers: %v", me, keys)
	options.Pool.Set(keys...)

	ch := make(chan podAddress)

	go watchPeers(kc, ch)

	for n := range ch {
		url := buildURL(n.address, options.GroupCachePort)
		options.Debugf("%s: peer=%s added=%t current peers: %v",
			me, url, n.added, maps.Keys(peers))
		count := len(peers)
		if n.added {
			peers[url] = true
		} else {
			delete(peers, url)
		}
		if len(peers) == count {
			continue
		}
		keys := maps.Keys(peers)
		options.Debugf("%s: updating peers: %v", me, keys)
		options.Pool.Set(keys...)
	}

	options.Errorf("%s: channel has been closed, nothing to do, exiting goroutine", me)
}

func watchPeers(kc kubeClient, ch chan<- podAddress) {
	errWatch := kc.watchPodsAddresses(ch)
	if errWatch != nil {
		kc.options.Fatalf("watchPeers: %v", errWatch)
	}
	kc.options.Errorf("watchPeers: nothing to do, exiting goroutine")
}
