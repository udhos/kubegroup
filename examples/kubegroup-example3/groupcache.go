package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/groupcache/groupcache-go/v3"
	"github.com/groupcache/groupcache-go/v3/transport"
	"github.com/udhos/groupcache_datadog/exporter"
	"github.com/udhos/kube/kubeclient"
	"github.com/udhos/kubegroup/kubegroup"
)

func startGroupcache(app *application, dogstatsd, mockDogstatsd bool) {

	//
	// create groupcache instance
	//

	myIP, errAddr := kubegroup.FindMyAddress()
	if errAddr != nil {
		log.Fatalf("find my address: %v", errAddr)
	}

	myAddr := myIP + app.groupCachePort

	daemon, errDaemon := groupcache.ListenAndServe(context.TODO(), myAddr, groupcache.Options{})
	if errDaemon != nil {
		log.Fatalf("groupcache daemon: %v", errDaemon)
	}

	//
	// start watcher for addresses of peers
	//

	const debug = true

	clientsetOpt := kubeclient.Options{DebugLog: debug}
	clientset, errClientset := kubeclient.New(clientsetOpt)
	if errClientset != nil {
		log.Fatalf("startGroupcache: kubeclient: %v", errClientset)
	}

	var dogstatsdClient kubegroup.DogstatsdClient
	if dogstatsd {
		if mockDogstatsd {
			dogstatsdClient = &kubegroup.DogstatsdClientMock{}
		} else {
			c, errClient := exporter.NewDatadogClient(exporter.DatadogClientOptions{
				Namespace: "kubegroup",
				Debug:     debug,
			})
			if errClient != nil {
				log.Fatalf("dogstatsd client: %v", errClient)
			}
			dogstatsdClient = c
		}
	}

	options := kubegroup.Options{
		Client:                clientset,
		Peers:                 daemon,
		LabelSelector:         "app=miniapi",
		GroupCachePort:        app.groupCachePort,
		Debug:                 debug,
		DogstatsdClient:       dogstatsdClient,
		ForceNamespaceDefault: true,
	}

	if app.registry != nil {
		options.MetricsRegisterer = app.registry
		options.MetricsGatherer = app.registry
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
		func(_ context.Context, filePath string, dest transport.Sink) error {

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

			return dest.SetBytes(data, expire)
		},
	)

	cache, errGroup := daemon.NewGroup("files", app.groupCacheSizeBytes, getter)
	if errGroup != nil {
		log.Fatalf("new group: %v", errGroup)
	}

	app.cache = cache
}
