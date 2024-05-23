[![license](http://img.shields.io/badge/license-MIT-blue.svg)](https://github.com/udhos/kubegroup/blob/main/LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/udhos/kubegroup)](https://goreportcard.com/report/github.com/udhos/kubegroup)
[![Go Reference](https://pkg.go.dev/badge/github.com/udhos/kubegroup.svg)](https://pkg.go.dev/github.com/udhos/kubegroup)

# kubegroup

[kubegroup](https://github.com/udhos/kubegroup) provides peer autodiscovery for pods running [groupcache](https://github.com/mailgun/groupcache) within a kubernetes cluster.

Peer pods are automatically discovered by continuously watching for other pods with the same label `app=<value>` as in the current pod, in current pod's namespace.

# Metrics

```
kubegroup_peers: Gauge: Number of peer PODs discovered.
kubegroup_events: Counter: Number of events received.
```

# Usage for groupcache3

Import these packages.

```go
import (
	"github.com/groupcache/groupcache-go/v3"
	"github.com/groupcache/groupcache-go/v3/transport"
	"github.com/udhos/kube/kubeclient"
	"github.com/udhos/kubegroup/kubegroup"
)
```

Initialize groupcache3 and add kubegroup auto-discovery with `kubegroup.UpdatePeers()`.

```go
groupcachePort := ":5000"

// 1. start groupcache3 daemon

myIP, errAddr := kubegroup.FindMyAddress()
if errAddr != nil {
  log.Fatalf("find my address: %v", errAddr)
}

myAddr := myIP + groupCachePort

daemon, errDaemon := groupcache.ListenAndServe(context.TODO(), myAddr, groupcache.Options{})
if errDaemon != nil {
  log.Fatalf("groupcache daemon: %v", errDaemon)
}

// 2. spawn peering autodiscovery

const debug = true

clientsetOpt := kubeclient.Options{DebugLog: debug}
clientset, errClientset := kubeclient.New(clientsetOpt)
if errClientset != nil {
  log.Fatalf("kubeclient: %v", errClientset)
}

options := kubegroup.Options{
  Client:                clientset,
  Peers:                 daemon,
  LabelSelector:         "app=miniapi",
  GroupCachePort:        groupCachePort,
  Debug:                 debug,
  MetricsRegisterer:     prometheus.DefaultRegisterer,
  MetricsGatherer:       prometheus.DefaultGatherer,
  ForceNamespaceDefault: true,
}

group, errDiscovery := kubegroup.UpdatePeers(options)
if errDiscovery != nil {
  log.Fatalf("kubegroup: %v", errDiscovery)
}

// 3. create groupcache groups

ttl := time.Minute

getter := groupcache.GetterFunc(
  func(_ context.Context, filePath string, dest transport.Sink) error {

    log.Printf("cache miss, loading file: %s (ttl:%v)",
      filePath, ttl)

    data, errRead := os.ReadFile(filePath)
    if errRead != nil {
      return errRead
    }

    var expire time.Time // zero value for expire means no expiration
    if app.groupCacheExpire != 0 {
      expire = time.Now().Add(ttl)
    }

    return dest.SetBytes(data, expire)
  },
)

cache, errGroup := daemon.NewGroup("files", 1_000_000, getter)
if errGroup != nil {
  log.Fatalf("new group: %v", errGroup)
}

// 4. query cache

var data []byte

errGet := cache.Get(context.TODO(), "filename.txt", transport.AllocatingByteSliceSink(&data))
if errGet != nil {
  log.Printf("cache error: %v", errGet)
} else {
  log.Printf("cache response: %s", string(data))
}
```

# Example for groupcache3

See [./examples/kubegroup-example3](./examples/kubegroup-example3)

# Usage for groupcache2

Import the package `github.com/udhos/kubegroup/kubegroup`.

```go
import "github.com/udhos/kubegroup/kubegroup"
```

Initialize auto-discovery with `kubegroup.UpdatePeers()`.

```go
groupcachePort := ":5000"

// 1. get my groupcache URL
myURL, errURL = kubegroup.FindMyURL(groupcachePort)

workspace := groupcache.NewWorkspace()

// 2. spawn groupcache peering server
pool := groupcache.NewHTTPPoolOptsWithWorkspace(workspace, myURL, &groupcache.HTTPPoolOptions{})
server := &http.Server{Addr: groupcachePort, Handler: pool}
go func() {
    log.Printf("groupcache server: listening on %s", groupcachePort)
    err := server.ListenAndServe()
    log.Printf("groupcache server: exited: %v", err)
}()

// 3. spawn peering autodiscovery

clientset, errClientset := kubeclient.New(kubeclient.Options{})
if errClientset != nil {
  log.Fatalf("kubeclient: %v", errClientset)
}

options := kubegroup.Options{
  Client:                clientset,
  Pool:                  pool,
  LabelSelector:         "app=my-app",
  GroupCachePort:        groupCachePort,
  MetricsRegisterer:     prometheus.DefaultRegisterer,
  MetricsGatherer:       prometheus.DefaultGatherer,
}

group, errGroup := kubegroup.UpdatePeers(options)
if errGroup != nil {
  log.Fatalf("kubegroup: %v", errGroup)
}

// 4. create groupcache groups, etc: groupOne := groupcache.NewGroup()

// 5. before shutdown you might stop kubegroup to release resources

group.Close() // release kubegroup resources
server.Shutdown(...) // do not forget to shutdown the peering server
```

# Example

See [./examples/kubegroup-example](./examples/kubegroup-example)

# POD Permissions

The application PODs will need permissions to get/list/watch PODs against kubernetes API, as illustrated by the role below.

## Role

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: role-name
rules:
- apiGroups:
  - ""
  resources:
  - 'pods'
  verbs:
  - 'get'
  - 'list'
  - 'watch'
```

Example chart template: https://github.com/udhos/gateboard/blob/main/charts/gateboard/templates/role.yaml

## Role Binding

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: rolebinding-name
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: role-name
subjects:
- kind: ServiceAccount
  name: put-pod-service-account-here
  namespace: put-pod-namespace-here
```

Example chart template: https://github.com/udhos/gateboard/blob/main/charts/gateboard/templates/rolebinding.yaml
