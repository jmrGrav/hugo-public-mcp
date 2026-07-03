package site

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jmrGrav/hugo-public-mcp/internal/security/pathguard"
	"golang.org/x/net/html"
)

type Index struct {
	Pages        []PageContent
	pageBySlug   map[string]int
	tags         map[string]int
	tagURLs      map[string]string
	categories   map[string]int
	categoryURLs map[string]string
	resources    map[string]string
	info         SiteInformation
}

func BuildIndex(ctx context.Context, root string, cfg Config) (*Index, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	canonicalRoot, err := pathguard.CanonicalDir(root)
	if err != nil {
		return nil, err
	}
	if cfg.MaxIndexEntries <= 0 {
		cfg.MaxIndexEntries = 5000
	}
	if cfg.DefaultLanguage == "" {
		cfg.DefaultLanguage = "en"
	}
	idx := &Index{
		pageBySlug:   map[string]int{},
		tags:         map[string]int{},
		tagURLs:      map[string]string{},
		categories:   map[string]int{},
		categoryURLs: map[string]string{},
		resources:    map[string]string{},
		info: SiteInformation{
			SiteName:        cfg.SiteName,
			SiteURL:         cfg.SiteURL,
			DefaultLanguage: cfg.DefaultLanguage,
		},
	}

	if err := loadStaticResources(canonicalRoot, idx); err != nil {
		return nil, err
	}

	err = filepath.WalkDir(canonicalRoot, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if p == canonicalRoot {
			return nil
		}
		rel, err := filepath.Rel(canonicalRoot, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if cfg.RejectHiddenPath && pathguard.IsHiddenPath(rel) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			if cfg.RejectSymlinks {
				return fmt.Errorf("symlink rejected: %s", rel)
			}
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		base := filepath.Base(rel)
		switch base {
		case "robots.txt", "llms.txt", "feed.json", "sitemap.xml", "index.xml":
			raw, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			idx.resources["/"+base] = string(raw)
			return nil
		}

		if !isHTMLFile(base) {
			return nil
		}
		if len(idx.Pages) >= cfg.MaxIndexEntries {
			return fmt.Errorf("index entry limit exceeded: %d", cfg.MaxIndexEntries)
		}

		info, err := os.Stat(p)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		page, err := parseHTMLPage(raw, rel, info.ModTime(), cfg)
		if err != nil {
			return err
		}
		if page.Slug == "" {
			return nil
		}
		if _, exists := idx.pageBySlug[page.Slug]; exists {
			return nil
		}
		idx.pageBySlug[page.Slug] = len(idx.Pages)
		idx.Pages = append(idx.Pages, PageContent{
			Summary:     page,
			ContentHTML: pageContentHTML(raw),
			ContentText: pageContentText(raw),
		})
		idx.info.PageCount++
		for _, tag := range page.Tags {
			idx.tags[tag]++
			if _, ok := idx.tagURLs[tag]; !ok {
				idx.tagURLs[tag] = joinSiteURL(cfg.SiteURL, "/tags/"+slugPart(tag)+"/")
			}
		}
		for _, cat := range page.Categories {
			idx.categories[cat]++
			if _, ok := idx.categoryURLs[cat]; !ok {
				idx.categoryURLs[cat] = joinSiteURL(cfg.SiteURL, "/categories/"+slugPart(cat)+"/")
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.SliceStable(idx.Pages, func(i, j int) bool {
		di := idx.Pages[i].Summary.Date
		dj := idx.Pages[j].Summary.Date
		if di.Equal(dj) {
			return idx.Pages[i].Summary.Slug < idx.Pages[j].Summary.Slug
		}
		return di.After(dj)
	})
	idx.reindex()
	return idx, nil
}

func loadStaticResources(root string, idx *Index) error {
	if idx == nil {
		return nil
	}
	for _, rel := range []string{
		"robots.txt",
		"llms.txt",
		"feed.json",
		"sitemap.xml",
		"index.xml",
		"auth.md",
		".well-known/api-catalog",
		".well-known/agent-skills/index.json",
		".well-known/agent-skills/discover_hugo_site.md",
		".well-known/agent-skills/search_public_pages.md",
		".well-known/agent-skills/read_public_page.md",
		".well-known/agent-skills/list_public_tags.md",
		".well-known/agent-skills/list_public_categories.md",
		".well-known/agent-skills/get_public_feed.md",
		".well-known/agent-skills/get_public_sitemap.md",
	} {
		full := filepath.Join(root, filepath.FromSlash(rel))
		raw, err := os.ReadFile(full)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		idx.resources["/"+filepath.ToSlash(rel)] = string(raw)
	}
	return nil
}

func (idx *Index) reindex() {
	idx.pageBySlug = make(map[string]int, len(idx.Pages))
	for i := range idx.Pages {
		idx.pageBySlug[idx.Pages[i].Summary.Slug] = i
	}
	idx.info.TagCount = len(idx.tags)
	idx.info.CategoryCount = len(idx.categories)
}

func (idx *Index) ListPages(limit int) []PageSummary {
	if idx == nil {
		return nil
	}
	if limit <= 0 || limit > len(idx.Pages) {
		limit = len(idx.Pages)
	}
	out := make([]PageSummary, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, idx.Pages[i].Summary)
	}
	return out
}

func (idx *Index) GetPage(slug string) (PageContent, error) {
	if idx == nil {
		return PageContent{}, fmt.Errorf("index not initialized")
	}
	norm, err := NormalizeSlug(slug)
	if err != nil {
		return PageContent{}, err
	}
	pos, ok := idx.pageBySlug[norm]
	if !ok {
		return PageContent{}, fmt.Errorf("page not found: %s", norm)
	}
	return idx.Pages[pos], nil
}

// GetPageMarkdown returns the Markdown-formatted full content of a published page.
// The slug must exist in the index; arbitrary paths are rejected by NormalizeSlug.
func (idx *Index) GetPageMarkdown(slug string) (PageMarkdown, error) {
	if idx == nil {
		return PageMarkdown{}, fmt.Errorf("index not initialized")
	}
	page, err := idx.GetPage(slug)
	if err != nil {
		return PageMarkdown{}, err
	}
	doc, err := html.Parse(strings.NewReader(page.ContentHTML))
	if err != nil {
		return PageMarkdown{Summary: page.Summary, MarkdownContent: page.ContentText}, nil
	}
	body := findElement(doc, "body")
	if body == nil {
		return PageMarkdown{Summary: page.Summary, MarkdownContent: page.ContentText}, nil
	}
	return PageMarkdown{
		Summary:         page.Summary,
		MarkdownContent: htmlBodyToMarkdown(body),
	}, nil
}

// GetFrontmatter returns structured metadata for a published page including an
// estimated reading time (words / 200 words-per-minute, minimum 1 when non-empty).
func (idx *Index) GetFrontmatter(slug string) (PageFrontmatter, error) {
	if idx == nil {
		return PageFrontmatter{}, fmt.Errorf("index not initialized")
	}
	page, err := idx.GetPage(slug)
	if err != nil {
		return PageFrontmatter{}, err
	}
	return PageFrontmatter{
		Summary:        page.Summary,
		ReadingTimeMin: estimateReadingMinutes(page.ContentText),
	}, nil
}

// RelatedPages returns pages sharing tags or categories with the given slug,
// sorted by number of shared taxonomy terms (descending) then by date.
func (idx *Index) RelatedPages(slug string, limit int) ([]RelatedPage, error) {
	if idx == nil {
		return nil, fmt.Errorf("index not initialized")
	}
	norm, err := NormalizeSlug(slug)
	if err != nil {
		return nil, err
	}
	ref, ok := idx.pageBySlug[norm]
	if !ok {
		return nil, fmt.Errorf("page not found: %s", norm)
	}
	refSummary := idx.Pages[ref].Summary
	tagSet := make(map[string]struct{}, len(refSummary.Tags))
	for _, t := range refSummary.Tags {
		tagSet[t] = struct{}{}
	}
	catSet := make(map[string]struct{}, len(refSummary.Categories))
	for _, c := range refSummary.Categories {
		catSet[c] = struct{}{}
	}
	if limit <= 0 || limit > 20 {
		limit = 5
	}
	type scored struct {
		page  PageSummary
		score int
		shared RelatedPage
	}
	var candidates []scored
	for i, p := range idx.Pages {
		if i == ref {
			continue
		}
		var sharedTags, sharedCats []string
		for _, t := range p.Summary.Tags {
			if _, ok := tagSet[t]; ok {
				sharedTags = append(sharedTags, t)
			}
		}
		for _, c := range p.Summary.Categories {
			if _, ok := catSet[c]; ok {
				sharedCats = append(sharedCats, c)
			}
		}
		score := len(sharedTags) + len(sharedCats)
		if score == 0 {
			continue
		}
		candidates = append(candidates, scored{
			page:  p.Summary,
			score: score,
			shared: RelatedPage{
				Slug:             p.Summary.Slug,
				Title:            p.Summary.Title,
				CanonicalURL:     p.Summary.CanonicalURL,
				SharedTags:       sharedTags,
				SharedCategories: sharedCats,
			},
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].page.Date.After(candidates[j].page.Date)
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	out := make([]RelatedPage, len(candidates))
	for i, c := range candidates {
		out[i] = c.shared
	}
	return out, nil
}

// GetAgentContext returns a complete enriched context bundle for a published
// page: summary, reading time, full Markdown content, and related pages.
// Response content is capped at 256 KB of Markdown to keep responses bounded.
func (idx *Index) GetAgentContext(slug string) (AgentContext, error) {
	if idx == nil {
		return AgentContext{}, fmt.Errorf("index not initialized")
	}
	fm, err := idx.GetFrontmatter(slug)
	if err != nil {
		return AgentContext{}, err
	}
	md, err := idx.GetPageMarkdown(slug)
	if err != nil {
		return AgentContext{}, err
	}
	related, _ := idx.RelatedPages(slug, 5)
	const maxMarkdownBytes = 256 * 1024
	content := md.MarkdownContent
	if len(content) > maxMarkdownBytes {
		content = content[:maxMarkdownBytes]
	}
	return AgentContext{
		Summary:         fm.Summary,
		ReadingTimeMin:  fm.ReadingTimeMin,
		MarkdownContent: content,
		Related:         related,
	}, nil
}

// ExportPages returns a paginated slice of page context bundles. cursor is the
// slug of the first page to include (empty = start from beginning). tag and
// category filter pages to those matching. limit is capped at 10.
// Returns the slice, the next cursor (empty when exhausted), and the total
// number of pages in the filtered set.
func (idx *Index) ExportPages(cursor, tag, category string, limit int) (ExportResult, error) {
	if idx == nil {
		return ExportResult{}, fmt.Errorf("index not initialized")
	}
	if limit <= 0 || limit > 10 {
		limit = 10
	}
	// Build filtered list (pages are already sorted by date desc).
	var filtered []int
	for i, p := range idx.Pages {
		if tag != "" && !sliceContains(p.Summary.Tags, tag) {
			continue
		}
		if category != "" && !sliceContains(p.Summary.Categories, category) {
			continue
		}
		filtered = append(filtered, i)
	}
	total := len(filtered)
	// Resolve cursor to offset.
	start := 0
	if cursor != "" {
		norm, err := NormalizeSlug(cursor)
		if err == nil {
			for offset, i := range filtered {
				if idx.Pages[i].Summary.Slug == norm {
					start = offset
					break
				}
			}
		}
	}
	if start >= len(filtered) {
		return ExportResult{Total: total}, nil
	}
	end := start + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	slice := filtered[start:end]
	var nextCursor string
	if end < len(filtered) {
		nextCursor = idx.Pages[filtered[end]].Summary.Slug
	}
	out := make([]PageExport, 0, len(slice))
	for _, i := range slice {
		p := idx.Pages[i]
		md, err := idx.GetPageMarkdown(p.Summary.Slug)
		if err != nil {
			continue
		}
		const maxMarkdownBytes = 512 * 1024 / 10 // ~51KB per page to stay under 512KB total
		content := md.MarkdownContent
		if len(content) > maxMarkdownBytes {
			content = content[:maxMarkdownBytes]
		}
		out = append(out, PageExport{
			Summary:         p.Summary,
			ReadingTimeMin:  estimateReadingMinutes(p.ContentText),
			MarkdownContent: content,
		})
	}
	return ExportResult{Pages: out, NextCursor: nextCursor, Total: total}, nil
}

func sliceContains(slice []string, v string) bool {
	for _, s := range slice {
		if s == v {
			return true
		}
	}
	return false
}

func estimateReadingMinutes(text string) int {
	words := len(strings.Fields(text))
	if words == 0 {
		return 0
	}
	minutes := words / 200
	if words%200 > 0 {
		minutes++
	}
	return minutes
}

func (idx *Index) Search(query string, limit int) []PageSummary {
	if idx == nil {
		return nil
	}
	if limit <= 0 || limit > len(idx.Pages) {
		limit = len(idx.Pages)
	}
	terms := tokenize(query)
	type scored struct {
		page  PageSummary
		score int
	}
	var scoredPages []scored
	for _, p := range idx.Pages {
		score := scorePage(p.Summary, terms)
		if len(terms) == 0 {
			score = 1
		}
		if score > 0 {
			scoredPages = append(scoredPages, scored{page: p.Summary, score: score})
		}
	}
	sort.SliceStable(scoredPages, func(i, j int) bool {
		if scoredPages[i].score == scoredPages[j].score {
			return scoredPages[i].page.Date.After(scoredPages[j].page.Date)
		}
		return scoredPages[i].score > scoredPages[j].score
	})
	if len(scoredPages) > limit {
		scoredPages = scoredPages[:limit]
	}
	out := make([]PageSummary, 0, len(scoredPages))
	for _, item := range scoredPages {
		out = append(out, item.page)
	}
	return out
}

func (idx *Index) RecentPosts(limit int) []PageSummary {
	if idx == nil {
		return nil
	}
	if limit <= 0 || limit > len(idx.Pages) {
		limit = len(idx.Pages)
	}
	var posts []PageSummary
	for _, p := range idx.Pages {
		if isPost(p.Summary) {
			posts = append(posts, p.Summary)
		}
	}
	if len(posts) > limit {
		posts = posts[:limit]
	}
	return posts
}

func (idx *Index) ListTags() []TagSummary {
	if idx == nil {
		return nil
	}
	out := make([]TagSummary, 0, len(idx.tags))
	for name, count := range idx.tags {
		out = append(out, TagSummary{Name: name, Count: count, URL: idx.tagURLs[name]})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
		}
		return out[i].Count > out[j].Count
	})
	return out
}

func (idx *Index) ListCategories() []CategorySummary {
	if idx == nil {
		return nil
	}
	out := make([]CategorySummary, 0, len(idx.categories))
	for name, count := range idx.categories {
		out = append(out, CategorySummary{Name: name, Count: count, URL: idx.categoryURLs[name]})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
		}
		return out[i].Count > out[j].Count
	})
	return out
}

func (idx *Index) GetResource(path string) (string, bool) {
	if idx == nil {
		return "", false
	}
	body, ok := idx.resources[path]
	return body, ok
}

func (idx *Index) SiteInformation() SiteInformation {
	if idx == nil {
		return SiteInformation{}
	}
	return idx.info
}

func isHTMLFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".html", ".htm":
		return true
	default:
		return false
	}
}

func parseHTMLPage(raw []byte, rel string, modTime time.Time, cfg Config) (PageSummary, error) {
	doc, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		return PageSummary{}, err
	}
	meta := collectHTMLMeta(doc)
	slug := meta.canonicalSlug
	if slug == "" {
		slug = slugFromRel(rel)
	}
	if slug == "" {
		return PageSummary{}, nil
	}
	p := PageSummary{
		Slug:         slug,
		Title:        firstNonEmpty(meta.ogTitle, meta.title, slugTitleFromSlug(slug)),
		Summary:      firstNonEmpty(meta.ogDescription, meta.description),
		CanonicalURL: firstNonEmpty(meta.canonicalURL, joinSiteURL(cfg.SiteURL, slug)),
		Language:     firstNonEmpty(meta.lang, cfg.DefaultLanguage),
		Date:         firstNonZero(meta.published, meta.modified, modTime),
		Tags:         uniqueStrings(meta.tags),
		Categories:   uniqueStrings(meta.categories),
		Type:         firstNonEmpty(meta.ogType, inferTypeFromSlug(slug)),
	}
	if len(p.Categories) == 0 && meta.section != "" {
		p.Categories = []string{meta.section}
	}
	if len(p.Tags) == 0 && strings.HasPrefix(strings.ToLower(slug), "/tags/") {
		p.Tags = []string{slugLeaf(slug)}
	}
	if len(p.Categories) == 0 && strings.HasPrefix(strings.ToLower(slug), "/categories/") {
		p.Categories = []string{slugLeaf(slug)}
	}
	return p, nil
}

type htmlMeta struct {
	title         string
	description   string
	canonicalURL  string
	canonicalSlug string
	lang          string
	ogTitle       string
	ogDescription string
	ogType        string
	section       string
	published     time.Time
	modified      time.Time
	tags          []string
	categories    []string
}

func collectHTMLMeta(doc *html.Node) htmlMeta {
	var meta htmlMeta
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n == nil {
			return
		}
		if n.Type == html.ElementNode {
			switch n.Data {
			case "html":
				if v := attr(n, "lang"); v != "" {
					meta.lang = v
				}
			case "title":
				if meta.title == "" {
					meta.title = strings.TrimSpace(nodeText(n))
				}
			case "meta":
				name := strings.ToLower(attr(n, "name"))
				property := strings.ToLower(attr(n, "property"))
				content := strings.TrimSpace(attr(n, "content"))
				switch {
				case name == "description" && meta.description == "":
					meta.description = content
				case property == "og:title" && meta.ogTitle == "":
					meta.ogTitle = content
				case property == "og:description" && meta.ogDescription == "":
					meta.ogDescription = content
				case property == "og:type" && meta.ogType == "":
					meta.ogType = content
				case property == "article:section" && meta.section == "":
					meta.section = content
				case property == "article:published_time" && meta.published.IsZero():
					meta.published = parseTime(content)
				case property == "article:modified_time" && meta.modified.IsZero():
					meta.modified = parseTime(content)
				case property == "article:tag" && content != "":
					meta.tags = append(meta.tags, content)
				case property == "article:category" && content != "":
					meta.categories = append(meta.categories, content)
				case name == "keywords" && content != "":
					meta.categories = append(meta.categories, splitKeywords(content)...)
				}
			case "link":
				rel := strings.ToLower(attr(n, "rel"))
				if strings.Contains(rel, "canonical") && meta.canonicalURL == "" {
					meta.canonicalURL = strings.TrimSpace(attr(n, "href"))
					meta.canonicalSlug = slugFromCanonical(meta.canonicalURL)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return meta
}

func nodeText(n *html.Node) string {
	var buf strings.Builder
	var walk func(*html.Node)
	walk = func(cur *html.Node) {
		if cur == nil {
			return
		}
		if cur.Type == html.TextNode {
			buf.WriteString(cur.Data)
			buf.WriteByte(' ')
		}
		for c := cur.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(buf.String()), " ")
}

func pageContentHTML(raw []byte) string {
	doc, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		return string(raw)
	}
	body := findElement(doc, "body")
	if body == nil {
		return string(raw)
	}
	var buf bytes.Buffer
	for c := body.FirstChild; c != nil; c = c.NextSibling {
		_ = html.Render(&buf, c)
	}
	return buf.String()
}

func pageContentText(raw []byte) string {
	doc, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		return strings.TrimSpace(string(raw))
	}
	body := findElement(doc, "body")
	if body == nil {
		return strings.TrimSpace(string(raw))
	}
	text := nodeText(body)
	if text != "" {
		return text
	}
	return strings.TrimSpace(nodeText(doc))
}

func findElement(n *html.Node, name string) *html.Node {
	var out *html.Node
	var walk func(*html.Node)
	walk = func(cur *html.Node) {
		if cur == nil || out != nil {
			return
		}
		if cur.Type == html.ElementNode && cur.Data == name {
			out = cur
			return
		}
		for c := cur.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return out
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

func parseTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t
		}
	}
	return time.Time{}
}

func slugFromCanonical(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if i := strings.Index(raw, "://"); i >= 0 {
		raw = raw[i+3:]
		if j := strings.IndexByte(raw, '/'); j >= 0 {
			raw = raw[j:]
		} else {
			raw = "/"
		}
	}
	if raw == "" {
		return ""
	}
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	if decoded, err := url.PathUnescape(raw); err == nil {
		raw = decoded
	}
	return normalizeSlug(raw)
}

func slugFromRel(rel string) string {
	rel = filepath.ToSlash(rel)
	switch {
	case rel == "index.html":
		return "/"
	case strings.HasSuffix(rel, "/index.html"):
		return normalizeSlug("/" + strings.TrimSuffix(rel, "/index.html"))
	case strings.HasSuffix(rel, ".html"):
		return normalizeSlug("/" + strings.TrimSuffix(rel, ".html"))
	default:
		return normalizeSlug("/" + strings.TrimSuffix(rel, filepath.Ext(rel)))
	}
}

func NormalizeSlug(input string) (string, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "/", nil
	}
	if strings.Contains(s, "\\") {
		return "", fmt.Errorf("slug contains backslashes")
	}
	s = strings.TrimPrefix(s, "/")
	rel, err := pathguard.ValidateRelative(s)
	if err != nil {
		return "", fmt.Errorf("slug traversal detected: %w", err)
	}
	if rel == "" {
		return "/", nil
	}
	return normalizeSlug("/" + rel), nil
}

func normalizeSlug(raw string) string {
	if raw == "" {
		return "/"
	}
	clean := pathClean(raw)
	if clean == "." {
		return "/"
	}
	if !strings.HasPrefix(clean, "/") {
		clean = "/" + clean
	}
	if strings.HasSuffix(clean, "/index") {
		clean = strings.TrimSuffix(clean, "index")
	}
	if strings.HasSuffix(clean, "/index/") {
		clean = strings.TrimSuffix(clean, "index/")
	}
	if clean != "/" && strings.HasSuffix(clean, "/") {
		return clean
	}
	return clean
}

func pathClean(raw string) string {
	if raw == "" {
		return "/"
	}
	raw = strings.ReplaceAll(raw, "//", "/")
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	clean := filepath.ToSlash(filepath.Clean(raw))
	if clean == "." {
		return "/"
	}
	return clean
}

func slugTitleFromSlug(slug string) string {
	leaf := slugLeaf(slug)
	if leaf == "" {
		return "Home"
	}
	leaf = strings.ReplaceAll(leaf, "-", " ")
	leaf = strings.ReplaceAll(leaf, "_", " ")
	return strings.ToUpper(leaf[:1]) + leaf[1:]
}

func slugLeaf(slug string) string {
	slug = strings.TrimSuffix(strings.TrimSpace(slug), "/")
	if slug == "" || slug == "/" {
		return ""
	}
	parts := strings.Split(slug, "/")
	return parts[len(parts)-1]
}

func slugPart(raw string) string {
	return slugLeaf(normalizeSlug("/" + strings.TrimSpace(raw)))
}

func inferTypeFromSlug(slug string) string {
	switch {
	case slug == "/":
		return "home"
	case strings.HasPrefix(strings.ToLower(slug), "/tags/"), strings.HasPrefix(strings.ToLower(slug), "/categories/"):
		return "taxonomy"
	case strings.HasPrefix(strings.ToLower(slug), "/posts/"):
		return "article"
	default:
		return "page"
	}
}

func joinSiteURL(siteURL, slug string) string {
	siteURL = strings.TrimRight(strings.TrimSpace(siteURL), "/")
	slug = strings.TrimSpace(slug)
	if siteURL == "" {
		return slug
	}
	if slug == "" || slug == "/" {
		return siteURL + "/"
	}
	return siteURL + normalizeSlug(slug)
}

func uniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		key := strings.ToLower(v)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, v)
	}
	return out
}

func splitKeywords(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func firstNonZero(values ...time.Time) time.Time {
	for _, v := range values {
		if !v.IsZero() {
			return v
		}
	}
	return time.Time{}
}

func isPost(p PageSummary) bool {
	if strings.EqualFold(p.Type, "article") {
		return true
	}
	return strings.HasPrefix(strings.ToLower(p.Slug), "/posts/")
}

func tokenize(query string) []string {
	parts := strings.Fields(strings.ToLower(query))
	return parts
}

func scorePage(p PageSummary, terms []string) int {
	if len(terms) == 0 {
		return 1
	}
	fields := []string{
		strings.ToLower(p.Title),
		strings.ToLower(p.Summary),
		strings.ToLower(p.CanonicalURL),
		strings.ToLower(strings.Join(p.Tags, " ")),
		strings.ToLower(strings.Join(p.Categories, " ")),
	}
	score := 0
	for _, term := range terms {
		for _, field := range fields {
			if strings.Contains(field, term) {
				score++
				break
			}
		}
	}
	return score
}
