# DistCache

![GitHub Actions Workflow Status](https://img.shields.io/github/actions/workflow/status/Tochemey/distcache/build.yml)
[![codecov](https://codecov.io/gh/Tochemey/distcache/graph/badge.svg?token=0eS0QphVUH)](https://codecov.io/gh/Tochemey/distcache)
[![GitHub go.mod Go version](https://badges.chse.dev/github/go-mod/go-version/Tochemey/distcache)](https://go.dev/doc/install)
[![Go Reference](https://pkg.go.dev/badge/github.com/tochemey/distcache.svg)](https://pkg.go.dev/github.com/tochemey/distcache)

DistCache is a **distributed read‑through cache engine** built in [Go](https://go.dev/).

In a read‑through cache, the cache sits between your application and the data source. When the application requests
data:

- If the data is in the cache (**cache hit**), it is returned immediately.
- If the data is not in the cache (**cache miss**), DistCache fetches it from the primary data source (database, API, etc.),
  stores it in the cache, and returns it to the caller.

This reduces direct load on your backend, lowers latency, and improves scalability.

DistCache is designed to be **scalable** and **highly available**. It allows you to quickly build fast, distributed
systems across a cluster of machines.

The caching engine is powered by the battle‑tested [groupcache-go](https://github.com/groupcache/groupcache-go).

## 👋 Table of Contents

- [Features](#-features)
- [Installation](#-installation)
- [Engine](#-engine)
- [How It Works](#-how-it-works)
- [Use Cases](#-use-cases)
- [Get Started](#-get-started)
  - [DataSource](#datasource)
  - [KeySpace](#keyspace)
- [Example](#example)
- [Contribution](#-contribution)

## ⭐️ Features

- **Automatic fetch on miss** – Data is loaded into the cache only when requested.
- **Distributed architecture** – Data is sharded across nodes for scalability and availability.
- **Reduced backend load** – Frequent reads are served from the cache instead of the database.
- **Configurable expiry & eviction** – Support for TTL, LRU, and custom policies.
- **Automatic node discovery** – Nodes automatically react to cluster topology changes.
- **Discovery provider API** – Implement custom discovery backends or use the built‑in ones:
  - [Kubernetes](./discovery/kubernetes/README.md) – discover peers via the Kubernetes API.
  - [NATS](./discovery/nats/README.md) – discover peers via [NATS](https://github.com/nats-io/nats.go).
  - [Static](./discovery/static/README.md) – fixed list of peers, ideal for tests and demos.
  - [DNS](./discovery/dnssd/README.md) – discover peers via Go’s DNS resolver.
- **TLS support** – End‑to‑end encrypted communication between nodes. All nodes must share the same root
  Certificate Authority (CA) for a successful handshake. TLS is enabled via the [`WithTLS`](./option.go) option
  in the configuration and applies to both client and server sides.

## 💻 Installation

```bash
go get github.com/tochemey/distcache
```

## ⚙️ Engine

All core capabilities are exposed through the DistCache [Engine](./engine.go), which provides a simple API for
interacting with the cache.

### Core Methods

- **Put** – Store a single key/value pair in a given keyspace.
- **PutMany** – Store multiple key/value pairs in a given keyspace.
- **Get** – Retrieve a specific key/value pair from a given keyspace.
- **Delete** – Remove a specific key/value pair from a given keyspace.
- **DeleteMany** – Remove multiple key/value pairs from a given keyspace.

### KeySpace Management

- **DeleteKeySpace** – Delete a single keyspace and all of its entries.
- **DeleteKeyspaces** – Delete multiple keyspaces at once.
- **KeySpaces** – List all available keyspaces.

The Engine is designed to provide high‑performance operations across a distributed cluster while keeping the API
simple and predictable.

## 📝 How It Works

1. **Cache lookup** – The application requests data from DistCache.
2. **Cache hit** – If the data is present, it is returned immediately.
3. **Cache miss**:
   - DistCache fetches data from the configured primary data source.
   - The data is stored in the cache according to the keyspace configuration.
   - The data is returned to the caller.
4. **Subsequent requests** – Future reads for the same key are served from the cache until the entry expires or is
   evicted by the eviction policy.

## 💡 Use Cases

- **High‑traffic applications** – Reduce database load (e‑commerce, social networks, analytics dashboards).
- **API response caching** – Cache expensive or frequently called external APIs.
- **Session and profile caching** – Keep user session or profile data close to your application across nodes.

## 🚀 Get Started

To integrate DistCache, configure a [`Config`](./config.go) and implement **two interfaces** before starting the
[Engine](./engine.go):

- [`DataSource`](./datasource.go): Defines how to fetch data on a cache miss. This can be a database, REST API, gRPC service, filesystem, or any
  other backend.
- [`KeySpace`](./keyspace.go): Defines a logical namespace for grouping key/value pairs. It controls metadata such as:
  - Keyspace name
  - Storage constraints
  - Expiration and eviction behavior

KeySpaces are loaded during DistCache bootstrap and dictate how data is partitioned and managed in the cluster.

## Example

A complete example can be found in the [`example`](./example) directory.

## 🤲 Contribution

Contributions are welcome.

This project follows:

- [Semantic Versioning](https://semver.org)
- [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/)

To contribute:

1. Fork the repository.
2. Create a feature branch.
3. Commit your changes using Conventional Commits.
4. Submit a [pull request](https://help.github.com/articles/using-pull-requests).
