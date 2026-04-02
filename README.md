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

The caching engine is powered by the battle‑tested [groupcache-go](https://github.com/groupcache/groupcache-go).

## Features

- **Automatic fetch on miss** – Data is loaded into the cache only when requested.
- **Distributed architecture** – Data is sharded across nodes for scalability and availability.
- **Reduced backend load** – Frequent reads are served from the cache instead of the database.
- **Configurable expiry & eviction** – Support for TTL, LRU, and custom policies.
- **Automatic node discovery** – Nodes automatically react to cluster topology changes.
- **KeySpace overrides** – Per‑keyspace TTL, timeouts, max bytes, warm keys, and protections.
- **Dynamic keyspace updates** – Replace keyspaces at runtime via `UpdateKeySpace`.
- **Warmup & hot key tracking** – Prefetch hot keys on join/leave events.
- **DataSource protection** – Rate limiting and circuit breaking, globally or per keyspace.
- **Admin diagnostics** – JSON endpoints for peers and keyspace stats.
- **Observability** – OpenTelemetry metrics and tracing around engine operations.
- **TLS support** – End‑to‑end encrypted communication between nodes.
- **Discovery providers** – Built‑in support for:
  - [Kubernetes](./discovery/kubernetes/README.md) – discover peers via the Kubernetes API.
  - [NATS](./discovery/nats/README.md) – discover peers via [NATS](https://github.com/nats-io/nats.go).
  - [Static](./discovery/static/README.md) – fixed list of peers, ideal for tests and demos.
  - [DNS](./discovery/dnssd/README.md) – discover peers via Go's DNS resolver.
  - [Standalone](./discovery/standalone) – single‑node, no cluster discovery.

## Installation

```bash
go get github.com/tochemey/distcache
```

## Quick Start

Integrate DistCache by implementing two interfaces:

- [`DataSource`](./datasource.go) – Fetches data from your backend on cache misses.
- [`KeySpace`](./keyspace.go) – Defines a cache namespace, storage limit, and expiration behavior.

Then create a config and start the engine:

1. **Implement `DataSource`** – Provide a `Fetch(ctx, key) ([]byte, error)` method that retrieves data from your backend (database, API, etc.) when a cache miss occurs.

2. **Implement `KeySpace`** – Define a cache namespace by returning its name, maximum byte capacity, the `DataSource` to use on misses, and an optional per‑key expiration time.

3. **Create a config** – Use `NewStandaloneConfig` for single‑node setups or `NewConfig` with a [discovery provider](#discovery-providers) for distributed clusters.

4. **Start the engine** – Call `distcache.NewEngine(cfg)` followed by `engine.Start(ctx)`.

5. **Read and write** – Use `engine.Get` / `engine.Put` (and their batch variants) to interact with the cache.

For a distributed setup, use `NewConfig` and supply a discovery provider
(e.g., [NATS](./discovery/nats/README.md), [Kubernetes](./discovery/kubernetes/README.md),
[Static](./discovery/static/README.md), or [DNS](./discovery/dnssd/README.md)).

A complete working example can be found in the [`example`](./example) directory.

## Engine API

All capabilities are exposed through the [Engine](./engine.go):

| Method | Description |
|---|---|
| `Put` | Store a key/value pair in a keyspace |
| `PutMany` | Store multiple key/value pairs |
| `Get` | Retrieve a key/value pair |
| `GetMany` | Retrieve multiple key/value pairs |
| `Delete` | Remove a key/value pair |
| `DeleteMany` | Remove multiple key/value pairs |
| `DeleteKeySpace` | Delete a keyspace and all entries |
| `DeleteKeyspaces` | Delete multiple keyspaces |
| `UpdateKeySpace` | Replace a keyspace definition at runtime |
| `KeySpaces` | List all keyspaces |

## Contribution

Contributions are welcome.

This project follows [Semantic Versioning](https://semver.org) and [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/).

1. Fork the repository.
2. Create a feature branch.
3. Commit your changes using Conventional Commits.
4. Submit a [pull request](https://help.github.com/articles/using-pull-requests).
