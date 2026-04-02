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

Then create a config and start the engine. The example below uses `NewStandaloneConfig` for a single‑node setup
with no cluster discovery:

```go
package main

import (
 "context"
 "encoding/json"
 "fmt"
 "os"
 "os/signal"
 "syscall"
 "time"

 "github.com/tochemey/distcache"
)

type userSource struct{}

func (userSource) Fetch(_ context.Context, key string) ([]byte, error) {
 return json.Marshal(map[string]string{"id": key, "name": "Alice"})
}

type userKeySpace struct{}

func (userKeySpace) Name() string                                    { return "users" }
func (userKeySpace) MaxBytes() int64                                 { return 64 << 20 }
func (userKeySpace) DataSource() distcache.DataSource                { return userSource{} }
func (userKeySpace) ExpiresAt(_ context.Context, _ string) time.Time { return time.Time{} }

func main() {
 ctx := context.Background()

 cfg := distcache.NewStandaloneConfig(
  []distcache.KeySpace{userKeySpace{}},
 )

 engine, err := distcache.NewEngine(cfg)
 if err != nil {
  fmt.Fprintln(os.Stderr, err)
  os.Exit(1)
 }

 if err := engine.Start(ctx); err != nil {
  fmt.Fprintln(os.Stderr, err)
  os.Exit(1)
 }

 // Put a value into the cache
 if err := engine.Put(ctx, "users", &distcache.Entry{
  KV:     distcache.KV{Key: "user:1", Value: []byte(`{"id":"user:1","name":"Alice"}`)},
  Expiry: time.Time{},
 }); err != nil {
  fmt.Fprintln(os.Stderr, "put failed:", err)
 }

 // Get a value from the cache (fetches from DataSource on miss)
 kv, err := engine.Get(ctx, "users", "user:1")
 if err != nil {
  fmt.Fprintln(os.Stderr, "get failed:", err)
 } else {
  fmt.Printf("cached: key=%s value=%s\n", kv.Key, kv.Value)
 }

 // Wait for interrupt
 sig := make(chan os.Signal, 1)
 signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
 <-sig

 _ = engine.Stop(ctx)
}
```

For a distributed setup, replace `NewStandaloneConfig` with `NewConfig` and supply a discovery provider
(e.g., [NATS](./discovery/nats/README.md), [Kubernetes](./discovery/kubernetes/README.md),
[Static](./discovery/static/README.md), or [DNS](./discovery/dnssd/README.md)).

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

## Example

A complete distributed example (using NATS discovery) can be found in the [`example`](./example) directory.

## Contribution

Contributions are welcome.

This project follows [Semantic Versioning](https://semver.org) and [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/).

1. Fork the repository.
2. Create a feature branch.
3. Commit your changes using Conventional Commits.
4. Submit a [pull request](https://help.github.com/articles/using-pull-requests).
