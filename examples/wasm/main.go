//go:build js && wasm

// Browser wrappers for Generate, Open, Query, and Combine. The stable API is
// the Go types; these JS helpers exist for the map example.
//
//	searchBuild(specJSON) -> build JSON
//	searchQuery(queryJSON) -> result JSON
//
// Both return a JSON string. Failures are {"error":"..."}. The previous
// shards stay in place when searchBuild fails.
package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"syscall/js"
	"time"

	"github.com/dendrascience/dendra-search"
)

func main() {
	js.Global().Set("searchBuild", keep(js.FuncOf(func(_ js.Value, args []js.Value) any {
		raw := ""
		if len(args) > 0 && !args[0].IsUndefined() && !args[0].IsNull() {
			raw = args[0].String()
		}
		out, err := build(raw)
		if err != nil {
			return jsonErr(err)
		}
		b, err := json.Marshal(out)
		if err != nil {
			return jsonErr(err)
		}
		return string(b)
	})))

	js.Global().Set("searchQuery", keep(js.FuncOf(func(_ js.Value, args []js.Value) any {
		raw := ""
		if len(args) > 0 && !args[0].IsUndefined() && !args[0].IsNull() {
			raw = args[0].String()
		}
		out, err := query(raw)
		if err != nil {
			return jsonErr(err)
		}
		b, err := json.Marshal(out)
		if err != nil {
			return jsonErr(err)
		}
		return string(b)
	})))

	if cb := js.Global().Get("__searchOnReady"); cb.Type() == js.TypeFunction {
		cb.Invoke()
	}
	select {}
}

// keep holds funcs for the life of the page. A js.Func that nothing in Go
// references can be collected, which drops the callback.
var heldFuncs []js.Func

func keep(fn js.Func) js.Func {
	heldFuncs = append(heldFuncs, fn)
	return fn
}

type orgShard struct {
	id    string
	name  string
	shard *search.Shard
}

var loaded []orgShard

type specIn struct {
	Name        string `json:"name"`
	Orgs        int    `json:"orgs"`
	Sites       int    `json:"sites"`
	Datastreams int    `json:"datastreams"`
	// Layout "demo" builds loggers with several datastreams and area outlines.
	// Empty uses the benchmark fixtures.
	Layout string `json:"layout"`
}

type orgChip struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type buildOut struct {
	Name        string     `json:"name"`
	Orgs        []orgChip  `json:"orgs"`
	Sites       int        `json:"sites"`
	Datastreams int        `json:"datastreams"`
	Bytes       int        `json:"bytes"`
	FinishMs    float64    `json:"finish_ms"`
	Areas       []areaJSON `json:"areas"`
}

// build opens one shard per organization and keeps them for later queries.
// Organization names come from a sites-mode read of each shard. A failed
// build leaves the previous shards in place.
func build(raw string) (buildOut, error) {
	if raw == "" {
		return buildOut{}, fmt.Errorf("search: name or orgs, sites, and datastreams required")
	}
	var in specIn
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return buildOut{}, err
	}
	fix, areas, err := loadCatalog(in)
	if err != nil {
		return buildOut{}, err
	}
	next := make([]orgShard, 0, len(fix.Blobs))
	bytes := 0
	limit := uint32(fix.Sites + 1)
	census := search.Query{
		Geometry:          search.GeometryShown,
		Zoom:              22,
		MapSiteLimit:      limit,
		MinSiteZoom:       0,
		IncludeMemberOnly: true,
	}
	for _, blob := range fix.Blobs {
		shard, err := search.Open(blob)
		if err != nil {
			return buildOut{}, err
		}
		bytes += len(blob)
		res, err := shard.Query(census)
		if err != nil {
			return buildOut{}, err
		}
		if len(res.Sites) == 0 {
			continue
		}
		site := res.Sites[0]
		next = append(next, orgShard{
			id:    site.OrganizationID,
			name:  site.OrganizationName,
			shard: shard,
		})
	}
	sort.Slice(next, func(i, j int) bool { return next[i].id < next[j].id })
	orgs := make([]orgChip, len(next))
	for i, org := range next {
		orgs[i] = orgChip{ID: org.id, Name: org.name}
	}
	loaded = next
	if areas == nil {
		areas = []areaJSON{}
	}
	return buildOut{
		Name:        fix.Name,
		Orgs:        orgs,
		Sites:       fix.Sites,
		Datastreams: fix.Datastreams,
		Bytes:       bytes,
		FinishMs:    float64(fix.Finish) / float64(time.Millisecond),
		Areas:       areas,
	}, nil
}

// loadCatalog chooses the catalog. layout "demo" is the map page: shared
// loggers and area outlines. A preset name with no layout is a benchmark
// fixture. Counts with no name call Generate.
func loadCatalog(in specIn) (search.Fixture, []areaJSON, error) {
	spec, named, err := catalogSpec(in)
	if err != nil {
		return search.Fixture{}, nil, err
	}
	if in.Layout == "demo" {
		return demoCatalog(spec)
	}
	if !named {
		fix, err := search.Generate(spec)
		return fix, nil, err
	}
	fix, err := fixture(in)
	return fix, nil, err
}

// catalogSpec maps a preset name to its size. The bool is false when the
// caller passed counts instead of a name.
func catalogSpec(in specIn) (search.Spec, bool, error) {
	switch in.Name {
	case "today-org":
		return search.Spec{Name: in.Name, Orgs: 1, Sites: 25, Datastreams: 600}, true, nil
	case "today-catalog":
		return search.Spec{Name: in.Name, Orgs: 15, Sites: 300, Datastreams: 8000}, true, nil
	case "large-org":
		return search.Spec{Name: in.Name, Orgs: 1, Sites: 250, Datastreams: 6000}, true, nil
	case "large-catalog":
		return search.Spec{Name: in.Name, Orgs: 150, Sites: 3000, Datastreams: 80000}, true, nil
	case "":
		if in.Orgs < 1 || in.Sites < 1 || in.Datastreams < 1 {
			return search.Spec{}, false, fmt.Errorf("search: name or orgs, sites, and datastreams required")
		}
		return search.Spec{Name: "custom", Orgs: in.Orgs, Sites: in.Sites, Datastreams: in.Datastreams}, false, nil
	default:
		return search.Spec{}, false, fmt.Errorf("search: unknown fixture %q", in.Name)
	}
}

// fixture loads a named benchmark catalog. Those catalogs have no area rings.
func fixture(in specIn) (search.Fixture, error) {
	switch in.Name {
	case "today-org":
		return search.TodayOrg()
	case "today-catalog":
		return search.TodayCatalog()
	case "large-org":
		return search.LargeOrg()
	case "large-catalog":
		return search.LargeCatalog()
	case "":
		if in.Orgs < 1 || in.Sites < 1 || in.Datastreams < 1 {
			return search.Fixture{}, fmt.Errorf("search: name or orgs, sites, and datastreams required")
		}
		return search.Generate(search.Spec{
			Name:        "custom",
			Orgs:        in.Orgs,
			Sites:       in.Sites,
			Datastreams: in.Datastreams,
		})
	default:
		return search.Fixture{}, fmt.Errorf("search: unknown fixture %q", in.Name)
	}
}

type boxIn struct {
	West  float64 `json:"west"`
	South float64 `json:"south"`
	East  float64 `json:"east"`
	North float64 `json:"north"`
}

type queryIn struct {
	Purpose           string   `json:"purpose"`
	Measurements      []string `json:"measurements"`
	SiteIDs           []string `json:"site_ids"`
	OrganizationIDs   []string `json:"organization_ids"`
	Viewport          *boxIn   `json:"viewport"`
	Geometry          string   `json:"geometry"`
	Zoom              uint32   `json:"zoom"`
	IncludeMemberOnly bool     `json:"include_member_only"`
	MapSiteLimit      uint32   `json:"map_site_limit"`
	MinSiteZoom       uint32   `json:"min_site_zoom"`
	PageSize          uint32   `json:"page_size"`
	PageAfter         uint32   `json:"page_after"`
}

type pointJSON struct {
	Lon float64 `json:"lon"`
	Lat float64 `json:"lat"`
}

type boxJSON struct {
	West  float64 `json:"west"`
	South float64 `json:"south"`
	East  float64 `json:"east"`
	North float64 `json:"north"`
}

type countJSON struct {
	Value string `json:"value"`
	Count uint32 `json:"count"`
}

type clusterJSON struct {
	Centroid  pointJSON `json:"centroid"`
	SiteCount uint32    `json:"site_count"`
	Expansion boxJSON   `json:"expansion"`
}

type siteJSON struct {
	SiteID           string    `json:"site_id"`
	Name             string    `json:"name"`
	OrganizationID   string    `json:"organization_id"`
	OrganizationName string    `json:"organization_name"`
	Point            pointJSON `json:"point"`
	Box              boxJSON   `json:"box"`
	DatastreamCount  uint32    `json:"datastream_count"`
}

type hitJSON struct {
	DatastreamID     string `json:"datastream_id"`
	Name             string `json:"name"`
	OrganizationID   string `json:"organization_id"`
	OrganizationName string `json:"organization_name"`
	StationID        string `json:"station_id,omitempty"`
	StationName      string `json:"station_name,omitempty"`
	SiteID           string `json:"site_id"`
	SiteName         string `json:"site_name"`
	Measurement      string `json:"measurement"`
	Purpose          string `json:"purpose"`
}

type queryOut struct {
	Kind              string        `json:"kind"`
	Clusters          []clusterJSON `json:"clusters"`
	Sites             []siteJSON    `json:"sites"`
	SiteTotal         uint32        `json:"site_total"`
	Bounds            boxJSON       `json:"bounds"`
	SiteCounts        []countJSON   `json:"site_counts"`
	MeasurementCounts []countJSON   `json:"measurement_counts"`
	Hits              []hitJSON     `json:"hits"`
	TotalCount        uint32        `json:"total_count"`
	NextPageAfter     uint32        `json:"next_page_after,omitempty"`
	HasMore           bool          `json:"has_more"`
	Paged             bool          `json:"paged"`
	ElapsedMs         float64       `json:"elapsed_ms"`
}

// query runs the same Query on each selected shard.
// One shard returns that shard's page cursor. Several shards go through
// Combine, which concatenates the first page and has no cross-org cursor.
func query(raw string) (queryOut, error) {
	if len(loaded) == 0 {
		return queryOut{}, fmt.Errorf("search: searchBuild first")
	}
	if raw == "" {
		return queryOut{}, fmt.Errorf("search: query required")
	}
	var in queryIn
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return queryOut{}, err
	}
	shards, err := selectShards(in.OrganizationIDs)
	if err != nil {
		return queryOut{}, err
	}
	q := search.Query{
		Purpose:           in.Purpose,
		Measurements:      in.Measurements,
		SiteIDs:           in.SiteIDs,
		Geometry:          in.Geometry,
		Zoom:              in.Zoom,
		IncludeMemberOnly: in.IncludeMemberOnly,
		MapSiteLimit:      in.MapSiteLimit,
		MinSiteZoom:       in.MinSiteZoom,
		PageSize:          in.PageSize,
		PageAfter:         in.PageAfter,
	}
	if in.Viewport != nil {
		q.Viewport = &search.Box{
			West:  in.Viewport.West,
			South: in.Viewport.South,
			East:  in.Viewport.East,
			North: in.Viewport.North,
		}
	}
	started := time.Now()
	parts := make([]search.Result, len(shards))
	for i, shard := range shards {
		parts[i], err = shard.Query(q)
		if err != nil {
			return queryOut{}, err
		}
	}
	var res search.Result
	paged := len(parts) == 1
	if paged {
		res = parts[0]
	} else {
		res, err = search.Combine(q, parts)
		if err != nil {
			return queryOut{}, err
		}
	}
	elapsed := time.Since(started)
	out := present(res, paged)
	out.ElapsedMs = float64(elapsed) / float64(time.Millisecond)
	return out, nil
}

// selectShards returns every loaded organization when ids is empty.
// Ids that were not built are dropped. An error means the caller named
// organizations and none of them are loaded.
func selectShards(ids []string) ([]*search.Shard, error) {
	if len(ids) == 0 {
		out := make([]*search.Shard, len(loaded))
		for i, org := range loaded {
			out[i] = org.shard
		}
		return out, nil
	}
	want := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		want[id] = struct{}{}
	}
	out := make([]*search.Shard, 0, len(ids))
	for _, org := range loaded {
		if _, ok := want[org.id]; ok {
			out = append(out, org.shard)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("search: no matching organizations")
	}
	return out, nil
}

// present copies the fields the page draws. Candidate sites, which Combine
// needs and the page must not receive, are left out. next_page_after is set
// only when paged is true.
func present(res search.Result, paged bool) queryOut {
	out := queryOut{
		Kind:              res.Kind.String(),
		Clusters:          make([]clusterJSON, len(res.Clusters)),
		Sites:             make([]siteJSON, len(res.Sites)),
		SiteTotal:         res.SiteTotal,
		Bounds:            toBox(res.Bounds),
		SiteCounts:        countsJSON(res.SiteCounts),
		MeasurementCounts: countsJSON(res.MeasurementCounts),
		Hits:              make([]hitJSON, len(res.Hits)),
		TotalCount:        res.TotalCount,
		HasMore:           res.HasMore,
		Paged:             paged,
	}
	for i, c := range res.Clusters {
		out.Clusters[i] = clusterJSON{
			Centroid:  toPoint(c.Centroid),
			SiteCount: c.SiteCount,
			Expansion: toBox(c.Expansion),
		}
	}
	for i, site := range res.Sites {
		out.Sites[i] = siteJSON{
			SiteID:           site.SiteID,
			Name:             site.Name,
			OrganizationID:   site.OrganizationID,
			OrganizationName: site.OrganizationName,
			Point:            toPoint(site.Point),
			Box:              toBox(site.Box),
			DatastreamCount:  site.DatastreamCount,
		}
	}
	for i, hit := range res.Hits {
		out.Hits[i] = hitJSON{
			DatastreamID:     hit.DatastreamID,
			Name:             hit.Name,
			OrganizationID:   hit.OrganizationID,
			OrganizationName: hit.OrganizationName,
			StationID:        hit.StationID,
			StationName:      hit.StationName,
			SiteID:           hit.SiteID,
			SiteName:         hit.SiteName,
			Measurement:      hit.Measurement,
			Purpose:          hit.Purpose,
		}
	}
	if paged {
		out.NextPageAfter = res.NextPageAfter
	}
	return out
}

func countsJSON(counts []search.Count) []countJSON {
	out := make([]countJSON, len(counts))
	for i, c := range counts {
		out[i] = countJSON{Value: c.Value, Count: c.Count}
	}
	return out
}

func toPoint(p search.Point) pointJSON {
	return pointJSON{Lon: p.Lon, Lat: p.Lat}
}

func toBox(b search.Box) boxJSON {
	return boxJSON{West: b.West, South: b.South, East: b.East, North: b.North}
}

func jsonErr(err error) string {
	b, _ := json.Marshal(struct {
		Error string `json:"error"`
	}{Error: err.Error()})
	return string(b)
}
