//go:build js && wasm

// Product browser host for Explore. The page fetches shard bytes and
// hands them here; this module opens them and answers picker and query
// questions.
//
//	searchLoadContainer(uint8array) -> {organizations} JSON
//	searchLoadMemberShard(orgId, uint8array) -> {organization} JSON
//	searchGroupIDs(boundsJSON) -> groups by organization JSON
//	searchReducePicker(pickerJSON) -> organization and group rows JSON
//	searchQuery(queryJSON) -> result JSON
//
// All return a JSON string. Failures are {"error":"..."}. A failed load
// keeps the shards already open. The module never fetches and never
// gunzips: the page's fetch has already stripped Content-Encoding.
package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"syscall/js"
	"time"

	"github.com/dendrascience/dendra-search"
)

type loadedShard struct {
	shard  *search.Shard
	member bool
}

var loaded = map[string]*loadedShard{}

func main() {
	js.Global().Set("searchLoadContainer", keep(js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) < 1 {
			return jsonErr(fmt.Errorf("search: container bytes required"))
		}
		out, err := loadContainer(bytesArg(args[0]))
		if err != nil {
			return jsonErr(err)
		}
		return jsonOut(out)
	})))

	js.Global().Set("searchLoadMemberShard", keep(js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) < 2 {
			return jsonErr(fmt.Errorf("search: organization id and shard bytes required"))
		}
		out, err := loadMemberShard(args[0].String(), bytesArg(args[1]))
		if err != nil {
			return jsonErr(err)
		}
		return jsonOut(out)
	})))

	js.Global().Set("searchGroupIDs", keep(js.FuncOf(func(_ js.Value, args []js.Value) any {
		raw := ""
		if len(args) > 0 && !args[0].IsUndefined() && !args[0].IsNull() {
			raw = args[0].String()
		}
		out, err := groupIDs(raw)
		if err != nil {
			return jsonErr(err)
		}
		return jsonOut(out)
	})))

	js.Global().Set("searchReducePicker", keep(js.FuncOf(func(_ js.Value, args []js.Value) any {
		raw := ""
		if len(args) > 0 && !args[0].IsUndefined() && !args[0].IsNull() {
			raw = args[0].String()
		}
		out, err := reducePicker(raw)
		if err != nil {
			return jsonErr(err)
		}
		return jsonOut(out)
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
		return jsonOut(out)
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

func bytesArg(v js.Value) []byte {
	b := make([]byte, v.Length())
	js.CopyBytesToGo(b, v)
	return b
}

type loadContainerOut struct {
	Organizations []string `json:"organizations"`
}

// loadContainer opens every public chunk in the frame. Member shards
// already open are kept: a member organization replaces its public
// chunk, so a later container load must not resurrect the chunk.
func loadContainer(frame []byte) (loadContainerOut, error) {
	chunks, err := search.ParseContainer(frame)
	if err != nil {
		return loadContainerOut{}, err
	}
	next := make(map[string]*loadedShard, len(loaded)+len(chunks))
	for id, entry := range loaded {
		if entry.member {
			next[id] = entry
		}
	}
	for _, chunk := range chunks {
		if _, ok := next[chunk.OrganizationID]; ok {
			continue
		}
		shard, err := search.Open(chunk.Bytes)
		if err != nil {
			return loadContainerOut{}, err
		}
		next[chunk.OrganizationID] = &loadedShard{shard: shard}
	}
	loaded = next
	return loadContainerOut{Organizations: sortedIDs(loaded)}, nil
}

type loadMemberShardOut struct {
	Organization string `json:"organization"`
}

// loadMemberShard opens one private shard. It replaces the public chunk
// for the same organization: querying or grouping both would count the
// organization twice.
func loadMemberShard(orgID string, b []byte) (loadMemberShardOut, error) {
	if orgID == "" {
		return loadMemberShardOut{}, fmt.Errorf("search: organization id required")
	}
	shard, err := search.Open(b)
	if err != nil {
		return loadMemberShardOut{}, err
	}
	loaded[orgID] = &loadedShard{shard: shard, member: true}
	return loadMemberShardOut{Organization: orgID}, nil
}

func sortedIDs(m map[string]*loadedShard) []string {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

type groupIDsIn struct {
	BeginsAt   int64 `json:"begins_at"`
	EndsBefore int64 `json:"ends_before"`
}

type groupIDsOut struct {
	GroupsByOrg map[string][]string `json:"groups_by_org"`
}

// groupIDs calls GroupIDs on every open shard. Public chunks answer
// with public access; a private shard answers with member access. The
// page uses the same time bounds as the search query.
func groupIDs(raw string) (groupIDsOut, error) {
	var in groupIDsIn
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &in); err != nil {
			return groupIDsOut{}, err
		}
	}
	out := groupIDsOut{GroupsByOrg: make(map[string][]string, len(loaded))}
	for _, id := range sortedIDs(loaded) {
		entry := loaded[id]
		ids, err := entry.shard.GroupIDs(in.BeginsAt, in.EndsBefore, entry.member)
		if err != nil {
			return groupIDsOut{}, err
		}
		if ids == nil {
			ids = []string{}
		}
		out.GroupsByOrg[id] = ids
	}
	return out, nil
}

type catalogNameJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type pickerIn struct {
	Orgs          []catalogNameJSON   `json:"orgs"`
	Groups        []catalogNameJSON   `json:"groups"`
	CheckedOrgs   []string            `json:"checked_orgs"`
	CheckedGroups []string            `json:"checked_groups"`
	GroupsByOrg   map[string][]string `json:"groups_by_org"`
}

type pickerRowJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Grey bool   `json:"grey"`
}

type pickerOut struct {
	Orgs   []pickerRowJSON `json:"orgs"`
	Groups []pickerRowJSON `json:"groups"`
}

// reducePicker calls ReducePicker. The page has already applied the text filter.
func reducePicker(raw string) (pickerOut, error) {
	if raw == "" {
		return pickerOut{}, fmt.Errorf("search: picker required")
	}
	var in pickerIn
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return pickerOut{}, err
	}
	orgRows, groupRows := search.ReducePicker(
		catalogNames(in.Orgs),
		catalogNames(in.Groups),
		in.CheckedOrgs,
		in.CheckedGroups,
		in.GroupsByOrg,
	)
	return pickerOut{Orgs: pickerRows(orgRows), Groups: pickerRows(groupRows)}, nil
}

func catalogNames(in []catalogNameJSON) []search.CatalogName {
	out := make([]search.CatalogName, len(in))
	for i, row := range in {
		out[i] = search.CatalogName{ID: row.ID, Name: row.Name}
	}
	return out
}

func pickerRows(in []search.PickerRow) []pickerRowJSON {
	out := make([]pickerRowJSON, len(in))
	for i, row := range in {
		out[i] = pickerRowJSON{ID: row.ID, Name: row.Name, Grey: row.Grey}
	}
	return out
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
	GroupIDs          []string `json:"group_ids"`
	OrganizationIDs   []string `json:"organization_ids"`
	Viewport          *boxIn   `json:"viewport"`
	Geometry          string   `json:"geometry"`
	Zoom              uint32   `json:"zoom"`
	IncludeMemberOnly bool     `json:"include_member_only"`
	MapSiteLimit      uint32   `json:"map_site_limit"`
	MinSiteZoom       uint32   `json:"min_site_zoom"`
	PageSize          uint32   `json:"page_size"`
	PageAfter         uint32   `json:"page_after"`
	AllHits           bool     `json:"all_hits"`
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
	Value            string `json:"value"`
	Count            uint32 `json:"count"`
	Name             string `json:"name,omitempty"`
	OrganizationID   string `json:"organization_id,omitempty"`
	OrganizationName string `json:"organization_name,omitempty"`
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
	DatastreamID     string   `json:"datastream_id"`
	Name             string   `json:"name"`
	OrganizationID   string   `json:"organization_id"`
	OrganizationName string   `json:"organization_name"`
	StationID        string   `json:"station_id,omitempty"`
	StationName      string   `json:"station_name,omitempty"`
	SiteID           string   `json:"site_id"`
	SiteName         string   `json:"site_name"`
	Measurement      string   `json:"measurement"`
	Purpose          string   `json:"purpose"`
	FieldNames       []string `json:"field_names"`
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

// query runs the same Query on each selected shard. One shard returns that
// shard's page cursor. Several shards go through Combine, which concatenates
// the first page and has no cross-org cursor.
func query(raw string) (queryOut, error) {
	if len(loaded) == 0 {
		return queryOut{}, fmt.Errorf("search: load shards first")
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
		GroupIDs:          in.GroupIDs,
		Geometry:          in.Geometry,
		Zoom:              in.Zoom,
		IncludeMemberOnly: in.IncludeMemberOnly,
		MapSiteLimit:      in.MapSiteLimit,
		MinSiteZoom:       in.MinSiteZoom,
		PageSize:          in.PageSize,
		PageAfter:         in.PageAfter,
		AllHits:           in.AllHits,
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
// Ids that were not loaded are dropped. An error means the caller named
// organizations and none of them are loaded.
func selectShards(ids []string) ([]*search.Shard, error) {
	if len(ids) == 0 {
		out := make([]*search.Shard, 0, len(loaded))
		for _, id := range sortedIDs(loaded) {
			out = append(out, loaded[id].shard)
		}
		return out, nil
	}
	want := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		want[id] = struct{}{}
	}
	out := make([]*search.Shard, 0, len(ids))
	for _, id := range sortedIDs(loaded) {
		if _, ok := want[id]; ok {
			out = append(out, loaded[id].shard)
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
			FieldNames:       fieldNamesJSON(hit.FieldNames),
		}
	}
	if paged {
		out.NextPageAfter = res.NextPageAfter
	}
	return out
}

func fieldNamesJSON(names []string) []string {
	if names == nil {
		return []string{}
	}
	return names
}

func countsJSON(counts []search.Count) []countJSON {
	out := make([]countJSON, len(counts))
	for i, c := range counts {
		out[i] = countJSON{
			Value:            c.Value,
			Count:            c.Count,
			Name:             c.Name,
			OrganizationID:   c.OrganizationID,
			OrganizationName: c.OrganizationName,
		}
	}
	return out
}

func toPoint(p search.Point) pointJSON {
	return pointJSON{Lon: p.Lon, Lat: p.Lat}
}

func toBox(b search.Box) boxJSON {
	return boxJSON{West: b.West, South: b.South, East: b.East, North: b.North}
}

func jsonOut(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return jsonErr(err)
	}
	return string(b)
}

func jsonErr(err error) string {
	b, _ := json.Marshal(struct {
		Error string `json:"error"`
	}{Error: err.Error()})
	return string(b)
}
