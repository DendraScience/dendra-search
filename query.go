package search

import "github.com/RoaringBitmap/roaring"

// Query evaluates the predicates that are set. Shard bitmaps are not modified.
func (s *Shard) Query(q Query) (Result, error) {
	if s == nil {
		return Result{}, ErrInvalid
	}
	if err := validateQuery(q); err != nil {
		return Result{}, err
	}
	hits, withoutSites, withoutMeas, siteBM := s.evaluate(q)
	res := lay(q, s.candidates(q, hits, siteBM))
	res.TotalCount = uint32(hits.GetCardinality())
	res.MeasurementCounts = measurementCounts(s.measurement, withoutMeas)
	if res.Kind == KindSites {
		res.SiteCounts = siteCounts(q, withoutSites, siteBM)
	}
	if q.PageSize > 0 {
		res.Hits, res.NextPageAfter, res.HasMore = s.page(q, hits)
	}
	return res, nil
}

func (s *Shard) evaluate(q Query) (hits, withoutSites, withoutMeas *roaring.Bitmap, siteBM map[string]*roaring.Bitmap) {
	base := s.publicOK.Clone()
	if q.IncludeMemberOnly {
		base.Or(s.memberOK)
	}
	if q.Purpose != "" {
		if p := s.purpose[q.Purpose]; p == nil {
			base.Clear()
		} else {
			base.And(p)
		}
	}

	var timeBase *roaring.Bitmap
	if q.BeginsAt == 0 && q.EndsBefore == 0 {
		timeBase = base
		siteBM = s.currentSite
	} else {
		timeDocs := roaring.New()
		siteBM = map[string]*roaring.Bitmap{}
		for doc, intervals := range s.byDoc {
			if !base.Contains(uint32(doc)) {
				continue
			}
			for _, iv := range intervals {
				if !overlaps(iv.begins, iv.ends, q.BeginsAt, q.EndsBefore) {
					continue
				}
				id := uint32(doc)
				timeDocs.Add(id)
				key := s.sites[iv.site].ID
				bm := siteBM[key]
				if bm == nil {
					bm = roaring.New()
					siteBM[key] = bm
				}
				bm.Add(id)
			}
		}
		timeBase = timeDocs
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
	var siteFilter *roaring.Bitmap
	if len(q.SiteIDs) > 0 {
		siteFilter = roaring.New()
		for _, id := range q.SiteIDs {
			if bm := siteBM[id]; bm != nil {
				siteFilter.Or(bm)
			}
		}
	}
	var geo *roaring.Bitmap
	if q.Viewport != nil {
		geo = roaring.New()
		for _, idx := range s.searchBoxes(q.Geometry, *q.Viewport) {
			if bm := siteBM[s.sites[idx].ID]; bm != nil {
				geo.Or(bm)
			}
		}
	}

	withoutSites = narrow(timeBase, meas, geo)
	withoutMeas = narrow(timeBase, siteFilter, geo)
	hits = narrow(withoutSites, siteFilter)
	return hits, withoutSites, withoutMeas, siteBM
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

func siteCounts(q Query, withoutSites *roaring.Bitmap, siteBM map[string]*roaring.Bitmap) []Count {
	vals := map[string]uint32{}
	for id, bm := range siteBM {
		if bm == nil {
			continue
		}
		n := withoutSites.AndCardinality(bm)
		if n > 0 {
			vals[id] = uint32(n)
		}
	}
	for _, id := range q.SiteIDs {
		if _, ok := vals[id]; !ok {
			vals[id] = 0
		}
	}
	return countsFromMap(vals)
}

func countsFromBitmaps(postings map[string]*roaring.Bitmap, mask *roaring.Bitmap) []Count {
	vals := make(map[string]uint32, len(postings))
	for name, bm := range postings {
		vals[name] = uint32(mask.AndCardinality(bm))
	}
	return countsFromMap(vals)
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
		siteIdx = s.siteInWindow(doc, q.BeginsAt, q.EndsBefore, siteIdx)
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
	}
}

func (s *Shard) siteInWindow(doc uint32, begins, ends int64, fallback int) int {
	best := fallback
	var bestBegins int64
	seen := false
	for _, iv := range s.byDoc[doc] {
		if !overlaps(iv.begins, iv.ends, begins, ends) {
			continue
		}
		if !seen || iv.begins >= bestBegins {
			best = iv.site
			bestBegins = iv.begins
			seen = true
		}
	}
	return best
}
