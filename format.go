package search

import (
	"bytes"
	"encoding/binary"
	"math"
	"sort"

	"github.com/RoaringBitmap/roaring"
)

const (
	formatVersion uint16 = 2
	maxField             = 64 << 20
)

var formatMagic = [4]byte{'D', 'S', '0', '1'}

type storedInterval struct {
	doc    uint32
	site   int
	begins int64
	ends   int64
	group  string
}

type forwardRow struct {
	datastreamID   string
	name           string
	stationID      string
	stationName    string
	unit           string
	measurement    string
	purpose        string
	organizationID string
	openSite       int
}

type shardImage struct {
	sites       []Site
	intervals   []storedInterval
	forward     []forwardRow
	purpose     map[string]*roaring.Bitmap
	measurement map[string]*roaring.Bitmap
	currentSite map[string]*roaring.Bitmap
	openGroup   map[string]*roaring.Bitmap
	publicOK    *roaring.Bitmap
	memberOK    *roaring.Bitmap
}

func writeShard(img shardImage) ([]byte, error) {
	var e enc
	e.b = append(e.b, formatMagic[:]...)
	e.u16(formatVersion)
	e.u32(uint32(len(img.forward)))
	e.u32(uint32(len(img.sites)))
	for i := range img.sites {
		s := img.sites[i]
		e.str(s.ID)
		e.str(s.Name)
		e.str(s.OrganizationID)
		e.str(s.OrganizationName)
		e.box(s.Shown)
		e.box(s.Exact)
		e.point(s.ShownPoint)
		e.point(s.ExactPoint)
	}
	e.u32(uint32(len(img.intervals)))
	for _, iv := range img.intervals {
		e.u32(iv.doc)
		e.u32(uint32(iv.site))
		e.i64(iv.begins)
		e.i64(iv.ends)
		e.str(iv.group)
	}
	for _, row := range img.forward {
		e.str(row.datastreamID)
		e.str(row.name)
		e.str(row.stationID)
		e.str(row.stationName)
		e.str(row.unit)
		e.str(row.measurement)
		e.str(row.purpose)
		e.str(row.organizationID)
		e.u32(uint32(row.openSite))
	}
	if err := e.bitmaps(img.purpose); err != nil {
		return nil, err
	}
	if err := e.bitmaps(img.measurement); err != nil {
		return nil, err
	}
	if err := e.bitmap(img.publicOK); err != nil {
		return nil, err
	}
	if err := e.bitmap(img.memberOK); err != nil {
		return nil, err
	}
	if err := e.bitmaps(img.currentSite); err != nil {
		return nil, err
	}
	if err := e.bitmaps(img.openGroup); err != nil {
		return nil, err
	}
	return e.b, nil
}

// writePublicShard writes a DS01 shard of the public_ok rows in img.
// Exact boxes and points in the file are copies of the shown values.
// Doc ids and site indexes are packed in their original order, and a site
// is stored only when a public row uses it. Postings are rebuilt from those
// rows. img is not modified.
//
// This can only be done after all datastreams are added. The compact site
// indexes depend on the full set of public rows, so they are assigned while
// writing. Leaving img intact lets Finish run again.
func writePublicShard(img shardImage) ([]byte, uint32, error) {
	pubDoc := make([]uint32, len(img.forward))
	isPublic := make([]bool, len(img.forward))
	var publicDocs uint32
	for i := range img.forward {
		if img.publicOK != nil && img.publicOK.Contains(uint32(i)) {
			pubDoc[i] = publicDocs
			isPublic[i] = true
			publicDocs++
		}
	}
	publicSite := make([]bool, len(img.sites))
	var publicIntervals uint32
	var publicSites uint32
	for _, iv := range img.intervals {
		if int(iv.doc) >= len(isPublic) || !isPublic[iv.doc] {
			continue
		}
		publicIntervals++
		if iv.site < 0 || iv.site >= len(publicSite) {
			return nil, 0, ErrFormat
		}
		if !publicSite[iv.site] {
			publicSite[iv.site] = true
			publicSites++
		}
	}
	siteRemap := make([]int, len(img.sites))
	for i := range siteRemap {
		siteRemap[i] = -1
	}

	var e enc
	e.b = append(e.b, formatMagic[:]...)
	e.u16(formatVersion)
	e.u32(publicDocs)
	e.u32(publicSites)
	var next uint32
	for i := range img.sites {
		if !publicSite[i] {
			continue
		}
		siteRemap[i] = int(next)
		next++
		s := img.sites[i]
		e.str(s.ID)
		e.str(s.Name)
		e.str(s.OrganizationID)
		e.str(s.OrganizationName)
		e.box(s.Shown)
		e.box(s.Shown)
		e.point(s.ShownPoint)
		e.point(s.ShownPoint)
	}
	e.u32(publicIntervals)
	for _, iv := range img.intervals {
		if int(iv.doc) >= len(isPublic) || !isPublic[iv.doc] {
			continue
		}
		site := siteRemap[iv.site]
		if site < 0 {
			return nil, 0, ErrFormat
		}
		e.u32(pubDoc[iv.doc])
		e.u32(uint32(site))
		e.i64(iv.begins)
		e.i64(iv.ends)
		e.str(iv.group)
	}
	openGroupByDoc := make([]string, len(img.forward))
	for _, iv := range img.intervals {
		if iv.ends == 0 && int(iv.doc) < len(openGroupByDoc) {
			openGroupByDoc[iv.doc] = iv.group
		}
	}
	purpose := map[string]*roaring.Bitmap{}
	measurement := map[string]*roaring.Bitmap{}
	currentSite := map[string]*roaring.Bitmap{}
	openGroup := map[string]*roaring.Bitmap{}
	memberOK := roaring.New()
	for i, row := range img.forward {
		if !isPublic[i] {
			continue
		}
		if row.openSite < 0 || row.openSite >= len(siteRemap) || siteRemap[row.openSite] < 0 {
			return nil, 0, ErrFormat
		}
		doc := pubDoc[i]
		e.str(row.datastreamID)
		e.str(row.name)
		e.str(row.stationID)
		e.str(row.stationName)
		e.str(row.unit)
		e.str(row.measurement)
		e.str(row.purpose)
		e.str(row.organizationID)
		e.u32(uint32(siteRemap[row.openSite]))
		if row.purpose != "" {
			addPosting(purpose, row.purpose, doc)
		}
		if row.measurement != "" {
			addPosting(measurement, row.measurement, doc)
		}
		if img.memberOK != nil && img.memberOK.Contains(uint32(i)) {
			memberOK.Add(doc)
		}
		addPosting(currentSite, img.sites[row.openSite].ID, doc)
		if g := openGroupByDoc[i]; g != "" {
			addPosting(openGroup, g, doc)
		}
	}
	publicOK := roaring.New()
	if publicDocs > 0 {
		publicOK.AddRange(0, uint64(publicDocs))
	}
	if err := e.bitmaps(purpose); err != nil {
		return nil, 0, err
	}
	if err := e.bitmaps(measurement); err != nil {
		return nil, 0, err
	}
	if err := e.bitmap(publicOK); err != nil {
		return nil, 0, err
	}
	if err := e.bitmap(memberOK); err != nil {
		return nil, 0, err
	}
	if err := e.bitmaps(currentSite); err != nil {
		return nil, 0, err
	}
	if err := e.bitmaps(openGroup); err != nil {
		return nil, 0, err
	}
	return e.b, publicDocs, nil
}

func readShard(raw []byte) (*Shard, error) {
	d := dec{b: raw}
	magic, err := d.bytes(4)
	if err != nil || !bytes.Equal(magic, formatMagic[:]) {
		return nil, ErrFormat
	}
	ver, err := d.u16()
	if err != nil || ver != formatVersion {
		return nil, ErrFormat
	}
	docCount, err := d.u32()
	if err != nil {
		return nil, err
	}
	siteCount, err := d.u32()
	if err != nil {
		return nil, err
	}
	s := &Shard{
		sites:       make([]Site, siteCount),
		forward:     make([]forwardRow, docCount),
		byDoc:       make([][]storedInterval, docCount),
		purpose:     map[string]*roaring.Bitmap{},
		measurement: map[string]*roaring.Bitmap{},
		currentSite: map[string]*roaring.Bitmap{},
		openGroup:   map[string]*roaring.Bitmap{},
		docCount:    docCount,
	}
	for i := range s.sites {
		site, err := d.site()
		if err != nil {
			return nil, err
		}
		s.sites[i] = site
	}
	nInterval, err := d.u32()
	if err != nil {
		return nil, err
	}
	for i := uint32(0); i < nInterval; i++ {
		iv, err := d.interval(docCount, siteCount)
		if err != nil {
			return nil, err
		}
		s.byDoc[iv.doc] = append(s.byDoc[iv.doc], iv)
	}
	for i := range s.forward {
		row, err := d.forward(siteCount)
		if err != nil {
			return nil, err
		}
		s.forward[i] = row
	}
	s.purpose, err = d.bitmapMap()
	if err != nil {
		return nil, err
	}
	s.measurement, err = d.bitmapMap()
	if err != nil {
		return nil, err
	}
	s.publicOK, err = d.bitmap()
	if err != nil {
		return nil, err
	}
	s.memberOK, err = d.bitmap()
	if err != nil {
		return nil, err
	}
	s.currentSite, err = d.bitmapMap()
	if err != nil {
		return nil, err
	}
	s.openGroup, err = d.bitmapMap()
	if err != nil {
		return nil, err
	}
	if d.i != len(d.b) {
		return nil, ErrFormat
	}
	return s, nil
}

type enc struct {
	b []byte
}

func (e *enc) u16(v uint16) {
	e.b = binary.LittleEndian.AppendUint16(e.b, v)
}

func (e *enc) u32(v uint32) {
	e.b = binary.LittleEndian.AppendUint32(e.b, v)
}

func (e *enc) i64(v int64) {
	e.b = binary.LittleEndian.AppendUint64(e.b, uint64(v))
}

func (e *enc) f64(v float64) {
	e.b = binary.LittleEndian.AppendUint64(e.b, math.Float64bits(v))
}

func (e *enc) str(s string) {
	e.u32(uint32(len(s)))
	e.b = append(e.b, s...)
}

func (e *enc) box(b Box) {
	e.f64(b.West)
	e.f64(b.South)
	e.f64(b.East)
	e.f64(b.North)
}

func (e *enc) point(p Point) {
	e.f64(p.Lon)
	e.f64(p.Lat)
}

func (e *enc) bitmap(bm *roaring.Bitmap) error {
	if bm == nil {
		bm = roaring.New()
	} else {
		// RunOptimize mutates. Copy so a second Finish sees the same bits.
		bm = bm.Clone()
	}
	bm.RunOptimize()
	var buf bytes.Buffer
	if _, err := bm.WriteTo(&buf); err != nil {
		return err
	}
	if buf.Len() > maxField {
		return ErrFormat
	}
	e.u32(uint32(buf.Len()))
	e.b = append(e.b, buf.Bytes()...)
	return nil
}

func (e *enc) bitmaps(m map[string]*roaring.Bitmap) error {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	e.u32(uint32(len(keys)))
	for _, k := range keys {
		e.str(k)
		if err := e.bitmap(m[k]); err != nil {
			return err
		}
	}
	return nil
}

type dec struct {
	b []byte
	i int
}

func (d *dec) need(n int) error {
	if n < 0 || d.i+n > len(d.b) {
		return ErrFormat
	}
	return nil
}

func (d *dec) bytes(n int) ([]byte, error) {
	if n > maxField {
		return nil, ErrFormat
	}
	if err := d.need(n); err != nil {
		return nil, err
	}
	p := d.b[d.i : d.i+n]
	d.i += n
	return p, nil
}

func (d *dec) u16() (uint16, error) {
	if err := d.need(2); err != nil {
		return 0, err
	}
	v := binary.LittleEndian.Uint16(d.b[d.i:])
	d.i += 2
	return v, nil
}

func (d *dec) u32() (uint32, error) {
	if err := d.need(4); err != nil {
		return 0, err
	}
	v := binary.LittleEndian.Uint32(d.b[d.i:])
	d.i += 4
	return v, nil
}

func (d *dec) i64() (int64, error) {
	if err := d.need(8); err != nil {
		return 0, err
	}
	v := binary.LittleEndian.Uint64(d.b[d.i:])
	d.i += 8
	return int64(v), nil
}

func (d *dec) f64() (float64, error) {
	if err := d.need(8); err != nil {
		return 0, err
	}
	v := binary.LittleEndian.Uint64(d.b[d.i:])
	d.i += 8
	return math.Float64frombits(v), nil
}

func (d *dec) str() (string, error) {
	n, err := d.u32()
	if err != nil {
		return "", err
	}
	p, err := d.bytes(int(n))
	if err != nil {
		return "", err
	}
	return string(p), nil
}

func (d *dec) box() (Box, error) {
	var b Box
	var err error
	if b.West, err = d.f64(); err != nil {
		return Box{}, err
	}
	if b.South, err = d.f64(); err != nil {
		return Box{}, err
	}
	if b.East, err = d.f64(); err != nil {
		return Box{}, err
	}
	if b.North, err = d.f64(); err != nil {
		return Box{}, err
	}
	return b, nil
}

func (d *dec) point() (Point, error) {
	var p Point
	var err error
	if p.Lon, err = d.f64(); err != nil {
		return Point{}, err
	}
	if p.Lat, err = d.f64(); err != nil {
		return Point{}, err
	}
	return p, nil
}

func (d *dec) site() (Site, error) {
	var s Site
	var err error
	if s.ID, err = d.str(); err != nil {
		return Site{}, err
	}
	if s.Name, err = d.str(); err != nil {
		return Site{}, err
	}
	if s.OrganizationID, err = d.str(); err != nil {
		return Site{}, err
	}
	if s.OrganizationName, err = d.str(); err != nil {
		return Site{}, err
	}
	if s.Shown, err = d.box(); err != nil {
		return Site{}, err
	}
	if s.Exact, err = d.box(); err != nil {
		return Site{}, err
	}
	if s.ShownPoint, err = d.point(); err != nil {
		return Site{}, err
	}
	if s.ExactPoint, err = d.point(); err != nil {
		return Site{}, err
	}
	return s, nil
}

func (d *dec) interval(docCount, siteCount uint32) (storedInterval, error) {
	doc, err := d.u32()
	if err != nil {
		return storedInterval{}, err
	}
	site, err := d.u32()
	if err != nil {
		return storedInterval{}, err
	}
	begins, err := d.i64()
	if err != nil {
		return storedInterval{}, err
	}
	ends, err := d.i64()
	if err != nil {
		return storedInterval{}, err
	}
	group, err := d.str()
	if err != nil {
		return storedInterval{}, err
	}
	if doc >= docCount || site >= siteCount {
		return storedInterval{}, ErrFormat
	}
	return storedInterval{doc: doc, site: int(site), begins: begins, ends: ends, group: group}, nil
}

func (d *dec) forward(siteCount uint32) (forwardRow, error) {
	var row forwardRow
	var err error
	if row.datastreamID, err = d.str(); err != nil {
		return forwardRow{}, err
	}
	if row.name, err = d.str(); err != nil {
		return forwardRow{}, err
	}
	if row.stationID, err = d.str(); err != nil {
		return forwardRow{}, err
	}
	if row.stationName, err = d.str(); err != nil {
		return forwardRow{}, err
	}
	if row.unit, err = d.str(); err != nil {
		return forwardRow{}, err
	}
	if row.measurement, err = d.str(); err != nil {
		return forwardRow{}, err
	}
	if row.purpose, err = d.str(); err != nil {
		return forwardRow{}, err
	}
	if row.organizationID, err = d.str(); err != nil {
		return forwardRow{}, err
	}
	open, err := d.u32()
	if err != nil {
		return forwardRow{}, err
	}
	if siteCount == 0 || open >= siteCount {
		return forwardRow{}, ErrFormat
	}
	row.openSite = int(open)
	return row, nil
}

func (d *dec) bitmap() (*roaring.Bitmap, error) {
	n, err := d.u32()
	if err != nil {
		return nil, err
	}
	raw, err := d.bytes(int(n))
	if err != nil {
		return nil, err
	}
	bm := roaring.New()
	read, err := bm.ReadFrom(bytes.NewReader(raw))
	if err != nil || read != int64(len(raw)) {
		return nil, ErrFormat
	}
	return bm, nil
}

func (d *dec) bitmapMap() (map[string]*roaring.Bitmap, error) {
	n, err := d.u32()
	if err != nil {
		return nil, err
	}
	m := make(map[string]*roaring.Bitmap, n)
	for i := uint32(0); i < n; i++ {
		k, err := d.str()
		if err != nil {
			return nil, err
		}
		bm, err := d.bitmap()
		if err != nil {
			return nil, err
		}
		m[k] = bm
	}
	return m, nil
}
