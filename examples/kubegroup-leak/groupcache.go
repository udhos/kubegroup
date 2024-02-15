package main

import (
	"log"
	"net/http"

	"github.com/modernprogram/groupcache/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/udhos/kubegroup/kubegroup"
)

func startGroupcache(app *application) *groupcache.HTTPPool {

	//
	// create groupcache pool
	//

	myURL, errURL := kubegroup.FindMyURL(app.groupCachePort)
	if errURL != nil {
		log.Fatalf("my URL: %v", errURL)
	}

	log.Printf("groupcache my URL: %s", myURL)

	pool := groupcache.NewHTTPPoolOpts(myURL, &groupcache.HTTPPoolOptions{})

	//
	// start groupcache server
	//

	app.serverGroupCache = &http.Server{Addr: app.groupCachePort, Handler: pool}

	go func() {
		log.Printf("groupcache server: listening on %s", app.groupCachePort)
		err := app.serverGroupCache.ListenAndServe()
		log.Printf("groupcache server: exited: %v", err)
	}()

	return pool
}

func startPeerWatcher(app *application, pool *groupcache.HTTPPool) {

	//
	// metrics
	//
	app.registry = prometheus.NewRegistry()
	app.registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	app.registry.MustRegister(prometheus.NewGoCollector())

	//
	// start watcher for addresses of peers
	//

	options := kubegroup.Options{
		Pool:           pool,
		GroupCachePort: app.groupCachePort,
		//PodLabelKey:    "app",         // default is "app"
		//PodLabelValue:  "my-app-name", // default is current PODs label value for label key
		Debug:             false,
		Errorf:            func(_ /*format*/ string, _ /*v*/ ...any) {},
		Engine:            kubegroup.NewKubeBogus(),
		MetricsRegisterer: app.registry,
		MetricsGatherer:   app.registry,
	}

	group, errGroup := kubegroup.UpdatePeers(options)
	if errGroup != nil {
		log.Fatalf("kubegroup: %v", errGroup)
	}

	group.Close() // release watcher resources
}
