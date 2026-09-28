# WASM example

Build dendra-search for `GOOS=js GOARCH=wasm`. The stable API is the Go types; the
`search*` JS functions are convenience wrappers for this example and the map page.

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
(`purpose`, `measurements`, `site_ids`, `viewport`, `geometry`, `zoom`,
`include_member_only`, `map_site_limit`, `min_site_zoom`, `page_size`,
`page_after`). `geometry` is `"shown"` or `"exact"`.

The reply is one combined result: `kind` (`"clusters"` or `"sites"`), `clusters`,
`sites`, `site_total`, `bounds`, `site_counts`, `measurement_counts`, `hits`,
`total_count`, `has_more`, `paged`, `elapsed_ms`. Candidate sites used only to
merge shards are left out. `paged` is true for a single shard, and
`next_page_after` is set only then. Several shards are concatenated with `Combine`
and have no cross-org cursor.

Set `globalThis.__searchOnReady` before `go.run`. The module calls it once the
functions exist, then waits.
