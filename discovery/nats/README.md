# NATS Discovery Provider

To use the NATS discovery provider one needs to provide the following:

- `Server`: the NATS Server address
- `Subject`: the NATS subject to use
- `Timeout`: the nodes discovery timeout
- `MaxJoinAttempts`: the maximum number of attempts to connect an existing NATs server. Defaults to `5`
- `ReconnectWait`: the time to backoff after attempting a reconnect to a server that we were already connected to
  previously. Default to `2 seconds`
- `Host`: the given node host address
- `DiscoveryPort`: the discovery port of the given node

```go
package main

import "github.com/tochemey/distcache/discovery/nats"

const (
    natsServerAddr   = "nats://127.0.0.1:4248"
    natsSubject      = "goakt-gossip"
)

// define the discovery options
config := nats.Config{
    Server:      natsServerAddr,
    Subject:     natsSubject,
    Host:        "127.0.0.1",
    DiscoveryPort:   20380,
}

// instantiate the NATS discovery provider by passing the config and the hostNode
provider := nats.NewDiscovery(&config)
```
