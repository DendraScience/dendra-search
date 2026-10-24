package search

import (
	"sort"

	"github.com/RoaringBitmap/roaring"
)

// Query evaluates the predicates that are set. Shard bitmaps are not modified.
func (s *Shard) Query(q Query) (Result, error) {
	if s == nil {
		return Result{}, ErrInvalid
	}
	if err := validateQuery(q); err != nil {
		return Result{}, err
	}
	hits, scope, withoutSites, withoutMeas, siteBM := s.evaluate(q)
	res := lay(q, s.candidates(q, hits, siteBM))
	res.TotalCount = uint32(hits.GetCardinality())
	res.MeasurementCounts = measurementCounts(s.measurement, withoutMeas)
	if res.Kind == KindSites {
		res.SiteCounts = s.siteCounts(scope, withoutSites, siteBM)
	}
	switch {
	case q.AllHits && res.Kind == KindSites:
		res.Hits = s.allHits(q, hits)
	case !q.AllHits && q.PageSize > 0:
		res.Hits, res.NextPageAfter, res.HasMore = s.page(q, hits)
	}
	return res, nil
}

func (s *Shard) evaluate(q Query) (hits, scope, withoutSites, withoutMeas *roaring.Bitmap, siteBM map[string]*roaring.Bitmap) {
	base := s.accessBitmap(q.IncludeMemberOnly)
	if q.Purpose != "" {
		if p := s.purpose[q.Purpose]; p == nil {
			base.Clear()
		} else {
			base.And(p)
		}
	}

	var meas *roaring.Bitmap
	if len(q.Measurements) > 0 {
		meas = roaring.New()
		for _, name := range q.Measurements {
			if bm := s.measurement[name]; bm != nil {
				meas.Or(bm)
			}
		}
	}

	// Open path: one interval per doc, so posting lists are the joint test.
	// Window path: site, group, and viewport qualify on the same interval.
	var timeBase *roaring.Bitmap
	var geo *roaring.Bitmap
	if q.BeginsAt == 0 && q.EndsBefore == 0 {
		base.And(openDocs(s.currentSite))
		timeBase = base
		if len(q.GroupIDs) > 0 {
			timeBase = base.Clone()
			timeBase.And(s.openGroupMask(q.GroupIDs))
		}
		siteBM = s.currentSite
		if q.Viewport != nil {
			geo = s.viewportMask(q, siteBM)
		}
	} else {
		siteBM = s.windowSites(q, base)
		timeBase = roaring.New()
		for _, bm := range siteBM {
			timeBase.Or(bm)
		}
	}

	// Scope is the site-facet universe: purpose, group, access, time, and
	// viewport — not measurement or site. withoutSites adds measurement.
	scope = narrow(timeBase, geo)

	// SiteIDs outside the geographic viewport are ignored for the hit
	// predicate so a remembered selection does not empty the current view.
	// If every SiteID is off-map, siteFilter stays nil (no site predicate).
	// If at least one is in view but has no matching docs (e.g. group joint
	// mismatch), siteFilter stays an empty bitmap so hits stay empty.
	var siteFilter *roaring.Bitmap
	if len(q.SiteIDs) > 0 {
		inView := s.viewportSiteSet(q)
		siteFilter = roaring.New()
		anyInView := false
		for _, id := range q.SiteIDs {
			if inView != nil {
				if _, ok := inView[id]; !ok {
					continue
				}
			}
			anyInView = true
			if bm := siteBM[id]; bm != nil {
				siteFilter.Or(bm)
			}
		}
		if !anyInView {
			siteFilter = nil
		}
	}

	withoutSites = narrow(scope, meas)
	withoutMeas = narrow(timeBase, siteFilter, geo)
	hits = narrow(withoutSites, siteFilter)
	return hits, scope, withoutSites, withoutMeas, siteBM
}

// viewportSiteSet is the site ids whose boxes intersect the viewport.
// Nil means no viewport (every site is in view).
func (s *Shard) viewportSiteSet(q Query) map[string]struct{} {
	if q.Viewport == nil {
		return nil
	}
	out := map[string]struct{}{}
	for _, idx := range s.searchBoxes(q.Geometry, *q.Viewport) {
		out[s.sites[idx].ID] = struct{}{}
	}
	return out
}

// accessBitmap is public_ok, or public_ok ∪ member_ok. It does not apply purpose.
func (s *Shard) accessBitmap(includeMemberOnly bool) *roaring.Bitmap {
	base := s.publicOK.Clone()
	if includeMemberOnly {
		base.Or(s.memberOK)
	}
	return base
}

// openDocs is the datastreams that have an open deployment. A fully closed
// history is absent from a query that sets no time window.
func openDocs(postings map[string]*roaring.Bitmap) *roaring.Bitmap {
	out := roaring.New()
	for _, bm := range postings {
		if bm != nil {
			out.Or(bm)
		}
	}
	return out
}

// openGroupMask is the union of open-group postings for ids. Empty ids are skipped.
// A datastream whose open interval has no group is absent.
func (s *Shard) openGroupMask(ids []string) *roaring.Bitmap {
	out := roaring.New()
	for _, id := range ids {
		if id == "" {
			continue
		}
		if bm := s.openGroup[id]; bm != nil {
			out.Or(bm)
		}
	}
	return out
}

func (s *Shard) viewportMask(q Query, siteBM map[string]*roaring.Bitmap) *roaring.Bitmap {
	geo := roaring.New()
	for _, idx := range s.searchBoxes(q.Geometry, *q.Viewport) {
		if bm := siteBM[s.sites[idx].ID]; bm != nil {
			geo.Or(bm)
		}
	}
	return geo
}

// windowSites records docs on intervals that overlap the window and pass the
// group filter and the viewport. The site filter is applied later, so a site
// count can omit it. An empty GroupID qualifies only when GroupIDs is empty.
func (s *Shard) windowSites(q Query, base *roaring.Bitmap) map[string]*roaring.Bitmap {
	groups := idSet(q.GroupIDs)
	var view map[int]struct{}
	if q.Viewport != nil {
		view = map[int]struct{}{}
		for _, idx := range s.searchBoxes(q.Geometry, *q.Viewport) {
			view[idx] = struct{}{}
		}
	}
	siteBM := map[string]*roaring.Bitmap{}
	for doc, intervals := range s.byDoc {
		if !base.Contains(uint32(doc)) {
			continue
		}
		for _, iv := range intervals {
			if !intervalQualifies(iv, q.BeginsAt, q.EndsBefore, groups, view) {
				continue
			}
			key := s.sites[iv.site].ID
			bm := siteBM[key]
			if bm == nil {
				bm = roaring.New()
				siteBM[key] = bm
			}
			bm.Add(uint32(doc))
		}
	}
	return siteBM
}

// intervalQualifies reports whether iv overlaps the window and passes the group
// filter and the viewport. A nil groups set means the group filter is empty.
// view nil means no viewport. An empty GroupID matches only when groups is nil.
func intervalQualifies(iv storedInterval, begins, ends int64, groups map[string]struct{}, view map[int]struct{}) bool {
	if !overlaps(iv.begins, iv.ends, begins, ends) {
		return false
	}
	if groups != nil {
		if _, ok := groups[iv.group]; !ok {
			return false
		}
	}
	if view != nil {
		if _, ok := view[iv.site]; !ok {
			return false
		}
	}
	return true
}

// idSet is nil when ids is empty, which means the filter is off. Empty strings
// are dropped. A non-empty list that contains only empty strings matches nothing.
func idSet(ids []string) map[string]struct{} {
	if len(ids) == 0 {
		return nil
	}
	m := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		m[id] = struct{}{}
	}
	if len(m) == 0 {
		return map[string]struct{}{}
	}
	return m
}

func narrow(base *roaring.Bitmap, masks ...*roaring.Bitmap) *roaring.Bitmap {
	out := base.Clone()
	for _, m := range masks {
		if m == nil {
			continue
		}
		out.And(m)
	}
	return out
}

func overlaps(begins, ends, windowStart, windowEnd int64) bool {
	if ends != 0 && ends <= windowStart {
		return false
	}
	if begins >= windowEnd {
		return false
	}
	return true
}

func (s *Shard) searchBoxes(geometry string, v Box) []int {
	tr := &s.shown
	if geometry == GeometryExact {
		tr = &s.exact
	}
	seen := map[int]struct{}{}
	var out []int
	visit := func(_, _ [2]float64, idx int) bool {
		if _, ok := seen[idx]; ok {
			return true
		}
		seen[idx] = struct{}{}
		out = append(out, idx)
		return true
	}
	if v.East < v.West {
		tr.Search([2]float64{v.West, v.South}, [2]float64{180, v.North}, visit)
		tr.Search([2]float64{-180, v.South}, [2]float64{v.East, v.North}, visit)
		return out
	}
	tr.Search([2]float64{v.West, v.South}, [2]float64{v.East, v.North}, visit)
	return out
}

func (s *Shard) candidates(q Query, hits *roaring.Bitmap, siteBM map[string]*roaring.Bitmap) []SiteMarker {
	out := make([]SiteMarker, 0)
	for i := range s.sites {
		bm := siteBM[s.sites[i].ID]
		if bm == nil {
			continue
		}
		n := hits.AndCardinality(bm)
		if n == 0 {
			continue
		}
		out = append(out, marker(s.sites[i], q.Geometry, uint32(n)))
	}
	return out
}

func marker(site Site, geometry string, n uint32) SiteMarker {
	m := SiteMarker{
		SiteID:           site.ID,
		Name:             site.Name,
		OrganizationID:   site.OrganizationID,
		OrganizationName: site.OrganizationName,
		DatastreamCount:  n,
	}
	if geometry == GeometryExact {
		m.Point = site.ExactPoint
		m.Box = site.Exact
	} else {
		m.Point = site.ShownPoint
		m.Box = site.Shown
	}
	return m
}

func measurementCounts(postings map[string]*roaring.Bitmap, withoutMeas *roaring.Bitmap) []Count {
	return countsFromBitmaps(postings, withoutMeas)
}

// siteCounts lists every site with hits under scope (viewport / purpose /
// group / time / access), including zeros after the measurement filter.
// Selecting a measurement must not remove in-view sites from the facet —
// only their counts change. Sites outside scope are omitted; the host
// clears site selections that leave the view. Each row carries the Site
// name so zero-count facet rows do not fall back to bare ids.
func (s *Shard) siteCounts(scope, withoutSites *roaring.Bitmap, siteBM map[string]*roaring.Bitmap) []Count {
	vals := map[string]Count{}
	for id, bm := range siteBM {
		if bm == nil {
			continue
		}
		if scope.AndCardinality(bm) == 0 {
			continue
		}
		c := Count{
			Value: id,
			Count: uint32(withoutSites.AndCardinality(bm)),
		}
		if i, ok := s.siteAt[id]; ok {
			site := s.sites[i]
			c.Name = site.Name
			c.OrganizationID = site.OrganizationID
			c.OrganizationName = site.OrganizationName
		}
		vals[id] = c
	}
	return countsFromCountMap(vals)
}

func countsFromBitmaps(postings map[string]*roaring.Bitmap, mask *roaring.Bitmap) []Count {
	vals := make(map[string]uint32, len(postings))
	for name, bm := range postings {
		vals[name] = uint32(mask.AndCardinality(bm))
	}
	return countsFromMap(vals)
}

func (s *Shard) allHits(q Query, hits *roaring.Bitmap) []Hit {
	if hits.IsEmpty() {
		return nil
	}
	ids := hits.ToArray()
	out := make([]Hit, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.hit(id, q))
	}
	return out
}

func (s *Shard) page(q Query, hits *roaring.Bitmap) ([]Hit, uint32, bool) {
	if hits.IsEmpty() {
		return nil, 0, false
	}
	ids := hits.ToArray()
	out := make([]Hit, 0, q.PageSize)
	var next uint32
	more := false
	for _, id := range ids {
		if id < q.PageAfter {
			continue
		}
		if uint32(len(out)) == q.PageSize {
			more = true
			next = id
			break
		}
		out = append(out, s.hit(id, q))
	}
	return out, next, more
}

func (s *Shard) hit(doc uint32, q Query) Hit {
	row := s.forward[doc]
	siteIdx := row.openSite
	if q.BeginsAt != 0 || q.EndsBefore != 0 {
		siteIdx = s.siteInWindow(doc, q, siteIdx)
	}
	site := s.sites[siteIdx]
	return Hit{
		DocID:            doc,
		DatastreamID:     row.datastreamID,
		Name:             row.name,
		OrganizationID:   row.organizationID,
		OrganizationName: site.OrganizationName,
		StationID:        row.stationID,
		StationName:      row.stationName,
		SiteID:           site.ID,
		SiteName:         site.Name,
		UnitName:         row.unit,
		Measurement:      row.measurement,
		Purpose:          row.purpose,
		FieldNames:       append([]string(nil), row.fieldNames...),
	}
}

// siteInWindow returns the latest qualifying interval's site. Group, site, and
// viewport are applied to that interval, so the hit is not labeled with one
// the filters excluded.
func (s *Shard) siteInWindow(doc uint32, q Query, fallback int) int {
	groups := idSet(q.GroupIDs)
	sites := idSet(q.SiteIDs)
	var view map[int]struct{}
	if q.Viewport != nil {
		view = map[int]struct{}{}
		for _, idx := range s.searchBoxes(q.Geometry, *q.Viewport) {
			view[idx] = struct{}{}
		}
	}
	best := fallback
	var bestBegins int64
	seen := false
	for _, iv := range s.byDoc[doc] {
		if !intervalQualifies(iv, q.BeginsAt, q.EndsBefore, groups, view) {
			continue
		}
		if sites != nil {
			if _, ok := sites[s.sites[iv.site].ID]; !ok {
				continue
			}
		}
		if !seen || iv.begins >= bestBegins {
			best = iv.site
			bestBegins = iv.begins
			seen = true
		}
	}
	return best
}

// GroupIDs returns the distinct group ids on in-play intervals of datastreams
// the caller may see. The result is sorted. Empty group ids are omitted.
// There is no count. Purpose, measurement, site, viewport, and Query.GroupIDs
// are not applied. includeMemberOnly false uses public_ok. True uses
// public_ok ∪ member_ok.
func (s *Shard) GroupIDs(beginsAt, endsBefore int64, includeMemberOnly bool) ([]string, error) {
	if s == nil {
		return nil, ErrInvalid
	}
	if err := validateWindow(beginsAt, endsBefore); err != nil {
		return nil, err
	}
	access := s.accessBitmap(includeMemberOnly)
	seen := map[string]struct{}{}
	if beginsAt == 0 && endsBefore == 0 {
		for id, bm := range s.openGroup {
			if id == "" || bm == nil {
				continue
			}
			if access.AndCardinality(bm) > 0 {
				seen[id] = struct{}{}
			}
		}
	} else {
		for doc, intervals := range s.byDoc {
			if !access.Contains(uint32(doc)) {
				continue
			}
			for _, iv := range intervals {
				if iv.group == "" || !overlaps(iv.begins, iv.ends, beginsAt, endsBefore) {
					continue
				}
				seen[iv.group] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}
