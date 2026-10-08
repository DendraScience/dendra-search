# WASM example

Build dendra-search for `GOOS=js GOARCH=wasm` so a browser can build fixtures and run `Query` without a server. The stable API is still the Go types. The `search*` JS functions below are convenience wrappers for this example and the map page; do not treat them as the public contract for a product host.

```bash
./examples/wasm/build.sh
```

Writes `examples/wasm/search.wasm` and copies `wasm_exec.js` from GOROOT. Both are gitignored.
Pass another path to write the pair somewhere else:

```bash
./examples/wasm/build.sh examples/map/search.wasm
```

JS (after `Go` from `wasm_exec.js` has run the module):

```js
JSON.parse(globalThis.searchBuild('{"name":"today-catalog"}'))
JSON.parse(globalThis.searchBuild('{"orgs":1,"sites":25,"datastreams":600}'))
JSON.parse(globalThis.searchQuery(JSON.stringify({
  geometry: 'shown',
  zoom: 2,
  viewport: {west: -180, south: -90, east: 180, north: 90}
})))
JSON.parse(globalThis.searchGroupIDs('{}'))
JSON.parse(globalThis.searchReducePicker(JSON.stringify({
  orgs: [{id: 'org-000', name: 'Org 000'}],
  groups: [{id: 'field', name: 'Field crew'}],
  groups_by_org: {'org-000': ['field']}
})))
```

`searchBuild(specJSON)` replaces the shards held in the module. `specJSON` is
`{"name":"today-org"|"today-catalog"|"large-org"|"large-catalog"}` or
`{"orgs","sites","datastreams"}` for `Generate`. Add `"layout":"demo"` for the
map catalog: several datastreams share a logger, and some sites carry an outline.
The reply is
`{name, orgs:[{id, name}], sites, datastreams, bytes, finish_ms, areas}`.
`areas` is `[{id, shown, exact}]` rings in `[lon, lat]` order. The benchmark
layout leaves `areas` empty. On error the previous shards are kept.

`searchQuery(queryJSON)` reads those shards. `organization_ids` chooses which
ones; an empty list is every org from the build. The other fields match `Query`
(`purpose`, `measurements`, `site_ids`, `group_ids`, `viewport`, `geometry`, `zoom`,
`include_member_only`, `map_site_limit`, `min_site_zoom`, `page_size`,
`page_after`). `geometry` is `"shown"` or `"exact"`.

The reply is one combined result: `kind` (`"clusters"` or `"sites"`), `clusters`,
`sites`, `site_total`, `bounds`, `site_counts`, `measurement_counts`, `hits`,
`total_count`, `has_more`, `paged`, `elapsed_ms`. `measurement_counts` is set
for both kinds. `site_counts` is set only for `"sites"`. Candidate sites used only to
merge shards are left out. `paged` is true for a single shard, and
`next_page_after` is set only then. Several shards are concatenated with `Combine`
and have no cross-org cursor.

`searchGroupIDs` calls `GroupIDs` on every loaded organization. The body is
`begins_at`, `ends_before`, and `include_member_only`. Both time fields zero,
or an empty body, is the open deployment. The reply is
`{groups_by_org: {org id: [group id]}}`.

`searchReducePicker` calls `ReducePicker`. The body is `orgs` and `groups`
(`[{id, name}]`, already text-filtered), `checked_orgs`, `checked_groups`, and
`groups_by_org`. The reply is `{orgs, groups}`, each row `{id, name, grey}`.

Set `globalThis.__searchOnReady` before `go.run`. The module calls it once the
functions exist, then waits.
