package search

import (
	"runtime"
	"testing"
	"time"
)

func TestTodayGates(t *testing.T) {
	prev := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(prev)

	org := mustFixture(t, TodayOrg)
	catalog := mustFixture(t, TodayCatalog)
	orgShards := openAll(t, org.Blobs)

	heap, catalogShards := heapAfterOpen(t, catalog.Blobs)
	t.Logf("today-catalog heap %d bytes", heap)
	if heap > 32<<20 {
		t.Fatalf("today-catalog heap %d exceeds 32MB", heap)
	}

	timeCases(t, "today-org", orgShards, org)
	timeCases(t, "today-catalog", catalogShards, catalog)
}

func timeCases(t *testing.T, name string, shards []*Shard, fix Fixture) {
	t.Helper()
	for _, c := range BenchCases(fix) {
		for _, shard := range shards {
			if _, err := shard.Query(c.Query); err != nil {
				t.Fatal(err)
			}
		}
		start := time.Now()
		for _, shard := range shards {
			if _, err := shard.Query(c.Query); err != nil {
				t.Fatal(err)
			}
		}
		elapsed := time.Since(start)
		t.Logf("%s %s %s", name, c.Name, elapsed)
		if elapsed > 10*time.Millisecond {
			t.Fatalf("%s %s took %s, limit is 10ms", name, c.Name, elapsed)
		}
	}
}

func heapAfterOpen(t *testing.T, blobs [][]byte) (uint64, []*Shard) {
	t.Helper()
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	shards := openAll(t, blobs)
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	if after.HeapAlloc < before.HeapAlloc {
		return 0, shards
	}
	return after.HeapAlloc - before.HeapAlloc, shards
}
