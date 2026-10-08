# Map demo

A static page in the shape of Explore. It builds a fixture in the browser, then draws clusters or sites from `searchQuery`. Use this when you want to see how zoom, viewport, shown vs exact, and member-only filters change the layer without standing up the API. MapLibre and the OpenFreeMap Liberty style load from the network. The search module does not.

```bash
./examples/wasm/build.sh examples/map/search.wasm
go run ./examples/map
```

Run both from the repository root. Open http://127.0.0.1:8080.

The Go server is only there to send `application/wasm`. Point Caddy at `examples/map` if you already serve static files.

The dataset, shown/exact, and member-only controls are demo chrome. The panel is the guest flow: purpose, then organizations and groups, then measurements at any zoom. Organizations are on the left and groups are on the right, each in a scrollable list. Each side has a text filter. Checked organizations select shards. Checked groups are `Query.GroupIDs`. A grey row is still checked until it is unchecked. The lists come from `ReducePicker`. Local names stand in for `ListOrganizations` and `ListGroups`. Sites and a page of datastreams appear once the layer is sites. Datastreams are grouped under the station name stored on the shard. Some sites are drawn as an outline; the index still matches the outline's box. A checked datastream stays selected across pans. The map does not check one when a site is clicked.

`large-catalog` builds on the main thread and can hitch. Several organizations share one concatenated page; select one organization to page.
