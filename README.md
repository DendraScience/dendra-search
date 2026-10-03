# dendra-search

[dendra-search](https://github.com/dendrascience/dendra-search) is the Go library [Dendra](https://dendra.science) uses for datastream discovery. A host passes in the sites, stations, and datastreams for one organization, and the library builds a single immutable index that can answer a faceted filter, a map viewport, and a deployment-interval query. What you get back is bytes. Serving those bytes, and deciding who may download them, belongs to the host.

You fill a `Builder` in a fixed order: sites, then stations, then datastreams. `Finish` serializes that index into a shard, which is the byte slice you store. Later, `Open` loads the slice and builds two R-trees, one on the shown location and one on the exact location. `Query` reads a single shard. A search that covers more than one organization queries each shard and passes the results to `Combine`, which merges them and clusters the sites once.

The same filled builder can produce two shards, and you will usually want both. `Finish(FinishPrivate)` writes the full shard: every datastream, with each site's shown box and exact box stored as you added them. `Finish(FinishPublic)` writes the public shard from those same rows. Only datastreams marked `public_ok` are kept, so a member-only datastream is simply not in the file, and a site appears only when at least one public datastream uses it. The exact box and exact point written there are copies of the shown values. Anyone who can read the file can read every coordinate in it, so the public shard has no true coordinates of its own. Doc ids in that shard are assigned again, following the order the public rows were added.

`Finish` does not throw the index away. After the private shard is written you can call `Finish(FinishPublic)` on the same builder, or the other way around, and a repeated call with the same mode returns the same bytes. You can upload the first buffer and drop it before the second call, which keeps you from holding both files at once. Adding another site, station, or datastream after the first `Finish` returns `ErrFinished`. Drop the builder once you have the slices you need.

On each query you name the geometry, `shown` or `exact`, and whether member-only datastreams should be included. The library will not make that choice for you. It also does not serve traffic, talk to an object store, import protobuf, or gzip. Storing shards, pinning organizations, and enforcing membership stay with the host.

## Install

```bash
go get github.com/dendrascience/dendra-search
```

The module needs Go 1.22 or newer. Its dependencies are portable Roaring bitmaps (`github.com/RoaringBitmap/roaring`) and an R-tree (`github.com/tidwall/rtree`). Build and test with `CGO_ENABLED=0`.

## How a shard is built

Each organization is one shard. Sites go in first, then stations, then datastreams, and the builder rejects a row that arrives out of that order. A site stores a shown box and an exact box, along with the digest points clustering uses. A datastream either names a station, whose deployment intervals were added with that station, or it carries its own intervals when it is attached to a site directly. An interval is a half-open range of Unix milliseconds, `[BeginsAt, EndsBefore)`. `EndsBefore == 0` means the deployment is still open. Pass intervals that do not overlap. The builder returns an error if they do, and it will not merge them for you.

`AddDatastream` copies what it needs into the index and then drops the struct you passed. The row is recorded in the posting lists (purpose, measurement, `public_ok`, `member_ok`, and the site of the open deployment), in the interval table, and in the forward rows that `Query` later turns into hits. `Finish` writes that index out as one blob and leaves the in-memory copy where it is, which is what makes the second `Finish` possible. `Open` reads the blob and inserts each site's shown box and exact box into its own R-tree. The trees are not stored in the file. `Query` intersects the predicates you set, allocates temporary bitmaps only, and is safe for concurrent readers on the same shard.

Leave a query field empty and that predicate is skipped. An empty measurement list means every measurement, and the same is true for sites, for purpose, and for a nil viewport. `IncludeMemberOnly` set to false keeps the `public_ok` rows. Set to true, it uses the union of `public_ok` and `member_ok`. On a public shard that union cannot surface a member-only datastream, because the row was never written.

Zoom 0 is the whole world. A zero `MapSiteLimit` is treated as 100, and a zero `MinSiteZoom` is treated as 6. When the matching sites are above that limit, or the zoom is below that floor, the result is clusters and measurement counts, and the per-site counts stay empty. Otherwise you get one marker per site, both count arrays, and an optional page of hits.

Across organizations, query each shard on its own and then call `Combine`. Clustering happens in that merge, so you should not cluster inside the per-shard query and again afterwards. If the host pins one organization at a time, do not hold every shard's query open across the others.

The query rules are written out in [docs/query.md](docs/query.md). The byte layout, which you want when a blob will not open, is in [docs/format.md](docs/format.md).

## The public container

Once each organization has a public shard, those shards can travel as one object. `WriteContainer` packs them into a single uncompressed frame, and `ParseContainer` takes the frame apart. The first four bytes are `DSPC`. That string is file magic, the same kind of marker as `DS01` at the start of a shard, and the letters do not stand for a longer name.

Inside the frame, each chunk is an organization id, the etag of that organization's public shard, and the shard bytes themselves. The bytes are copied through as they were given. The library does not look for a `DS01` header inside them, and it does not sort the organizations, so the order you pass is the order that gets stored.

The etag is there for the next time the frame is assembled. Parse the frame, replace the chunks whose etag changed, and write it again. `Open` never looks at the etag. Fetching the chunks is the caller's job, and so is compression. If a host stores a gzip copy for download, it compresses outside this package and records that fact on the version file below. A browser `fetch` of an object served with `Content-Encoding: gzip` has already decompressed the body by the time your code sees it, so you call `ParseContainer` on those bytes and do not gunzip them again.

## The version file

`VersionFile` is the JSON object stored as `current.json` beside a shard or beside a container. A reader follows that file and then fetches the object it names, rather than listing a directory and guessing which bin is current. The fields that do the naming are the object key, the etag, the size, and `built_at`.

A pointer for a private shard can also carry `doc_count` and the organization flags `is_enabled`, `is_hidden`, and `public_ok`. Those fields are pointers in the struct, so leaving them unset omits them from the JSON. A container pointer leaves them unset. `keep` lists older object keys that should be spared until `until`, which is how a reader who already resolved the previous pointer can finish that download. `encoding` is omitted when it is empty, and empty means the object is the raw bytes from `Finish` or `WriteContainer`. Setting it to `gzip` only labels the public download. `Marshal` writes JSON and does not compress.

## Usage

A single organization looks like this. `Finish` returns the shard bytes and the number of datastreams written.

```go
b := search.NewBuilder()
err = b.AddSite(site)
err = b.AddStation(station)
err = b.AddDatastream(datastream)
blob, n, err := b.Finish(search.FinishPrivate)

shard, err := search.Open(blob)
res, err := shard.Query(search.Query{
    Purpose:  search.PurposeReadyToUse,
    Geometry: search.GeometryShown,
    Zoom:     2,
    Viewport: &search.Box{West: -180, South: -90, East: 180, North: 90},
})
```

Call `Finish(search.FinishPublic)` on that same builder when you also want the public shard. It returns its own byte slice and its own doc count.

`AddSites`, `AddStations`, and `AddDatastreams` loop the singular methods. If you already have a page from the database, call `AddSite` (or the station or datastream method) once per row and skip rows the index should not contain:

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

Several organizations are queried separately and then merged:

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

`demo` prints a private query and then a public one from the same builder, including a container round trip and a version file. `bench` runs the fixture matrix. The map page is a static Explore-style view over those fixtures, and it serves at http://127.0.0.1:8080. Notes for each program are in [examples/README.md](examples/README.md).

## Development

```bash
CGO_ENABLED=0 go test ./...
go run ./examples/bench    # writes BENCHMARKS.md
```

Tests need no network. The fixtures and the benchmark cases are described in [docs/fixtures.md](docs/fixtures.md). `go test` enforces the today-scale limits, 10 ms per query and 32 MiB of heap for `today-catalog`. How long `Finish` takes is written to `BENCHMARKS.md` and is not a pass or fail. The large-scale numbers are recorded there next to 100 ms and 512 MB.

## License

BSD-3-Clause. Copyright The Dendra Science Contributors.
