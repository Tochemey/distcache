# DistCache

![GitHub Actions Workflow Status](https://img.shields.io/github/actions/workflow/status/Tochemey/distcache/build.yml)
[![codecov](https://codecov.io/gh/Tochemey/distcache/graph/badge.svg?token=0eS0QphVUH)](https://codecov.io/gh/Tochemey/distcache)
[![GitHub go.mod Go version](https://badges.chse.dev/github/go-mod/go-version/Tochemey/distcache)](https://go.dev/doc/install)

`DistCache` is a Distributed Read-Through Cache Engine built in [Go](https://go.dev/).

A Distributed Read-Through Cache is a caching strategy where cache sits between the application and the data source,
automatically fetching and storing data when requested. If data is not in the cache (cache miss), it retrieves it from the primary data source (e.g., database, API), stores it
in the cache, and serves it to the client. This approach reduces direct database queries, improves response times, and enhances system scalability.

`DistCache` was built to be scalable and high available. With `DistCache`, you can instantly create a fast, scalable, distributed system across a cluster of computers.

`DistCache` caching engine is powered by the battle-tested [group cache](https://github.com/groupcache/groupcache-go).

## Table Of Content

- [Features](#features)
- [How It Works](#how-it-works)
- [Use Cases](#use-cases)
- [Installation](#installation)
- [Get Started](#get-started)
    - [DataSource](#datasource)
    - [Keyspace](#keyspace)

## Features

- **Automatic Fetching**: Data is loaded into the cache on a cache miss.
- **Distributed Architecture**: The cache is spread across multiple nodes for scalability and availability.
- **Reduced Load on Backend**: Frequent reads are served from the cache, minimizing database hits.
- **Configurable Expiry & Eviction**: Data can be automatically expired or evicted based on policies (TTL, LRU, etc.).
- **Automatic Nodes discovery**: All nodes in the cluster are aware of the cluster topology change and react to it
  accordingly.
- **Discovery Provider API**: The developer can build custom nodes discovery providers or use the built-in providers.
- **Built-in Discovery Providers**:
    - [kubernetes](./discovery/kubernetes/README.md) - helps discover cluster nodes during boostrap using the kubernetes client.
    - [NATS](./discovery/nats/README.md) - helps discover cluster nodes during bootstrap using [NATS](https://github.com/nats-io/nats.go).
    - [Static](./discovery/static/README.md) - the provided static cluster nodes help form a cluster. This provider is recommended for tests or demo purpose.
    - [DNS](./discovery/dnssd/README.md) - helps discover cluster nodes during bootstrap using the Go's DNS resolver.
- **TLS Support**: `DistCache` comes bundled with TLS support. All nodes must share the same root Certificate Authority (CA) to ensure a successful handshake.
  TLS can be enabled in the [config](./config.go) with the option `WithTLS` method. This method allows you to configure both the TLS server and client settings.
  The TLS configuration is essential for enabling secure communication between nodes.

## How It Works

1. Cache Lookup: The application requests data from the cache.
2. Cache Hit (Data Exists): The cache returns the requested data.
3. Cache Miss (Data Absent):
    - The cache fetches the data from the primary data source.
    - The data is stored in the cache for future requests.
    - The requested data is returned to the client.
4. Subsequent Requests: Future requests for the same data are served from the cache until the data expires or is
   evicted.

## Use Cases

- **High-Traffic Applications** – Reduce load on databases (e.g., e-commerce, social media).
- **API Caching** – Store API responses to avoid redundant network calls.
- **Session Storage** – Maintain fast access to user sessions in distributed systems.

## Installation

```bash
go get github.com/tochemey/distcache
```

## Get Started

To integrate `DistCache` into your project, one only need to implement `two key interfaces` that are needed in the [Config](./config.go) to start the `DistCache` [Engine](./engine.go).

### DataSource

The [DataSource](./datasource.go) interface tells `DistCache` where to fetch data from when a cache miss occurs. This could be any external source such as a database, an API, or even a file system.

#### Example:

````go
// MyDataSource implements the DataSource interface.
type MyDataSource struct{}

// Fetch retrieves data for a given key. Replace this with your actual data fetching logic.
func (ds *MyDataSource) Fetch(ctx context.Context, key string) ([]byte, error) {
	if key == "" {
		return nil, errors.New("empty key provided")
	}
	// Simulate data fetching. In practice, query your database, API, etc.
	data := fmt.Sprintf("Data for key: %s", key)
	return []byte(data), nil
}
````

### KeySpace

The [KeySpace](./keyspace.go) interface defines a `logical namespace for grouping` key/value pairs. It provides metadata such as the namespace's name, storage limits, and expiration logic for keys.
KeySpaces are loaded during `DistCache` bootstrap. 

#### Example:

```go
// MyKeySpace implements the KeySpace interface.
type MyKeySpace struct {
	name       string
	maxBytes   int64
	dataSource *MyDataSource
}

// Name returns the name of the keyspace.
func (ks *MyKeySpace) Name() string {
	return ks.name
}

// MaxBytes returns the maximum number of bytes allocated for this keyspace.
func (ks *MyKeySpace) MaxBytes() int64 {
	return ks.maxBytes
}

// DataSource returns the data source for fetching data on a cache miss.
func (ks *MyKeySpace) DataSource() DataSource {
	return ks.dataSource
}

// ExpiresAt returns the expiration time for a given key.
// For example, keys may expire after 10 minutes.
func (ks *MyKeySpace) ExpiresAt(ctx context.Context, key string) time.Time {
	return time.Now().Add(10 * time.Minute)
}
```


## Contribution

Contributions are welcome!
The project adheres to [Semantic Versioning](https://semver.org)
and [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/).

To contribute please:

- Fork the repository
- Create a feature branch
- Submit a [pull request](https://help.github.com/articles/using-pull-requests)
