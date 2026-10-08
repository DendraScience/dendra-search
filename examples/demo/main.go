// Command demo builds a handful of sites and prints one cluster query
// and one site query, then the north and south group picker.
//
//	go run ./examples/demo
package main

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"github.com/dendrascience/dendra-search"
)

func main() {
	b := search.NewBuilder()
	meadow := place("meadow", "Meadow", -120.10, 38.20)
	meadow.Exact = search.Box{West: -119.50, South: 37.50, East: -119.50, North: 37.50}
	meadow.ExactPoint = search.Point{Lon: -119.50, Lat: 37.50}
	must(b.AddSite(meadow))
	must(b.AddSite(place("ridge", "Ridge", -120.12, 38.22)))
	must(b.AddSite(place("creek", "Creek", -120.08, 38.18)))
	must(b.AddSite(place("outlier", "Outlier", -105.0, 40.0)))

	open := func(siteID string) []search.Interval {
		return []search.Interval{{SiteID: siteID, BeginsAt: 1, EndsBefore: 0}}
	}
	must(b.AddStation(search.Station{ID: "station-meadow", Name: "Meadow logger", OrganizationID: "org-demo", Intervals: open("meadow")}))
	must(b.AddStation(search.Station{ID: "station-ridge", Name: "Ridge logger", OrganizationID: "org-demo", Intervals: open("ridge")}))
	must(b.AddStation(search.Station{ID: "station-creek", Name: "Creek logger", OrganizationID: "org-demo", Intervals: open("creek")}))
	must(b.AddStation(search.Station{ID: "station-outlier", Name: "Outlier logger", OrganizationID: "org-demo", Intervals: open("outlier")}))

	must(b.AddDatastream(row("datastream-air", "station-meadow", "Air", true)))
	must(b.AddDatastream(row("datastream-soil", "station-meadow", "Soil", true)))
	must(b.AddDatastream(row("datastream-wind", "station-ridge", "Wind", true)))
	must(b.AddDatastream(row("datastream-water", "station-creek", "Water", true)))
	must(b.AddDatastream(row("datastream-far", "station-outlier", "Air", true)))
	must(b.AddDatastream(row("datastream-member", "station-meadow", "Air", false)))

	blob, n, err := b.Finish(search.FinishPrivate)
	must(err)
	shard, err := search.Open(blob)
	must(err)

	world := search.Box{West: -180, South: -90, East: 180, North: 90}
	fmt.Printf("dendra-search demo\n%d datastreams, %d bytes\n\n", n, len(blob))

	clusters, err := shard.Query(search.Query{
		Geometry: search.GeometryShown,
		Viewport: &world,
		Zoom:     2,
	})
	must(err)
	fmt.Printf("zoom 2  %s  sites %d\n", clusters.Kind, clusters.SiteTotal)
	for _, c := range clusters.Clusters {
		fmt.Printf("  cluster %d sites  centroid %.2f, %.2f\n", c.SiteCount, c.Centroid.Lon, c.Centroid.Lat)
	}
	for _, c := range clusters.MeasurementCounts {
		fmt.Printf("  %s  %d\n", c.Value, c.Count)
	}

	air, err := shard.Query(search.Query{
		Geometry:     search.GeometryShown,
		Viewport:     &world,
		Zoom:         2,
		Measurements: []string{"Air"},
	})
	must(err)
	fmt.Printf("\nzoom 2, measurement Air  sites %d\n", air.SiteTotal)
	for _, c := range air.Clusters {
		fmt.Printf("  cluster %d sites  centroid %.2f, %.2f\n", c.SiteCount, c.Centroid.Lon, c.Centroid.Lat)
	}

	sites, err := shard.Query(search.Query{
		Geometry: search.GeometryShown,
		Viewport: &world,
		Zoom:     11,
		PageSize: 10,
	})
	must(err)
	fmt.Printf("\nzoom 11  %s  datastreams %d (guest)\n", sites.Kind, sites.TotalCount)
	for _, site := range sites.Sites {
		fmt.Printf("  %s  %d datastreams  %.2f, %.2f\n", site.Name, site.DatastreamCount, site.Point.Lon, site.Point.Lat)
	}
	for _, hit := range sites.Hits {
		fmt.Printf("  %s  %s\n", hit.Name, hit.StationName)
	}

	member, err := shard.Query(search.Query{
		Geometry:          search.GeometryShown,
		Zoom:              11,
		IncludeMemberOnly: true,
		PageSize:          10,
	})
	must(err)
	fmt.Printf("\nmember datastreams %d, guest datastreams %d\n", member.TotalCount, sites.TotalCount)
	for _, hit := range member.Hits {
		if hit.DatastreamID == "datastream-member" {
			fmt.Printf("  private hit %s\n", hit.DatastreamID)
		}
	}
	exact, err := shard.Query(search.Query{
		Geometry: search.GeometryExact,
		Viewport: &world,
		Zoom:     11,
	})
	must(err)
	for _, site := range exact.Sites {
		if site.SiteID == "meadow" {
			fmt.Printf("  private exact meadow %.2f, %.2f\n", site.Point.Lon, site.Point.Lat)
		}
	}

	publicBlob, publicN, err := b.Finish(search.FinishPublic)
	must(err)
	publicShard, err := search.Open(publicBlob)
	must(err)
	publicExact, err := publicShard.Query(search.Query{
		Geometry:          search.GeometryExact,
		Viewport:          &world,
		Zoom:              11,
		IncludeMemberOnly: true,
		PageSize:          10,
	})
	must(err)
	fmt.Printf("\npublic datastreams %d\n", publicN)
	var sawMember bool
	for _, hit := range publicExact.Hits {
		if hit.DatastreamID == "datastream-member" {
			sawMember = true
		}
	}
	if sawMember {
		must(fmt.Errorf("public shard contains datastream-member"))
	}
	var publicMeadow bool
	for _, site := range publicExact.Sites {
		if site.SiteID != "meadow" {
			continue
		}
		publicMeadow = true
		if site.Point != meadow.ShownPoint {
			must(fmt.Errorf("public exact meadow %.4f, %.4f", site.Point.Lon, site.Point.Lat))
		}
		fmt.Printf("  public exact meadow %.2f, %.2f\n", site.Point.Lon, site.Point.Lat)
	}
	if !publicMeadow {
		must(fmt.Errorf("public shard missing meadow"))
	}

	frame, err := search.WriteContainer([]search.ContainerChunk{{
		OrganizationID: "org-demo",
		ETag:           "etag-demo",
		Bytes:          publicBlob,
	}})
	must(err)
	chunks, err := search.ParseContainer(frame)
	must(err)
	if len(chunks) != 1 || chunks[0].ETag != "etag-demo" || !bytes.Equal(chunks[0].Bytes, publicBlob) {
		must(fmt.Errorf("container round trip"))
	}
	fmt.Printf("container chunks %d  etag %s\n", len(chunks), chunks[0].ETag)

	pointer := search.VersionFile{
		ObjectKey: "containers/demo.bin",
		ETag:      "etag-public",
		Size:      int64(len(publicBlob)),
		BuiltAt:   time.Date(2026, 10, 1, 15, 4, 5, 0, time.UTC),
		Encoding:  "gzip",
		Keep: []search.KeepEntry{{
			ObjectKey: "containers/previous.bin",
			Until:     time.Date(2026, 10, 2, 15, 4, 5, 0, time.UTC),
		}},
	}
	raw, err := pointer.Marshal()
	must(err)
	parsed, err := search.ParseVersionFile(raw)
	must(err)
	fmt.Printf("version file encoding %s  keep %s\n", parsed.Encoding, parsed.Keep[0].ObjectKey)

	groupsDemo()
}

// groupsDemo builds the north and south shards and prints GroupIDs, the
// picker rows, and the two query checks from docs/groups.md.
func groupsDemo() {
	creek := groupStream("north", "ds-creek", "", "Water", true)
	creek.Intervals = []search.Interval{{SiteID: "creek", BeginsAt: 1, EndsBefore: 0, GroupID: "volunteers"}}
	internal := groupStream("north", "ds-internal", "", "Air", false)
	internal.Intervals = []search.Interval{{SiteID: "meadow", BeginsAt: 1, EndsBefore: 0, GroupID: "internal"}}
	northShard := openOrg(groupOrg{
		sites: []search.Site{
			groupPlace("north", "North Reserve", "meadow", "Meadow", -120.10, 38.20),
			groupPlace("north", "North Reserve", "ridge", "Ridge", -120.12, 38.22),
			groupPlace("north", "North Reserve", "creek", "Creek", -120.08, 38.18),
		},
		stations: []search.Station{
			{ID: "station-meadow", Name: "Meadow logger", OrganizationID: "north", Intervals: []search.Interval{{SiteID: "meadow", BeginsAt: 1, EndsBefore: 0, GroupID: "field"}}},
			{ID: "station-ridge", Name: "Ridge logger", OrganizationID: "north", Intervals: []search.Interval{
				{SiteID: "ridge", BeginsAt: 1000, EndsBefore: 2000, GroupID: "field"},
				{SiteID: "ridge", BeginsAt: 3000, EndsBefore: 0, GroupID: "lab"},
			}},
			{ID: "station-moved", Name: "Moved logger", OrganizationID: "north", Intervals: []search.Interval{
				{SiteID: "meadow", BeginsAt: 1000, EndsBefore: 2000, GroupID: "field"},
				{SiteID: "ridge", BeginsAt: 3000, EndsBefore: 0, GroupID: "lab"},
			}},
		},
		streams: []search.Datastream{
			groupStream("north", "ds-meadow", "station-meadow", "Air", true),
			groupStream("north", "ds-ridge", "station-ridge", "Air", true),
			groupStream("north", "ds-moved", "station-moved", "Wind", true),
			creek,
			internal,
		},
	})
	southShard := openOrg(groupOrg{
		sites: []search.Site{
			groupPlace("south", "South Reserve", "south-meadow", "South meadow", -119.40, 37.80),
			groupPlace("south", "South Reserve", "south-bare", "South bare", -119.42, 37.82),
		},
		stations: []search.Station{
			{ID: "station-south-meadow", Name: "South meadow logger", OrganizationID: "south", Intervals: []search.Interval{{SiteID: "south-meadow", BeginsAt: 1, EndsBefore: 0, GroupID: "lab"}}},
			{ID: "station-south-bare", Name: "South bare logger", OrganizationID: "south", Intervals: []search.Interval{{SiteID: "south-bare", BeginsAt: 1, EndsBefore: 0}}},
		},
		streams: []search.Datastream{
			groupStream("south", "ds-south-lab", "station-south-meadow", "Air", true),
			groupStream("south", "ds-south-bare", "station-south-bare", "Soil", true),
		},
	})

	fmt.Printf("\ngroups\n")
	guestNorth := groupIDs(northShard, false)
	memberNorth := groupIDs(northShard, true)
	guestSouth := groupIDs(southShard, false)
	fmt.Printf("guest north  %s\n", joinComma(guestNorth))
	fmt.Printf("member north  %s\n", joinComma(memberNorth))
	fmt.Printf("guest south  %s\n", joinComma(guestSouth))
	if joinComma(guestNorth) != "field, lab, volunteers" {
		must(fmt.Errorf("guest north GroupIDs %s", joinComma(guestNorth)))
	}
	if joinComma(memberNorth) != "field, internal, lab, volunteers" {
		must(fmt.Errorf("member north GroupIDs %s", joinComma(memberNorth)))
	}
	if joinComma(guestSouth) != "lab" {
		must(fmt.Errorf("guest south GroupIDs %s", joinComma(guestSouth)))
	}

	orgs := []search.CatalogName{{ID: "north", Name: "North Reserve"}, {ID: "south", Name: "South Reserve"}}
	groups := []search.CatalogName{
		{ID: "field", Name: "Field crew"},
		{ID: "lab", Name: "Lab"},
		{ID: "volunteers", Name: "Volunteers"},
		{ID: "internal", Name: "Internal"},
	}
	byOrg := map[string][]string{"north": guestNorth, "south": guestSouth}
	printPicker("none", orgs, groups, nil, nil, byOrg, "north,south", "field,lab,volunteers,internal")
	printPicker("north and south", orgs, groups, []string{"north", "south"}, nil, byOrg, "north,south", "field,lab,volunteers")
	printPicker("volunteers", orgs, groups, nil, []string{"volunteers"}, byOrg, "north", "field,lab,volunteers,internal")
	printPicker("north, south, and volunteers", orgs, groups, []string{"north", "south"}, []string{"volunteers"}, byOrg, "north,south~", "field,lab,volunteers")
	// Unchecking south leaves north and volunteers checked, so the right side
	// is the groups north references. Internal is not among them.
	printPicker("volunteers, south unchecked", orgs, groups, []string{"north"}, []string{"volunteers"}, byOrg, "north", "field,lab,volunteers")
	printPicker("internal and north", orgs, groups, []string{"north"}, []string{"internal"}, byOrg, "north~", "field,lab,volunteers,internal~")

	volunteers := search.Query{Geometry: search.GeometryShown, Zoom: 11, GroupIDs: []string{"volunteers"}, PageSize: 20}
	northVol := mustQuery(northShard, volunteers)
	southVol := mustQuery(southShard, volunteers)
	fmt.Printf("\nvolunteers  north %d  south %d\n", northVol.TotalCount, southVol.TotalCount)
	for _, c := range northVol.MeasurementCounts {
		fmt.Printf("  %s  %d\n", c.Value, c.Count)
	}
	if !hitID(northVol, "ds-creek") || hitID(southVol, "ds-south-bare") {
		must(fmt.Errorf("volunteers query"))
	}

	window := search.Query{Geometry: search.GeometryShown, Zoom: 11, BeginsAt: 1000, EndsBefore: 4000, PageSize: 20}
	pairs := []struct {
		site, group string
		want        bool
	}{
		{"meadow", "field", true},
		{"meadow", "lab", false},
		{"ridge", "lab", true},
		{"ridge", "field", false},
	}
	fmt.Printf("\nmoved datastream  window [1000, 4000)\n")
	for _, pair := range pairs {
		q := window
		q.SiteIDs = []string{pair.site}
		q.GroupIDs = []string{pair.group}
		res := mustQuery(northShard, q)
		got := hitID(res, "ds-moved")
		fmt.Printf("  %s + %s  moved %v\n", pair.site, pair.group, got)
		if pair.site == "meadow" && pair.group == "lab" && got {
			must(fmt.Errorf("meadow and lab match together"))
		}
		if got != pair.want {
			must(fmt.Errorf("%s + %s moved %v", pair.site, pair.group, got))
		}
	}
}

type groupOrg struct {
	sites    []search.Site
	stations []search.Station
	streams  []search.Datastream
}

func openOrg(org groupOrg) *search.Shard {
	b := search.NewBuilder()
	must(b.AddSites(org.sites))
	must(b.AddStations(org.stations))
	must(b.AddDatastreams(org.streams))
	blob, _, err := b.Finish(search.FinishPrivate)
	must(err)
	shard, err := search.Open(blob)
	must(err)
	return shard
}

func place(id, name string, lon, lat float64) search.Site {
	p := search.Point{Lon: lon, Lat: lat}
	box := search.Box{West: lon, South: lat, East: lon, North: lat}
	return search.Site{
		ID: id, Name: name,
		OrganizationID: "org-demo", OrganizationName: "Demo",
		Shown: box, Exact: box, ShownPoint: p, ExactPoint: p,
	}
}

func row(id, stationID, measurement string, public bool) search.Datastream {
	return search.Datastream{
		ID: id, Name: id, StationID: stationID,
		OrganizationID: "org-demo",
		Measurement:    measurement,
		Purpose:        search.PurposeReadyToUse,
		PublicOK:       public,
		MemberOK:       true,
		UnitName:       "unit",
	}
}

func groupPlace(orgID, orgName, id, name string, lon, lat float64) search.Site {
	p := search.Point{Lon: lon, Lat: lat}
	box := search.Box{West: lon, South: lat, East: lon, North: lat}
	return search.Site{
		ID: id, Name: name,
		OrganizationID: orgID, OrganizationName: orgName,
		Shown: box, Exact: box, ShownPoint: p, ExactPoint: p,
	}
}

func groupStream(org, id, station, measurement string, public bool) search.Datastream {
	return search.Datastream{
		ID: id, Name: id, StationID: station,
		OrganizationID: org,
		Measurement:    measurement,
		Purpose:        search.PurposeReadyToUse,
		PublicOK:       public,
		MemberOK:       true,
		UnitName:       "unit",
	}
}

func groupIDs(shard *search.Shard, member bool) []string {
	ids, err := shard.GroupIDs(0, 0, member)
	must(err)
	return ids
}

func mustQuery(shard *search.Shard, q search.Query) search.Result {
	res, err := shard.Query(q)
	must(err)
	return res
}

func hitID(res search.Result, id string) bool {
	for _, hit := range res.Hits {
		if hit.DatastreamID == id {
			return true
		}
	}
	return false
}

func joinComma(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	out := ids[0]
	for _, id := range ids[1:] {
		out += ", " + id
	}
	return out
}

func printPicker(label string, orgs, groups []search.CatalogName, checkedOrgs, checkedGroups []string, byOrg map[string][]string, wantOrgs, wantGroups string) {
	orgRows, groupRows := search.ReducePicker(orgs, groups, checkedOrgs, checkedGroups, byOrg)
	left := formatPicker(orgRows)
	right := formatPicker(groupRows)
	fmt.Printf("picker %s\n  orgs  %s\n  groups  %s\n", label, left, right)
	if left != wantOrgs || right != wantGroups {
		must(fmt.Errorf("picker %s: orgs %s groups %s", label, left, right))
	}
}

func formatPicker(rows []search.PickerRow) string {
	if len(rows) == 0 {
		return ""
	}
	out := rows[0].ID
	if rows[0].Grey {
		out += "~"
	}
	for _, row := range rows[1:] {
		out += "," + row.ID
		if row.Grey {
			out += "~"
		}
	}
	return out
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
