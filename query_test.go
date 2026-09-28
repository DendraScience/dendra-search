package search

import (
	"testing"
)

func TestAlgebra(t *testing.T) {
	sites := []Site{
		place("meadow", -120, 38),
		place("ridge", -121, 39),
		place("far", -105, 40),
	}
	stations := []Station{{
		ID: "station-move",
		Intervals: []Interval{
			{SiteID: "meadow", BeginsAt: 1000, EndsBefore: 2000},
			{SiteID: "ridge", BeginsAt: 3000, EndsBefore: 0},
		},
	}}
	datastreams := []Datastream{
		{ID: "datastream-air-meadow", StationID: "station-1", Measurement: "Air", Purpose: PurposeReadyToUse, PublicOK: true, MemberOK: true},
		{ID: "datastream-soil-meadow", StationID: "station-2", Measurement: "Soil", Purpose: PurposeReadyToUse, PublicOK: true, MemberOK: true},
		{ID: "datastream-air-ridge", StationID: "station-3", Measurement: "Air", Purpose: PurposeReadyToUse, PublicOK: true, MemberOK: true},
		{ID: "datastream-raw", StationID: "station-4", Measurement: "Air", Purpose: PurposeRaw, PublicOK: true, MemberOK: true},
		{ID: "datastream-member", StationID: "station-5", Measurement: "Air", Purpose: PurposeReadyToUse, PublicOK: false, MemberOK: true},
		{ID: "datastream-move", StationID: "station-move", Measurement: "Wind", Purpose: PurposeReadyToUse, PublicOK: true, MemberOK: true},
		{ID: "datastream-far", StationID: "station-far", Measurement: "Heat", Purpose: PurposeReadyToUse, PublicOK: true, MemberOK: true},
	}
	open := []Interval{{SiteID: "meadow", BeginsAt: 1, EndsBefore: 0}}
	stations = append(stations,
		Station{ID: "station-1", Intervals: open},
		Station{ID: "station-2", Intervals: open},
		Station{ID: "station-3", Intervals: []Interval{{SiteID: "ridge", BeginsAt: 1, EndsBefore: 0}}},
		Station{ID: "station-4", Intervals: open},
		Station{ID: "station-5", Intervals: open},
		Station{ID: "station-far", Intervals: []Interval{{SiteID: "far", BeginsAt: 1, EndsBefore: 0}}},
	)
	shard := buildShard(t, rows{sites: sites, stations: stations, datastreams: datastreams})

	all := query(t, shard, Query{Geometry: GeometryShown, Zoom: 11, PageSize: 20, IncludeMemberOnly: true})
	if all.TotalCount != 7 {
		t.Fatalf("all %d", all.TotalCount)
	}
	if all.Kind != KindSites {
		t.Fatalf("kind %s", all.Kind)
	}

	air := query(t, shard, Query{Geometry: GeometryShown, Zoom: 11, Measurements: []string{"Air"}, IncludeMemberOnly: true, PageSize: 20})
	if air.TotalCount != 4 {
		t.Fatalf("air hits %d", air.TotalCount)
	}
	if countOf(air.MeasurementCounts, "Soil") != 1 {
		t.Fatalf("soil count disappeared: %+v", air.MeasurementCounts)
	}
	if countOf(air.SiteCounts, "meadow") == 0 || countOf(air.SiteCounts, "ridge") == 0 {
		t.Fatalf("site counts %+v", air.SiteCounts)
	}

	both := query(t, shard, Query{
		Geometry: GeometryShown, Zoom: 11, IncludeMemberOnly: true,
		Measurements: []string{"Air", "Soil"}, SiteIDs: []string{"meadow"}, PageSize: 20,
	})
	if both.TotalCount != 4 {
		t.Fatalf("meadow air|soil %d", both.TotalCount)
	}
	onlySoilMeadow := query(t, shard, Query{
		Geometry: GeometryShown, Zoom: 11, IncludeMemberOnly: true,
		Measurements: []string{"Soil"}, SiteIDs: []string{"ridge"}, PageSize: 10,
	})
	if onlySoilMeadow.TotalCount != 0 {
		t.Fatalf("soil AND ridge = %d", onlySoilMeadow.TotalCount)
	}

	ready := query(t, shard, Query{Geometry: GeometryShown, Zoom: 11, Purpose: PurposeReadyToUse, IncludeMemberOnly: true, PageSize: 20})
	if countOf(ready.MeasurementCounts, "Air") != 3 {
		t.Fatalf("purpose leaked into measurement counts: %+v", ready.MeasurementCounts)
	}
	for _, hit := range ready.Hits {
		if hit.Purpose == PurposeRaw {
			t.Fatal("raw datastream in ready query")
		}
	}

	guest := query(t, shard, Query{Geometry: GeometryShown, Zoom: 11, PageSize: 20})
	member := query(t, shard, Query{Geometry: GeometryShown, Zoom: 11, PageSize: 20, IncludeMemberOnly: true})
	if guest.TotalCount != 6 || member.TotalCount != 7 {
		t.Fatalf("guest %d member %d", guest.TotalCount, member.TotalCount)
	}

	openQ := query(t, shard, Query{Geometry: GeometryShown, Zoom: 11, Measurements: []string{"Wind"}, PageSize: 5})
	if !hasCandidate(openQ, "ridge") || hasCandidate(openQ, "meadow") {
		t.Fatalf("open deployment sites: %+v", idsOf(openQ))
	}
	past := query(t, shard, Query{Geometry: GeometryShown, Zoom: 11, Measurements: []string{"Wind"}, BeginsAt: 1000, EndsBefore: 2000, PageSize: 5})
	if !hasCandidate(past, "meadow") || hasCandidate(past, "ridge") {
		t.Fatalf("historical site: %+v", idsOf(past))
	}
	gap := query(t, shard, Query{Geometry: GeometryShown, Zoom: 11, Measurements: []string{"Wind"}, BeginsAt: 2000, EndsBefore: 3000, PageSize: 5})
	if gap.TotalCount != 0 {
		t.Fatalf("gap %d", gap.TotalCount)
	}

	world := query(t, shard, Query{Geometry: GeometryShown, Zoom: 11, PageSize: 1})
	if !hasCandidate(world, "far") {
		t.Fatal("nil viewport dropped the far site")
	}
	ca := Box{West: -124.2, South: 32.5, East: -114.1, North: 42}
	boxed := query(t, shard, Query{Geometry: GeometryShown, Zoom: 11, Viewport: &ca, PageSize: 1})
	if hasCandidate(boxed, "far") {
		t.Fatal("viewport kept the far site")
	}

	again := query(t, shard, Query{Geometry: GeometryShown, Zoom: 11, PageSize: 20, IncludeMemberOnly: true})
	if again.TotalCount != all.TotalCount {
		t.Fatalf("second query changed the shard: %d vs %d", again.TotalCount, all.TotalCount)
	}
}

func TestClusterVersusSites(t *testing.T) {
	var sites []Site
	var stations []Station
	var datastreams []Datastream
	for i := 0; i < 3; i++ {
		id := string(rune('a' + i))
		sites = append(sites, place(id, -120.01+float64(i)*0.01, 38))
		stations = append(stations, Station{ID: "station-" + id, Intervals: []Interval{{SiteID: id, BeginsAt: 1, EndsBefore: 0}}})
		datastreams = append(datastreams, Datastream{ID: "datastream-" + id, StationID: "station-" + id, Measurement: "Air", PublicOK: true, MemberOK: true})
	}
	sites = append(sites, place("far", -105, 40))
	stations = append(stations, Station{ID: "station-far", Intervals: []Interval{{SiteID: "far", BeginsAt: 1, EndsBefore: 0}}})
	datastreams = append(datastreams, Datastream{ID: "datastream-far", StationID: "station-far", Measurement: "Air", PublicOK: true, MemberOK: true})
	shard := buildShard(t, rows{sites: sites, stations: stations, datastreams: datastreams})

	clusters := query(t, shard, Query{Geometry: GeometryShown, Zoom: 2})
	if clusters.Kind != KindClusters {
		t.Fatalf("zoom 2 kind %s", clusters.Kind)
	}
	if len(clusters.Clusters) != 2 {
		t.Fatalf("clusters %+v", clusters.Clusters)
	}
	if len(clusters.MeasurementCounts) != 0 || len(clusters.SiteCounts) != 0 {
		t.Fatal("cluster layer returned facet counts")
	}
	var dominant Cluster
	for _, c := range clusters.Clusters {
		if c.SiteCount > dominant.SiteCount {
			dominant = c
		}
	}
	if dominant.SiteCount != 3 {
		t.Fatalf("dominant %+v", dominant)
	}

	sitesLayer := query(t, shard, Query{Geometry: GeometryShown, Zoom: 11})
	if sitesLayer.Kind != KindSites || len(sitesLayer.Sites) != 4 {
		t.Fatalf("site layer %+v", sitesLayer.Sites)
	}
	if len(sitesLayer.MeasurementCounts) == 0 {
		t.Fatal("site layer omitted measurement counts")
	}

	forced := query(t, shard, Query{Geometry: GeometryShown, Zoom: 11, MapSiteLimit: 1})
	if forced.Kind != KindClusters {
		t.Fatalf("limit kind %s", forced.Kind)
	}
}

func TestAreaBoxNotCentroid(t *testing.T) {
	site := place("reserve", -120.05, 38.05)
	site.Shown = Box{West: -120.10, South: 38.00, East: -120.00, North: 38.10}
	site.Exact = site.Shown
	shard := buildShard(t, rows{
		sites: []Site{site},
		datastreams: []Datastream{{
			ID: "datastream-1", PublicOK: true, MemberOK: true,
			Intervals: []Interval{{SiteID: "reserve", BeginsAt: 1, EndsBefore: 0}},
		}},
	})
	corner := Box{West: -120.10, South: 38.00, East: -120.09, North: 38.01}
	res := query(t, shard, Query{Geometry: GeometryShown, Zoom: 11, Viewport: &corner})
	if !hasCandidate(res, "reserve") {
		t.Fatal("viewport missed the area box")
	}
}

func TestAntimeridian(t *testing.T) {
	shard := buildShard(t, rows{
		sites: []Site{place("west", 170, 10), place("center", 0, 10)},
		datastreams: []Datastream{
			{ID: "datastream-w", PublicOK: true, MemberOK: true, Intervals: []Interval{{SiteID: "west", BeginsAt: 1, EndsBefore: 0}}},
			{ID: "datastream-c", PublicOK: true, MemberOK: true, Intervals: []Interval{{SiteID: "center", BeginsAt: 1, EndsBefore: 0}}},
		},
	})
	wrap := Box{West: 160, South: 0, East: -170, North: 20}
	res := query(t, shard, Query{Geometry: GeometryExact, Zoom: 11, Viewport: &wrap})
	if !hasCandidate(res, "west") || hasCandidate(res, "center") {
		t.Fatalf("wrap candidates %+v", idsOf(res))
	}
}

func TestPaging(t *testing.T) {
	var stations []Station
	var datastreams []Datastream
	for i := 0; i < 5; i++ {
		id := string(rune('a' + i))
		stations = append(stations, Station{ID: "station-" + id, Intervals: []Interval{{SiteID: "a", BeginsAt: 1, EndsBefore: 0}}})
		datastreams = append(datastreams, Datastream{
			ID: "datastream-" + id, StationID: "station-" + id, PublicOK: true, MemberOK: true,
		})
	}
	shard := buildShard(t, rows{sites: []Site{place("a", -120, 38)}, stations: stations, datastreams: datastreams})
	var got []string
	var after uint32
	for page := 0; page < 4; page++ {
		res := query(t, shard, Query{Geometry: GeometryShown, Zoom: 11, PageSize: 2, PageAfter: after})
		if len(res.Hits) == 0 {
			t.Fatal("empty page")
		}
		for _, hit := range res.Hits {
			got = append(got, hit.DatastreamID)
		}
		if !res.HasMore {
			break
		}
		after = res.NextPageAfter
	}
	if len(got) != 5 {
		t.Fatalf("paged %v", got)
	}
	for i := 1; i < len(got); i++ {
		if got[i] < got[i-1] {
			t.Fatalf("order %v", got)
		}
	}
}

func TestBadQuery(t *testing.T) {
	shard := buildShard(t, sampleRows())
	if _, err := shard.Query(Query{Geometry: "private"}); err == nil {
		t.Fatal("accepted geometry")
	}
	bad := Box{West: 0, South: 10, East: 1, North: 0}
	if _, err := shard.Query(Query{Geometry: GeometryShown, Viewport: &bad}); err == nil {
		t.Fatal("accepted inverted viewport")
	}
	if _, err := shard.Query(Query{Geometry: GeometryShown, BeginsAt: 10}); err == nil {
		t.Fatal("accepted half window")
	}
}

func query(t *testing.T, shard *Shard, q Query) Result {
	t.Helper()
	res, err := shard.Query(q)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func countOf(counts []Count, value string) uint32 {
	for _, c := range counts {
		if c.Value == value {
			return c.Count
		}
	}
	return 0
}

func hasCandidate(res Result, id string) bool {
	for _, c := range res.Candidates {
		if c.SiteID == id {
			return true
		}
	}
	return false
}

func idsOf(res Result) []string {
	out := make([]string, len(res.Candidates))
	for i, c := range res.Candidates {
		out[i] = c.SiteID
	}
	return out
}
