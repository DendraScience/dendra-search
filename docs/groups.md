# Groups

A group is a catalog record with an id, in the same sense as an organization: many of them, defined in advance, eventually a page of its own. It is not a facet. The search module stores the id on a deployment interval and answers two questions: which groups those intervals reference, and which datastreams match a selected set of groups. Names live in metadata. The host lists them.

## Group resource

Metadata (not this module) owns the group record. The shipped message is `Group`: `id`, `name`, `is_enabled`, `is_hidden`, and the profile fields beside those. Search reads the id off the deployment. It does not read the name.

`Deployment.group_id` is optional. Empty means that interval has no group. A non-empty id is a group that existed and was not deleted when the deployment was saved. That check is not an `is_enabled` check. A group does not grant membership and does not change who may download a shard.

`ListGroups` is the name catalog. The Explore page asks for `is_enabled` true and `is_hidden` false. `ListOrganizations` is unchanged: a signed-in user sees the orgs they may see today, and a guest sees public orgs.

Renaming, hiding, or enabling a group does not rebuild shards, because the shard stores the id. Setting or clearing `group_id` on a deployment does, because that write is a station or datastream change.

The association is the deployment, not the site and not the station as a whole. Station-attached datastreams take intervals from the station. Site-attached datastreams, including manual uploads at a site, take intervals from the datastream. Either interval may carry a `group_id`. One site can host streams whose groups differ. One group can appear in many organizations.

## What the shard stores

`Interval` has an optional `GroupID`. Empty is allowed. The builder does not require the group to have been added to the shard, and it does not store the name. Sites are the parent check. Groups are global ids.

Each stored interval keeps the group id beside the site index and the time bounds. `Finish` also writes open-group postings: group id to the bitmap of docs whose open interval (`EndsBefore == 0`) carries that id. Empty ids are omitted. Those postings are a filter accelerator for the open path. They are not a counted facet.

The shard format version is 2. `Open` rejects any other version. [format.md](format.md) describes the interval layout and the open-group section.

## Time

No window means the open interval, when the datastream has one. A deployment that has ended does not match. A window means every interval that overlaps `[BeginsAt, EndsBefore)`, including intervals that have ended. A datastream has at most one open interval. Overlaps are rejected at build time.

Group follows the interval. The open interval contributes its group, or none. A window can contribute several groups for one datastream when successive deployments name different groups. A gap contributes none.

## Query filter

`Query.GroupIDs` is a filter. Empty means every datastream, including those whose in-play intervals have no group. Several ids are a union: the datastream matches if one qualifying interval carries any selected id.

`Result` has no group count list. `Combine` does not sum one. Measurement counts and site counts stay the facets they are, including zeros for measurements that exist in the shard, and site counts when the layer is sites.

Purpose, measurement, and access stay on the datastream. Site, group, and viewport are on the interval, and they qualify together. An interval qualifies when all of these hold:

- It overlaps the window, or it is the open interval when both bounds are zero.
- The group filter is empty, or `GroupID` is one of `GroupIDs`. An empty `GroupID` qualifies only when the group filter is empty.
- The site filter is empty, or the interval's site is one of `SiteIDs`.
- The viewport is empty, or that site's box intersects the viewport.

The doc is in the hit set when any qualifying interval exists and the datastream-level predicates pass (access, purpose, measurement).

This joint test is required. A window that covers two deployments of one datastream, meadow with group field and later ridge with group lab, must not match site meadow and group lab. Those values never occur on the same interval. ANDing a site bitmap built from every overlapping interval with a group bitmap built the same way would accept that pair. The open path hides the bug because a datastream has at most one open interval.

The group filter stays inside site counts and measurement counts, the way purpose does. Site counts still omit the site filter. Measurement counts still omit the measurement filter. A site count under a group filter counts docs with a qualifying interval at that site, not docs that merely have the group on some other in-play interval.

## Group ids for the picker

```text
GroupIDs(beginsAt, endsBefore int64, includeMemberOnly bool) ([]string, error)
```

Returns the distinct group ids on in-play intervals of datastreams the caller may see. Sorted. Empty group ids are omitted. There is no count.

`includeMemberOnly` matches `Query`. False uses `public_ok`. True uses `public_ok ∪ member_ok`. A group that appears only on member-only datastreams is absent for a guest. Open-group posting keys are not the guest answer by themselves; the implementation intersects with the access bitmap.

`GroupIDs` does not apply purpose, measurement, site, viewport, or `Query.GroupIDs`. The picker is a reducer over deployment assignment. The facet panel remains the place that narrows by measurement and site.

The host calls `GroupIDs` on each visible organization shard with the same time bounds as the search query. Changing the date range rebuilds that map. The call is cheap next to `Query` and does not depend on the map viewport.

## Picker

The Explore panel, and the demo, show organizations on the left and groups on the right. Each side has a text filter. The filter runs on the list operation:

- Left: `ListOrganizations`, with today's visibility, `is_enabled` true, `is_hidden` false. The text filter narrows that page.
- Right: `ListGroups` with `is_enabled` true and `is_hidden` false. The text filter narrows that page. The API does not drop hidden rows when that filter is omitted, so the page sends it.

The index does not supply those catalogs and does not attach a datastream count to either row.

Checked ids are filters for the rest of search. Checked orgs select shards (empty means every visible org). Checked groups become `Query.GroupIDs` (empty means no group predicate). A greyed row is still checked, so it is still in that filter, until the user unchecks it.

`ReducePicker` turns the catalogs, the checks, and the per-org `GroupIDs` results into the two lists. The host passes catalogs after the text filter. Checked ids that the text filter hides stay checked and are omitted from the returned rows until the filter matches them again. Checked ids outside the visible catalog are dropped.

Let `groupsByOrg` be the `GroupIDs` result for each visible org at the current window and access.

- Relevant groups: if no org is checked, the filtered group catalog. If one or more orgs are checked, the union of `groupsByOrg` for those orgs, restricted to the filtered catalog. Checking two orgs shows groups referenced by either org. A group that appears in only one of them still shows. The reducer does not keep only groups common to every checked org.
- Relevant orgs: if no group is checked, the filtered org catalog. If one or more groups are checked, the union of orgs in `groupsByOrg` whose set intersects the checked groups, restricted to the filtered catalog.

A row is shown when it is relevant or it is checked. A row is grey when it is checked, the other side has at least one check, and it is not relevant. Unchecking a grey row removes it from the checked set. It then fails the "relevant or checked" test and leaves the list.

Neither side checked: both lists are the filtered catalogs, and nothing is grey. Checking orgs whittles groups. Checking groups whittles orgs. The two reductions are computed independently from `groupsByOrg`. They are not applied as a second intersection on top of each other.

Worked checks, using the demo data below, open deployments, guest access. The right catalog still lists `internal`, because that demo row is in the local catalog. Guest `GroupIDs` for north omits it, because only a member-only datastream references it. A hidden group is a different case: `ListGroups` with `is_hidden` false leaves it out of the catalog, and `ReducePicker` then omits it.

| Checks | Left | Right |
| --- | --- | --- |
| none | north, south | field, lab, volunteers, internal |
| north and south | north, south | field, lab, volunteers |
| volunteers | north | field, lab, volunteers, internal |
| north, south, and volunteers | north; south grey | field, lab, volunteers |
| volunteers, then uncheck south | north | field, lab, volunteers |
| internal, then north | north grey | field, lab, volunteers; internal grey |

With only volunteers checked, south is absent on the left because south does not reference it. The right stays the full catalog because no org is checked. Adding north and south as checks puts south back, grey, because it is checked and still not a reference for volunteers. The right side becomes the union of groups referenced by north or south, so volunteers stays and internal drops. Search with north, south, and volunteers still applies both filters: those two shards, `GroupIDs: [volunteers]`. South contributes no hits. Unchecking south drops the grey row and drops that shard from the query. North stays checked, so the right side is the groups north references and internal stays off the list.

Checking internal, then north, greys both. Guest `GroupIDs` for north does not contain internal, so the checked group is not relevant. No visible org references internal, so the checked org is not relevant either.

Facets stay a different control. Measurement and site lists show the options in play for discovery, including a measurement count of zero, because those vocabularies are short. The org/group panel does not do that. It does not list every group with a zero count. It lists the catalog, or the union the other side references, plus checked rows that no longer fit so the user can back out.

## Demo

`examples/demo` prints a single-org walkthrough (clusters, member row, public shard, container, version file). It then builds two organization shards and prints the picker and two query assertions. `examples/README.md` describes that second half.

Local catalogs stand in for `ListOrganizations` and `ListGroups`:

| Id | Name |
| --- | --- |
| north | North Reserve |
| south | South Reserve |
| field | Field crew |
| lab | Lab |
| volunteers | Volunteers |
| internal | Internal |

North:

| Place | Interval | Group |
| --- | --- | --- |
| station at meadow | open | field |
| station at ridge | `[1000, 2000)` | field |
| station at ridge | `[3000, 0)` | lab |
| station moved | `[1000, 2000)` at meadow | field |
| station moved | `[3000, 0)` at ridge | lab |
| creek, site-attached | open | volunteers |
| member-only datastream at meadow | open | internal |

South:

| Place | Interval | Group |
| --- | --- | --- |
| south meadow, station | open | lab |
| south bare, station | open | none |

The program prints:

1. Guest `GroupIDs` for north at the open deployment: field, lab, volunteers. Internal is absent. Member `GroupIDs` include internal. South returns lab. The picker catalog still lists internal for the guest.
2. The picker rows in the table above, including south grey, and internal plus north both grey.
3. Open `Query` with `GroupIDs: [volunteers]` returns the creek datastream and not the ungrouped south stream. The result has measurement counts and no group-count field.
4. Window `[1000, 4000)` overlaps both intervals of the moved datastream. Site meadow and group field matches. Site meadow and group lab does not. Site ridge and group lab matches. Site ridge and group field does not.

That last case is the joint-interval assertion. The demo exits non-zero if meadow and lab match together.

## Later, outside this library

The Explore picker in the Main App calls `ListOrganizations`, `ListGroups`, `GroupIDs`, and `ReducePicker`, then passes the checked ids into the same search query the map already runs. That host is the client-side datastream-search path in `dendra-api-services`. The map example under `examples/map` is that picker over the demo catalog. Local lists stand in for the two list calls, and the page calls `GroupIDs` and `ReducePicker`.
