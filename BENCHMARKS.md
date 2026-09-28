# Benchmarks

Recorded by `go run ./examples/bench`. Finish time is not a gate.

Queries use GOMAXPROCS=1. Each case is warmed up once, then timed. Catalog times are the sum of per-organization Query calls. Heap is HeapAlloc after GC with the opened shards retained, minus HeapAlloc after GC before Open (the bytes are already allocated).

- Machine: Newleaf-studio.local (darwin/arm64)
- Go: go1.26.5-X:jsonv2
- When: 2026-09-28T00:18:35Z

## today-org

- Organizations: 1, sites: 25, datastreams: 600
- Finish: 270.833µs
- Shard bytes: 92472
- Heap after Open: 199960 bytes (0.19 MiB)
- Design figure: 10 ms per query, 32 MiB heap

| Case | Query sum |
| --- | --- |
| world-z2 | 25.708µs |
| california-z8 | 24.25µs |
| window-z11 | 2.292µs |
| shifted-exact | 6.25µs |
| shifted-shown | 2.041µs |

## today-catalog

- Organizations: 15, sites: 300, datastreams: 8000
- Finish: 3.771333ms
- Shard bytes: 1228179
- Heap after Open: 2673832 bytes (2.55 MiB)
- Design figure: 10 ms per query, 32 MiB heap

| Case | Query sum |
| --- | --- |
| world-z2 | 363.291µs |
| california-z8 | 290.042µs |
| window-z11 | 32.333µs |
| shifted-exact | 42.75µs |
| shifted-shown | 35.208µs |

## large-org

- Organizations: 1, sites: 250, datastreams: 6000
- Finish: 4.238333ms
- Shard bytes: 920183
- Heap after Open: 1939016 bytes (1.85 MiB)
- Design figure: 100 ms per query, 512 MiB heap

| Case | Query sum |
| --- | --- |
| world-z2 | 595.875µs |
| california-z8 | 427µs |
| window-z11 | 12.542µs |
| shifted-exact | 48.959µs |
| shifted-shown | 8.875µs |

## large-catalog

- Organizations: 150, sites: 3000, datastreams: 80000
- Finish: 58.781913ms
- Shard bytes: 12281936
- Heap after Open: 26740352 bytes (25.50 MiB)
- Design figure: 100 ms per query, 512 MiB heap

| Case | Query sum |
| --- | --- |
| world-z2 | 3.199625ms |
| california-z8 | 2.713167ms |
| window-z11 | 352.458µs |
| shifted-exact | 466.875µs |
| shifted-shown | 364.209µs |

Builder duration above is Finish only, summed across organizations. It is not a pass/fail number.
