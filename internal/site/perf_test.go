package site

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
)

func TestBuildIndexMemoryBudget(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 300; i++ {
		writePage(t, root, filepath.Join("posts", fmt.Sprintf("m-%04d", i), "index.html"), fmt.Sprintf(`<!doctype html><html lang="en"><head><title>Post %04d</title><meta name="description" content="Post %04d"><link rel="canonical" href="https://example.test/posts/m-%04d/"><meta property="article:published_time" content="2026-02-%02dT03:04:05Z"><meta property="article:tag" content="Bulk"></head><body>Body %04d</body></html>`, i, i, i, (i%28)+1, i))
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	idx, err := BuildIndex(context.Background(), root, Config{SiteURL: "https://example.test", DefaultLanguage: "en", RejectHiddenPath: true, RejectSymlinks: true, MaxIndexEntries: 5000})
	if err != nil {
		t.Fatalf("BuildIndex() error = %v", err)
	}
	_ = idx
	runtime.GC()
	runtime.ReadMemStats(&after)
	used := int64(after.Alloc - before.Alloc)
	if used < 0 {
		used = 0
	}
	if used > 128<<20 {
		t.Fatalf("BuildIndex() memory delta too high: %d bytes", used)
	}
}

func BenchmarkBuildIndexSmall(b *testing.B) {
	root := filepath.Join("..", "..", "testdata", "fixtures", "public", "minimal")
	cfg := Config{SiteURL: "https://example.test", DefaultLanguage: "fr", RejectHiddenPath: true, RejectSymlinks: true, MaxIndexEntries: 1000}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := BuildIndex(context.Background(), root, cfg); err != nil {
			b.Fatal(err)
		}
	}
}
