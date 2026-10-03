# Query

`Query` evaluates one request against an immutable shard: which datastreams match, which sites those hits land on, and whether the map should show clusters or individual sites. It allocates temporary bitmaps and does not modify the shard. Concurrent calls on the same shard are safe.

Empty fields are left out of the predicate.

| Field | Empty means |
| --- | --- |
| `Purpose` | Every purpose. This is a filter, not a counted facet. |
| `Measurements` | Every measurement. Several values are a union. |
| `SiteIDs` | Every site in play. Several values are a union. |
| `BeginsAt` and `EndsBefore` | The open deployment (`EndsBefore == 0` on the interval). Set both, or neither. |
| `Viewport` | No geographic predicate. Clustering still uses `Geometry`. |
| `IncludeMemberOnly` | `public_ok` only. True uses `public_ok ∪ member_ok`. |
| `MapSiteLimit` | 100 |
| `MinSiteZoom` | 6 |
| `PageSize` | No hit rows. `TotalCount` is still set. |

`Geometry` is required. It is `"shown"` or `"exact"`. The module does not choose which one a caller may see.

Across facets the operator is intersection. Within measurement, and within site, it is union. Checking two measurements returns datastreams that are either one. Checking a measurement and a site returns datastreams that are that measurement and at that site.

Disjunctive counts omit one dimension:

- A site count is the number of hits at that site when the site filter is removed.
- A measurement count is the number of hits for that measurement when the measurement filter is removed.

Purpose stays in both. Measurement counts are returned for both layers, including zeros for measurements that exist in the shard, so selecting one measurement does not hide the others. Site counts are returned only when the layer is sites. Selected sites are listed even when their count is zero.

## Time

Intervals are half-open `[BeginsAt, EndsBefore)` in Unix milliseconds. `EndsBefore == 0` means open. Each datastream has exactly one open interval. Intervals must already be non-overlapping; the builder rejects overlaps rather than merging them.

With no window, a datastream is at its open site. Posting lists for that path are built at `Finish`. With a window, a datastream matches every interval that overlaps the window, including a site it has since left. That path builds temporary bitmaps from the interval table instead of the open-site postings.

## Map layer

Candidate sites are sites with at least one hit. The count is distinct sites, not datastreams.

The layer is `clusters` when the candidate count is above `MapSiteLimit`, or when `Zoom` is below `MinSiteZoom`. Zoom 0 is the world. That layer returns measurement counts and omits the site list, so a zoomed-out request does not return every site. `TotalCount` is still set.

Otherwise the layer is `sites`: one marker per candidate, measurement counts, site counts, and a page of hits.

Clusters are web-mercator cells 48 pixels on a side. The world width at zoom z is `256 * 2^z`. The centroid is the mean of the member digest points in the geometry you named. `Expansion` is the axis-aligned box of those members. A viewport with `East < West` is split at the antimeridian. Area sites are matched by their box, not by substituting the digest point.

Hits are in doc id order. `Query` has no order field. Each hit carries the datastream name, measurement, and station name from the forward row. Station name is empty when the datastream is site-attached. Site name is the site for this query: the open deployment, or the deployment that overlaps the date window. `PageAfter` is the first doc id to return. `NextPageAfter` is the cursor for the next page when `HasMore` is set. It is valid only for this shard's bytes. A rebuild assigns doc ids again. The RPC page token is not this cursor.

## Combine

When a host has queried several organization shards, `Combine` clusters the merged candidates once and sums measurement counts. Site counts are summed when the merged layer is sites. Two nearby sites from different organizations stay two sites, or two members of one cluster. They are not collapsed into a single organization.

`Combine` does not page across shards. Concatenated hits follow the order of the parts you pass. `HasMore` is set when any part has another page.
