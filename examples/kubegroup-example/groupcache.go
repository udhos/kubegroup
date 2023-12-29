package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/modernprogram/groupcache/v2"
	"github.com/udhos/kubegroup/kubegroup"
)

func startGroupcache(app *application) {

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

	//
	// start watcher for addresses of peers
	//

	options := kubegroup.Options{
		Pool:           pool,
		GroupCachePort: app.groupCachePort,
		//PodLabelKey:    "app",         // default is "app"
		//PodLabelValue:  "my-app-name", // default is current PODs label value for label key
		Debug: true,
	}

	if app.engineBogus {
		options.Engine = kubegroup.NewKubeBogus()
	}

	group, errGroup := kubegroup.UpdatePeers(options)
	if errGroup != nil {
		log.Fatalf("kubegroup: %v", errGroup)
	}

	app.group = group

	//
	// create cache
	//

	// https://talks.golang.org/2013/oscon-dl.slide#46
	//
	// 64 MB max per-node memory usage
	app.cache = groupcache.NewGroup("files", app.groupCacheSizeBytes, groupcache.GetterFunc(
		func(ctx context.Context, filePath string, dest groupcache.Sink) error {

			log.Printf("cache miss, loading file: %s (ttl:%v)", filePath, app.groupCacheExpire)

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
	))
}
