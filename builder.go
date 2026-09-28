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
// datastreams. Finish returns the shard bytes and the number of datastreams.
type Builder struct {
	stage        int
	sites        []Site
	siteAt       map[string]int
	stations     []Station
	stationAt    map[string]int
	datastreams  []Datastream
	datastreamAt map[string]struct{}
}

// NewBuilder returns an empty builder for one organization.
func NewBuilder() *Builder {
	return &Builder{
		siteAt:       map[string]int{},
		stationAt:    map[string]int{},
		datastreamAt: map[string]struct{}{},
	}
}

func (b *Builder) init() error {
	if b == nil {
		return ErrInvalid
	}
	if b.siteAt == nil {
		b.siteAt = map[string]int{}
		b.stationAt = map[string]int{}
		b.datastreamAt = map[string]struct{}{}
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

// AddDatastream appends one datastream and copies site-attached intervals.
// The doc id is the index of this datastream in add order.
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
	datastream.Intervals = cloneIntervals(datastream.Intervals)
	if _, err := b.resolve(datastream); err != nil {
		return err
	}
	b.stage = stageDatastreams
	b.datastreamAt[datastream.ID] = struct{}{}
	b.datastreams = append(b.datastreams, datastream)
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

// Finish serializes the shard. A second call returns ErrFinished.
func (b *Builder) Finish() ([]byte, uint32, error) {
	if err := b.init(); err != nil {
		return nil, 0, err
	}
	n := len(b.datastreams)
	img := shardImage{
		sites:       append([]Site(nil), b.sites...),
		forward:     make([]forwardRow, n),
		purpose:     map[string]*roaring.Bitmap{},
		measurement: map[string]*roaring.Bitmap{},
		currentSite: map[string]*roaring.Bitmap{},
		publicOK:    roaring.New(),
		memberOK:    roaring.New(),
	}
	for doc, datastream := range b.datastreams {
		resolved, err := b.resolve(datastream)
		if err != nil {
			return nil, 0, err
		}
		var openSite int
		var openID string
		for _, iv := range resolved.intervals {
			idx := b.siteAt[iv.SiteID]
			img.intervals = append(img.intervals, storedInterval{
				doc:    uint32(doc),
				site:   idx,
				begins: iv.BeginsAt,
				ends:   iv.EndsBefore,
			})
			if iv.EndsBefore == 0 {
				openSite = idx
				openID = iv.SiteID
			}
		}
		addPosting(img.currentSite, openID, uint32(doc))
		if datastream.Purpose != "" {
			addPosting(img.purpose, datastream.Purpose, uint32(doc))
		}
		if datastream.Measurement != "" {
			addPosting(img.measurement, datastream.Measurement, uint32(doc))
		}
		if datastream.PublicOK {
			img.publicOK.Add(uint32(doc))
		}
		if datastream.MemberOK {
			img.memberOK.Add(uint32(doc))
		}
		orgID := datastream.OrganizationID
		if orgID == "" {
			orgID = b.sites[openSite].OrganizationID
		}
		img.forward[doc] = forwardRow{
			datastreamID:   datastream.ID,
			name:           datastream.Name,
			stationID:      resolved.stationID,
			unit:           datastream.UnitName,
			measurement:    datastream.Measurement,
			purpose:        datastream.Purpose,
			organizationID: orgID,
			openSite:       openSite,
		}
	}
	sort.Slice(img.intervals, func(i, j int) bool {
		if img.intervals[i].doc != img.intervals[j].doc {
			return img.intervals[i].doc < img.intervals[j].doc
		}
		if img.intervals[i].begins != img.intervals[j].begins {
			return img.intervals[i].begins < img.intervals[j].begins
		}
		return img.intervals[i].site < img.intervals[j].site
	})
	blob, err := writeShard(img)
	if err != nil {
		return nil, 0, err
	}
	b.stage = stageFinished
	return blob, uint32(n), nil
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
