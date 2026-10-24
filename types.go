package search

// Geometry names stored on every site. The caller picks one per query.
const (
	GeometryShown = "shown"
	GeometryExact = "exact"
)

// Defaults when a query leaves the map limits at zero.
const (
	DefaultMapSiteLimit uint32 = 100
	DefaultMinSiteZoom  uint32 = 6
)

// Purpose values used by the fixtures. Query treats purpose as an ordinary string.
const (
	PurposeReadyToUse        = "ReadytoUse"
	PurposeRaw               = "Raw"
	PurposeStatusInformation = "StatusInformation"
)

// Box is a longitude/latitude rectangle. West and South are the minimums,
// East and North the maximums. A point is a box whose corners are equal.
// A viewport may wrap the antimeridian (East < West). A site box may not.
type Box struct {
	West, South, East, North float64
}

// Point is a WGS84 digest point. Clustering uses this, not a centroid
// substituted for an area box.
type Point struct {
	Lon, Lat float64
}

// Site is one place in an organization shard. OrganizationID is required.
// Shown and Exact are both stored. Open inserts each box into its own R-tree.
type Site struct {
	ID               string
	Name             string
	OrganizationID   string
	OrganizationName string
	Shown            Box
	Exact            Box
	ShownPoint       Point
	ExactPoint       Point
}

// Interval is a half-open deployment [BeginsAt, EndsBefore) in Unix milliseconds.
// EndsBefore == 0 means the interval is still open, the same as an unset
// ends_before on the wire. A non-zero EndsBefore means the deployment has
// ended. A datastream has at most one open interval. Every interval may be
// closed. GroupID is optional.
// Empty means the interval has no group. The builder stores the id and does
// not look up a name.
type Interval struct {
	SiteID     string
	BeginsAt   int64
	EndsBefore int64
	GroupID    string
}

// Station holds deployment intervals for station-attached datastreams.
// OrganizationID is required and must match the builder.
type Station struct {
	ID             string
	Name           string
	OrganizationID string
	Intervals      []Interval
}

// Datastream is one searchable row. Set StationID, or set Intervals for a
// site-attached datastream, not both. OrganizationID is required and must match
// the builder. Doc ids are assigned in AddDatastream order.
type Datastream struct {
	ID             string
	Name           string
	OrganizationID string
	StationID      string
	UnitName       string
	Measurement    string
	Purpose        string
	PublicOK       bool
	MemberOK       bool
	// FieldNames are the datastream output field names, in display order.
	// Empty means the datastream has none. The builder copies the slice.
	FieldNames []string
	Intervals  []Interval
}

// Kind is the map layer for a result.
type Kind int

const (
	KindClusters Kind = 1
	KindSites    Kind = 2
)

func (k Kind) String() string {
	switch k {
	case KindClusters:
		return "clusters"
	case KindSites:
		return "sites"
	default:
		return "unspecified"
	}
}

// Query is one read of a shard. Empty slices and a nil viewport leave that
// predicate out. Both time bounds zero means the open deployment.
type Query struct {
	Purpose      string
	Measurements []string
	SiteIDs      []string
	// GroupIDs filters intervals. Empty means every datastream, including
	// intervals with no group. Several ids are a union: a datastream matches
	// when one qualifying interval carries any listed id.
	GroupIDs          []string
	BeginsAt          int64
	EndsBefore        int64
	Viewport          *Box
	Geometry          string
	Zoom              uint32
	IncludeMemberOnly bool
	MapSiteLimit      uint32
	MinSiteZoom       uint32
	// AllHits returns every matching hit when the layer is sites, and no hit
	// rows when the layer is clusters. TotalCount is still set. PageSize and
	// PageAfter are ignored. HasMore is false.
	AllHits bool
	// PageSize is the maximum number of hits to return. Zero returns none.
	// TotalCount is still set. Ignored when AllHits is set.
	PageSize uint32
	// PageAfter is the first doc id to return, inclusive. Zero starts at the
	// beginning. Pass Result.NextPageAfter only when Result.HasMore is set.
	// The cursor is valid only for the byte slice this shard was opened from.
	PageAfter uint32
}

// Count is one disjunctive facet row.
// Name and organization fields are set on site counts (from the Site
// record) so a host can label zero-count rows that never appear as map
// markers. Measurement counts leave them empty.
type Count struct {
	Value            string
	Count            uint32
	Name             string
	OrganizationID   string
	OrganizationName string
}

// Cluster is a web-mercator cell of candidate sites.
type Cluster struct {
	Centroid  Point
	SiteCount uint32
	Expansion Box
}

// SiteMarker is one candidate site in the geometry the caller named.
type SiteMarker struct {
	SiteID           string
	Name             string
	OrganizationID   string
	OrganizationName string
	Point            Point
	Box              Box
	DatastreamCount  uint32
}

// Hit is one matching datastream in doc id order.
type Hit struct {
	DocID            uint32
	DatastreamID     string
	Name             string
	OrganizationID   string
	OrganizationName string
	StationID        string
	StationName      string
	SiteID           string
	SiteName         string
	UnitName         string
	Measurement      string
	Purpose          string
	FieldNames       []string
}

// Result is the map layer for one shard, or for Combine.
// Candidates is the filtered site list before the cluster/site switch.
// Combine uses it. Measurement counts are always set. Site counts are set
// only for KindSites.
type Result struct {
	Kind              Kind
	Clusters          []Cluster
	Sites             []SiteMarker
	Candidates        []SiteMarker
	SiteTotal         uint32
	Bounds            Box
	SiteCounts        []Count
	MeasurementCounts []Count
	Hits              []Hit
	TotalCount        uint32
	NextPageAfter     uint32
	HasMore           bool
}
