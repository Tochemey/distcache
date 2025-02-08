# DNS-based Discovery Provider

This provider performs nodes discovery based upon the domain name provided. This is very useful when doing local
development
using docker.

To use the DNS discovery provider one needs to provide the following:

- `DomainName`: the domain name
- `IPv6`: it states whether to lookup for IPv6 addresses.

```go
package main

import "github.com/tochemey/distcache/discovery/dnssd"

const domainName = "accounts"

// define the discovery options
config := dnssd.Config{
    DomainName: domainName,
    IPv6:       false,
}
// instantiate the dnssd discovery provider
provider := dnssd.NewDiscovery(&config)
```