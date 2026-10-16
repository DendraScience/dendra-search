# Query

`Query` evaluates one request against an immutable shard: which datastreams match, which sites those hits land on, and whether the map should show clusters or individual sites. It allocates temporary bitmaps and does not modify the shard. Concurrent calls on the same shard are safe.

Empty fields are left out of the predicate.

| Field | Empty means |
| --- | --- |
| `Purpose` | Every purpose. This is a filter, not a counted facet. |
| `Measurements` | Every measurement. Several values are a union. |
| `SiteIDs` | Every site in play. Several values are a union. |
| `GroupIDs` | Every datastream, including intervals with no group. Several values are a union. |
| `BeginsAt` and `EndsBefore` | The open deployment (`EndsBefore == 0`). A datastream whose deployments have all ended does not match. Set both, or neither. |
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

Purpose stays in both. A group filter stays in both, the way purpose does. Measurement counts are returned for both layers, including zeros for measurements that exist in the shard, so selecting one measurement does not hide the others. Site counts are returned only when the layer is sites. Selected sites are listed even when their count is zero. There is no group count. `Combine` does not sum one.

## Time

Intervals are half-open `[BeginsAt, EndsBefore)` in Unix milliseconds. `EndsBefore == 0` means the deployment is still open, the same as an unset `ends_before` on the wire. A non-zero `EndsBefore` means the deployment has ended. A datastream has at most one open interval. Every interval may be closed. Intervals must already be non-overlapping; the builder rejects overlaps rather than merging them.

With no window, a datastream is at its open site. A fully closed history does not match. Posting lists for that path are built at `Finish`. With a window, a datastream matches every interval that overlaps the window, including one that has ended and a site it has since left. That path builds temporary bitmaps from the interval table instead of the open-site postings.

Site, group, and viewport qualify on the same interval. An interval qualifies when it overlaps the window (or it is the open interval when both bounds are zero), the group filter is empty or `GroupID` is one of `GroupIDs`, the site filter is empty or the interval's site is one of `SiteIDs`, and the viewport is empty or that site's box intersects the viewport. An empty `GroupID` qualifies only when the group filter is empty. The datastream is a hit when any qualifying interval exists and the datastream predicates pass. A window that covers meadow with group field and later ridge with group lab does not match site meadow and group lab.

On the open path the open-site and open-group postings are that test, and a datastream with no open interval is absent. On the window path the per-site bitmaps are built from intervals that already pass time, group, and viewport, including intervals that have ended. The site filter is a union of those bitmaps. A site count under a group filter counts datastreams with a qualifying interval at that site.

The hit's site is a qualifying interval: the open site, or, for a window, the overlapping interval with the latest `BeginsAt` that also passes group, site, and viewport.

```text
GroupIDs(beginsAt, endsBefore int64, includeMemberOnly bool) ([]string, error)
```

`GroupIDs` returns the distinct group ids on in-play intervals of datastreams the caller may see. Sorted. Empty group ids are omitted. There is no count. It does not apply purpose, measurement, site, viewport, or `Query.GroupIDs`. `includeMemberOnly` false uses `public_ok`. True uses `public_ok ∪ member_ok`. Open-group keys are intersected with that access bitmap.

`ReducePicker` turns the host's catalogs, the checked ids, and the per-organization `GroupIDs` results into the two picker lists. The rules are in [groups.md](groups.md).

## Map layer

Candidate sites are sites with at least one hit. The count is distinct sites, not datastreams.

The layer is `clusters` when the candidate count is above `MapSiteLimit`, or when `Zoom` is below `MinSiteZoom`. Zoom 0 is the world. That layer returns measurement counts and omits the site list, so a zoomed-out request does not return every site. `TotalCount` is still set.

Otherwise the layer is `sites`: one marker per candidate, measurement counts, site counts, and a page of hits.

Clusters are web-mercator cells 48 pixels on a side. The world width at zoom z is `256 * 2^z`. The centroid is the mean of the member digest points in the geometry you named. `Expansion` is the axis-aligned box of those members. A viewport with `East < West` is split at the antimeridian. Area sites are matched by their box, not by substituting the digest point.

Hits are in doc id order. `Query` has no order field. Each hit carries the datastream name, measurement, and station name from the forward row. Station name is empty when the datastream is site-attached. Site name is the site for this query: the open deployment, or the deployment that overlaps the date window. `PageAfter` is the first doc id to return. `NextPageAfter` is the cursor for the next page when `HasMore` is set. It is valid only for this shard's bytes. A rebuild assigns doc ids again. The RPC page token is not this cursor.

## Combine

When a host has queried several organization shards, `Combine` clusters the merged candidates once and sums measurement counts. Site counts are summed when the merged layer is sites. Two nearby sites from different organizations stay two sites, or two members of one cluster. They are not collapsed into a single organization.

`Combine` does not page across shards. Concatenated hits follow the order of the parts you pass. `HasMore` is set when any part has another page.
