# Map demo

A static page in the shape of Explore. It builds a fixture in the browser, then draws clusters or sites from `searchQuery`. MapLibre and the OpenFreeMap Liberty style load from the network. The search module does not.

```bash
./examples/wasm/build.sh examples/map/search.wasm
go run ./examples/map
```

Run both from the repository root. Open http://127.0.0.1:8080.

The Go server is only there to send `application/wasm`. Point Caddy at `examples/map` if you already serve static files.

The dataset, shown/exact, and member-only controls are demo chrome. The panel is the guest flow: purpose, organization chips, then measurements, sites, and a page of datastreams once the layer is sites. Datastreams are grouped under the logger that records them. Some sites are drawn as an outline; the index still matches the outline's box. A checked datastream stays selected across pans. The map does not check one when a site is clicked.

`large-catalog` builds on the main thread and can hitch. Several organizations share one concatenated page; select one organization to page.
