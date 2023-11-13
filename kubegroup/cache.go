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
}

// UpdatePeers continuously updates groupcache peers.
// groupcachePort example: ":5000".
func UpdatePeers(options Options) {

	kc, errClient := newKubeClient(options.PodLabelKey, options.PodLabelValue)
	if errClient != nil {
		log.Fatalf("updatePeers: kube client: %v", errClient)
	}

	addresses, errList := kc.listPodsAddresses()
	if errList != nil {
		log.Fatalf("updatePeers: list addresses: %v", errList)
	}

	var myAddr string

	for myAddr == "" {
		var errAddr error
		myAddr, errAddr = findMyAddr()
		if errAddr != nil {
			log.Printf("updatePeers: %v", errAddr)
		}
		if myAddr == "" {
			const cooldown = 5 * time.Second
			log.Printf("updatePeers: could not find my address, sleeping %v", cooldown)
			time.Sleep(cooldown)
		}
	}

	addresses = append(addresses, myAddr) // force my own addr

	peers := map[string]bool{}

	for _, addr := range addresses {
		url := buildURL(addr, options.GroupCachePort)
		peers[url] = true
	}

	keys := maps.Keys(peers)
	log.Printf("updatePeers: initial peers: %v", keys)
	options.Pool.Set(keys...)

	ch := make(chan podAddress)

	go watchPeers(kc, ch)

	for n := range ch {
		url := buildURL(n.address, options.GroupCachePort)
		log.Printf("updatePeers: peer=%s added=%t current peers: %v",
			url, n.added, maps.Keys(peers))
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
		log.Printf("updatePeers: updating peers: %v", keys)
		options.Pool.Set(keys...)
	}

	log.Printf("updatePeers: channel has been closed, nothing to do, exiting")
}

func watchPeers(kc kubeClient, ch chan<- podAddress) {
	errWatch := kc.watchPodsAddresses(ch)
	if errWatch != nil {
		log.Fatalf("watchPeers: %v", errWatch)
	}
	log.Printf("watchPeers: nothing to do, exiting")
}
