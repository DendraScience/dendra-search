//go:build js && wasm

package main

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/dendrascience/dendra-search"
)

const demoEpoch int64 = 1_700_000_000_000

var demoOutliers = []search.Point{
	{-70.9, 43.7},
	{-71.1, 42.4},
	{-73.8, 42.7},
	{-114.0, 43.6},
	{-111.9, 40.8},
	{-123.1, 49.2},
	{-122.3, 47.6},
	{-110.0, 35.0},
	{-105.0, 40.0},
}

var placeNames = []string{
	"Laguna Meadow", "North Ridge", "Creek Terrace", "Oak Flat", "Sage Basin",
	"Pine Bench", "Alkali Marsh", "Dune Slack", "Spring Box", "Canyon Mouth",
	"Vernal Pool", "Chaparral", "Fen", "Bluff", "Alluvial Fan",
}

var measLabels = []string{
	"Air temperature", "Relative humidity", "Wind speed", "Wind direction",
	"Precipitation", "Soil moisture", "Soil temperature", "Solar radiation",
	"Barometric pressure", "Water temperature", "Water level", "Snow depth",
}

type areaJSON struct {
	ID    string      `json:"id"`
	Shown [][]float64 `json:"shown"`
	Exact [][]float64 `json:"exact"`
}

type placedSite struct {
	site  search.Site
	index int
}

// demoCatalog builds one shard per organization at spec's size.
// Sites are points in the California box, plus the nine outlier stand-ins.
// About one in-box site in five is a reserve outline. The shard stores that
// outline's box. The ring is returned beside the bytes so the page can draw it.
// Datastreams share a logger, about five sensors each.
func demoCatalog(spec search.Spec) (search.Fixture, []areaJSON, error) {
	if spec.Orgs < 1 || spec.Sites < 1 || spec.Datastreams < 1 {
		return search.Fixture{}, nil, fmt.Errorf("search: fixture %s: %w", spec.Name, search.ErrInvalid)
	}
	if spec.Datastreams < spec.Sites {
		return search.Fixture{}, nil, fmt.Errorf("search: fixture %s needs a datastream for every site", spec.Name)
	}
	nOut := int(math.Round(float64(spec.Sites) * 9 / 286))
	if nOut >= spec.Sites {
		nOut = spec.Sites - 1
	}
	if nOut < 0 {
		nOut = 0
	}
	rng := rand.New(rand.NewSource(search.FixtureSeed))
	placed := make([]placedSite, spec.Sites)
	var areas []areaJSON
	for i := 0; i < spec.Sites; i++ {
		var p search.Point
		if i < nOut {
			p = demoOutliers[i%len(demoOutliers)]
		} else {
			p = search.Point{
				Lon: -124.2 + rng.Float64()*(-114.1 - -124.2),
				Lat: 32.5 + rng.Float64()*(42.0-32.5),
			}
		}
		site := search.Site{
			ID:         fmt.Sprintf("site-%04d", i),
			Name:       placeName(i),
			Exact:      search.Box{West: p.Lon, South: p.Lat, East: p.Lon, North: p.Lat},
			ExactPoint: p,
		}
		var area *areaJSON
		if i >= nOut && i%5 == 0 {
			exact, box := reserveRing(p, rng)
			site.Exact = box
			site.ExactPoint = ringPoint(exact)
			shown := shiftRing(exact, 0, 0)
			area = &areaJSON{ID: site.ID, Exact: exact, Shown: shown}
			areas = append(areas, *area)
		}
		site.Shown = site.Exact
		site.ShownPoint = site.ExactPoint
		if i == nOut {
			site.Shown, site.ShownPoint = shiftGeo(site.Exact, site.ExactPoint, 1)
		} else if i%10 == 0 && area == nil {
			site.Shown, site.ShownPoint = shiftGeo(site.Exact, site.ExactPoint, 0.1)
		}
		if area != nil && i == nOut {
			area.Shown = shiftRing(area.Exact, 1, 1)
			areas[len(areas)-1] = *area
		}
		placed[i] = placedSite{site: site, index: i}
	}

	perSite := spread(spec.Datastreams, spec.Sites)
	sitePortions := spread(spec.Sites, spec.Orgs)
	fix := search.Fixture{
		Name:        spec.Name,
		Seed:        search.FixtureSeed,
		Orgs:        spec.Orgs,
		Sites:       spec.Sites,
		Datastreams: spec.Datastreams,
		Blobs:       make([][]byte, spec.Orgs),
		DocCounts:   make([]uint32, spec.Orgs),
	}
	siteCursor := 0
	dsCursor := 0
	for org := 0; org < spec.Orgs; org++ {
		nSites := sitePortions[org]
		orgID := fmt.Sprintf("org-%03d", org)
		orgName := fmt.Sprintf("Org %03d", org)
		orgSites := make([]search.Site, nSites)
		var stations []search.Station
		var streams []search.Datastream
		for s := 0; s < nSites; s++ {
			ps := placed[siteCursor+s]
			ps.site.OrganizationID = orgID
			ps.site.OrganizationName = orgName
			orgSites[s] = ps.site
			chunks := loggerSizes(perSite[siteCursor+s])
			local := 0
			for ord, n := range chunks {
				stationID := fmt.Sprintf("logger-%04d-%d", ps.index, ord+1)
				stations = append(stations, search.Station{
					ID:             stationID,
					Name:           fmt.Sprintf("Logger %d", ord+1),
					OrganizationID: orgID,
					Intervals: []search.Interval{{
						SiteID:     ps.site.ID,
						BeginsAt:   demoEpoch,
						EndsBefore: 0,
					}},
				})
				for k := 0; k < n; k++ {
					meas := search.Measurement(local)
					streams = append(streams, search.Datastream{
						ID:             fmt.Sprintf("datastream-%06d", dsCursor),
						Name:           measLabel(meas),
						OrganizationID: orgID,
						StationID:      stationID,
						UnitName:       "unit",
						Measurement:    meas,
						Purpose:        demoPurpose(dsCursor),
						PublicOK:       dsCursor%10 != 0,
						MemberOK:       true,
					})
					local++
					dsCursor++
				}
			}
		}
		b := search.NewBuilder()
		if err := b.AddSites(orgSites); err != nil {
			return search.Fixture{}, nil, err
		}
		if err := b.AddStations(stations); err != nil {
			return search.Fixture{}, nil, err
		}
		if err := b.AddDatastreams(streams); err != nil {
			return search.Fixture{}, nil, err
		}
		started := time.Now()
		blob, n, err := b.Finish()
		fix.Finish += time.Since(started)
		if err != nil {
			return search.Fixture{}, nil, err
		}
		fix.Blobs[org] = blob
		fix.DocCounts[org] = n
		siteCursor += nSites
	}
	return fix, areas, nil
}

func placeName(i int) string {
	return fmt.Sprintf("%s %d", placeNames[i%len(placeNames)], i/len(placeNames)+1)
}

func measLabel(meas string) string {
	var n int
	if _, err := fmt.Sscanf(meas, "Meas%02d", &n); err != nil || n < 1 || n > len(measLabels) {
		return meas
	}
	return measLabels[n-1]
}

func demoPurpose(i int) string {
	switch i % 10 {
	case 8:
		return search.PurposeRaw
	case 9:
		return search.PurposeStatusInformation
	default:
		return search.PurposeReadyToUse
	}
}

// loggerSizes splits one site's datastreams into loggers of about five sensors.
func loggerSizes(n int) []int {
	if n < 1 {
		return nil
	}
	const size = 5
	nStat := n / size
	if nStat < 1 {
		nStat = 1
	}
	out := spread(n, nStat)
	return out
}

func spread(total, parts int) []int {
	out := make([]int, parts)
	if parts == 0 {
		return out
	}
	base := total / parts
	rem := total % parts
	for i := range out {
		out[i] = base
		if i < rem {
			out[i]++
		}
	}
	return out
}

// reserveRing returns a closed outline around p and the box the index stores.
// The ring is what the page draws. Query matches the box.
func reserveRing(p search.Point, rng *rand.Rand) ([][]float64, search.Box) {
	n := 5 + rng.Intn(3)
	r0 := 0.04 + rng.Float64()*0.08
	type vert struct {
		ang      float64
		lon, lat float64
	}
	verts := make([]vert, n)
	cos := math.Cos(p.Lat * math.Pi / 180)
	if cos < 0.2 {
		cos = 0.2
	}
	for k := 0; k < n; k++ {
		ang := (float64(k) + rng.Float64()*0.35) / float64(n) * 2 * math.Pi
		r := r0 * (0.62 + rng.Float64()*0.5)
		verts[k] = vert{
			ang: ang,
			lon: p.Lon + r*math.Cos(ang)/cos,
			lat: p.Lat + r*math.Sin(ang),
		}
	}
	ring := make([][]float64, n+1)
	box := search.Box{West: verts[0].lon, East: verts[0].lon, South: verts[0].lat, North: verts[0].lat}
	for i, v := range verts {
		ring[i] = []float64{v.lon, v.lat}
		if v.lon < box.West {
			box.West = v.lon
		}
		if v.lon > box.East {
			box.East = v.lon
		}
		if v.lat < box.South {
			box.South = v.lat
		}
		if v.lat > box.North {
			box.North = v.lat
		}
	}
	ring[n] = []float64{ring[0][0], ring[0][1]}
	return ring, box
}

func ringPoint(ring [][]float64) search.Point {
	n := len(ring)
	if n > 1 && ring[0][0] == ring[n-1][0] && ring[0][1] == ring[n-1][1] {
		n--
	}
	var lon, lat float64
	for i := 0; i < n; i++ {
		lon += ring[i][0]
		lat += ring[i][1]
	}
	return search.Point{Lon: lon / float64(n), Lat: lat / float64(n)}
}

func shiftRing(ring [][]float64, dLon, dLat float64) [][]float64 {
	out := make([][]float64, len(ring))
	for i, pt := range ring {
		out[i] = []float64{pt[0] + dLon, pt[1] + dLat}
	}
	return out
}

func shiftGeo(box search.Box, p search.Point, delta float64) (search.Box, search.Point) {
	box.West += delta
	box.East += delta
	box.South += delta
	box.North += delta
	p.Lon += delta
	p.Lat += delta
	return box, p
}
