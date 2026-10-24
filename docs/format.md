# Shard bytes

`Finish` returns one byte slice that a host can put in object storage and hand to `Open` later. Hosts should treat the slice as opaque. This note is for debugging a bad blob or understanding what lives on disk versus what `Open` builds in memory.

The R-trees are not in the file. `Open` inserts each site's shown box and exact box into `github.com/tidwall/rtree`. Roaring buffers are the portable format from `Bitmap.WriteTo` (the Roaring format spec), not the frozen format. `Open` copies them onto the heap with `ReadFrom`. It does not memory-map the slice, so the same bytes work the same way on API replicas and in wasm.

Integers are little-endian. A string is a `uint32` byte length followed by the bytes. A bitmap is a `uint32` byte length followed by the roaring buffer. Floats are IEEE-754 binary64.

```text
"DS01"                 4 bytes
version                uint16 = 3
docCount               uint32
siteCount              uint32
site records           siteCount times
intervalCount          uint32
intervals
forward rows           docCount times
purpose postings
measurement postings
public_ok              one bitmap
member_ok              one bitmap
open-site postings
open-group postings
```

Each site record:

```text
id, name, organization id, organization name
shown box     west, south, east, north
exact box     west, south, east, north
shown point   longitude, latitude
exact point   longitude, latitude
```

Each interval:

```text
doc id          uint32
site index      uint32   // index into the site records
begins_at       int64    // Unix milliseconds
ends_before     int64    // 0 means open
group id        string   // empty means no group
```

Each forward row, in doc id order:

```text
datastream id, name, station id, station name, unit, measurement, purpose, organization id
open site index    uint32   // ^uint32(0) means every deployment has ended
field name count   uint32
field names        count times, each a string
```

Station name is copied from the station. A site-attached datastream leaves it empty. Site name is not on this row. Field names are the datastream's output field names, in order. A count of zero means the datastream has none.

A posting section is a `uint32` count, then that many pairs of string key and bitmap. Keys are sorted. Purpose and measurement use the caller's strings. Open-site keys are site ids, and those bitmaps hold the datastreams whose open deployment is that site. A datastream whose deployments have all ended is omitted. Open-group keys are group ids, and those bitmaps hold the datastreams whose open interval carries that id. Empty group ids are omitted. Window queries do not use the open-site or open-group bitmaps; they scan the interval table, including intervals that have ended. The open-group bitmaps are a filter accelerator, not a counted facet.

`Open` rejects a bad magic, a version other than 3, a truncated buffer, or trailing bytes.

`Finish(FinishPrivate)` writes the layout above. `Finish(FinishPublic)` writes a second `DS01` blob from the same index. Exact boxes and exact points in that blob are copies of the shown values. Only `public_ok` datastreams are stored, and a site is stored only when one of those datastreams uses it. Doc ids and site indexes in the public blob are renumbered. Member-only datastreams are absent.

## Container

`WriteContainer` frames public shards. `ParseContainer` is the reader. Chunk bytes are not inspected. Integers are little-endian, and a string is a `uint32` length plus bytes, matching the shard.

```text
"DSPC"                 4 bytes
version                uint16 = 1
chunkCount             uint32
chunkCount times:
    org id             string
    chunk etag         string
    chunk length       uint32
    chunk bytes
```

`ParseContainer` rejects a bad magic, a version other than 1, a truncated buffer, a string or chunk longer than 64 MiB, or trailing bytes.

## Version file

`VersionFile` is the JSON object a host stores as `current.json`. It is not part of the shard or the container. `encoding` is omitted when empty, and empty means the named object is raw bytes. `"gzip"` is a label. This package does not compress. `doc_count`, `is_enabled`, `is_hidden`, and `public_ok` are omitted when unset, so a frame pointer can leave them out while a private pointer can still record `is_hidden: false`. `keep` lists version bins to spare until `until`.
