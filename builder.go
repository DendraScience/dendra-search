package search

import (
	"fmt"
	"sort"

	"github.com/RoaringBitmap/roaring"
)

const (
	stageSites = iota
	stageStations
	stageDatastreams
	stageFinished
)

// Builder accumulates one organization. Feed sites, then stations, then
// datastreams. Each accepted datastream is recorded into the index immediately.
// Finish serializes that index and returns the shard bytes and the doc count.
type Builder struct {
	stage        int
	sites        []Site
	siteAt       map[string]int
	stations     []Station
	stationAt    map[string]int
	datastreamAt map[string]struct{}
	intervals    []storedInterval
	forward      []forwardRow
	purpose      map[string]*roaring.Bitmap
	measurement  map[string]*roaring.Bitmap
	currentSite  map[string]*roaring.Bitmap
	publicOK     *roaring.Bitmap
	memberOK     *roaring.Bitmap
}

// NewBuilder returns an empty builder for one organization.
func NewBuilder() *Builder {
	b := &Builder{}
	b.resetIndex()
	return b
}

func (b *Builder) resetIndex() {
	b.siteAt = map[string]int{}
	b.stationAt = map[string]int{}
	b.datastreamAt = map[string]struct{}{}
	b.purpose = map[string]*roaring.Bitmap{}
	b.measurement = map[string]*roaring.Bitmap{}
	b.currentSite = map[string]*roaring.Bitmap{}
	b.publicOK = roaring.New()
	b.memberOK = roaring.New()
}

func (b *Builder) init() error {
	if b == nil {
		return ErrInvalid
	}
	if b.siteAt == nil {
		b.resetIndex()
	}
	if b.stage == stageFinished {
		return ErrFinished
	}
	return nil
}

// AddSite appends one site. It copies the struct into the builder.
func (b *Builder) AddSite(site Site) error {
	if err := b.init(); err != nil {
		return err
	}
	if b.stage != stageSites {
		return ErrOrder
	}
	if site.ID == "" {
		return fmt.Errorf("search: site id: %w", ErrInvalid)
	}
	if err := validateBox(site.Shown); err != nil {
		return err
	}
	if err := validateBox(site.Exact); err != nil {
		return err
	}
	if err := validatePoint(site.ShownPoint); err != nil {
		return err
	}
	if err := validatePoint(site.ExactPoint); err != nil {
		return err
	}
	if _, ok := b.siteAt[site.ID]; ok {
		return fmt.Errorf("search: site %s: %w", site.ID, ErrDuplicate)
	}
	b.siteAt[site.ID] = len(b.sites)
	b.sites = append(b.sites, site)
	return nil
}

// AddSites adds each site in order.
func (b *Builder) AddSites(sites []Site) error {
	for i := range sites {
		if err := b.AddSite(sites[i]); err != nil {
			return err
		}
	}
	return nil
}

// AddStation appends one station and copies its intervals.
func (b *Builder) AddStation(station Station) error {
	if err := b.init(); err != nil {
		return err
	}
	if b.stage > stageStations {
		return ErrOrder
	}
	if station.ID == "" {
		return fmt.Errorf("search: station id: %w", ErrInvalid)
	}
	if _, ok := b.stationAt[station.ID]; ok {
		return fmt.Errorf("search: station %s: %w", station.ID, ErrDuplicate)
	}
	station.Intervals = cloneIntervals(station.Intervals)
	if err := validateIntervals(station.Intervals, b.siteAt); err != nil {
		return err
	}
	b.stage = stageStations
	b.stationAt[station.ID] = len(b.stations)
	b.stations = append(b.stations, station)
	return nil
}

// AddStations adds each station in order.
func (b *Builder) AddStations(stations []Station) error {
	for i := range stations {
		if err := b.AddStation(stations[i]); err != nil {
			return err
		}
	}
	return nil
}

// AddDatastream records one datastream into the posting lists, interval table,
// and forward rows. The doc id is the accept order. The struct is not retained.
func (b *Builder) AddDatastream(datastream Datastream) error {
	if err := b.init(); err != nil {
		return err
	}
	if datastream.ID == "" {
		return fmt.Errorf("search: datastream id: %w", ErrInvalid)
	}
	if _, ok := b.datastreamAt[datastream.ID]; ok {
		return fmt.Errorf("search: datastream %s: %w", datastream.ID, ErrDuplicate)
	}
	resolved, err := b.resolve(datastream)
	if err != nil {
		return err
	}
	b.stage = stageDatastreams
	b.datastreamAt[datastream.ID] = struct{}{}
	doc := uint32(len(b.forward))
	var openSite int
	var openID string
	for _, iv := range resolved.intervals {
		idx := b.siteAt[iv.SiteID]
		b.intervals = append(b.intervals, storedInterval{
			doc:    doc,
			site:   idx,
			begins: iv.BeginsAt,
			ends:   iv.EndsBefore,
		})
		if iv.EndsBefore == 0 {
			openSite = idx
			openID = iv.SiteID
		}
	}
	addPosting(b.currentSite, openID, doc)
	if datastream.Purpose != "" {
		addPosting(b.purpose, datastream.Purpose, doc)
	}
	if datastream.Measurement != "" {
		addPosting(b.measurement, datastream.Measurement, doc)
	}
	if datastream.PublicOK {
		b.publicOK.Add(doc)
	}
	if datastream.MemberOK {
		b.memberOK.Add(doc)
	}
	orgID := datastream.OrganizationID
	if orgID == "" {
		orgID = b.sites[openSite].OrganizationID
	}
	b.forward = append(b.forward, forwardRow{
		datastreamID:   datastream.ID,
		name:           datastream.Name,
		stationID:      resolved.stationID,
		unit:           datastream.UnitName,
		measurement:    datastream.Measurement,
		purpose:        datastream.Purpose,
		organizationID: orgID,
		openSite:       openSite,
	})
	return nil
}

// AddDatastreams adds each datastream in order.
func (b *Builder) AddDatastreams(datastreams []Datastream) error {
	for i := range datastreams {
		if err := b.AddDatastream(datastreams[i]); err != nil {
			return err
		}
	}
	return nil
}

// Finish serializes the index recorded by AddDatastream. A second call returns ErrFinished.
func (b *Builder) Finish() ([]byte, uint32, error) {
	if err := b.init(); err != nil {
		return nil, 0, err
	}
	sort.Slice(b.intervals, func(i, j int) bool {
		if b.intervals[i].doc != b.intervals[j].doc {
			return b.intervals[i].doc < b.intervals[j].doc
		}
		if b.intervals[i].begins != b.intervals[j].begins {
			return b.intervals[i].begins < b.intervals[j].begins
		}
		return b.intervals[i].site < b.intervals[j].site
	})
	n := uint32(len(b.forward))
	blob, err := writeShard(shardImage{
		sites:       b.sites,
		intervals:   b.intervals,
		forward:     b.forward,
		purpose:     b.purpose,
		measurement: b.measurement,
		currentSite: b.currentSite,
		publicOK:    b.publicOK,
		memberOK:    b.memberOK,
	})
	if err != nil {
		return nil, 0, err
	}
	b.stage = stageFinished
	b.sites = nil
	b.stations = nil
	b.intervals = nil
	b.forward = nil
	b.purpose = nil
	b.measurement = nil
	b.currentSite = nil
	b.publicOK = nil
	b.memberOK = nil
	return blob, n, nil
}

type resolvedDatastream struct {
	intervals []Interval
	stationID string
}

func (b *Builder) resolve(datastream Datastream) (resolvedDatastream, error) {
	if datastream.StationID != "" && len(datastream.Intervals) > 0 {
		return resolvedDatastream{}, fmt.Errorf("search: datastream %s: %w", datastream.ID, ErrDatastream)
	}
	var intervals []Interval
	if datastream.StationID != "" {
		idx, ok := b.stationAt[datastream.StationID]
		if !ok {
			return resolvedDatastream{}, fmt.Errorf("search: station %s: %w", datastream.StationID, ErrNotFound)
		}
		intervals = b.stations[idx].Intervals
	} else {
		if len(datastream.Intervals) == 0 {
			return resolvedDatastream{}, fmt.Errorf("search: datastream %s: %w", datastream.ID, ErrDatastream)
		}
		intervals = datastream.Intervals
		if err := validateIntervals(intervals, b.siteAt); err != nil {
			return resolvedDatastream{}, err
		}
	}
	var openSite string
	for _, iv := range intervals {
		if iv.EndsBefore == 0 {
			openSite = iv.SiteID
		}
	}
	if datastream.OrganizationID != "" && openSite != "" {
		if org := b.sites[b.siteAt[openSite]].OrganizationID; org != "" && org != datastream.OrganizationID {
			return resolvedDatastream{}, fmt.Errorf("search: datastream %s organization: %w", datastream.ID, ErrDatastream)
		}
	}
	return resolvedDatastream{intervals: intervals, stationID: datastream.StationID}, nil
}

func addPosting(m map[string]*roaring.Bitmap, key string, doc uint32) {
	bm := m[key]
	if bm == nil {
		bm = roaring.New()
		m[key] = bm
	}
	bm.Add(doc)
}

func cloneIntervals(in []Interval) []Interval {
	if len(in) == 0 {
		return nil
	}
	out := make([]Interval, len(in))
	copy(out, in)
	return out
}

func validateIntervals(intervals []Interval, sites map[string]int) error {
	if len(intervals) == 0 {
		return ErrInterval
	}
	sorted := append([]Interval(nil), intervals...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].BeginsAt != sorted[j].BeginsAt {
			return sorted[i].BeginsAt < sorted[j].BeginsAt
		}
		return sorted[i].SiteID < sorted[j].SiteID
	})
	open := 0
	for i, iv := range sorted {
		if iv.SiteID == "" {
			return ErrInterval
		}
		if _, ok := sites[iv.SiteID]; !ok {
			return fmt.Errorf("search: site %s: %w", iv.SiteID, ErrNotFound)
		}
		if iv.BeginsAt < 0 || iv.EndsBefore < 0 {
			return ErrInterval
		}
		if iv.EndsBefore == 0 {
			open++
		} else if iv.EndsBefore <= iv.BeginsAt {
			return ErrInterval
		}
		if i > 0 {
			prev := sorted[i-1]
			if prev.EndsBefore == 0 || iv.BeginsAt < prev.EndsBefore {
				return ErrInterval
			}
		}
	}
	if open != 1 {
		return ErrInterval
	}
	return nil
}
