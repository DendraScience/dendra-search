package search

import "github.com/RoaringBitmap/roaring"
import "github.com/tidwall/rtree"

// Shard is an immutable index. Query may run from many goroutines.
// Open inserts both R-trees. Query allocates temporary bitmaps only.
type Shard struct {
	sites       []Site
	siteAt      map[string]int
	forward     []forwardRow
	byDoc       [][]storedInterval
	publicOK    *roaring.Bitmap
	memberOK    *roaring.Bitmap
	purpose     map[string]*roaring.Bitmap
	measurement map[string]*roaring.Bitmap
	currentSite map[string]*roaring.Bitmap
	shown       rtree.RTreeG[int]
	exact       rtree.RTreeG[int]
	docCount    uint32
}

// Open unmarshals a shard and inserts each site's shown and exact box.
func Open(b []byte) (*Shard, error) {
	s, err := readShard(b)
	if err != nil {
		return nil, err
	}
	s.siteAt = make(map[string]int, len(s.sites))
	for i := range s.sites {
		id := s.sites[i].ID
		if id == "" {
			return nil, ErrFormat
		}
		if _, ok := s.siteAt[id]; ok {
			return nil, ErrFormat
		}
		s.siteAt[id] = i
		site := s.sites[i]
		s.shown.Insert(
			[2]float64{site.Shown.West, site.Shown.South},
			[2]float64{site.Shown.East, site.Shown.North},
			i,
		)
		s.exact.Insert(
			[2]float64{site.Exact.West, site.Exact.South},
			[2]float64{site.Exact.East, site.Exact.North},
			i,
		)
	}
	return s, nil
}

// DocCount is the number of datastreams in the shard.
func (s *Shard) DocCount() int {
	if s == nil {
		return 0
	}
	return int(s.docCount)
}
