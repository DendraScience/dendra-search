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
		ID:             "station-move",
		Name:           "Moved logger",
		OrganizationID: "org",
		Intervals: []Interval{
			{SiteID: "meadow", BeginsAt: 1000, EndsBefore: 2000},
			{SiteID: "ridge", BeginsAt: 3000, EndsBefore: 0},
		},
	}}
	datastreams := []Datastream{
		{ID: "datastream-air-meadow", OrganizationID: "org", StationID: "station-1", Measurement: "Air", Purpose: PurposeReadyToUse, PublicOK: true, MemberOK: true},
		{ID: "datastream-soil-meadow", OrganizationID: "org", StationID: "station-2", Measurement: "Soil", Purpose: PurposeReadyToUse, PublicOK: true, MemberOK: true},
		{ID: "datastream-air-ridge", OrganizationID: "org", StationID: "station-3", Measurement: "Air", Purpose: PurposeReadyToUse, PublicOK: true, MemberOK: true},
		{ID: "datastream-raw", OrganizationID: "org", StationID: "station-4", Measurement: "Air", Purpose: PurposeRaw, PublicOK: true, MemberOK: true},
		{ID: "datastream-member", OrganizationID: "org", StationID: "station-5", Measurement: "Air", Purpose: PurposeReadyToUse, PublicOK: false, MemberOK: true},
		{ID: "datastream-move", OrganizationID: "org", StationID: "station-move", Measurement: "Wind", Purpose: PurposeReadyToUse, PublicOK: true, MemberOK: true},
		{ID: "datastream-far", OrganizationID: "org", StationID: "station-far", Measurement: "Heat", Purpose: PurposeReadyToUse, PublicOK: true, MemberOK: true},
	}
	open := []Interval{{SiteID: "meadow", BeginsAt: 1, EndsBefore: 0}}
	stations = append(stations,
		Station{ID: "station-1", OrganizationID: "org", Intervals: open},
		Station{ID: "station-2", OrganizationID: "org", Intervals: open},
		Station{ID: "station-3", OrganizationID: "org", Intervals: []Interval{{SiteID: "ridge", BeginsAt: 1, EndsBefore: 0}}},
		Station{ID: "station-4", OrganizationID: "org", Intervals: open},
		Station{ID: "station-5", OrganizationID: "org", Intervals: open},
		Station{ID: "station-far", OrganizationID: "org", Intervals: []Interval{{SiteID: "far", BeginsAt: 1, EndsBefore: 0}}},
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
	if len(openQ.Hits) != 1 || openQ.Hits[0].SiteName != "ridge" || openQ.Hits[0].StationName != "Moved logger" {
		t.Fatalf("open hit %+v", openQ.Hits)
	}
	past := query(t, shard, Query{Geometry: GeometryShown, Zoom: 11, Measurements: []string{"Wind"}, BeginsAt: 1000, EndsBefore: 2000, PageSize: 5})
	if !hasCandidate(past, "meadow") || hasCandidate(past, "ridge") {
		t.Fatalf("historical site: %+v", idsOf(past))
	}
	if len(past.Hits) != 1 || past.Hits[0].SiteName != "meadow" || past.Hits[0].StationName != "Moved logger" {
		t.Fatalf("window hit %+v", past.Hits)
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
		stations = append(stations, Station{ID: "station-" + id, OrganizationID: "org", Intervals: []Interval{{SiteID: id, BeginsAt: 1, EndsBefore: 0}}})
		datastreams = append(datastreams, Datastream{ID: "datastream-" + id, OrganizationID: "org", StationID: "station-" + id, Measurement: "Air", PublicOK: true, MemberOK: true})
	}
	sites = append(sites, place("far", -105, 40))
	stations = append(stations, Station{ID: "station-far", OrganizationID: "org", Intervals: []Interval{{SiteID: "far", BeginsAt: 1, EndsBefore: 0}}})
	datastreams = append(datastreams, Datastream{ID: "datastream-far", OrganizationID: "org", StationID: "station-far", Measurement: "Air", PublicOK: true, MemberOK: true})
	shard := buildShard(t, rows{sites: sites, stations: stations, datastreams: datastreams})

	clusters := query(t, shard, Query{Geometry: GeometryShown, Zoom: 2})
	if clusters.Kind != KindClusters {
		t.Fatalf("zoom 2 kind %s", clusters.Kind)
	}
	below := query(t, shard, Query{Geometry: GeometryShown, Zoom: 5})
	if below.Kind != KindClusters {
		t.Fatalf("zoom 5 kind %s", below.Kind)
	}
	regional := query(t, shard, Query{Geometry: GeometryShown, Zoom: 6})
	if regional.Kind != KindSites || len(regional.Sites) != 4 {
		t.Fatalf("zoom 6 %+v", regional.Sites)
	}
	if len(clusters.Clusters) != 2 {
		t.Fatalf("clusters %+v", clusters.Clusters)
	}
	if countOf(clusters.MeasurementCounts, "Air") != 4 {
		t.Fatalf("cluster measurement counts %+v", clusters.MeasurementCounts)
	}
	if len(clusters.SiteCounts) != 0 {
		t.Fatalf("cluster site counts %+v", clusters.SiteCounts)
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
	if countOf(sitesLayer.MeasurementCounts, "Air") != 4 || len(sitesLayer.SiteCounts) != 4 {
		t.Fatalf("site layer counts meas %+v sites %+v", sitesLayer.MeasurementCounts, sitesLayer.SiteCounts)
	}

	forced := query(t, shard, Query{Geometry: GeometryShown, Zoom: 11, MapSiteLimit: 1})
	if forced.Kind != KindClusters {
		t.Fatalf("limit kind %s", forced.Kind)
	}
	if countOf(forced.MeasurementCounts, "Air") != 4 {
		t.Fatalf("limit measurement counts %+v", forced.MeasurementCounts)
	}
	if len(forced.SiteCounts) != 0 {
		t.Fatalf("limit site counts %+v", forced.SiteCounts)
	}
}

func TestAreaBoxNotCentroid(t *testing.T) {
	site := place("reserve", -120.05, 38.05)
	site.Shown = Box{West: -120.10, South: 38.00, East: -120.00, North: 38.10}
	site.Exact = site.Shown
	shard := buildShard(t, rows{
		sites: []Site{site},
		datastreams: []Datastream{{
			ID: "datastream-1", OrganizationID: "org", PublicOK: true, MemberOK: true,
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
			{ID: "datastream-w", OrganizationID: "org", PublicOK: true, MemberOK: true, Intervals: []Interval{{SiteID: "west", BeginsAt: 1, EndsBefore: 0}}},
			{ID: "datastream-c", OrganizationID: "org", PublicOK: true, MemberOK: true, Intervals: []Interval{{SiteID: "center", BeginsAt: 1, EndsBefore: 0}}},
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
	// Names sort Z..V while datastream ids sort a..e. Paging stays in doc id order.
	names := []string{"Zebra", "Yankee", "Xray", "Whiskey", "Victor"}
	for i := 0; i < 5; i++ {
		id := string(rune('a' + i))
		stations = append(stations, Station{
			ID: "station-" + id, Name: names[i], OrganizationID: "org",
			Intervals: []Interval{{SiteID: "a", BeginsAt: 1, EndsBefore: 0}},
		})
		datastreams = append(datastreams, Datastream{
			ID: "datastream-" + id, OrganizationID: "org", StationID: "station-" + id, PublicOK: true, MemberOK: true,
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
			if hit.StationName != names[len(got)-1] {
				t.Fatalf("station name %q on %s", hit.StationName, hit.DatastreamID)
			}
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

func TestClosedDeployment(t *testing.T) {
	r := rows{
		sites: []Site{place("meadow", -120, 38)},
		stations: []Station{{
			ID: "station-retired", Name: "Retired logger", OrganizationID: "org",
			Intervals: []Interval{{SiteID: "meadow", BeginsAt: 1000, EndsBefore: 2000, GroupID: "field"}},
		}},
		datastreams: []Datastream{
			{ID: "ds-retired", Name: "Air", OrganizationID: "org", StationID: "station-retired", Measurement: "Air", Purpose: PurposeReadyToUse, PublicOK: true, MemberOK: true},
			{ID: "ds-site", Name: "Soil", OrganizationID: "org", Measurement: "Soil", Purpose: PurposeReadyToUse, PublicOK: true, MemberOK: true, Intervals: []Interval{{SiteID: "meadow", BeginsAt: 1000, EndsBefore: 2000, GroupID: "lab"}}},
		},
	}
	b := NewBuilder()
	if err := b.AddSites(r.sites); err != nil {
		t.Fatal(err)
	}
	if err := b.AddStations(r.stations); err != nil {
		t.Fatal(err)
	}
	if err := b.AddDatastreams(r.datastreams); err != nil {
		t.Fatal(err)
	}
	priv, n, err := b.Finish(FinishPrivate)
	if err != nil || n != 2 {
		t.Fatalf("private docs: %d %v", n, err)
	}
	pub, nPub, err := b.Finish(FinishPublic)
	if err != nil || nPub != 2 {
		t.Fatalf("public docs: %d %v", nPub, err)
	}
	for _, blob := range [][]byte{priv, pub} {
		shard, err := Open(blob)
		if err != nil {
			t.Fatal(err)
		}
		open := query(t, shard, Query{Geometry: GeometryShown, Zoom: 11, PageSize: 10, IncludeMemberOnly: true})
		if open.TotalCount != 0 || len(open.Candidates) != 0 {
			t.Fatalf("open count %d candidates %v", open.TotalCount, idsOf(open))
		}
		groups, err := shard.GroupIDs(0, 0, true)
		if err != nil || len(groups) != 0 {
			t.Fatalf("open groups %v %v", groups, err)
		}
		window := query(t, shard, Query{Geometry: GeometryShown, Zoom: 11, BeginsAt: 1000, EndsBefore: 2000, PageSize: 10, IncludeMemberOnly: true})
		if window.TotalCount != 2 || !hasCandidate(window, "meadow") {
			t.Fatalf("window count %d candidates %v", window.TotalCount, idsOf(window))
		}
		for _, hit := range window.Hits {
			if hit.SiteID != "meadow" {
				t.Fatalf("hit %+v", hit)
			}
		}
		groups, err = shard.GroupIDs(1000, 2000, true)
		if err != nil {
			t.Fatal(err)
		}
		if len(groups) != 2 || groups[0] != "field" || groups[1] != "lab" {
			t.Fatalf("window groups %v", groups)
		}
		after := query(t, shard, Query{Geometry: GeometryShown, Zoom: 11, BeginsAt: 2000, EndsBefore: 3000, PageSize: 10, IncludeMemberOnly: true})
		if after.TotalCount != 0 {
			t.Fatalf("after end %d", after.TotalCount)
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
