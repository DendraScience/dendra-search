package search

import (
	"bytes"
	"errors"
	"testing"
)

func TestAddOrder(t *testing.T) {
	b := NewBuilder()
	if err := b.AddStation(Station{ID: "station-1", OrganizationID: "org", Intervals: []Interval{{SiteID: "a", BeginsAt: 1, EndsBefore: 0}}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("station before its site: %v", err)
	}
	if b.HasSite("a") || b.HasStation("station-1") {
		t.Fatal("membership before add")
	}
	site := place("a", -120, 38)
	if err := b.AddSite(site); err != nil {
		t.Fatal(err)
	}
	if !b.HasSite("a") || b.HasSite("missing") {
		t.Fatal("site membership")
	}
	if err := b.AddSite(site); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate: %v", err)
	}
	station := Station{ID: "station-1", OrganizationID: "org", Intervals: []Interval{{SiteID: "a", BeginsAt: 1, EndsBefore: 0}}}
	if err := b.AddStation(station); err != nil {
		t.Fatal(err)
	}
	if !b.HasStation("station-1") || b.HasStation("missing") {
		t.Fatal("station membership")
	}
	if err := b.AddSite(place("b", -121, 38)); !errors.Is(err, ErrOrder) {
		t.Fatalf("site after station: %v", err)
	}
	if err := b.AddDatastream(Datastream{
		ID: "datastream-1", OrganizationID: "org", StationID: "station-1", PublicOK: true, MemberOK: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := b.AddStation(Station{ID: "station-2", OrganizationID: "org", Intervals: []Interval{{SiteID: "a", BeginsAt: 1, EndsBefore: 0}}}); !errors.Is(err, ErrOrder) {
		t.Fatalf("station after datastream: %v", err)
	}
	blob, n, err := b.Finish(FinishPrivate)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || len(blob) == 0 {
		t.Fatalf("n=%d len=%d", n, len(blob))
	}
	again, n2, err := b.Finish(FinishPrivate)
	if err != nil {
		t.Fatal(err)
	}
	if n2 != n || !bytes.Equal(blob, again) {
		t.Fatalf("second finish bytes differ n=%d", n2)
	}
	if err := b.AddDatastream(Datastream{
		ID: "datastream-2", OrganizationID: "org", StationID: "station-1", PublicOK: true,
	}); !errors.Is(err, ErrFinished) {
		t.Fatalf("add after finish: %v", err)
	}
}

func TestIntervalRules(t *testing.T) {
	sites := []Site{place("a", -120, 38), place("b", -121, 39)}
	b := NewBuilder()
	if err := b.AddSites(sites); err != nil {
		t.Fatal(err)
	}
	if err := b.AddStation(Station{ID: "station-closed", OrganizationID: "org", Intervals: []Interval{
		{SiteID: "a", BeginsAt: 1, EndsBefore: 10},
		{SiteID: "b", BeginsAt: 10, EndsBefore: 20},
	}}); err != nil {
		t.Fatalf("closed deployment: %v", err)
	}
	err := b.AddStation(Station{ID: "station-1", OrganizationID: "org", Intervals: []Interval{
		{SiteID: "a", BeginsAt: 1, EndsBefore: 10},
		{SiteID: "b", BeginsAt: 5, EndsBefore: 0},
	}})
	if !errors.Is(err, ErrInterval) {
		t.Fatalf("overlap: %v", err)
	}
	err = b.AddStation(Station{ID: "station-1", OrganizationID: "org", Intervals: []Interval{
		{SiteID: "a", BeginsAt: 1, EndsBefore: 0},
		{SiteID: "b", BeginsAt: 10, EndsBefore: 0},
	}})
	if !errors.Is(err, ErrInterval) {
		t.Fatalf("two open: %v", err)
	}
	if err := b.AddStation(Station{ID: "station-1", OrganizationID: "org", Intervals: []Interval{
		{SiteID: "missing", BeginsAt: 1, EndsBefore: 0},
	}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing site: %v", err)
	}
	if err := b.AddDatastream(Datastream{ID: "datastream-1", OrganizationID: "org", StationID: "nope", PublicOK: true}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing station: %v", err)
	}
}

func TestOrganization(t *testing.T) {
	b := NewBuilder()
	bare := place("a", -120, 38)
	bare.OrganizationID = ""
	if err := b.AddSite(bare); !errors.Is(err, ErrInvalid) {
		t.Fatalf("site organization: %v", err)
	}
	if err := b.AddSite(place("a", -120, 38)); err != nil {
		t.Fatal(err)
	}
	other := place("b", -121, 39)
	other.OrganizationID = "other"
	if err := b.AddSite(other); !errors.Is(err, ErrInvalid) {
		t.Fatalf("second site organization: %v", err)
	}
	if err := b.AddSite(place("b", -121, 39)); err != nil {
		t.Fatal(err)
	}
	if err := b.AddStation(Station{
		ID:        "station-1",
		Intervals: []Interval{{SiteID: "a", BeginsAt: 1, EndsBefore: 0}},
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("station organization: %v", err)
	}
	if err := b.AddStation(Station{
		ID:             "station-1",
		OrganizationID: "other",
		Intervals:      []Interval{{SiteID: "a", BeginsAt: 1, EndsBefore: 0}},
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("station mismatch: %v", err)
	}
	if err := b.AddStation(Station{
		ID:             "station-1",
		OrganizationID: "org",
		Intervals: []Interval{
			{SiteID: "a", BeginsAt: 1, EndsBefore: 10},
			{SiteID: "b", BeginsAt: 10, EndsBefore: 0},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := b.AddDatastream(Datastream{ID: "ds-1", StationID: "station-1"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("datastream organization: %v", err)
	}
	if err := b.AddDatastream(Datastream{ID: "ds-1", OrganizationID: "other", StationID: "station-1"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("datastream mismatch: %v", err)
	}
	if err := b.AddDatastream(Datastream{
		ID:             "ds-site",
		OrganizationID: "other",
		Intervals:      []Interval{{SiteID: "a", BeginsAt: 1, EndsBefore: 0}},
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("site-attached mismatch: %v", err)
	}
	if err := b.AddDatastream(Datastream{ID: "ds-1", OrganizationID: "org", StationID: "station-1"}); err != nil {
		t.Fatal(err)
	}
}

func TestDeterministicBytes(t *testing.T) {
	a := buildBytes(t, sampleRows())
	b := buildBytes(t, sampleRows())
	if !bytes.Equal(a, b) {
		t.Fatal("shard bytes differ")
	}
	if _, err := Open(a); err != nil {
		t.Fatal(err)
	}
	if _, err := Open([]byte("nope")); !errors.Is(err, ErrFormat) {
		t.Fatalf("bad magic: %v", err)
	}
	if _, err := Open(a[:len(a)-1]); !errors.Is(err, ErrFormat) {
		t.Fatalf("truncated: %v", err)
	}
}

func TestSliceHelpersLoop(t *testing.T) {
	shard := buildShard(t, sampleRows())
	if shard.DocCount() != 2 {
		t.Fatalf("docs %d", shard.DocCount())
	}
	res := query(t, shard, Query{Geometry: GeometryShown, Zoom: 11, PageSize: 10, IncludeMemberOnly: true})
	if len(res.Hits) != 2 {
		t.Fatalf("hits %d", len(res.Hits))
	}
	byID := map[string]Hit{}
	for _, hit := range res.Hits {
		byID[hit.DatastreamID] = hit
	}
	station := byID["datastream-1"]
	if station.StationID != "station-1" || station.StationName != "Alpha logger" {
		t.Fatalf("station hit %+v", station)
	}
	siteAttached := byID["datastream-2"]
	if siteAttached.StationID != "" || siteAttached.StationName != "" {
		t.Fatalf("site-attached hit %+v", siteAttached)
	}
}

type rows struct {
	sites       []Site
	stations    []Station
	datastreams []Datastream
}

func sampleRows() rows {
	return rows{
		sites: []Site{place("a", -120, 38), place("b", -121, 39)},
		stations: []Station{{
			ID:             "station-1",
			Name:           "Alpha logger",
			OrganizationID: "org",
			Intervals: []Interval{
				{SiteID: "a", BeginsAt: 1, EndsBefore: 10},
				{SiteID: "b", BeginsAt: 10, EndsBefore: 0},
			},
		}},
		datastreams: []Datastream{
			{ID: "datastream-1", OrganizationID: "org", StationID: "station-1", Measurement: "Meas01", Purpose: PurposeReadyToUse, PublicOK: true, MemberOK: true},
			{ID: "datastream-2", OrganizationID: "org", Measurement: "Meas02", Purpose: PurposeReadyToUse, PublicOK: true, MemberOK: true, Intervals: []Interval{{SiteID: "a", BeginsAt: 1, EndsBefore: 0}}},
		},
	}
}

func place(id string, lon, lat float64) Site {
	p := Point{Lon: lon, Lat: lat}
	box := Box{West: lon, South: lat, East: lon, North: lat}
	return Site{
		ID: id, Name: id, OrganizationID: "org", OrganizationName: "Org",
		Shown: box, Exact: box, ShownPoint: p, ExactPoint: p,
	}
}

func buildShard(t *testing.T, r rows) *Shard {
	t.Helper()
	blob := buildBytes(t, r)
	shard, err := Open(blob)
	if err != nil {
		t.Fatal(err)
	}
	return shard
}

func buildBytes(t *testing.T, r rows) []byte {
	t.Helper()
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
	blob, _, err := b.Finish(FinishPrivate)
	if err != nil {
		t.Fatal(err)
	}
	return blob
}

func TestFinishModes(t *testing.T) {
	privFirst := NewBuilder()
	fillModes(t, privFirst)
	privA, nPriv, err := privFirst.Finish(FinishPrivate)
	if err != nil {
		t.Fatal(err)
	}
	pubA, nPub, err := privFirst.Finish(FinishPublic)
	if err != nil {
		t.Fatal(err)
	}
	again, _, err := privFirst.Finish(FinishPrivate)
	if err != nil || !bytes.Equal(privA, again) {
		t.Fatalf("private finish is not repeatable: %v", err)
	}
	again, _, err = privFirst.Finish(FinishPublic)
	if err != nil || !bytes.Equal(pubA, again) {
		t.Fatalf("public finish is not repeatable: %v", err)
	}
	if err := privFirst.AddSite(place("late", -119, 37)); !errors.Is(err, ErrFinished) {
		t.Fatalf("add after finish: %v", err)
	}
	if _, _, err := privFirst.Finish(FinishMode(9)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown mode: %v", err)
	}

	openBuilder := NewBuilder()
	if err := openBuilder.AddSite(place("a", -120, 38)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := openBuilder.Finish(FinishMode(9)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown mode sealed the builder: %v", err)
	}
	if err := openBuilder.AddStation(Station{ID: "station-1", OrganizationID: "org", Intervals: []Interval{{SiteID: "a", BeginsAt: 1, EndsBefore: 0}}}); err != nil {
		t.Fatalf("add after rejected finish: %v", err)
	}

	pubFirst := NewBuilder()
	fillModes(t, pubFirst)
	pubB, nPubB, err := pubFirst.Finish(FinishPublic)
	if err != nil {
		t.Fatal(err)
	}
	privB, nPrivB, err := pubFirst.Finish(FinishPrivate)
	if err != nil {
		t.Fatal(err)
	}
	if nPriv != 5 || nPrivB != nPriv || nPub != 2 || nPubB != nPub {
		t.Fatalf("docs private %d/%d public %d/%d", nPriv, nPrivB, nPub, nPubB)
	}
	if !bytes.Equal(privA, privB) || !bytes.Equal(pubA, pubB) {
		t.Fatal("finish order changed the bytes")
	}

	opened, err := Open(privA)
	if err != nil {
		t.Fatal(err)
	}
	if opened.DocCount() != 5 {
		t.Fatalf("private docs %d", opened.DocCount())
	}
	var sawShift bool
	for _, site := range opened.sites {
		if site.Exact != site.Shown || site.ExactPoint != site.ShownPoint {
			sawShift = true
		}
	}
	if !sawShift {
		t.Fatal("private shard lost the shifted exact box")
	}
	if !forwardHas(opened, "ds-member") || !forwardHas(opened, "ds-secret") || !forwardHas(opened, "ds-neither") {
		t.Fatal("private shard dropped a non-public row")
	}

	pubShard, err := Open(pubA)
	if err != nil {
		t.Fatal(err)
	}
	if pubShard.DocCount() != 2 || len(pubShard.sites) != 2 {
		t.Fatalf("public docs %d sites %d", pubShard.DocCount(), len(pubShard.sites))
	}
	for _, site := range pubShard.sites {
		if site.ID == "member" {
			t.Fatal("member-only site leaked into the public shard")
		}
		if site.Exact != site.Shown || site.ExactPoint != site.ShownPoint {
			t.Fatalf("public site %s kept a distinct exact geometry", site.ID)
		}
	}
	for _, id := range []string{"ds-member", "ds-secret", "ds-neither"} {
		if forwardHas(pubShard, id) {
			t.Fatalf("public forward contains %s", id)
		}
	}
	world := Box{West: -180, South: -90, East: 180, North: 90}
	hits := query(t, pubShard, Query{
		Geometry:          GeometryExact,
		Zoom:              11,
		IncludeMemberOnly: true,
		PageSize:          10,
		Viewport:          &world,
	})
	shown := query(t, pubShard, Query{
		Geometry: GeometryShown,
		Zoom:     11,
		PageSize: 10,
		Viewport: &world,
	})
	if len(hits.Hits) != 2 || len(shown.Sites) != len(hits.Sites) {
		t.Fatalf("public hits %d sites %d/%d", len(hits.Hits), len(hits.Sites), len(shown.Sites))
	}
	for _, hit := range hits.Hits {
		if hit.DatastreamID == "ds-member" || hit.DatastreamID == "ds-secret" {
			t.Fatalf("member row %s", hit.DatastreamID)
		}
	}
	byID := map[string]SiteMarker{}
	for _, site := range shown.Sites {
		byID[site.SiteID] = site
	}
	for _, site := range hits.Sites {
		other, ok := byID[site.SiteID]
		if !ok || site.Point != other.Point || site.Box != other.Box {
			t.Fatalf("exact marker %+v shown %+v", site, other)
		}
	}

	memberOnly := NewBuilder()
	if err := memberOnly.AddSite(shifted("member", -122, 40, 0.3, 0.3)); err != nil {
		t.Fatal(err)
	}
	if err := memberOnly.AddStation(Station{ID: "station-member", OrganizationID: "org", Intervals: []Interval{{SiteID: "member", BeginsAt: 1, EndsBefore: 0}}}); err != nil {
		t.Fatal(err)
	}
	if err := memberOnly.AddDatastream(Datastream{ID: "ds-secret", OrganizationID: "org", StationID: "station-member", PublicOK: false, MemberOK: true}); err != nil {
		t.Fatal(err)
	}
	empty, n, err := memberOnly.Finish(FinishPublic)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("empty public docs %d", n)
	}
	emptyShard, err := Open(empty)
	if err != nil {
		t.Fatal(err)
	}
	if emptyShard.DocCount() != 0 || len(emptyShard.sites) != 0 {
		t.Fatalf("empty public docs %d sites %d", emptyShard.DocCount(), len(emptyShard.sites))
	}
}

func fillModes(t *testing.T, b *Builder) {
	t.Helper()
	if err := b.AddSites([]Site{
		shifted("public", -120, 38, 0.5, 0.5),
		shifted("shared", -121, 39, 0.4, 0.4),
		shifted("member", -122, 40, 0.3, 0.3),
	}); err != nil {
		t.Fatal(err)
	}
	open := func(id string) []Interval {
		return []Interval{{SiteID: id, BeginsAt: 1, EndsBefore: 0}}
	}
	if err := b.AddStations([]Station{
		{ID: "station-public", Name: "Public logger", OrganizationID: "org", Intervals: open("public")},
		{ID: "station-shared", Name: "Shared logger", OrganizationID: "org", Intervals: open("shared")},
		{ID: "station-member", Name: "Member logger", OrganizationID: "org", Intervals: open("member")},
	}); err != nil {
		t.Fatal(err)
	}
	if err := b.AddDatastreams([]Datastream{
		{ID: "ds-public", OrganizationID: "org", StationID: "station-public", PublicOK: true, Measurement: "Air", Purpose: PurposeReadyToUse},
		{ID: "ds-shared", OrganizationID: "org", StationID: "station-shared", PublicOK: true, MemberOK: true, Measurement: "Soil", Purpose: PurposeReadyToUse},
		{ID: "ds-member", OrganizationID: "org", StationID: "station-shared", PublicOK: false, MemberOK: true, Measurement: "Wind", Purpose: PurposeReadyToUse},
		{ID: "ds-secret", OrganizationID: "org", StationID: "station-member", PublicOK: false, MemberOK: true, Measurement: "Heat", Purpose: PurposeReadyToUse},
		{ID: "ds-neither", OrganizationID: "org", StationID: "station-shared", PublicOK: false, Measurement: "Note", Purpose: PurposeRaw},
	}); err != nil {
		t.Fatal(err)
	}
}

func shifted(id string, lon, lat, dlon, dlat float64) Site {
	s := place(id, lon, lat)
	s.Exact = Box{West: lon + dlon, South: lat + dlat, East: lon + dlon, North: lat + dlat}
	s.ExactPoint = Point{Lon: lon + dlon, Lat: lat + dlat}
	return s
}

func forwardHas(shard *Shard, id string) bool {
	for _, row := range shard.forward {
		if row.datastreamID == id {
			return true
		}
	}
	return false
}
