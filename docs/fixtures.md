# Fixtures

`Generate` builds shard bytes from `FixtureSeed` (`1`). There is no network and no production coordinates. The same seed produces the same bytes.

| Name | Orgs | Sites | Datastreams |
| --- | ---: | ---: | ---: |
| `today-org` | 1 | 25 | 600 |
| `today-catalog` | 15 | 300 | 8,000 |
| `large-org` | 1 | 250 | 6,000 |
| `large-catalog` | 150 | 3,000 | 80,000 |

Organizations are contiguous ranges of sites and of datastreams, so a catalog walk is one shard at a time. Doc ids restart at 0 in each shard.

The California box is longitude −124.2…−114.1, latitude 32.5…42.0. The outlier count is `round(sites × 9/286)`. Those sites cycle nine fixed points, in this order: (−70.9, 43.7), (−71.1, 42.4), (−73.8, 42.7), (−114.0, 43.6), (−111.9, 40.8), (−123.1, 49.2), (−122.3, 47.6), (−110.0, 35.0), (−105.0, 40.0). Every other site is inside the box. In-box positions come from `math/rand` seeded with `FixtureSeed`.

About one site in twenty is a polygon about 0.05° across. The R-tree stores that box. Ten percent of sites, by index, shift shown by +0.1° in longitude and latitude. The first in-box site shifts shown by +1.0° instead, so a viewport on the exact point misses shown.

There are twelve measurement names, `Meas01` through `Meas12`. About 80% of datastreams are purpose `ReadytoUse`, and the rest are `Raw` or `StatusInformation`. About 10% are `member_ok` only (`public_ok` false). Each datastream has one site and one to three non-overlapping intervals. The last interval is open. Indexes divisible by 5 or 7 leave a gap, so a window from 10s to 20s after the epoch `1_700_000_000_000` excludes them. Indexes divisible by 11 are site-attached. The others use a station.

## Query cases

`BenchCases` builds the matrix. Catalog fixtures run each case on every shard; the reported time is the sum.

1. World viewport, zoom 2, no measurement. Shown geometry.
2. California box, zoom 8, measurement `Meas01`.
3. A 0.2° window centered on (−120.3, 39.4), zoom 11, page size 50.
4. A viewport 0.05° around the +1.0° site, once with `exact` and once with `shown`.

Today-scale gates live in `go test`: each of those queries under 10 ms, and heap growth across `Open` of every `today-catalog` shard under 32 MB. Queries run with `GOMAXPROCS=1`. `Finish` time is printed in `BENCHMARKS.md` and is not a gate. Large-scale numbers are recorded there next to 100 ms and 512 MB.
