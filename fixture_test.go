package search

import (
	"bytes"
	"math"
	"testing"
)

func TestTodayRoundTrip(t *testing.T) {
	fix := mustFixture(t, TodayOrg)
	if got := sumCounts(fix.DocCounts); got != 600 {
		t.Fatalf("docs %d", got)
	}
	shard, err := Open(fix.Blobs[0])
	if err != nil {
		t.Fatal(err)
	}
	if shard.DocCount() != 600 {
		t.Fatalf("open docs %d", shard.DocCount())
	}
	cases := BenchCases(fix)
	exact := query(t, shard, cases[3].Query)
	shown := query(t, shard, cases[4].Query)
	if !hasCandidate(exact, fix.ShiftedSiteID) {
		t.Fatalf("exact viewport missed %s", fix.ShiftedSiteID)
	}
	if hasCandidate(shown, fix.ShiftedSiteID) {
		t.Fatalf("shown viewport hit %s", fix.ShiftedSiteID)
	}
}

func TestCatalogZoom2(t *testing.T) {
	fix := mustFixture(t, TodayCatalog)
	shards := openAll(t, fix.Blobs)
	q := BenchCases(fix)[0].Query
	parts := make([]Result, len(shards))
	for i, shard := range shards {
		parts[i] = query(t, shard, q)
	}
	res, err := Combine(q, parts)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindClusters {
		t.Fatalf("kind %s", res.Kind)
	}
	if res.SiteTotal < 250 {
		t.Fatalf("sites %d", res.SiteTotal)
	}
	if len(res.Clusters) == 0 || len(res.Clusters) >= int(res.SiteTotal)/2 {
		t.Fatalf("clusters %d for %d sites", len(res.Clusters), res.SiteTotal)
	}
	dominant := res.Clusters[0]
	outside := 0
	for _, c := range res.Clusters {
		if c.SiteCount > dominant.SiteCount {
			dominant = c
		}
		if !inCalifornia(c.Centroid) {
			outside++
		}
	}
	if !inCalifornia(dominant.Centroid) {
		t.Fatalf("dominant centroid %+v count %d", dominant.Centroid, dominant.SiteCount)
	}
	if dominant.SiteCount*2 <= res.SiteTotal {
		t.Fatalf("dominant %d is not a majority of %d", dominant.SiteCount, res.SiteTotal)
	}
	if outside < 1 {
		t.Fatal("no outlier badge")
	}
	if len(res.MeasurementCounts) == 0 {
		t.Fatal("zoom 2 omitted measurement counts")
	}
	if len(res.SiteCounts) != 0 {
		t.Fatalf("zoom 2 site counts %d", len(res.SiteCounts))
	}
}

func TestGeneratorShape(t *testing.T) {
	fix := mustFixture(t, TodayOrg)
	again, err := TodayOrg()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fix.Blobs[0], again.Blobs[0]) {
		t.Fatal("generator is not deterministic")
	}
	shard, err := Open(fix.Blobs[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(shard.sites) != 25 {
		t.Fatalf("sites %d", len(shard.sites))
	}
	var outside, polygons, shiftSmall, shiftLarge int
	for _, site := range shard.sites {
		if !inCalifornia(site.ExactPoint) {
			outside++
		}
		if site.Exact.East-site.Exact.West > 0.04 {
			polygons++
		}
		dLon := site.ShownPoint.Lon - site.ExactPoint.Lon
		dLat := site.ShownPoint.Lat - site.ExactPoint.Lat
		switch {
		case math.Abs(dLon-1) < 1e-9 && math.Abs(dLat-1) < 1e-9:
			shiftLarge++
			if site.ID != fix.ShiftedSiteID {
				t.Fatalf("shifted id %s want %s", site.ID, fix.ShiftedSiteID)
			}
		case math.Abs(dLon-0.1) < 1e-9 && math.Abs(dLat-0.1) < 1e-9:
			shiftSmall++
		default:
			if dLon != 0 || dLat != 0 {
				t.Fatalf("unexpected shift %+v", site)
			}
		}
	}
	if outside != outlierCount(25) {
		t.Fatalf("outliers %d", outside)
	}
	if polygons != 2 {
		t.Fatalf("polygons %d", polygons)
	}
	if shiftLarge != 1 || shiftSmall != 3 {
		t.Fatalf("shifts large %d small %d", shiftLarge, shiftSmall)
	}

	guest := query(t, shard, Query{Geometry: GeometryExact, Zoom: 11, PageSize: 1})
	member := query(t, shard, Query{Geometry: GeometryExact, Zoom: 11, PageSize: 1, IncludeMemberOnly: true})
	if guest.TotalCount != 540 || member.TotalCount != 600 {
		t.Fatalf("guest %d member %d", guest.TotalCount, member.TotalCount)
	}
	ready := query(t, shard, Query{Geometry: GeometryExact, Zoom: 11, Purpose: PurposeReadyToUse, PageSize: 1})
	if ready.TotalCount != 420 {
		t.Fatalf("ready %d", ready.TotalCount)
	}

	gap := query(t, shard, Query{
		Geometry: GeometryExact, Zoom: 11, PageSize: 600,
		BeginsAt: fixtureEpoch + 10000, EndsBefore: fixtureEpoch + 20000,
	})
	if gap.TotalCount >= guest.TotalCount {
		t.Fatalf("gap %d unconstrained %d", gap.TotalCount, guest.TotalCount)
	}
	ids := map[string]bool{}
	for _, hit := range gap.Hits {
		ids[hit.DatastreamID] = true
	}
	if ids["datastream-000005"] {
		t.Fatal("gap included a datastream with a hole in its deployments")
	}
	if !ids["datastream-000001"] {
		t.Fatal("gap excluded an open datastream")
	}

	named := query(t, shard, Query{
		Geometry: GeometryExact, Zoom: 11, PageSize: 600, IncludeMemberOnly: true,
	})
	var siteAttached, stationAttached bool
	for _, hit := range named.Hits {
		switch hit.DatastreamID {
		case "datastream-000000":
			siteAttached = true
			if hit.StationID != "" || hit.StationName != "" {
				t.Fatalf("site-attached %+v", hit)
			}
		case "datastream-000001":
			stationAttached = true
			if hit.StationName != "Station 000001" {
				t.Fatalf("station name %+v", hit)
			}
		}
	}
	if !siteAttached || !stationAttached {
		t.Fatal("fixture hits missing station-name cases")
	}
}

func mustFixture(t *testing.T, gen func() (Fixture, error)) Fixture {
	t.Helper()
	fix, err := gen()
	if err != nil {
		t.Fatal(err)
	}
	return fix
}

func openAll(t *testing.T, blobs [][]byte) []*Shard {
	t.Helper()
	out := make([]*Shard, len(blobs))
	for i, blob := range blobs {
		shard, err := Open(blob)
		if err != nil {
			t.Fatal(err)
		}
		out[i] = shard
	}
	return out
}

func sumCounts(n []uint32) int {
	var s int
	for _, v := range n {
		s += int(v)
	}
	return s
}
