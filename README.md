# dendra-search

Go library for [Dendra](https://dendra.science) datastream discovery. Faceted filters, a map viewport, and deployment intervals share one immutable index per organization.

A host feeds sites, stations, and datastreams into a `Builder`. `Finish` returns a byte slice. `Open` loads those bytes into a read-only `Shard` and builds the shown and exact R-trees. `Query` answers from that shard. `Combine` merges one result per organization when a caller looks across several orgs.

This is the search algebra only. It does not serve traffic, talk to an object store, import protobuf, or decide who may see a site. The caller names the geometry (`shown` or `exact`) and whether member-only datastreams are included. Storing shards, pinning organizations, and enforcing membership are host concerns.

## Install

```bash
go get github.com/dendrascience/dendra-search
```

Requires Go 1.22+. Dependencies are portable Roaring bitmaps (`github.com/RoaringBitmap/roaring`) and an R-tree (`github.com/tidwall/rtree`). Build and test with `CGO_ENABLED=0`.

## How it works

Each organization is one shard. Feed sites first, then stations, then datastreams. Sites store both a shown box and an exact box (plus digest points used for clustering). A datastream either names a station, or carries its own deployment intervals when it is attached to a site directly. Intervals are half-open Unix milliseconds; `EndsBefore == 0` means the deployment is still open. Pass non-overlapping intervals. The builder rejects overlaps; it does not merge them.

`AddDatastream` records the row into the posting lists (purpose, measurement, `public_ok` / `member_ok`, open-site), the interval table, and the forward rows. The datastream struct is not kept. `Finish` writes that index into one blob. `Open` unmarshals that blob and inserts each site's shown and exact boxes into R-trees. `Query` intersects the predicates you set, allocates temporary bitmaps only, and is safe for concurrent readers on the same shard.

Empty query fields are left out of the predicate. An empty measurement list means all measurements; the same is true for sites, purpose, and a nil viewport. `IncludeMemberOnly` false uses `public_ok`; true uses `public_ok ∪ member_ok`.

Zoom 0 is the world. A zero `MapSiteLimit` means 40, and a zero `MinSiteZoom` means 8. Above that site limit, or below that zoom, the result is clusters and the facet arrays are empty. Otherwise it is one marker per site, disjunctive counts, and an optional page of hits.

When looking across organizations, query each shard, then `Combine`. That merges results and clusters the sites once. Do not hold every shard's query open across the others if the host is pinning one organization at a time.

Details: [docs/query.md](docs/query.md). Byte layout (for debugging a bad blob): [docs/format.md](docs/format.md).

## Usage

```go
b := search.NewBuilder()
err = b.AddSite(site)
err = b.AddStation(station)
err = b.AddDatastream(datastream)
blob, n, err := b.Finish()

shard, err := search.Open(blob)
res, err := shard.Query(search.Query{
    Purpose:  search.PurposeReadyToUse,
    Geometry: search.GeometryShown,
    Zoom:     2,
    Viewport: &search.Box{West: -180, South: -90, East: 180, North: 90},
})
```

`AddSites`, `AddStations`, and `AddDatastreams` loop the singular methods. A host that already has a page should call `AddSite` (or station, or datastream) once per row:

```go
for _, row := range sitePage {
    if !row.Enabled || row.Hidden {
        continue
    }
    err = b.AddSite(search.Site{
        ID: row.ID, Name: row.Name,
        Shown: row.Shown, Exact: row.Exact,
        ShownPoint: row.ShownPoint, ExactPoint: row.ExactPoint,
        OrganizationID: row.OrganizationID, OrganizationName: row.OrganizationName,
    })
}
```

Across organizations:

```go
parts := make([]search.Result, len(shards))
for i, shard := range shards {
    parts[i], err = shard.Query(q)
}
res, err = search.Combine(q, parts)
```

## Examples

```bash
go run ./examples/demo
go run ./examples/bench
./examples/wasm/build.sh examples/map/search.wasm && go run ./examples/map
```

The map page serves at http://127.0.0.1:8080. See [examples/README.md](examples/README.md) for demo, bench, wasm, and map notes.

## Development

```bash
CGO_ENABLED=0 go test ./...
go run ./examples/bench    # writes BENCHMARKS.md
```

Tests need no network. Fixtures and the benchmark cases are described in [docs/fixtures.md](docs/fixtures.md). Today-scale limits (10 ms per query, 32 MiB heap for `today-catalog`) are enforced by `go test`. `Finish` duration is recorded in `BENCHMARKS.md` and is not a pass/fail number; large-scale numbers sit next to 100 ms and 512 MB.

## License

BSD-3-Clause. Copyright The Dendra Science Contributors.
