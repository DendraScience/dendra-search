# Benchmarks

Recorded by `go run ./examples/bench`. Finish time is not a gate.

Queries use GOMAXPROCS=1. Each case is warmed up once, then timed. Catalog times are the sum of per-organization Query calls. Heap is HeapAlloc after GC with the opened shards retained, minus HeapAlloc after GC before Open (the bytes are already allocated).

- Machine: Newleaf-studio.local (darwin/arm64)
- Go: go1.26.5-X:jsonv2
- When: 2026-09-28T20:39:14Z

## today-org

- Organizations: 1, sites: 25, datastreams: 600
- Finish: 123.041µs
- Shard bytes: 92472
- Heap after Open: 199960 bytes (0.19 MiB)
- Design figure: 10 ms per query, 32 MiB heap

| Case | Query sum |
| --- | --- |
| world-z2 | 24.875µs |
| california-z8 | 22.875µs |
| window-z11 | 2.417µs |
| shifted-exact | 6.292µs |
| shifted-shown | 2.084µs |

## today-catalog

- Organizations: 15, sites: 300, datastreams: 8000
- Finish: 1.404875ms
- Shard bytes: 1228179
- Heap after Open: 2674256 bytes (2.55 MiB)
- Design figure: 10 ms per query, 32 MiB heap

| Case | Query sum |
| --- | --- |
| world-z2 | 324.083µs |
| california-z8 | 283.25µs |
| window-z11 | 32.542µs |
| shifted-exact | 107.5µs |
| shifted-shown | 34.208µs |

## large-org

- Organizations: 1, sites: 250, datastreams: 6000
- Finish: 1.599417ms
- Shard bytes: 920183
- Heap after Open: 1938968 bytes (1.85 MiB)
- Design figure: 100 ms per query, 512 MiB heap

| Case | Query sum |
| --- | --- |
| world-z2 | 529.041µs |
| california-z8 | 374.875µs |
| window-z11 | 8µs |
| shifted-exact | 39.542µs |
| shifted-shown | 8.041µs |

## large-catalog

- Organizations: 150, sites: 3000, datastreams: 80000
- Finish: 10.126957ms
- Shard bytes: 12281936
- Heap after Open: 26740824 bytes (25.50 MiB)
- Design figure: 100 ms per query, 512 MiB heap

| Case | Query sum |
| --- | --- |
| world-z2 | 2.740958ms |
| california-z8 | 2.672709ms |
| window-z11 | 298.458µs |
| shifted-exact | 290.791µs |
| shifted-shown | 290.625µs |

Builder duration above is Finish only, summed across organizations. It is not a pass/fail number.
