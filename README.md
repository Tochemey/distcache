# DistCache

![GitHub Actions Workflow Status](https://img.shields.io/github/actions/workflow/status/Tochemey/distcache/build.yml)
[![codecov](https://codecov.io/gh/Tochemey/distcache/graph/badge.svg?token=0eS0QphVUH)](https://codecov.io/gh/Tochemey/distcache)
[![GitHub go.mod Go version](https://badges.chse.dev/github/go-mod/go-version/Tochemey/distcache)](https://go.dev/doc/install)

DistCache is a Distributed Read-Through Cache Engine built in [Go](https://go.dev/). 

A Distributed Read-Through Cache is a caching strategy where cache sits between the application and the data source,
automatically fetching and storing data when requested. If data is not in the cache (cache miss), it retrieves it from the primary data source (e.g., database, API), stores it
in the cache, and serves it to the client. This approach reduces direct database queries, improves response times, and enhances system scalability.

DistCache has been built to be scalable and high available. With DistCache, you can instantly create a fast, scalable, distributed system across a cluster of computers.

## Table Of Content

- [Features](#features)
- [How It Works](#how-it-works)
- [Use Cases](#use-cases)
- [Installation](#installation)
- [Get Started](#get-started)

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

- Implement the [`DataSource`](./datasource.go) interface: This tells DistCache where to fetch the missing data from in case of cache-miss. The DataSource can be a database, API or file system.
- Implement the [`KeySpace`](./keyspace.go) interface. The `KeySpace` helps group key/value pairs in some sort of space which can fine tune based upon the need of the application.
- Create an instance of the `Config`. More information can be found on the [Config](./config.go) reference doc.
- Create an instance of [`Engine`](./engine.go) by calling the method: `NewEngine(config *Config) (Engine, error)`

## Contribution

Contributions are welcome!
The project adheres to [Semantic Versioning](https://semver.org)
and [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/).

To contribute please:

- Fork the repository
- Create a feature branch
- Submit a [pull request](https://help.github.com/articles/using-pull-requests)