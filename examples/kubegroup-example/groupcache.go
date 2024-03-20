package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/modernprogram/groupcache/v2"
	"github.com/udhos/kube/kubeclient"
	"github.com/udhos/kubegroup/kubegroup"
)

func startGroupcache(app *application) {

	//
	// create groupcache pool
	//

	myURL, errURL := kubegroup.FindMyURL(app.groupCachePort)
	if errURL != nil {
		log.Fatalf("my URL: %v", errURL)
	}
	log.Printf("groupcache my URL: %s", myURL)

	workspace := groupcache.NewWorkspace()

	pool := groupcache.NewHTTPPoolOptsWithWorkspace(workspace, myURL,
		&groupcache.HTTPPoolOptions{})

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

	const debug = true

	clientsetOpt := kubeclient.Options{DebugLog: debug}
	clientset, errClientset := kubeclient.New(clientsetOpt)
	if errClientset != nil {
		log.Fatalf("startGroupcache: kubeclient: %v", errClientset)
	}

	options := kubegroup.Options{
		Client:                clientset,
		Pool:                  pool,
		LabelSelector:         "app=miniapi",
		GroupCachePort:        app.groupCachePort,
		Debug:                 debug,
		MetricsRegisterer:     app.registry,
		MetricsGatherer:       app.registry,
		ForceNamespaceDefault: true,
	}

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

			log.Printf("cache miss, loading file: %s (ttl:%v)",
				filePath, app.groupCacheExpire)

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
	app.cache = groupcache.NewGroupWithWorkspace(workspace, "files",
		app.groupCacheSizeBytes, getter)
}
