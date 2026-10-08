package search

import (
	"fmt"
	"math"
	"math/rand"
	"time"
)

// FixtureSeed is the generator seed. The same seed always yields the same bytes.
const FixtureSeed int64 = 1

const (
	caWest  = -124.2
	caEast  = -114.1
	caSouth = 32.5
	caNorth = 42.0

	fixtureEpoch int64 = 1_700_000_000_000
)

// outlierPoints are the nine stand-ins, in cycle order. They are not station records.
var outlierPoints = []Point{
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

// Spec is one generated catalog.
type Spec struct {
	Name        string
	Orgs        int
	Sites       int
	Datastreams int
}

// Fixture is a deterministic catalog of shard bytes.
type Fixture struct {
	Name          string
	Seed          int64
	Orgs          int
	Sites         int
	Datastreams   int
	Blobs         [][]byte
	DocCounts     []uint32
	ShiftedSiteID string
	ShiftedExact  Point
	ShiftedShown  Point
	Finish        time.Duration
}

// TodayOrg is about one organization in the public census.
func TodayOrg() (Fixture, error) {
	return Generate(Spec{Name: "today-org", Orgs: 1, Sites: 25, Datastreams: 600})
}

// TodayCatalog is the public catalog, rounded.
func TodayCatalog() (Fixture, error) {
	return Generate(Spec{Name: "today-catalog", Orgs: 15, Sites: 300, Datastreams: 8000})
}

// LargeOrg is ten times TodayOrg.
func LargeOrg() (Fixture, error) {
	return Generate(Spec{Name: "large-org", Orgs: 1, Sites: 250, Datastreams: 6000})
}

// LargeCatalog is ten times TodayCatalog.
func LargeCatalog() (Fixture, error) {
	return Generate(Spec{Name: "large-catalog", Orgs: 150, Sites: 3000, Datastreams: 80000})
}

// Generate builds shard bytes for spec. Coordinates come from FixtureSeed.
func Generate(spec Spec) (Fixture, error) {
	if spec.Orgs < 1 || spec.Sites < 1 || spec.Datastreams < 1 {
		return Fixture{}, fmt.Errorf("search: fixture %s: %w", spec.Name, ErrInvalid)
	}
	nOut := outlierCount(spec.Sites)
	if nOut > spec.Sites {
		nOut = spec.Sites
	}
	if nOut >= spec.Sites {
		return Fixture{}, fmt.Errorf("search: fixture %s has no in-box site: %w", spec.Name, ErrInvalid)
	}
	firstInBox := nOut
	rng := rand.New(rand.NewSource(FixtureSeed))
	sites := make([]Site, spec.Sites)
	for i := 0; i < spec.Sites; i++ {
		var p Point
		if i < nOut {
			p = outlierPoints[i%len(outlierPoints)]
		} else {
			p = Point{
				Lon: caWest + rng.Float64()*(caEast-caWest),
				Lat: caSouth + rng.Float64()*(caNorth-caSouth),
			}
		}
		box := Box{West: p.Lon, South: p.Lat, East: p.Lon, North: p.Lat}
		if i%20 == 0 {
			const half = 0.025
			box.West -= half
			box.East += half
			box.South -= half
			box.North += half
		}
		shownBox, shownPoint := box, p
		switch {
		case i == firstInBox:
			shownBox, shownPoint = shiftGeo(box, p, 1)
		case i%10 == 0:
			shownBox, shownPoint = shiftGeo(box, p, 0.1)
		}
		sites[i] = Site{
			ID:         fmt.Sprintf("site-%04d", i),
			Name:       fmt.Sprintf("Site %04d", i),
			Shown:      shownBox,
			Exact:      box,
			ShownPoint: shownPoint,
			ExactPoint: p,
		}
	}

	sitePortions := portions(spec.Sites, spec.Orgs)
	datastreamPortions := portions(spec.Datastreams, spec.Orgs)
	fix := Fixture{
		Name:        spec.Name,
		Seed:        FixtureSeed,
		Orgs:        spec.Orgs,
		Sites:       spec.Sites,
		Datastreams: spec.Datastreams,
		Blobs:       make([][]byte, spec.Orgs),
		DocCounts:   make([]uint32, spec.Orgs),
	}
	siteCursor := 0
	datastreamCursor := 0
	for org := 0; org < spec.Orgs; org++ {
		nSites := sitePortions[org]
		nDatastreams := datastreamPortions[org]
		if nDatastreams > 0 && nSites == 0 {
			return Fixture{}, fmt.Errorf("search: fixture %s org %d: %w", spec.Name, org, ErrInvalid)
		}
		orgID := fmt.Sprintf("org-%03d", org)
		orgName := fmt.Sprintf("Org %03d", org)
		orgSites := make([]Site, nSites)
		for i := 0; i < nSites; i++ {
			site := sites[siteCursor+i]
			site.OrganizationID = orgID
			site.OrganizationName = orgName
			orgSites[i] = site
			if siteCursor+i == firstInBox {
				fix.ShiftedSiteID = site.ID
				fix.ShiftedExact = site.ExactPoint
				fix.ShiftedShown = site.ShownPoint
			}
		}
		stations := make([]Station, 0, nDatastreams)
		datastreams := make([]Datastream, nDatastreams)
		for j := 0; j < nDatastreams; j++ {
			global := datastreamCursor + j
			site := orgSites[j%nSites]
			intervals := intervalsFor(global, site.ID)
			datastream := Datastream{
				ID:             fmt.Sprintf("datastream-%06d", global),
				Name:           fmt.Sprintf("Datastream %06d", global),
				OrganizationID: orgID,
				UnitName:       "unit",
				Measurement:    Measurement(global),
				Purpose:        purposeOf(global),
				PublicOK:       global%10 != 0,
				MemberOK:       true,
			}
			if global%11 == 0 {
				datastream.Intervals = intervals
			} else {
				stationID := fmt.Sprintf("station-%06d", global)
				stations = append(stations, Station{
					ID:             stationID,
					Name:           fmt.Sprintf("Station %06d", global),
					OrganizationID: orgID,
					Intervals:      intervals,
				})
				datastream.StationID = stationID
			}
			datastreams[j] = datastream
		}
		b := NewBuilder()
		if err := b.AddSites(orgSites); err != nil {
			return Fixture{}, err
		}
		if err := b.AddStations(stations); err != nil {
			return Fixture{}, err
		}
		if err := b.AddDatastreams(datastreams); err != nil {
			return Fixture{}, err
		}
		started := time.Now()
		blob, n, err := b.Finish(FinishPrivate)
		fix.Finish += time.Since(started)
		if err != nil {
			return Fixture{}, err
		}
		fix.Blobs[org] = blob
		fix.DocCounts[org] = n
		siteCursor += nSites
		datastreamCursor += nDatastreams
	}
	return fix, nil
}

// Measurement returns one of the twelve synthetic measurement names.
func Measurement(i int) string {
	if i < 0 {
		i = 0
	}
	return fmt.Sprintf("Meas%02d", i%12+1)
}

func purposeOf(i int) string {
	switch i % 10 {
	case 8:
		return PurposeRaw
	case 9:
		return PurposeStatusInformation
	default:
		return PurposeReadyToUse
	}
}

func intervalsFor(i int, siteID string) []Interval {
	open := groupFor(i)
	closed := otherPublicGroup(open)
	switch {
	case i%7 == 0:
		return []Interval{
			{SiteID: siteID, BeginsAt: fixtureEpoch, EndsBefore: fixtureEpoch + 5000, GroupID: closed},
			{SiteID: siteID, BeginsAt: fixtureEpoch + 8000, EndsBefore: fixtureEpoch + 9000, GroupID: closed},
			{SiteID: siteID, BeginsAt: fixtureEpoch + 25000, EndsBefore: 0, GroupID: open},
		}
	case i%5 == 0:
		return []Interval{
			{SiteID: siteID, BeginsAt: fixtureEpoch, EndsBefore: fixtureEpoch + 5000, GroupID: closed},
			{SiteID: siteID, BeginsAt: fixtureEpoch + 25000, EndsBefore: 0, GroupID: open},
		}
	default:
		return []Interval{{SiteID: siteID, BeginsAt: fixtureEpoch, EndsBefore: 0, GroupID: open}}
	}
}

// groupFor assigns a demo group id. internal is only on member-only rows
// (i%10 == 0, which is PublicOK false). Some other rows have no group.
func groupFor(i int) string {
	if i%10 == 0 {
		return "internal"
	}
	if i%13 == 0 {
		return ""
	}
	switch i % 3 {
	case 0:
		return "field"
	case 1:
		return "lab"
	default:
		return "volunteers"
	}
}

func otherPublicGroup(g string) string {
	switch g {
	case "field":
		return "lab"
	case "lab":
		return "volunteers"
	case "volunteers":
		return "field"
	default:
		return g
	}
}

func outlierCount(sites int) int {
	return int(math.Round(float64(sites) * 9 / 286))
}

func portions(total, parts int) []int {
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

func shiftGeo(box Box, p Point, delta float64) (Box, Point) {
	box.West += delta
	box.East += delta
	box.South += delta
	box.North += delta
	p.Lon += delta
	p.Lat += delta
	return box, p
}

func inCalifornia(p Point) bool {
	return p.Lon >= caWest && p.Lon < caEast && p.Lat >= caSouth && p.Lat < caNorth
}

// BenchCase is one timed query in the benchmark matrix.
type BenchCase struct {
	Name  string
	Query Query
}

// BenchCases returns the four design cases. The shown-versus-exact case is two queries.
func BenchCases(f Fixture) []BenchCase {
	world := Box{West: -180, South: -90, East: 180, North: 90}
	california := Box{West: caWest, South: caSouth, East: caEast, North: caNorth}
	window := Box{West: -120.4, South: 39.3, East: -120.2, North: 39.5}
	tight := Box{
		West:  f.ShiftedExact.Lon - 0.05,
		South: f.ShiftedExact.Lat - 0.05,
		East:  f.ShiftedExact.Lon + 0.05,
		North: f.ShiftedExact.Lat + 0.05,
	}
	return []BenchCase{
		{Name: "world-z2", Query: Query{Geometry: GeometryShown, Viewport: &world, Zoom: 2}},
		{Name: "california-z8", Query: Query{
			Geometry:     GeometryShown,
			Viewport:     &california,
			Zoom:         8,
			Measurements: []string{Measurement(0)},
		}},
		{Name: "window-z11", Query: Query{Geometry: GeometryShown, Viewport: &window, Zoom: 11, PageSize: 50}},
		{Name: "shifted-exact", Query: Query{Geometry: GeometryExact, Viewport: &tight, Zoom: 11}},
		{Name: "shifted-shown", Query: Query{Geometry: GeometryShown, Viewport: &tight, Zoom: 11}},
	}
}
