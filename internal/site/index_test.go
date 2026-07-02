package site

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBuildIndexParsesPublicFixture(t *testing.T) {
	idx := mustBuildMinimalIndex(t)
	if got := len(idx.Pages); got != 3 {
		t.Fatalf("BuildIndex() pages = %d want 3", got)
	}
	if idx.Pages[0].Summary.Slug == "" {
		t.Fatal("BuildIndex() empty slug")
	}
	if idx.SiteInformation().PageCount != 3 {
		t.Fatalf("SiteInformation() page count = %d want 3", idx.SiteInformation().PageCount)
	}
	if got, ok := idx.GetResource("/sitemap.xml"); !ok || !strings.Contains(got, "<urlset") {
		t.Fatalf("GetResource(sitemap) = %q, %v", got, ok)
	}
	if got, ok := idx.GetResource("/auth.md"); !ok || !strings.Contains(got, "no registration required") {
		t.Fatalf("GetResource(auth) = %q, %v", got, ok)
	}
	if got, ok := idx.GetResource("/.well-known/api-catalog"); !ok || !strings.Contains(got, "mcp.arleo.eu/mcp") {
		t.Fatalf("GetResource(api-catalog) = %q, %v", got, ok)
	}
	if got, ok := idx.GetResource("/.well-known/agent-skills/index.json"); !ok || !strings.Contains(got, "discover_hugo_site") {
		t.Fatalf("GetResource(agent-skills index) = %q, %v", got, ok)
	}
}

func TestSearchUsesMetadataOnly(t *testing.T) {
	idx := mustBuildMinimalIndex(t)
	got := idx.Search("security", 10)
	if len(got) == 0 {
		t.Fatal("Search() returned no results")
	}
	if got[0].Slug != "/posts/bonjour" {
		t.Fatalf("Search() top result = %q want /posts/bonjour", got[0].Slug)
	}
}

func TestGetPageBySlug(t *testing.T) {
	idx := mustBuildMinimalIndex(t)
	got, err := idx.GetPage("/posts/hello")
	if err != nil {
		t.Fatalf("GetPage() error = %v", err)
	}
	if got.Summary.Language != "en" {
		t.Fatalf("GetPage() language = %q want en", got.Summary.Language)
	}
	if got.Summary.CanonicalURL != "https://example.test/posts/hello/" {
		t.Fatalf("GetPage() canonical = %q", got.Summary.CanonicalURL)
	}
	if !strings.Contains(got.ContentText, "Minimal English content.") {
		t.Fatalf("GetPage() text = %q", got.ContentText)
	}
}

func TestNormalizeSlugRejectsTraversal(t *testing.T) {
	for _, input := range []string{"../escape", "posts/../escape", "posts\\hello"} {
		if _, err := NormalizeSlug(input); err == nil {
			t.Fatalf("NormalizeSlug(%q) expected error", input)
		}
	}
}

func TestBuildIndexRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "posts"), 0o755); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(root, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "index.html"), []byte("<html><head><title>x</title></head><body>x</body></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(root, "posts", "escape")); err != nil {
		t.Fatal(err)
	}
	_, err := BuildIndex(context.Background(), root, Config{RejectSymlinks: true, RejectHiddenPath: true, MaxIndexEntries: 100})
	if err == nil {
		t.Fatal("BuildIndex() expected symlink rejection")
	}
}

func TestUnicodeAndMultilingual(t *testing.T) {
	root := t.TempDir()
	writePage(t, root, "index.html", `<!doctype html><html lang="fr"><head><title>Accueil</title><meta name="description" content="Bienvenue 🌍"><link rel="canonical" href="https://example.test/"></head><body>Bonjour 🌍</body></html>`)
	writePage(t, root, filepath.Join("posts", "cafe", "index.fr.html"), `<!doctype html><html lang="fr"><head><title>Café déjà vu</title><meta name="description" content="Résumé caféiné"><link rel="canonical" href="https://example.test/posts/cafe/"><meta property="article:published_time" content="2026-01-02T03:04:05Z"><meta property="article:tag" content="Sécurité"></head><body>Café déjà vu ☕</body></html>`)
	idx, err := BuildIndex(context.Background(), root, Config{SiteURL: "https://example.test", DefaultLanguage: "fr", RejectHiddenPath: true, RejectSymlinks: true, MaxIndexEntries: 100})
	if err != nil {
		t.Fatalf("BuildIndex() error = %v", err)
	}
	got, err := idx.GetPage("/posts/cafe")
	if err != nil {
		t.Fatalf("GetPage() error = %v", err)
	}
	if got.Summary.Title != "Café déjà vu" {
		t.Fatalf("GetPage() title = %q", got.Summary.Title)
	}
	if got.Summary.Language != "fr" {
		t.Fatalf("GetPage() language = %q", got.Summary.Language)
	}
	if !strings.Contains(got.ContentText, "☕") {
		t.Fatalf("GetPage() unicode text missing: %q", got.ContentText)
	}
}

func TestMalformedHTMLAndMetadata(t *testing.T) {
	root := t.TempDir()
	writePage(t, root, filepath.Join("posts", "broken", "index.html"), `<!doctype html><html><head><title>Broken<title><meta name="description" content="unterminated"><link rel="canonical" href="https://example.test/posts/broken/"><meta property="article:published_time" content="not-a-time"></head><body><h1>Broken</h1><p>Still readable`)
	idx, err := BuildIndex(context.Background(), root, Config{SiteURL: "https://example.test", DefaultLanguage: "en", RejectHiddenPath: true, RejectSymlinks: true, MaxIndexEntries: 100})
	if err != nil {
		t.Fatalf("BuildIndex() error = %v", err)
	}
	got, err := idx.GetPage("/posts/broken")
	if err != nil {
		t.Fatalf("GetPage() error = %v", err)
	}
	if got.Summary.Title == "" {
		t.Fatal("expected title fallback for malformed HTML")
	}
	if !strings.Contains(got.ContentText, "Still readable") {
		t.Fatalf("GetPage() text = %q", got.ContentText)
	}
}

func TestRecentPostsAndTaxonomies(t *testing.T) {
	idx := mustBuildMinimalIndex(t)
	recent := idx.RecentPosts(1)
	if len(recent) != 1 {
		t.Fatalf("RecentPosts() len = %d", len(recent))
	}
	if recent[0].Slug != "/posts/hello" {
		t.Fatalf("RecentPosts() top result = %q want /posts/hello", recent[0].Slug)
	}
	tags := idx.ListTags()
	if len(tags) != 3 {
		t.Fatalf("ListTags() len = %d want 3", len(tags))
	}
	if tags[0].Name == "" {
		t.Fatal("ListTags() empty name")
	}
}

func TestConcurrentReadOnlyAccess(t *testing.T) {
	idx := mustBuildMinimalIndex(t)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = idx.Search("hugo", 10)
			_, _ = idx.GetPage("/posts/hello")
			_ = idx.ListPages(10)
		}()
	}
	wg.Wait()
}

func TestHugeSiteStartupAndMemoryBudget(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 1000; i++ {
		writePage(t, root, filepath.Join("posts", fmt.Sprintf("p-%04d", i), "index.html"), fmt.Sprintf(`<!doctype html><html lang="en"><head><title>Post %04d</title><meta name="description" content="Post %04d"><link rel="canonical" href="https://example.test/posts/p-%04d/"><meta property="article:published_time" content="2026-01-%02dT03:04:05Z"><meta property="article:tag" content="Bulk"></head><body>Body %04d</body></html>`, i, i, i, (i%28)+1, i))
	}
	start := time.Now()
	idx, err := BuildIndex(context.Background(), root, Config{SiteURL: "https://example.test", DefaultLanguage: "en", RejectHiddenPath: true, RejectSymlinks: true, MaxIndexEntries: 5000})
	if err != nil {
		t.Fatalf("BuildIndex() error = %v", err)
	}
	if len(idx.Pages) != 1000 {
		t.Fatalf("BuildIndex() pages = %d want 1000", len(idx.Pages))
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("BuildIndex() too slow: %s", elapsed)
	}
}

func mustBuildMinimalIndex(t *testing.T) *Index {
	t.Helper()
	root := filepath.Join("..", "..", "testdata", "fixtures", "public", "minimal")
	idx, err := BuildIndex(context.Background(), root, Config{
		SiteURL:          "https://example.test",
		SiteName:         "example.test",
		DefaultLanguage:  "fr",
		MaxIndexEntries:  1000,
		RejectSymlinks:   true,
		RejectHiddenPath: true,
	})
	if err != nil {
		t.Fatalf("BuildIndex() error = %v", err)
	}
	return idx
}

func writePage(t *testing.T, root, rel, raw string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
}
