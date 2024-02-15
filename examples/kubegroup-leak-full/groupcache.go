package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/modernprogram/groupcache/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/udhos/kubegroup/kubegroup"
)

func startGroupcache(app *application) {

	//
	// metrics
	//
	app.registry = prometheus.NewRegistry()
	app.registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	app.registry.MustRegister(prometheus.NewGoCollector())

	//
	// create groupcache pool
	//

	var myURL string
	for myURL == "" {
		var errURL error
		myURL, errURL = kubegroup.FindMyURL(app.groupCachePort)
		if errURL != nil {
			log.Printf("my URL: %v", errURL)
		}
		if myURL == "" {
			const cooldown = 5 * time.Second
			log.Printf("could not find my URL, sleeping %v", cooldown)
			time.Sleep(cooldown)
		}
	}

	//log.Printf("groupcache my URL: %s", myURL)

	ws := groupcache.NewWorkspace()

	pool := groupcache.NewHTTPPoolOptsWithWorkspace(ws, myURL, &groupcache.HTTPPoolOptions{})

	/*
		//
		// start groupcache server
		//

		app.serverGroupCache = &http.Server{Addr: app.groupCachePort, Handler: pool}

		go func() {
			log.Printf("groupcache server: listening on %s", app.groupCachePort)
			err := app.serverGroupCache.ListenAndServe()
			log.Printf("groupcache server: exited: %v", err)
		}()
	*/

	//
	// start watcher for addresses of peers
	//

	options := kubegroup.Options{
		Pool:           pool,
		GroupCachePort: app.groupCachePort,
		//PodLabelKey:    "app",         // default is "app"
		//PodLabelValue:  "my-app-name", // default is current PODs label value for label key
		Debug:             false,
		Engine:            kubegroup.NewKubeBogus(),
		Errorf:            func(_ /*format*/ string, _ /*v*/ ...any) {},
		MetricsRegisterer: app.registry,
		MetricsGatherer:   app.registry,
	}

	if app.once {
		options.Errorf = nil
	}

	/*
		if app.engineBogus {
			options.Engine = kubegroup.NewKubeBogus()
		}
	*/

	group, errGroup := kubegroup.UpdatePeers(options)
	if errGroup != nil {
		log.Fatalf("kubegroup: %v", errGroup)
	}

	app.group = group

	//
	// create cache
	//

	getter := groupcache.GetterFunc(
		func(_ /*ctx*/ context.Context, filePath string, dest groupcache.Sink) error {

			if app.once {
				log.Printf("cache miss, loading file: %s (ttl:%v)", filePath, app.groupCacheExpire)
			}

			data, errRead := os.ReadFile(filePath)
			if errRead != nil {
				return errRead
			}

			var expire time.Time // zero value for expire means no expiration
			if app.groupCacheExpire != 0 {
				expire = time.Now().Add(app.groupCacheExpire)
			}

			dest.SetBytes(data, expire)

			return nil
		},
	)

	// https://talks.golang.org/2013/oscon-dl.slide#46
	//
	// 64 MB max per-node memory usage
	app.cache = groupcache.NewGroupWithWorkspace(ws, "files",
		app.groupCacheSizeBytes, getter)

	for i := 0; i < 3; i++ {
		var dst []byte
		errGet := app.cache.Get(context.TODO(), "build.sh",
			groupcache.AllocatingByteSliceSink(&dst))
		if errGet != nil {
			log.Printf("cache get error: %v", errGet)
			continue
		}
		if app.once {
			log.Printf("cache get ok: %d bytes", len(dst))
		}
	}
}
