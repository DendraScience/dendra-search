// Package search is the datastream search index.
//
// A builder takes sites, then stations, then datastreams for one organization.
// The first record sets OrganizationID. Later records must use that same id.
// Finish writes that organization's shard. FinishPrivate keeps
// shown and exact geometry and every datastream. FinishPublic copies shown
// into the exact fields and keeps public_ok datastreams only. The index stays
// in place, so Finish may be called again with the other mode. Open loads those bytes into an
// immutable shard and builds the shown and exact R-trees. Query reads the
// shard. Combine merges one result per organization and clusters the sites once.
// An interval may carry a group id. Query.GroupIDs filters on that id.
// GroupIDs lists the ids on intervals in play. ReducePicker turns organization
// and group catalogs, the checked ids, and those lists into the picker rows.
//
// WriteContainer and ParseContainer frame public shards in an uncompressed
// DSPC container. VersionFile marshals the current.json pointer. An empty
// Encoding means the object is raw. This package does not compress.
//
// The caller chooses the geometry name and whether member-only datastreams
// are included. This package does not open sockets, and it does not decide
// who may see a site.
package search
