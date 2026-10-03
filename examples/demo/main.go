// Command demo builds a handful of sites and prints one cluster query
// and one site query.
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
