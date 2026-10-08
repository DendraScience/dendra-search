# Examples

Small programs that exercise the library without a network or production data. Start with `demo` if you want a short stdout walkthrough; use `map` if you want to see clusters and facets on a page.

```bash
go run ./examples/demo
go run ./examples/bench
./examples/wasm/build.sh
./examples/wasm/build.sh examples/map/search.wasm && go run ./examples/map
```

## demo

A few sites in memory. One query at zoom 2 returns clusters and measurement counts. One at zoom 11 returns sites. A member-only datastream is visible only when `IncludeMemberOnly` is set. The same builder then writes a public shard: that member row is gone, and the exact point matches shown. The public bytes are framed with `WriteContainer`, and a `VersionFile` is marshaled with `encoding: gzip`. It then builds a north shard and a south shard, prints `GroupIDs` and the picker rows, and checks a volunteers query plus the meadow and lab window. The program exits non-zero if meadow and lab match together. Useful as a first read of the Go API before you open fixtures or the map page.

## bench

Builds the four fixtures and runs the query matrix with `GOMAXPROCS=1`. Writes `BENCHMARKS.md` in the current directory. Pass `-out` to choose another path.

```bash
go run ./examples/bench
go run ./examples/bench -out /tmp/BENCHMARKS.md
```

`Finish` time is printed and is not a gate. Today-scale limits are enforced by `go test`, not by this program. Large-scale numbers are recorded next to 100 ms and 512 MB. Fixture sizes and cases: [docs/fixtures.md](../docs/fixtures.md).

## wasm

Browser wrappers for build and query. See [wasm/README.md](wasm/README.md). The JS helpers are example-only; the stable surface is the Go API. Hosts that ship search in a browser can copy the build script pattern; they should not treat `searchBuild` / `searchQuery` as the product contract.

## map

A static Explore-style page over the fixtures. See [map/README.md](map/README.md).

```bash
./examples/wasm/build.sh examples/map/search.wasm
go run ./examples/map
```

Open http://127.0.0.1:8080 from the repository root. Map tiles come from the network. Search runs in the wasm module.
