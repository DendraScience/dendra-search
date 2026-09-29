// Command demo builds a handful of sites and prints one cluster query
// and one site query.
//
//	go run ./examples/demo
package main

import (
	"fmt"
	"os"

	"github.com/dendrascience/dendra-search"
)

func main() {
	b := search.NewBuilder()
	must(b.AddSite(place("meadow", "Meadow", -120.10, 38.20)))
	must(b.AddSite(place("ridge", "Ridge", -120.12, 38.22)))
	must(b.AddSite(place("creek", "Creek", -120.08, 38.18)))
	must(b.AddSite(place("outlier", "Outlier", -105.0, 40.0)))

	open := func(siteID string) []search.Interval {
		return []search.Interval{{SiteID: siteID, BeginsAt: 1, EndsBefore: 0}}
	}
	must(b.AddStation(search.Station{ID: "station-meadow", Name: "Meadow logger", Intervals: open("meadow")}))
	must(b.AddStation(search.Station{ID: "station-ridge", Name: "Ridge logger", Intervals: open("ridge")}))
	must(b.AddStation(search.Station{ID: "station-creek", Name: "Creek logger", Intervals: open("creek")}))
	must(b.AddStation(search.Station{ID: "station-outlier", Name: "Outlier logger", Intervals: open("outlier")}))

	must(b.AddDatastream(row("datastream-air", "station-meadow", "Air", true)))
	must(b.AddDatastream(row("datastream-soil", "station-meadow", "Soil", true)))
	must(b.AddDatastream(row("datastream-wind", "station-ridge", "Wind", true)))
	must(b.AddDatastream(row("datastream-water", "station-creek", "Water", true)))
	must(b.AddDatastream(row("datastream-far", "station-outlier", "Air", true)))
	must(b.AddDatastream(row("datastream-member", "station-meadow", "Air", false)))

	blob, n, err := b.Finish()
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
	})
	must(err)
	fmt.Printf("\nmember datastreams %d, guest datastreams %d\n", member.TotalCount, sites.TotalCount)
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

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
