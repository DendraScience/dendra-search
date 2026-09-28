// Package search is the datastream search index.
//
// A builder takes sites, then stations, then datastreams, and Finish writes
// one organization's shard. Open loads those bytes into an immutable shard
// and builds the shown and exact R-trees. Query reads the shard. Combine
// merges one result per organization and clusters the sites once.
//
// The caller chooses the geometry name and whether member-only datastreams
// are included. This package does not open sockets, and it does not decide
// who may see a site.
package search
