# dendra-search

Search index for datastream discovery. One organization goes in as sites, stations, and datastreams. `Finish` returns bytes. `Open` loads an immutable shard. `Query` answers from that shard.

The caller names the geometry (`shown` or `exact`) and whether member-only datastreams are included. This module does not decide either one. It does not serve traffic, and it does not import protobuf or an object store.

## Install

```bash
go get github.com/dendrascience/dendra-search
```

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

Feed sites, then stations, then datastreams. A datastream names a station, or carries its own intervals when it is attached to a site directly. Intervals are half-open Unix milliseconds. `EndsBefore == 0` means the deployment is still open. Pass intervals that do not overlap. The module does not normalize them.

`Query` applies only the fields you set. An empty measurement list means all measurements. The same is true for sites, purpose, and a nil viewport. `IncludeMemberOnly` false uses `public_ok`. True uses `public_ok ∪ member_ok`.

Zoom 0 is the world. A zero `MapSiteLimit` means 40, and a zero `MinSiteZoom` means 8. Above that limit, or below that zoom, the result is clusters and the facet arrays are empty. Otherwise it is one marker per site, disjunctive counts, and a page of hits.

`Combine` merges one result per organization and clusters the sites once. Query each shard, then combine. Do not hold every shard's query open across the others if the host is pinning one organization at a time.

```go
parts := make([]search.Result, len(shards))
for i, shard := range shards {
    parts[i], err = shard.Query(q)
}
res, err = search.Combine(q, parts)
```

Details: [docs/query.md](docs/query.md). Byte layout: [docs/format.md](docs/format.md).

## Examples

```bash
go run ./examples/demo
go run ./examples/bench
./examples/wasm/build.sh examples/map/search.wasm && go run ./examples/map
```

[examples/README.md](examples/README.md). The map page serves at http://127.0.0.1:8080.

## Development

```bash
CGO_ENABLED=0 go test ./...
go run ./examples/bench    # writes BENCHMARKS.md
```

Tests need no network. Roaring is `github.com/RoaringBitmap/roaring` in portable format, not CRoaring. The R-tree is `github.com/tidwall/rtree`, built in `Open`. Fixtures and the benchmark cases are described in [docs/fixtures.md](docs/fixtures.md). `Finish` duration is recorded and is not a pass/fail number.

## License

BSD-3-Clause. Copyright The Dendra Science Contributors.
