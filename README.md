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

## Table of Contents

- [Features](#features)
- [Installation](#installation)
- [Engine](#engine)
  - [Core Methods](#core-methods)
  - [KeySpace Management](#keyspace-management)
- [Observability](#observability)
- [Admin & Diagnostics](#-admin--diagnostics)
- [Example Requests](#example-requests)
- [Warmup & Hot Keys](#warmup--hot-keys)
  - [Behavior](#behavior)
- [DataSource Protection](#datasource-protection)
  - [Error Semantics](#error-semantics)
- [KeySpace Defaults & Overrides](#keyspace-defaults--overrides)
  - [Precedence](#precedence)
- [How It Works](#how-it-works)
- [Use Cases](#use-cases)
- [Get Started](#get-started)
  - [Quick Start](#quick-start)
  - [Configuration Highlights](#configuration-highlights)
  - [DataSource](#datasource)
  - [KeySpace](#keyspace)
- [Production Notes](#production-notes)
- [Example](#example)
- [Contribution](#contribution)

## Features

- ⚡️ **Automatic fetch on miss** – Data is loaded into the cache only when requested.
- 🌍 **Distributed architecture** – Data is sharded across nodes for scalability and availability.
- 🧠 **Reduced backend load** – Frequent reads are served from the cache instead of the database.
- ⏳ **Configurable expiry & eviction** – Support for TTL, LRU, and custom policies.
- 🛰️ **Automatic node discovery** – Nodes automatically react to cluster topology changes.
- 🧩 **KeySpace overrides** – Per‑keyspace TTL, timeouts, max bytes, warm keys, and protections.
- 🔁 **Dynamic keyspace updates** – Replace keyspaces at runtime via `UpdateKeySpace`.
- 🌶️ **Warmup & hot key tracking** – Prefetch hot keys on join/leave events.
- 🧯 **DataSource protection** – Rate limiting and circuit breaking, globally or per keyspace.
- 🧪 **Admin diagnostics** – JSON endpoints for peers and keyspace stats.
- 📈 **Observability** – OpenTelemetry metrics and tracing around engine operations.
- 🔐 **TLS support** – End‑to‑end encrypted communication between nodes.
- 🧰 **Discovery provider** – Implement custom discovery backends or use the built‑in ones:
  - [Kubernetes](./discovery/kubernetes/README.md) – discover peers via the Kubernetes API.
  - [NATS](./discovery/nats/README.md) – discover peers via [NATS](https://github.com/nats-io/nats.go).
  - [Static](./discovery/static/README.md) – fixed list of peers, ideal for tests and demos.
  - [DNS](./discovery/dnssd/README.md) – discover peers via Go’s DNS resolver.

## Installation

```bash
go get github.com/tochemey/distcache
```

## Engine

All core capabilities are exposed through the DistCache [Engine](./engine.go), which provides a simple API for
interacting with the cache.

### Core Methods

- **Put** – Store a single key/value pair in a given keyspace.
- **PutMany** – Store multiple key/value pairs in a given keyspace.
- **Get** – Retrieve a specific key/value pair from a given keyspace.
- **GetMany** – Retrieve multiple key/value pairs in a given keyspace.
- **Delete** – Remove a specific key/value pair from a given keyspace.
- **DeleteMany** – Remove multiple key/value pairs from a given keyspace.

### KeySpace Management

- **DeleteKeySpace** – Delete a single keyspace and all of its entries.
- **DeleteKeyspaces** – Delete multiple keyspaces at once.
- **UpdateKeySpace** – Replace a keyspace definition at runtime (recreates the group and can trigger warmup).
- **KeySpaces** – List all available keyspaces.

The Engine is designed to provide high‑performance operations across a distributed cluster while keeping the API
simple and predictable.

## Observability

DistCache ships with OpenTelemetry instrumentation for engine operations:

- **Metrics** – Counters and latency histograms for `Put`, `Get`, `Delete`, `GetMany`, `DeleteMany`, etc.
- **Tracing** – Spans tagged with `distcache.operation` and `distcache.keyspace`.

Enable it using `WithMetrics` and `WithTracing` in your [`Config`](./config.go).

## Admin & Diagnostics

DistCache exposes lightweight HTTP endpoints for diagnostics and operational visibility.

- **Default base path**: `/_distcache/admin`
- **Endpoints**:
  - `GET /peers` – returns cluster peers.
  - `GET /keyspaces` – returns keyspace snapshots, cache sizes, and optional stats.

Enable the admin server with:

```go
config := distcache.NewConfig(
    provider,
    keyspaces,
    distcache.WithAdminServer("127.0.0.1:9090"),
    // or
    distcache.WithAdminConfig(admin.Config{
        ListenAddr: "127.0.0.1:9090",
        BasePath:   "/_distcache/admin",
    }),
)
```

Use `admin.Config` from the `github.com/tochemey/distcache/admin` package for advanced settings.

Keyspace stats include cache hits, loads, peer errors, and remove‑key counters when supported by the underlying
group implementation.

## Example Requests

```bash
curl -s http://127.0.0.1:9090/_distcache/admin/peers
curl -s http://127.0.0.1:9090/_distcache/admin/keyspaces
```

Example response for `GET /keyspaces`:

```json
[
  {
    "name": "users",
    "max_bytes": 67108864,
    "default_ttl": "5m0s",
    "read_timeout": "250ms",
    "write_timeout": "500ms",
    "warm_keys": ["user:1", "user:2"],
    "main_cache_bytes": 2048,
    "hot_cache_bytes": 256,
    "stats": {
      "gets": 120,
      "cache_hits": 98,
      "peer_loads": 5,
      "peer_errors": 0,
      "loads": 22,
      "loads_deduped": 7,
      "local_loads": 17,
      "local_load_errs": 0,
      "remove_keys_requests": 1,
      "removed_keys": 3
    }
  }
]
```

## Warmup & Hot Keys

Warmup prefetches hot keys when the cluster topology changes (join/leave). DistCache tracks hot keys during reads
and combines them with explicit warm keys configured per keyspace.

Enable warmup with the `warmup` package:

```go
config := distcache.NewConfig(
    provider,
    keyspaces,
    distcache.WithWarmup(warmup.Config{
        MaxHotKeys:  100,
        MinHits:     1,
        Concurrency: 4,
        Timeout:     2 * time.Second,
        WarmOnJoin:  true,
        WarmOnLeave: true,
    }),
)
```

The warmup configuration lives in `github.com/tochemey/distcache/warmup`.

### Behavior

- Warmup triggers on cluster **join** and/or **leave** events (configurable).
- Prefetch keys include **explicit warm keys** plus **hot keys** observed at runtime.
- Prefetch concurrency is bounded by the warmup config.
- Each prefetch uses the smaller of the warmup timeout and the keyspace read timeout.

## DataSource Protection

Protect upstream dependencies with rate limiting and circuit breaking:

- **Engine‑level** – `WithRateLimiter`, `WithCircuitBreaker`
- **Per‑keyspace overrides** – `WithKeySpaceRateLimiter`, `WithKeySpaceCircuitBreaker`

These protections guard `DataSource.Fetch` calls and reduce load during spikes or outages.

### Error Semantics

- **`WaitTimeout == 0`** → immediate allow/deny; denied requests return `ErrDataSourceRateLimited`.
- **`WaitTimeout > 0`** → waits up to the timeout; if exceeded, you may see `context.DeadlineExceeded`.
- **Circuit breaker open** → returns `ErrDataSourceCircuitOpen`.

## KeySpace Defaults & Overrides

KeySpaces can override engine defaults without changing the `KeySpace` implementation:

- `WithKeySpaceMaxBytes`
- `WithKeySpaceDefaultTTL`
- `WithKeySpaceReadTimeout`
- `WithKeySpaceWriteTimeout`
- `WithKeySpaceWarmKeys`
- `WithKeySpaceRateLimiter`
- `WithKeySpaceCircuitBreaker`

Overrides are validated at startup to catch invalid TTLs, timeouts, or limits early.

### Precedence

- **MaxBytes**: `WithKeySpaceMaxBytes` overrides `KeySpace.MaxBytes`.
- **Read/Write timeouts**: keyspace overrides take precedence; otherwise engine defaults.
- **Default TTL**: applied only when `KeySpace.ExpiresAt` returns zero.
- **DataSource protection**: keyspace rate limiter / circuit breaker override engine-level settings.

## How It Works

1. **Cache lookup** – The application requests data from DistCache.
2. **Cache hit** – If the data is present, it is returned immediately.
3. **Cache miss**:
   - DistCache fetches data from the configured primary data source.
   - The data is stored in the cache according to the keyspace configuration.
   - The data is returned to the caller.
4. **Subsequent requests** – Future reads for the same key are served from the cache until the entry expires or is
   evicted by the eviction policy.

## Use Cases

- **High‑traffic applications** – Reduce database load (e‑commerce, social networks, analytics dashboards).
- **API response caching** – Cache expensive or frequently called external APIs.
- **Session and profile caching** – Keep user session or profile data close to your application across nodes.

## Get Started

To integrate DistCache, configure a [`Config`](./config.go) and implement **two interfaces** before starting the
[Engine](./engine.go):

- [`DataSource`](#datasource) – Fetches data on cache misses.
- [`KeySpace`](#keyspace) – Defines the cache namespace and storage behavior.

### Quick Start

```go
package main

import (
	"context"
	"time"

	"github.com/tochemey/distcache"
	"github.com/tochemey/distcache/admin"
	"github.com/tochemey/distcache/discovery/static"
	"github.com/tochemey/distcache/warmup"
)

type userSource struct{}

func (userSource) Fetch(_ context.Context, _ string) ([]byte, error) {
	return []byte("ok"), nil
}

type userKeySpace struct {
	source distcache.DataSource
}

func (k userKeySpace) Name() string                                 { return "users" }
func (k userKeySpace) MaxBytes() int64                               { return 64 << 20 }
func (k userKeySpace) DataSource() distcache.DataSource             { return k.source }
func (k userKeySpace) ExpiresAt(context.Context, string) time.Time  { return time.Time{} }

func main() {
	provider := static.NewDiscovery(&static.Config{
		Addresses: []string{"127.0.0.1:3320"},
	})

	cfg := distcache.NewConfig(
		provider,
		[]distcache.KeySpace{userKeySpace{source: userSource{}}},
		distcache.WithAdminConfig(admin.Config{ListenAddr: "127.0.0.1:9090"}),
		distcache.WithWarmup(warmup.Config{WarmOnJoin: true, WarmOnLeave: true}),
	)

	engine, err := distcache.NewEngine(cfg)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	if err := engine.Start(ctx); err != nil {
		panic(err)
	}
	defer engine.Stop(ctx)
}
```

### Configuration Highlights

```go
config := distcache.NewConfig(
    provider,
    []distcache.KeySpace{usersKeySpace, sessionsKeySpace},
    distcache.WithAdminServer("0.0.0.0:9090"),
    distcache.WithWarmup(warmup.Config{
        MaxHotKeys:  200,
        MinHits:     3,
        Concurrency: 8,
        Timeout:     500 * time.Millisecond,
        WarmOnJoin:  true,
        WarmOnLeave: true,
    }),
    distcache.WithRateLimiter(distcache.RateLimitConfig{
        RequestsPerSecond: 100,
        Burst:             200,
        WaitTimeout:       50 * time.Millisecond,
    }),
    distcache.WithCircuitBreaker(distcache.CircuitBreakerConfig{
        FailureThreshold: 5,
        ResetTimeout:     10 * time.Second,
    }),
    distcache.WithKeySpaceDefaultTTL("users", 5*time.Minute),
    distcache.WithKeySpaceReadTimeout("users", 250*time.Millisecond),
    distcache.WithKeySpaceWarmKeys("users", []string{"user:1", "user:2"}),
)
```

### DataSource

[`DataSource`](./datasource.go) defines how to fetch data on a cache miss. This can be a database, REST API, gRPC
service, filesystem, or any other backend.

### KeySpace

[`KeySpace`](./keyspace.go) defines a logical namespace for grouping key/value pairs. It controls metadata such as:

- Keyspace name
- Storage constraints
- Expiration and eviction behavior

KeySpaces are loaded during DistCache bootstrap and dictate how data is partitioned and managed in the cluster.

## Production Notes

- **Bootstrap and discovery** – Set `BootstrapTimeout`, `JoinRetryInterval`, and `MinimumPeersQuorum` to match your
  environment and discovery backend behavior.
- **Admin endpoints** – Protect diagnostics endpoints behind network ACLs or a reverse proxy.
- **Timeouts** – Keep read/write timeouts and `DataSource` timeouts aligned to avoid cascading delays.
- **Warmup tuning** – Tune `Concurrency` and `Timeout` to avoid flooding upstreams during topology changes.
- **Rate limiting** – Use `WaitTimeout` to bound latency; `0` means immediate deny when tokens are exhausted.
- **Dynamic updates** – `UpdateKeySpace` recreates the underlying group; consider warmup to rehydrate hot keys.

## Example

A complete example can be found in the [`example`](./example) directory.

## Contribution

Contributions are welcome.

This project follows:

- [Semantic Versioning](https://semver.org)
- [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/)

To contribute:

1. Fork the repository.
2. Create a feature branch.
3. Commit your changes using Conventional Commits.
4. Submit a [pull request](https://help.github.com/articles/using-pull-requests).
