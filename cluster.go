package search

import (
	"math"
	"sort"
)

const clusterCellPx = 48

// Combine merges per-organization results and chooses the map layer once.
// Measurement counts and site counts are summed. Hits are concatenated in
// the order of parts; paging stays on each shard. Pass the same query that
// produced the parts, including zoom and the site limit.
func Combine(q Query, parts []Result) (Result, error) {
	if err := validateQuery(q); err != nil {
		return Result{}, err
	}
	var candidates []SiteMarker
	var total uint32
	meas := map[string]uint32{}
	sites := map[string]uint32{}
	var hits []Hit
	hasMore := false
	for _, part := range parts {
		candidates = append(candidates, part.Candidates...)
		total += part.TotalCount
		for _, c := range part.MeasurementCounts {
			meas[c.Value] += c.Count
		}
		for _, c := range part.SiteCounts {
			sites[c.Value] += c.Count
		}
		hits = append(hits, part.Hits...)
		if part.HasMore {
			hasMore = true
		}
	}
	res := lay(q, candidates)
	res.TotalCount = total
	res.Hits = hits
	res.HasMore = hasMore
	if res.Kind == KindSites {
		res.MeasurementCounts = countsFromMap(meas)
		res.SiteCounts = countsFromMap(sites)
	}
	return res, nil
}

func lay(q Query, candidates []SiteMarker) Result {
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].SiteID < candidates[j].SiteID
	})
	res := Result{
		Candidates: candidates,
		SiteTotal:  uint32(len(candidates)),
	}
	if len(candidates) > 0 {
		res.Bounds = candidates[0].Box
		for _, c := range candidates[1:] {
			res.Bounds = unionBox(res.Bounds, c.Box)
		}
	}
	if layerKind(q, len(candidates)) == KindClusters {
		res.Kind = KindClusters
		res.Clusters = clusterSites(candidates, q.Zoom)
		return res
	}
	res.Kind = KindSites
	res.Sites = candidates
	return res
}

func layerKind(q Query, n int) Kind {
	limit := q.MapSiteLimit
	if limit == 0 {
		limit = DefaultMapSiteLimit
	}
	minZoom := q.MinSiteZoom
	if minZoom == 0 {
		minZoom = DefaultMinSiteZoom
	}
	if uint32(n) > limit || q.Zoom < minZoom {
		return KindClusters
	}
	return KindSites
}

func clusterSites(candidates []SiteMarker, zoom uint32) []Cluster {
	if len(candidates) == 0 {
		return nil
	}
	world := 256 * math.Pow(2, float64(zoom))
	type acc struct {
		n              int
		sumLon, sumLat float64
		box            Box
		ready          bool
	}
	cells := map[[2]int]*acc{}
	var keys [][2]int
	for _, c := range candidates {
		x, y := webMercator(c.Point.Lon, c.Point.Lat, world)
		key := [2]int{int(math.Floor(x / clusterCellPx)), int(math.Floor(y / clusterCellPx))}
		cell := cells[key]
		if cell == nil {
			cell = &acc{}
			cells[key] = cell
			keys = append(keys, key)
		}
		cell.n++
		cell.sumLon += c.Point.Lon
		cell.sumLat += c.Point.Lat
		if !cell.ready {
			cell.box = c.Box
			cell.ready = true
		} else {
			cell.box = unionBox(cell.box, c.Box)
		}
	}
	out := make([]Cluster, 0, len(keys))
	for _, key := range keys {
		cell := cells[key]
		out = append(out, Cluster{
			Centroid: Point{
				Lon: cell.sumLon / float64(cell.n),
				Lat: cell.sumLat / float64(cell.n),
			},
			SiteCount: uint32(cell.n),
			Expansion: cell.box,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Centroid.Lon != out[j].Centroid.Lon {
			return out[i].Centroid.Lon < out[j].Centroid.Lon
		}
		if out[i].Centroid.Lat != out[j].Centroid.Lat {
			return out[i].Centroid.Lat < out[j].Centroid.Lat
		}
		return out[i].SiteCount < out[j].SiteCount
	})
	return out
}

func webMercator(lon, lat, world float64) (float64, float64) {
	const maxLat = 85.05112878
	if lat > maxLat {
		lat = maxLat
	} else if lat < -maxLat {
		lat = -maxLat
	}
	x := (lon + 180) / 360 * world
	s := math.Sin(lat * math.Pi / 180)
	y := (0.5 - math.Log((1+s)/(1-s))/(4*math.Pi)) * world
	return x, y
}

func countsFromMap(vals map[string]uint32) []Count {
	if len(vals) == 0 {
		return nil
	}
	keys := make([]string, 0, len(vals))
	for k := range vals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]Count, len(keys))
	for i, k := range keys {
		out[i] = Count{Value: k, Count: vals[k]}
	}
	return out
}
