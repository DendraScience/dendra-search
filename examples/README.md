# Examples

```bash
go run ./examples/demo
go run ./examples/bench
./examples/wasm/build.sh
./examples/wasm/build.sh examples/map/search.wasm && go run ./examples/map
```

## demo

A few sites in memory. One query at zoom 2 returns clusters. One at zoom 11 returns sites. A member-only datastream is visible only when `IncludeMemberOnly` is set.

## bench

Builds the four fixtures and runs the query matrix with `GOMAXPROCS=1`. Writes `BENCHMARKS.md` in the current directory. Pass `-out` to choose another path.

```bash
go run ./examples/bench
go run ./examples/bench -out /tmp/BENCHMARKS.md
```

`Finish` time is printed and is not a gate. Today-scale limits are enforced by `go test`, not by this program. Large-scale numbers are recorded next to 100 ms and 512 MB.

## wasm

Browser wrappers for build and query. See [wasm/README.md](wasm/README.md). The JS helpers are example-only; the stable surface is the Go API.

## map

A static Explore-style page over the fixtures. See [map/README.md](map/README.md).

```bash
./examples/wasm/build.sh examples/map/search.wasm
go run ./examples/map
```

Open http://127.0.0.1:8080 from the repository root. Map tiles come from the network. Search runs in the wasm module.
