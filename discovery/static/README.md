# Static Discovery Provider

The Static Discovery Provider helps `DistCache` during bootstrap to discover existing nodes in the cluster.
Once the nodes are discovered `DistCache` makes use of the gossip protocol to manage cluster topolgy changes.

This provider performs nodes discovery based upon the list of static hosts addresses.
The address of each host is the form of `host:port` where `port` is the discovery protocol port.

```go
package main

import "github.com/tochemey/goakt/v2/discovery/static"

// define the discovery configuration
config := static.Config{
  Addresses: []string{
    "node1:3322",
    "node2:3322",
    "node3:3322",
    },
}
// instantiate the dnssd discovery provider
provider := static.NewDiscovery(&config)
```