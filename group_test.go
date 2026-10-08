package search

import (
	"encoding/binary"
	"errors"
	"testing"
)

func TestFormatVersion(t *testing.T) {
	blob := buildBytes(t, sampleRows())
	if _, err := Open(blob); err != nil {
		t.Fatal(err)
	}
	raw := append([]byte(nil), blob...)
	binary.LittleEndian.PutUint16(raw[4:], 1)
	if _, err := Open(raw); !errors.Is(err, ErrFormat) {
		t.Fatalf("version 1: %v", err)
	}
}

func TestGroupIDsAndPublicShard(t *testing.T) {
	r := rows{
		sites: []Site{place("meadow", -120, 38), place("ridge", -121, 39)},
		stations: []Station{
			{ID: "station-field", OrganizationID: "org", Intervals: []Interval{{SiteID: "meadow", BeginsAt: 1, EndsBefore: 0, GroupID: "field"}}},
			{ID: "station-moved", OrganizationID: "org", Intervals: []Interval{
				{SiteID: "meadow", BeginsAt: 1000, EndsBefore: 2000, GroupID: "lab"},
				{SiteID: "ridge", BeginsAt: 3000, EndsBefore: 0, GroupID: "field"},
			}},
			{ID: "station-internal", OrganizationID: "org", Intervals: []Interval{{SiteID: "meadow", BeginsAt: 1, EndsBefore: 0, GroupID: "internal"}}},
			{ID: "station-bare", OrganizationID: "org", Intervals: []Interval{{SiteID: "ridge", BeginsAt: 1, EndsBefore: 0}}},
		},
		datastreams: []Datastream{
			{ID: "ds-field", OrganizationID: "org", StationID: "station-field", PublicOK: true, MemberOK: true},
			{ID: "ds-moved", OrganizationID: "org", StationID: "station-moved", PublicOK: true, MemberOK: true},
			{ID: "ds-internal", OrganizationID: "org", StationID: "station-internal", PublicOK: false, MemberOK: true},
			{ID: "ds-bare", OrganizationID: "org", StationID: "station-bare", PublicOK: true, MemberOK: true},
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
	priv, _, err := b.Finish(FinishPrivate)
	if err != nil {
		t.Fatal(err)
	}
	shard, err := Open(priv)
	if err != nil {
		t.Fatal(err)
	}
	guest, err := shard.GroupIDs(0, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if joinIDs(guest) != "field" {
		t.Fatalf("guest open %v", guest)
	}
	member, err := shard.GroupIDs(0, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if joinIDs(member) != "field,internal" {
		t.Fatalf("member open %v", member)
	}
	window, err := shard.GroupIDs(1000, 2000, false)
	if err != nil {
		t.Fatal(err)
	}
	if joinIDs(window) != "field,lab" {
		t.Fatalf("window %v", window)
	}
	if _, err := shard.GroupIDs(10, 0, false); !errors.Is(err, ErrWindow) {
		t.Fatalf("half window: %v", err)
	}

	pub, _, err := b.Finish(FinishPublic)
	if err != nil {
		t.Fatal(err)
	}
	public, err := Open(pub)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := public.GroupIDs(0, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if joinIDs(ids) != "field" {
		t.Fatalf("public %v", ids)
	}
}

func TestGroupJoint(t *testing.T) {
	r := rows{
		sites: []Site{place("meadow", -120, 38), place("ridge", -121, 39)},
		stations: []Station{{
			ID: "station-moved", OrganizationID: "org",
			Intervals: []Interval{
				{SiteID: "meadow", BeginsAt: 1000, EndsBefore: 2000, GroupID: "field"},
				{SiteID: "ridge", BeginsAt: 3000, EndsBefore: 0, GroupID: "lab"},
			},
		}},
		datastreams: []Datastream{{
			ID: "ds-moved", OrganizationID: "org", StationID: "station-moved",
			Measurement: "Wind", PublicOK: true, MemberOK: true,
		}},
	}
	shard := buildShard(t, r)
	window := Query{
		Geometry: GeometryShown, Zoom: 11, IncludeMemberOnly: true,
		BeginsAt: 1000, EndsBefore: 4000, PageSize: 10,
	}
	pairs := []struct {
		site, group string
		want        bool
	}{
		{"meadow", "field", true},
		{"meadow", "lab", false},
		{"ridge", "lab", true},
		{"ridge", "field", false},
	}
	for _, pair := range pairs {
		q := window
		q.SiteIDs = []string{pair.site}
		q.GroupIDs = []string{pair.group}
		res := query(t, shard, q)
		got := false
		for _, hit := range res.Hits {
			if hit.DatastreamID == "ds-moved" {
				got = true
				if pair.want && hit.SiteID != pair.site {
					t.Fatalf("%s+%s labeled %s", pair.site, pair.group, hit.SiteID)
				}
			}
		}
		if got != pair.want {
			t.Fatalf("%s+%s match %v", pair.site, pair.group, got)
		}
	}

	counts := query(t, shard, Query{
		Geometry: GeometryShown, Zoom: 11, IncludeMemberOnly: true,
		BeginsAt: 1000, EndsBefore: 4000, GroupIDs: []string{"field"},
	})
	if countOf(counts.SiteCounts, "meadow") != 1 || countOf(counts.SiteCounts, "ridge") != 0 {
		t.Fatalf("site counts under field %+v", counts.SiteCounts)
	}
	if len(counts.MeasurementCounts) == 0 {
		t.Fatal("missing measurement counts")
	}
}

func joinIDs(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	out := ids[0]
	for _, id := range ids[1:] {
		out += "," + id
	}
	return out
}
