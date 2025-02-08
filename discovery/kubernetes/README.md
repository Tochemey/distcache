# Kubernetes Discovery Provider

The kubernetes Discovery Provider helps `DistCache` during bootstrap to discover existing nodes in the cluster.
Once the nodes are discovered `DistCache` makes use of the gossip protocol to manage cluster topolgy changes.

## Get Started

```go
package main

import "github.com/tochemey/distcache/discovery/kubernetes"

const (
    namespace          = "default"
    applicationName    = "cacheEngine"
    discoveryPortName  = "discovery-port"
)

// define the discovery config
config := kubernetes.Config{
    Namespace:        namespace,
    DiscoveryPortName: discoveryPortName,
    PodLabels:  map[string]string{
        "app.kubernetes.io/part-of": applicatioName,
        "app.kubernetes.io/component": applicationName,
        "app.kubernetes.io/name": applicationName
    },
}

// instantiate the k8 discovery provider
provider := kubernetes.NewDiscovery(&config)
```

## Role Based Access

You’ll also have to grant the Service Account that your pods run under access to list pods. The following configuration
can be used as a starting point.
It creates a Role, pod-reader, which grants access to query pod information. It then binds the default Service Account
to the Role by creating a RoleBinding.
Adjust as necessary:

```
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: pod-reader
rules:
  - apiGroups: [""] # "" indicates the core API group
    resources: ["pods"]
    verbs: ["get", "watch", "list"]
---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: read-pods
subjects:
  # Uses the default service account. Consider creating a new one.
  - kind: ServiceAccount
    name: default
roleRef:
  kind: Role
  name: pod-reader
  apiGroup: rbac.authorization.k8s.io
```
