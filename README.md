# DistCache

![GitHub Actions Workflow Status](https://img.shields.io/github/actions/workflow/status/Tochemey/distcache/build.yml)
[![codecov](https://codecov.io/gh/Tochemey/distcache/graph/badge.svg?token=0eS0QphVUH)](https://codecov.io/gh/Tochemey/distcache)
[![GitHub go.mod Go version](https://badges.chse.dev/github/go-mod/go-version/Tochemey/distcache)](https://go.dev/doc/install)
[![Go Reference](https://pkg.go.dev/badge/github.com/tochemey/distcache.svg)](https://pkg.go.dev/github.com/tochemey/distcache)

DistCache is a **distributed read-through cache engine** built in [Go](https://go.dev/).

In a read-through cache, the cache sits between your application and the data source. When the application requests
data:

- If the data is in the cache (**cache hit**), it is returned immediately.
- If the data is not in the cache (**cache miss**), DistCache fetches it from the primary data source (database, API, etc.),
  stores it in the cache, and returns it to the caller.

This reduces direct load on your backend, lowers latency, and improves scalability.

The caching engine is powered by the battle-tested [groupcache-go](https://github.com/groupcache/groupcache-go).

## Features

- **Automatic fetch on miss**: data is loaded into the cache only when requested.
- **Distributed architecture**: data is sharded across nodes for scalability and availability.
- **Reduced backend load**: frequent reads are served from the cache instead of the database.
- **TTL and LRU eviction**: per-entry and per-keyspace TTL; bounded by per-keyspace `MaxBytes` with LRU eviction provided by groupcache. Optional negative caching via `WithKeySpaceNegativeTTL`.
- **Automatic node discovery**: nodes automatically react to cluster topology changes.
- **KeySpace overrides**: per-keyspace TTL, timeouts, max bytes, warm keys, and protections.
- **Dynamic keyspace updates**: replace keyspaces at runtime via `UpdateKeySpace`.
- **Warmup and hot key tracking**: prefetch hot keys on join/leave events, with optional periodic refresh-ahead via `warmup.Config.RefreshInterval`.
- **DataSource protection**: rate limiting and circuit breaking, globally or per keyspace.
- **Cluster events**: subscribe to peer-joined / left / updated notifications via `Engine.Events()`.
- **Admin diagnostics**: JSON endpoints for peers and keyspace stats, plus `/healthz` and `/readyz` probes.
- **Observability**: OpenTelemetry tracing and metrics for engine operations, cache misses, and DataSource fetch latency.
- **TLS and gossip auth**: end-to-end encrypted communication between nodes; optional symmetric `WithGossipSecret` to authenticate cluster membership.
- **Discovery providers**: built-in support for:
  - [Kubernetes](./discovery/kubernetes/README.md): discover peers via the Kubernetes API.
  - [NATS](./discovery/nats/README.md): discover peers via [NATS](https://github.com/nats-io/nats.go).
  - [Static](./discovery/static/README.md): fixed list of peers, ideal for tests and demos.
  - [DNS](./discovery/dnssd/README.md): discover peers via Go's DNS resolver.
  - [Standalone](./discovery/standalone): single-node, no cluster discovery.

## Installation

```bash
go get github.com/tochemey/distcache
```

## Quick Start

Integrate DistCache by implementing two interfaces:

- [`DataSource`](./datasource.go): fetches data from your backend on cache misses.
- [`KeySpace`](./keyspace.go): defines a cache namespace, storage limit, and expiration behavior.

Then create a config and start the engine:

1. **Implement `DataSource`**: provide a `Fetch(ctx, key) ([]byte, error)` method that retrieves data from your backend (database, API, etc.) when a cache miss occurs.

2. **Implement `KeySpace`**: define a cache namespace by returning its name, maximum byte capacity, the `DataSource` to use on misses, and an optional per-key expiration time.

3. **Create a config**: use `NewStandaloneConfig` for single-node setups or `NewConfig` with a [discovery provider](#discovery-providers) for distributed clusters.

4. **Start the engine**: call `distcache.NewEngine(cfg)` followed by `engine.Start(ctx)`.

5. **Read and write**: use `engine.Get` / `engine.Put` (and their batch variants) to interact with the cache.

For a distributed setup, use `NewConfig` and supply a discovery provider
(e.g., [NATS](./discovery/nats/README.md), [Kubernetes](./discovery/kubernetes/README.md),
[Static](./discovery/static/README.md), or [DNS](./discovery/dnssd/README.md)).

Two runnable examples are provided:

- [`example`](./example): a distributed setup using NATS for peer discovery.
- [`example/advanced`](./example/advanced): a single-node walkthrough of the
  optional features: negative caching, periodic refresh-ahead, cluster event
  subscription, gossip authentication, and the admin server.

## Engine API

All capabilities are exposed through the [Engine](./engine.go):

| Method            | Description                              |
|-------------------|------------------------------------------|
| `Put`             | Store a key/value pair in a keyspace     |
| `PutMany`         | Store multiple key/value pairs           |
| `Get`             | Retrieve a key/value pair                |
| `GetMany`         | Retrieve multiple key/value pairs        |
| `Delete`          | Remove a key/value pair                  |
| `DeleteMany`      | Remove multiple key/value pairs          |
| `DeleteKeySpace`  | Delete a keyspace and all entries        |
| `DeleteKeyspaces` | Delete multiple keyspaces                |
| `UpdateKeySpace`  | Replace a keyspace definition at runtime |
| `KeySpaces`       | List all keyspaces                       |
| `Events`          | Subscribe to cluster-membership events   |

## Consistency

> **Note:** DistCache is **eventually consistent**. It is built for fast reads
> with bounded staleness, not for linearizable or transactional workloads.

Per-operation contract:

- **`Get`**: returns the most recently observed value at the queried node.
  That value may briefly lag the writer or the source-of-truth.
- **`Put`** / **`PutMany`**: returns once the key's owner has accepted the
  write. The new value is then asynchronously fanned out to every other peer.
  Failures on non-owner peers are logged and not retried.
- **`Delete`** / **`DeleteMany`**: RPCs the owner and every other peer.
  Returns a multi-error if any peer is unreachable. Surviving peers may serve
  the stale value until TTL or LRU eviction.
- **`DeleteKeySpace`** / **`UpdateKeySpace`**: local to the calling node.
  Re-issue the call on every node to roll out cluster-wide.

What this means in practice:

- After a `Put`, a read on the same node sees the new value immediately. A
  read on a different peer sees it within milliseconds in steady state, later
  if the fan-out RPC failed.
- During a network partition, each side accepts reads and writes independently.
  There is no quorum and no fencing. Set TTLs short enough to bound the
  staleness window after the partition heals.
- If you write to your source-of-truth out of band, the cache continues to
  serve the old value until TTL expires or until you call `Engine.Delete`.

DistCache is a good fit for read-heavy workloads that tolerate seconds of
staleness. It is not a fit for linearizable reads, counters, or anything
requiring a strict order of writes.

## Contribution

Contributions are welcome.

This project follows [Semantic Versioning](https://semver.org) and [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/).

1. Fork the repository.
2. Create a feature branch.
3. Commit your changes using Conventional Commits.
4. Submit a [pull request](https://help.github.com/articles/using-pull-requests).
