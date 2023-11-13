[![license](http://img.shields.io/badge/license-MIT-blue.svg)](https://github.com/udhos/kubegroup/blob/main/LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/udhos/kubegroup)](https://goreportcard.com/report/github.com/udhos/kubegroup)
[![Go Reference](https://pkg.go.dev/badge/github.com/udhos/kubegroup.svg)](https://pkg.go.dev/github.com/udhos/kubegroup)

# kubegroup

[kubegroup](https://github.com/udhos/kubegroup) provides peer autodiscovery for pods running [groupcache](https://github.com/mailgun/groupcache) within a kubernetes cluster.

Peer pods are automatically discovered by continuously watching for other pods with the same label `app=<value>` as in the current pod, in current pod's namespace.

# Usage

Import the package `github.com/udhos/kubegroup/kubegroup`.

```go
import "github.com/udhos/kubegroup/kubegroup"
```

Spawn a goroutine for `kubegroup.UpdatePeers(pool, groupcachePort)`.

```go
groupcachePort := ":5000"

// 1. get my groupcache URL
myURL, errURL = kubegroup.FindMyURL(groupcachePort)

// 2. spawn groupcache peering server
pool := groupcache.NewHTTPPoolOpts(myURL, &groupcache.HTTPPoolOptions{})
server := &http.Server{Addr: groupcachePort, Handler: pool}
go func() {
    log.Printf("groupcache server: listening on %s", groupcachePort)
    err := server.ListenAndServe()
    log.Printf("groupcache server: exited: %v", err)
}()

// 3. spawn peering autodiscovery
options := kubegroup.Options{
    Pool:           pool,
    GroupCachePort: app.groupCachePort,
    //PodLabelKey:    "app",         // default is "app"
    //PodLabelValue:  "my-app-name", // default is current PODs label value for label key
}

go kubegroup.UpdatePeers(options)

// 4. create groupcache groups, etc: groupOne := groupcache.NewGroup()
```

# Example

See [./examples/kubegroup-example](./examples/kubegroup-example)
