# Shard bytes

`Finish` returns one byte slice that a host can put in object storage and hand to `Open` later. Hosts should treat the slice as opaque. This note is for debugging a bad blob or understanding what lives on disk versus what `Open` builds in memory.

The R-trees are not in the file. `Open` inserts each site's shown box and exact box into `github.com/tidwall/rtree`. Roaring buffers are the portable format from `Bitmap.WriteTo` (the Roaring format spec), not the frozen format. `Open` copies them onto the heap with `ReadFrom`. It does not memory-map the slice, so the same bytes work the same way on API replicas and in wasm.

Integers are little-endian. A string is a `uint32` byte length followed by the bytes. A bitmap is a `uint32` byte length followed by the roaring buffer. Floats are IEEE-754 binary64.

```text
"DS01"                 4 bytes
version                uint16 = 1
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
```

Each forward row, in doc id order:

```text
datastream id, name, station id, station name, unit, measurement, purpose, organization id
open site index    uint32
```

Station name is copied from the station. A site-attached datastream leaves it empty. Site name is not on this row.

A posting section is a `uint32` count, then that many pairs of string key and bitmap. Keys are sorted. Purpose and measurement use the caller's strings. Open-site keys are site ids, and those bitmaps hold the datastreams whose open deployment is that site. Window queries do not use those bitmaps; they scan the interval table.

`Open` rejects a bad magic, a version other than 1, a truncated buffer, or trailing bytes.
