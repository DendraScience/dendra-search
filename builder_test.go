package search

import (
	"bytes"
	"errors"
	"testing"
)

func TestAddOrder(t *testing.T) {
	b := NewBuilder()
	if err := b.AddStation(Station{ID: "station-1", Intervals: []Interval{{SiteID: "a", BeginsAt: 1, EndsBefore: 0}}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("station before its site: %v", err)
	}
	site := place("a", -120, 38)
	if err := b.AddSite(site); err != nil {
		t.Fatal(err)
	}
	if err := b.AddSite(site); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate: %v", err)
	}
	station := Station{ID: "station-1", Intervals: []Interval{{SiteID: "a", BeginsAt: 1, EndsBefore: 0}}}
	if err := b.AddStation(station); err != nil {
		t.Fatal(err)
	}
	if err := b.AddSite(place("b", -121, 38)); !errors.Is(err, ErrOrder) {
		t.Fatalf("site after station: %v", err)
	}
	if err := b.AddDatastream(Datastream{
		ID: "datastream-1", StationID: "station-1", PublicOK: true, MemberOK: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := b.AddStation(Station{ID: "station-2", Intervals: []Interval{{SiteID: "a", BeginsAt: 1, EndsBefore: 0}}}); !errors.Is(err, ErrOrder) {
		t.Fatalf("station after datastream: %v", err)
	}
	blob, n, err := b.Finish()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || len(blob) == 0 {
		t.Fatalf("n=%d len=%d", n, len(blob))
	}
	if _, _, err := b.Finish(); !errors.Is(err, ErrFinished) {
		t.Fatalf("second finish: %v", err)
	}
}

func TestIntervalRules(t *testing.T) {
	sites := []Site{place("a", -120, 38), place("b", -121, 39)}
	b := NewBuilder()
	if err := b.AddSites(sites); err != nil {
		t.Fatal(err)
	}
	err := b.AddStation(Station{ID: "station-1", Intervals: []Interval{
		{SiteID: "a", BeginsAt: 1, EndsBefore: 10},
		{SiteID: "b", BeginsAt: 5, EndsBefore: 0},
	}})
	if !errors.Is(err, ErrInterval) {
		t.Fatalf("overlap: %v", err)
	}
	err = b.AddStation(Station{ID: "station-1", Intervals: []Interval{
		{SiteID: "a", BeginsAt: 1, EndsBefore: 0},
		{SiteID: "b", BeginsAt: 10, EndsBefore: 0},
	}})
	if !errors.Is(err, ErrInterval) {
		t.Fatalf("two open: %v", err)
	}
	if err := b.AddStation(Station{ID: "station-1", Intervals: []Interval{
		{SiteID: "missing", BeginsAt: 1, EndsBefore: 0},
	}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing site: %v", err)
	}
	if err := b.AddDatastream(Datastream{ID: "datastream-1", StationID: "nope", PublicOK: true}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing station: %v", err)
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
			ID: "station-1",
			Intervals: []Interval{
				{SiteID: "a", BeginsAt: 1, EndsBefore: 10},
				{SiteID: "b", BeginsAt: 10, EndsBefore: 0},
			},
		}},
		datastreams: []Datastream{
			{ID: "datastream-1", StationID: "station-1", Measurement: "Meas01", Purpose: PurposeReadyToUse, PublicOK: true, MemberOK: true},
			{ID: "datastream-2", Measurement: "Meas02", Purpose: PurposeReadyToUse, PublicOK: true, MemberOK: true, Intervals: []Interval{{SiteID: "a", BeginsAt: 1, EndsBefore: 0}}},
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
	blob, _, err := b.Finish()
	if err != nil {
		t.Fatal(err)
	}
	return blob
}
