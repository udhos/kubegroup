package main

import (
	"log"
	"net/http"

	"github.com/mailgun/groupcache"
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
	// start watcher for addresses of peers
	//

	options := kubegroup.Options{
		Pool:           pool,
		GroupCachePort: app.groupCachePort,
		//PodLabelKey:    "app",         // default is "app"
		//PodLabelValue:  "my-app-name", // default is current PODs label value for label key
		Debug:  false,
		Errorf: func(format string, v ...any) {},
		Engine: kubegroup.NewKubeBogus(),
	}

	group, errGroup := kubegroup.UpdatePeers(options)
	if errGroup != nil {
		log.Fatalf("kubegroup: %v", errGroup)
	}

	app.group = group

	/*
		//
		// create cache
		//

		// https://talks.golang.org/2013/oscon-dl.slide#46
		//
		// 64 MB max per-node memory usage
		app.cache = groupcache.NewGroup("files", app.groupCacheSizeBytes, groupcache.GetterFunc(
			func(ctx groupcache.Context, filePath string, dest groupcache.Sink) error {

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
	*/

	group.Close()
}
