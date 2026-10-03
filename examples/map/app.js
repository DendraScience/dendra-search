// Explore-shaped page. searchBuild and searchQuery live in the wasm module.
// This file holds the selection and draws the result. It does not run the index.

const STYLE = "https://tiles.openfreemap.org/styles/liberty";
const PAGE_SIZE = 25;
// Below this zoom the layer is always clusters. The map opens at 3, so that
// first view stays clustered. From 4 in, the site count decides.
const MIN_SITE_ZOOM = 4;
// More sites than this in the view stays clustered once the zoom floor is met.
const MAP_SITE_LIMIT = 40;

const measurementLabels = {
  Meas01: "Air temperature",
  Meas02: "Relative humidity",
  Meas03: "Wind speed",
  Meas04: "Wind direction",
  Meas05: "Precipitation",
  Meas06: "Soil moisture",
  Meas07: "Soil temperature",
  Meas08: "Solar radiation",
  Meas09: "Barometric pressure",
  Meas10: "Water temperature",
  Meas11: "Water level",
  Meas12: "Snow depth",
};

const purposes = new Set(["", "ReadytoUse", "Raw", "StatusInformation"]);

const state = {
  purpose: "ReadytoUse",
  orgs: new Set(),
  measurements: new Set(),
  sites: new Set(),
  cart: new Map(),
  geometry: "shown",
  includeMemberOnly: false,
  pageAfter: 0,
  preset: "today-catalog",
  built: null,
  areas: new Map(),
};

const siteInfo = new Map();
let map;
let markers = [];
let moveTimer = 0;
let last = null;
let mapReady = false;
let wasmReady = false;
let started = false;
let initial = { bbox: null, z: null };

const $ = (id) => document.getElementById(id);

// parseURL restores the shareable selection. Dataset size, geometry, and
// member-only stay on the demo strip and are not part of the URL.
function parseURL() {
  const params = new URLSearchParams(location.search);
  if (params.has("purpose") && purposes.has(params.get("purpose"))) {
    state.purpose = params.get("purpose");
  }
  splitInto(state.orgs, params.get("org"));
  splitInto(state.measurements, params.get("meas"));
  splitInto(state.sites, params.get("site"));
  const raw = params.get("bbox");
  if (raw) {
    const parts = raw.split(",").map(Number);
    if (parts.length === 4 && parts.every((n) => Number.isFinite(n))) initial.bbox = parts;
  }
  if (params.has("z")) {
    const z = Number(params.get("z"));
    if (Number.isFinite(z)) initial.z = z;
  }
}

function splitInto(set, raw) {
  set.clear();
  if (!raw) return;
  for (const part of raw.split(",")) {
    if (part) set.add(part);
  }
}

// MapLibre reports longitudes outside [-180, 180] when the canvas is wider
// than the world. The index accepts a wrapped box (east < west) and rejects
// anything outside that range.
function viewportFromMap() {
  const bounds = map.getBounds();
  let west = bounds.getWest();
  let east = bounds.getEast();
  let south = Math.min(90, Math.max(-90, bounds.getSouth()));
  let north = Math.min(90, Math.max(-90, bounds.getNorth()));
  if (south > north) {
    const swap = south;
    south = north;
    north = swap;
  }
  if (east - west >= 360) {
    return { west: -180, south, east: 180, north };
  }
  while (west < -180) {
    west += 360;
    east += 360;
  }
  while (west > 180) {
    west -= 360;
    east -= 360;
  }
  if (east > 180) east -= 360;
  return { west, south, east, north };
}

// writeURL records the camera and the filters. replaceState keeps a pan
// from adding a history entry.
function writeURL() {
  const params = new URLSearchParams();
  const bounds = map.getBounds();
  const nums = [bounds.getWest(), bounds.getSouth(), bounds.getEast(), bounds.getNorth()].map((n) =>
    n.toFixed(4)
  );
  params.set("bbox", nums.join(","));
  params.set("z", String(Math.max(0, Math.round(map.getZoom()))));
  params.set("purpose", state.purpose);
  if (state.orgs.size) params.set("org", [...state.orgs].join(","));
  if (state.measurements.size) params.set("meas", [...state.measurements].join(","));
  if (state.sites.size) params.set("site", [...state.sites].join(","));
  history.replaceState(null, "", `?${params.toString()}`);
}

function setStatus(text) {
  $("status").textContent = text;
}

function countNoun(n, one, many) {
  return `${Number(n).toLocaleString()} ${Number(n) === 1 ? one : many}`;
}

function measurementLabel(id) {
  return measurementLabels[id] || id;
}

function syncControls() {
  $("fixture").value = state.preset;
  const geom = document.querySelector(`input[name="geom"][value="${state.geometry}"]`);
  if (geom) geom.checked = true;
  $("member").checked = state.includeMemberOnly;
  $("purpose").value = state.purpose;
}

function bindControls() {
  $("fixture").addEventListener("change", (ev) => {
    state.preset = ev.target.value;
    state.pageAfter = 0;
    state.sites.clear();
    state.cart.clear();
    siteInfo.clear();
    rebuild();
  });
  for (const el of document.querySelectorAll('input[name="geom"]')) {
    el.addEventListener("change", () => {
      if (!el.checked) return;
      state.geometry = el.value;
      refresh();
    });
  }
  $("member").addEventListener("change", (ev) => {
    state.includeMemberOnly = ev.target.checked;
    refresh();
  });
  $("purpose").addEventListener("change", (ev) => {
    state.purpose = ev.target.value;
    refresh();
  });
  $("page-first").addEventListener("click", () => {
    state.pageAfter = 0;
    runQuery();
  });
  $("page-next").addEventListener("click", () => {
    if (!last || !last.paged || !last.has_more) return;
    state.pageAfter = last.next_page_after;
    runQuery();
  });
}

// refresh queries now and starts again at the first page. Filter changes
// use this. The map's own moveend is ignored until it settles.
function refresh() {
  if (!state.built) return;
  window.clearTimeout(moveTimer);
  state.pageAfter = 0;
  runQuery();
}

// onMoveEnd waits for the camera to settle, then queries. The first page
// starts over because the extent changed.
function onMoveEnd() {
  window.clearTimeout(moveTimer);
  moveTimer = window.setTimeout(() => {
    state.pageAfter = 0;
    runQuery();
  }, 250);
}

// maybeStart runs once the style and the wasm module are both ready.
// A shared bbox is applied before the first build so the opening query
// uses that camera.
function maybeStart() {
  if (!mapReady || !wasmReady || started) return;
  started = true;
  if (initial.bbox) {
    const [west, south, east, north] = initial.bbox;
    map.jumpTo({
      center: [(west + east) / 2, (south + north) / 2],
      zoom: initial.z ?? map.getZoom(),
    });
  } else if (initial.z != null) {
    map.jumpTo({ zoom: initial.z });
  }
  map.on("moveend", onMoveEnd);
  rebuild();
}

// rebuild asks the module for the selected dataset, then queries.
// searchBuild keeps the previous shards when the new build fails.
function rebuild() {
  setStatus("Building…");
  window.setTimeout(() => {
    let built;
    try {
      built = JSON.parse(searchBuild(JSON.stringify({ name: state.preset, layout: "demo" })));
    } catch (err) {
      setStatus(String(err));
      return;
    }
    if (built.error) {
      setStatus(built.error);
      return;
    }
    state.built = built;
    state.areas = new Map();
    for (const area of built.areas || []) state.areas.set(area.id, area);
    const ids = new Set(built.orgs.map((org) => org.id));
    for (const id of state.orgs) {
      if (!ids.has(id)) state.orgs.delete(id);
    }
    renderOrgs();
    renderCart();
    runQuery();
  }, 0);
}

// runQuery sends the whole selection: viewport, zoom, purpose, and any
// org, measurement, or site filters. The module decides clusters or sites.
function runQuery() {
  if (!state.built || !map) return;
  const body = {
    purpose: state.purpose,
    measurements: [...state.measurements],
    site_ids: [...state.sites],
    organization_ids: [...state.orgs],
    viewport: viewportFromMap(),
    geometry: state.geometry,
    zoom: Math.max(0, Math.round(map.getZoom())),
    min_site_zoom: MIN_SITE_ZOOM,
    map_site_limit: MAP_SITE_LIMIT,
    include_member_only: state.includeMemberOnly,
    page_size: PAGE_SIZE,
    page_after: state.pageAfter,
  };
  let res;
  try {
    res = JSON.parse(searchQuery(JSON.stringify(body)));
  } catch (err) {
    setStatus(String(err));
    return;
  }
  if (res.error) {
    setStatus(res.error);
    return;
  }
  last = res;
  writeURL();
  renderResult(res);
}

// renderResult shows measurement counts at every zoom. Sites and datastreams
// appear only when the layer is sites.
function renderResult(res) {
  const built = state.built;
  const bytes =
    built.bytes >= 1_000_000
      ? `${(built.bytes / 1_000_000).toFixed(1)} MB`
      : `${Math.max(1, Math.round(built.bytes / 1000))} KB`;
  setStatus(
    `${countNoun(built.sites, "site", "sites")}, ${countNoun(built.datastreams, "datastream", "datastreams")}, ${bytes}. Build ${built.finish_ms.toFixed(0)} ms. Query ${res.elapsed_ms.toFixed(1)} ms, ${res.kind}, ${countNoun(res.site_total, "site", "sites")} in view, ${countNoun(res.total_count, "datastream", "datastreams")}.`
  );
  const sitesMode = res.kind === "sites";
  $("layer-note").hidden = sitesMode;
  if (!sitesMode) {
    $("layer-note").textContent = `${countNoun(res.site_total, "site", "sites")} in this view. Zoom in to list datastreams.`;
  }
  $("meas-section").hidden = false;
  $("site-section").hidden = !sitesMode;
  $("hit-section").hidden = !sitesMode;
  drawMarkers(res);
  drawAreas(sitesMode ? res.sites : null);
  renderMeasurements(res.measurement_counts);
  if (!sitesMode) return;
  for (const site of res.sites) siteInfo.set(site.site_id, site);
  renderSites(res.site_counts);
  renderHits(res);
}

// drawMarkers paints count badges, or one pin per point site.
// A site with an outline is drawn by drawAreas instead of a pin.
function drawMarkers(res) {
  for (const marker of markers) marker.remove();
  markers = [];
  const reduce = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  if (res.kind === "clusters") {
    for (const cluster of res.clusters) {
      const el = document.createElement("button");
      el.type = "button";
      el.className = "pin pin-cluster";
      el.textContent = String(cluster.site_count);
      el.setAttribute("aria-label", cluster.site_count === 1 ? "1 site" : `${cluster.site_count} sites`);
      el.addEventListener("click", (ev) => {
        ev.stopPropagation();
        fitBox(cluster.expansion, reduce);
      });
      markers.push(new maplibregl.Marker({ element: el, anchor: "center" }).setLngLat([cluster.centroid.lon, cluster.centroid.lat]).addTo(map));
    }
    return;
  }
  for (const site of res.sites) {
    if (state.areas.has(site.site_id)) continue;
    const el = document.createElement("button");
    el.type = "button";
    el.className = "pin pin-site";
    if (state.sites.has(site.site_id)) el.classList.add("is-selected");
    el.setAttribute("aria-label", site.name);
    el.title = `${site.name} · ${site.organization_name}`;
    el.addEventListener("click", (ev) => {
      ev.stopPropagation();
      toggle(state.sites, site.site_id);
      refresh();
    });
    markers.push(new maplibregl.Marker({ element: el, anchor: "center" }).setLngLat([site.point.lon, site.point.lat]).addTo(map));
  }
}

// drawAreas fills the outlines stored at build time for sites in this result.
// Cluster mode clears them. The ring is not part of the query response.
function drawAreas(sites) {
  const src = map.getSource("areas");
  if (!src) return;
  const features = [];
  for (const site of sites || []) {
    const area = state.areas.get(site.site_id);
    if (!area) continue;
    const ring = state.geometry === "exact" ? area.exact : area.shown;
    if (!ring || ring.length < 4) continue;
    features.push({
      type: "Feature",
      properties: {
        id: site.site_id,
        selected: state.sites.has(site.site_id) ? 1 : 0,
      },
      geometry: { type: "Polygon", coordinates: [ring] },
    });
  }
  src.setData({ type: "FeatureCollection", features });
}

// fitBox frames a cluster's expansion. A point-sized box is padded so the
// camera does not zoom to its limit.
function fitBox(box, reduce) {
  let { west, south, east, north } = box;
  if (!(south < north) || (east === west && south === north)) {
    west -= 0.02;
    east += 0.02;
    south -= 0.02;
    north += 0.02;
  } else if (east > west) {
    const lonPad = Math.max(0.02, (east - west) * 0.08);
    const latPad = Math.max(0.02, (north - south) * 0.08);
    west -= lonPad;
    east += lonPad;
    south -= latPad;
    north += latPad;
  }
  south = Math.max(-85, south);
  north = Math.min(85, north);
  const opts = { padding: 56, maxZoom: 12 };
  if (reduce) opts.duration = 0;
  map.fitBounds(
    [
      [west, south],
      [east, north],
    ],
    opts
  );
}

// renderOrgs lists the organizations from the build. None pressed means
// every organization. A pressed chip is sent on the next query.
function renderOrgs() {
  const list = $("org-list");
  list.replaceChildren();
  const orgs = state.built ? state.built.orgs : [];
  $("org-hint").hidden = state.orgs.size > 0;
  for (const org of orgs) {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "chip";
    btn.textContent = org.name;
    btn.setAttribute("aria-pressed", state.orgs.has(org.id) ? "true" : "false");
    btn.addEventListener("click", () => {
      toggle(state.orgs, org.id);
      renderOrgs();
      refresh();
    });
    list.append(btn);
  }
}

// renderMeasurements lists every measurement the shard knows. A count of
// zero stays on screen so selecting one does not hide the others.
function renderMeasurements(counts) {
  const list = $("meas-list");
  list.replaceChildren();
  for (const row of counts) {
    list.append(facetButton(measurementLabel(row.value), row.value, row.count, state.measurements, () => {
      renderMeasurements(counts);
      refresh();
    }));
  }
}

function renderSites(counts) {
  const list = $("site-list");
  list.replaceChildren();
  const rows = [...counts].sort((a, b) => b.count - a.count || a.value.localeCompare(b.value));
  for (const row of rows) {
    const known = siteInfo.get(row.value);
    const name = known ? known.name : row.value;
    const extra = known ? known.organization_name : "";
    list.append(facetButton(name, extra, row.count, state.sites, () => {
      renderSites(counts);
      refresh();
    }, row.value));
  }
}

function facetButton(title, sub, count, set, onClick, id) {
  const key = id || sub;
  const btn = document.createElement("button");
  btn.type = "button";
  btn.className = "facet";
  if (count === 0) btn.classList.add("is-zero");
  btn.setAttribute("aria-pressed", set.has(key) ? "true" : "false");
  const label = document.createElement("span");
  label.className = "label";
  const strong = document.createElement("span");
  strong.textContent = title;
  label.append(strong);
  if (sub && sub !== title) {
    const idEl = document.createElement("span");
    idEl.className = "id";
    idEl.textContent = sub;
    label.append(idEl);
  }
  const num = document.createElement("span");
  num.className = "count";
  num.textContent = count.toLocaleString();
  btn.append(label, num);
  btn.addEventListener("click", () => {
    toggle(set, key);
    onClick();
  });
  return btn;
}

// renderHits groups the page under each station. Checking a row updates the
// cart and does not query again.
function renderHits(res) {
  $("hit-count").textContent = res.total_count.toLocaleString();
  const list = $("hit-list");
  list.replaceChildren();
  if (res.hits.length === 0) {
    const p = document.createElement("p");
    p.className = "empty";
    p.textContent = "No datastreams in this view.";
    list.append(p);
  }
  let group = "";
  for (const hit of res.hits) {
    const label = stationLabel(hit);
    if (label !== group) {
      group = label;
      const heading = document.createElement("h3");
      heading.className = "station";
      heading.textContent = label;
      list.append(heading);
    }
    const row = document.createElement("label");
    row.className = "hit";
    const box = document.createElement("input");
    box.type = "checkbox";
    box.checked = state.cart.has(hit.datastream_id);
    box.addEventListener("change", () => {
      if (box.checked) state.cart.set(hit.datastream_id, hit);
      else state.cart.delete(hit.datastream_id);
      renderCart();
    });
    const text = document.createElement("span");
    const name = document.createElement("strong");
    name.textContent = measurementLabel(hit.measurement);
    const meta = document.createElement("span");
    meta.className = "meta";
    meta.textContent = `${hit.site_name} · ${hit.organization_name}`;
    text.append(name, meta);
    row.append(box, text);
    list.append(row);
  }
  $("page-first").disabled = state.pageAfter === 0;
  $("page-next").disabled = !(res.paged && res.has_more);
  const note = $("page-note");
  if (!res.paged && res.hits.length < res.total_count) {
    note.textContent = "Select one organization to page the rest.";
  } else if (res.hits.length) {
    note.textContent = `${res.hits.length.toLocaleString()} on this page`;
  } else {
    note.textContent = "";
  }
}

// stationLabel is the station name stored on the shard. A site-attached
// datastream has none, so the heading is the site name.
function stationLabel(hit) {
  return hit.station_name || hit.site_name;
}

// renderCart draws the checked datastreams. A query does not clear them,
// so the selection survives a pan.
function renderCart() {
  $("cart-count").textContent = state.cart.size.toLocaleString();
  const list = $("cart-list");
  list.replaceChildren();
  if (state.cart.size === 0) {
    const p = document.createElement("p");
    p.className = "empty";
    p.textContent = "None selected.";
    list.append(p);
    return;
  }
  for (const hit of state.cart.values()) {
    const row = document.createElement("div");
    row.className = "cart-row";
    const text = document.createElement("span");
    const name = document.createElement("strong");
    name.textContent = hit.name;
    const meta = document.createElement("span");
    meta.className = "meta";
    meta.textContent = `${hit.site_name} · ${hit.organization_name}`;
    text.append(name, meta);
    const remove = document.createElement("button");
    remove.type = "button";
    remove.textContent = "Remove";
    remove.addEventListener("click", () => {
      state.cart.delete(hit.datastream_id);
      renderCart();
      if (last && last.kind === "sites") renderHits(last);
    });
    row.append(text, remove);
    list.append(row);
  }
}

function toggle(set, id) {
  if (set.has(id)) set.delete(id);
  else set.add(id);
}

// showStateBoundaries darkens the Liberty state lines and draws them from
// the starting zoom. The style keeps them light gray, dashed, and hidden
// until zoom 5.
function showStateBoundaries() {
  if (!map.getLayer("boundary_3")) return;
  map.setLayerZoomRange("boundary_3", 3, 24);
  map.setPaintProperty("boundary_3", "line-color", "#2c3a48");
  map.setPaintProperty("boundary_3", "line-opacity", 0.9);
  map.setPaintProperty("boundary_3", "line-width", ["interpolate", ["linear"], ["zoom"], 3, 1.4, 7, 1.8, 11, 2.2]);
  map.setPaintProperty("boundary_3", "line-dasharray", [3, 1.5]);
}

// boot starts the map and the wasm module. The module calls __searchOnReady
// before it blocks, which is how the page learns searchBuild exists.
function boot() {
  parseURL();
  syncControls();
  bindControls();
  renderCart();
  map = new maplibregl.Map({
    container: "map",
    style: STYLE,
    center: [-119.5, 37.2],
    zoom: 3,
    cooperativeGestures: true,
  });
  map.addControl(new maplibregl.NavigationControl({ showCompass: false }), "top-right");
  map.on("load", () => {
    showStateBoundaries();
    map.addSource("areas", { type: "geojson", data: { type: "FeatureCollection", features: [] } });
    map.addLayer({
      id: "areas-fill",
      type: "fill",
      source: "areas",
      paint: {
        "fill-color": "#f0b429",
        "fill-opacity": ["case", ["==", ["get", "selected"], 1], 0.38, 0.16],
      },
    });
    map.addLayer({
      id: "areas-line",
      type: "line",
      source: "areas",
      paint: {
        "line-color": "#8a5a00",
        "line-width": ["case", ["==", ["get", "selected"], 1], 2.5, 1.5],
      },
    });
    map.on("click", "areas-fill", (ev) => {
      const id = ev.features && ev.features[0] && ev.features[0].properties.id;
      if (!id) return;
      toggle(state.sites, id);
      refresh();
    });
    map.on("mouseenter", "areas-fill", () => {
      map.getCanvas().style.cursor = "pointer";
    });
    map.on("mouseleave", "areas-fill", () => {
      map.getCanvas().style.cursor = "";
    });
    mapReady = true;
    maybeStart();
  });
  globalThis.__searchOnReady = () => {
    wasmReady = true;
    maybeStart();
  };
  const go = new Go();
  WebAssembly.instantiateStreaming(fetch("search.wasm", { cache: "no-store" }), go.importObject)
    .then((result) => {
      go.run(result.instance);
    })
    .catch((err) => {
      setStatus(err && err.message ? err.message : String(err));
    });
}

boot();
