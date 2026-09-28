// Command bench runs the fixture matrix and writes BENCHMARKS.md.
// Finish time is printed and is not a gate.
//
//	go run ./examples/bench
//	go run ./examples/bench -out BENCHMARKS.md
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/dendrascience/dendra-search"
)

func main() {
	outPath := flag.String("out", "BENCHMARKS.md", "markdown file to write")
	flag.Parse()
	runtime.GOMAXPROCS(1)

	type gen func() (search.Fixture, error)
	fixtures := []gen{search.TodayOrg, search.TodayCatalog, search.LargeOrg, search.LargeCatalog}

	host, _ := os.Hostname()
	text := fmt.Sprintf("# Benchmarks\n\nRecorded by `go run ./examples/bench`. Finish time is not a gate.\n\nQueries use GOMAXPROCS=1. Each case is warmed up once, then timed. Catalog times are the sum of per-organization Query calls. Heap is HeapAlloc after GC with the opened shards retained, minus HeapAlloc after GC before Open (the bytes are already allocated).\n\n- Machine: %s (%s/%s)\n- Go: %s\n- When: %s\n\n",
		host, runtime.GOOS, runtime.GOARCH, runtime.Version(), time.Now().UTC().Format(time.RFC3339))

	for _, g := range fixtures {
		fix, err := g()
		if err != nil {
			fatal(err)
		}
		var bytesSum int
		for _, blob := range fix.Blobs {
			bytesSum += len(blob)
		}
		heap, shards, err := residentHeap(fix.Blobs)
		if err != nil {
			fatal(err)
		}
		text += fmt.Sprintf("## %s\n\n", fix.Name)
		text += fmt.Sprintf("- Organizations: %d, sites: %d, datastreams: %d\n", fix.Orgs, fix.Sites, fix.Datastreams)
		text += fmt.Sprintf("- Finish: %s\n", fix.Finish)
		text += fmt.Sprintf("- Shard bytes: %d\n", bytesSum)
		text += fmt.Sprintf("- Heap after Open: %d bytes (%.2f MiB)\n", heap, float64(heap)/(1<<20))
		limit := "10 ms per query, 32 MiB heap"
		if fix.Name == "large-org" || fix.Name == "large-catalog" {
			limit = "100 ms per query, 512 MiB heap"
		}
		text += fmt.Sprintf("- Design figure: %s\n\n", limit)
		text += "| Case | Query sum |\n| --- | --- |\n"
		fmt.Printf("%s  finish %s  bytes %d  heap %.2f MiB\n", fix.Name, fix.Finish, bytesSum, float64(heap)/(1<<20))
		for _, c := range search.BenchCases(fix) {
			elapsed, err := timeCase(shards, c.Query)
			if err != nil {
				fatal(err)
			}
			text += fmt.Sprintf("| %s | %s |\n", c.Name, elapsed)
			fmt.Printf("  %s  %s\n", c.Name, elapsed)
		}
		text += "\n"
		shards = nil
		runtime.GC()
	}

	text += "Builder duration above is Finish only, summed across organizations. It is not a pass/fail number.\n"
	if err := os.WriteFile(*outPath, []byte(text), 0o644); err != nil {
		fatal(err)
	}
	fmt.Printf("wrote %s\n", *outPath)
}

func timeCase(shards []*search.Shard, q search.Query) (time.Duration, error) {
	for _, shard := range shards {
		if _, err := shard.Query(q); err != nil {
			return 0, err
		}
	}
	start := time.Now()
	for _, shard := range shards {
		if _, err := shard.Query(q); err != nil {
			return 0, err
		}
	}
	return time.Since(start), nil
}

func residentHeap(blobs [][]byte) (uint64, []*search.Shard, error) {
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	shards := make([]*search.Shard, len(blobs))
	for i, blob := range blobs {
		shard, err := search.Open(blob)
		if err != nil {
			return 0, nil, err
		}
		shards[i] = shard
	}
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	if after.HeapAlloc < before.HeapAlloc {
		return 0, shards, nil
	}
	return after.HeapAlloc - before.HeapAlloc, shards, nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
