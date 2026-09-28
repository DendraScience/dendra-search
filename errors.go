package search

import "errors"

var (
	// ErrOrder means a site, station, or datastream was added out of feed order.
	// Sites come first, then stations, then datastreams.
	ErrOrder = errors.New("search: sites, then stations, then datastreams")

	// ErrFinished means the builder has already returned its bytes.
	ErrFinished = errors.New("search: builder is finished")

	// ErrDuplicate means a site, station, or datastream id was added twice.
	ErrDuplicate = errors.New("search: duplicate id")

	// ErrNotFound means a datastream names a site or station that was not added.
	ErrNotFound = errors.New("search: unknown site or station")

	// ErrInterval means a deployment interval is empty, overlapping, or missing
	// its single open end.
	ErrInterval = errors.New("search: invalid interval")

	// ErrDatastream means a datastream has no place, or both a station and its own intervals.
	ErrDatastream = errors.New("search: invalid datastream")

	// ErrBox means a site box or digest point is outside WGS84 or wraps the antimeridian.
	// Split a wrapping site before AddSite.
	ErrBox = errors.New("search: invalid box")

	// ErrGeometry means Query.Geometry is not "shown" or "exact".
	ErrGeometry = errors.New("search: geometry must be shown or exact")

	// ErrViewport means a viewport is outside WGS84 or has south above north.
	ErrViewport = errors.New("search: invalid viewport")

	// ErrWindow means only one time bound was set, or the end is not after the start.
	ErrWindow = errors.New("search: invalid time window")

	// ErrFormat means the bytes are not a shard this package can open.
	ErrFormat = errors.New("search: invalid shard")

	// ErrInvalid means a required id is empty, or the receiver is nil.
	ErrInvalid = errors.New("search: invalid value")
)
