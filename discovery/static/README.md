# Static Discovery Provider

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