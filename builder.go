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

// FinishMode selects which shard Finish writes. The index stays in place,
// so Finish may be called again with the other mode.
type FinishMode int

const (
	// FinishPrivate writes shown and exact boxes and every accepted datastream.
	FinishPrivate FinishMode = iota
	// FinishPublic writes exact boxes and points copied from shown, and
	// public_ok datastreams only. Sites with no public datastream are omitted.
	FinishPublic
)

// Builder accumulates one organization. The first added record sets
// OrganizationID. Later records must use that same id. Feed sites, then
// stations, then datastreams. Each accepted datastream is recorded into the
// index immediately. Finish serializes that index and returns the shard bytes
// and the doc count.
type Builder struct {
	organizationID string
	stage          int
	sites          []Site
	siteAt         map[string]int
	stations       []Station
	stationAt      map[string]int
	datastreamAt   map[string]struct{}
	intervals      []storedInterval
	forward        []forwardRow
	purpose        map[string]*roaring.Bitmap
	measurement    map[string]*roaring.Bitmap
	currentSite    map[string]*roaring.Bitmap
	openGroup      map[string]*roaring.Bitmap
	publicOK       *roaring.Bitmap
	memberOK       *roaring.Bitmap
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
	b.openGroup = map[string]*roaring.Bitmap{}
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
	if err := b.organization(site.OrganizationID); err != nil {
		return err
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

// HasSite reports whether AddSite accepted id. The lookup uses the id map
// AddSite already keeps.
func (b *Builder) HasSite(id string) bool {
	if b == nil || b.siteAt == nil {
		return false
	}
	_, ok := b.siteAt[id]
	return ok
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
	if err := b.organization(station.OrganizationID); err != nil {
		return err
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

// HasStation reports whether AddStation accepted id. The lookup uses the id
// map AddStation already keeps.
func (b *Builder) HasStation(id string) bool {
	if b == nil || b.stationAt == nil {
		return false
	}
	_, ok := b.stationAt[id]
	return ok
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
	if err := b.organization(datastream.OrganizationID); err != nil {
		return err
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
	openSite := noOpenSite
	var openGroup string
	for _, iv := range resolved.intervals {
		b.intervals = append(b.intervals, storedInterval{
			doc:    doc,
			site:   b.siteAt[iv.SiteID],
			begins: iv.BeginsAt,
			ends:   iv.EndsBefore,
			group:  iv.GroupID,
		})
		if iv.EndsBefore == 0 {
			openSite = b.siteAt[iv.SiteID]
			openGroup = iv.GroupID
		}
	}
	if resolved.openID != "" {
		addPosting(b.currentSite, resolved.openID, doc)
		if openGroup != "" {
			addPosting(b.openGroup, openGroup, doc)
		}
	}
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
	b.forward = append(b.forward, forwardRow{
		datastreamID:   datastream.ID,
		name:           datastream.Name,
		stationID:      resolved.stationID,
		stationName:    resolved.stationName,
		unit:           datastream.UnitName,
		measurement:    datastream.Measurement,
		purpose:        datastream.Purpose,
		organizationID: datastream.OrganizationID,
		fieldNames:     append([]string(nil), datastream.FieldNames...),
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

// Finish serializes the index recorded by AddDatastream.
// FinishPrivate writes every datastream. FinishPublic writes public_ok
// datastreams only, with exact geometry copied from shown.
// The index is left in place. Another Finish is allowed, in either order,
// and the same mode twice returns the same bytes. Add after the first Finish
// returns ErrFinished. An unknown mode returns ErrInvalid and does not seal.
func (b *Builder) Finish(mode FinishMode) ([]byte, uint32, error) {
	if b == nil {
		return nil, 0, ErrInvalid
	}
	if mode != FinishPrivate && mode != FinishPublic {
		return nil, 0, ErrInvalid
	}
	if b.siteAt == nil {
		b.resetIndex()
	}
	if b.stage != stageFinished {
		sort.Slice(b.intervals, func(i, j int) bool {
			if b.intervals[i].doc != b.intervals[j].doc {
				return b.intervals[i].doc < b.intervals[j].doc
			}
			if b.intervals[i].begins != b.intervals[j].begins {
				return b.intervals[i].begins < b.intervals[j].begins
			}
			return b.intervals[i].site < b.intervals[j].site
		})
		b.stage = stageFinished
	}
	img := shardImage{
		sites:       b.sites,
		intervals:   b.intervals,
		forward:     b.forward,
		purpose:     b.purpose,
		measurement: b.measurement,
		currentSite: b.currentSite,
		openGroup:   b.openGroup,
		publicOK:    b.publicOK,
		memberOK:    b.memberOK,
	}
	if mode == FinishPublic {
		return writePublicShard(img)
	}
	blob, err := writeShard(img)
	if err != nil {
		return nil, 0, err
	}
	return blob, uint32(len(b.forward)), nil
}

type resolvedDatastream struct {
	intervals   []Interval
	openID      string
	stationID   string
	stationName string
}

func (b *Builder) resolve(datastream Datastream) (resolvedDatastream, error) {
	if datastream.StationID != "" && len(datastream.Intervals) > 0 {
		return resolvedDatastream{}, fmt.Errorf("search: datastream %s: %w", datastream.ID, ErrDatastream)
	}
	var intervals []Interval
	var stationName string
	if datastream.StationID != "" {
		idx, ok := b.stationAt[datastream.StationID]
		if !ok {
			return resolvedDatastream{}, fmt.Errorf("search: station %s: %w", datastream.StationID, ErrNotFound)
		}
		intervals = b.stations[idx].Intervals
		stationName = b.stations[idx].Name
	} else {
		if len(datastream.Intervals) == 0 {
			return resolvedDatastream{}, fmt.Errorf("search: datastream %s: %w", datastream.ID, ErrDatastream)
		}
		intervals = datastream.Intervals
		if err := validateIntervals(intervals, b.siteAt); err != nil {
			return resolvedDatastream{}, err
		}
	}
	var openID string
	for _, iv := range intervals {
		if iv.EndsBefore == 0 {
			openID = iv.SiteID
			break
		}
	}
	return resolvedDatastream{
		intervals:   intervals,
		openID:      openID,
		stationID:   datastream.StationID,
		stationName: stationName,
	}, nil
}

// organization captures the builder organization from the first record and
// rejects any later id that differs, including an empty one.
func (b *Builder) organization(id string) error {
	if id == "" || (b.organizationID != "" && id != b.organizationID) {
		return ErrInvalid
	}
	if b.organizationID == "" {
		b.organizationID = id
	}
	return nil
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
	if open > 1 {
		return ErrInterval
	}
	return nil
}
