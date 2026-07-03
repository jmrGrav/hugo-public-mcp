package site

import "time"

type Config struct {
	SiteURL          string
	SiteName         string
	DefaultLanguage  string
	MaxIndexEntries  int
	RejectSymlinks   bool
	RejectHiddenPath bool
}

type PageSummary struct {
	Slug         string    `json:"slug"`
	Title        string    `json:"title"`
	Summary      string    `json:"summary,omitempty"`
	CanonicalURL string    `json:"canonical_url"`
	Language     string    `json:"language,omitempty"`
	Date         time.Time `json:"date,omitempty"`
	Tags         []string  `json:"tags,omitempty"`
	Categories   []string  `json:"categories,omitempty"`
	Type         string    `json:"type,omitempty"`
}

type PageContent struct {
	Summary     PageSummary `json:"summary"`
	ContentHTML string      `json:"content_html,omitempty"`
	ContentText string      `json:"content_text,omitempty"`
}

// PageMarkdown is the authenticated view of a published page: summary metadata
// plus full content rendered as Markdown. Only accessible with a valid bearer.
type PageMarkdown struct {
	Summary         PageSummary `json:"summary"`
	MarkdownContent string      `json:"markdown_content"`
}

// PageFrontmatter is structured metadata for a published page including an
// estimated reading time derived from the indexed content text.
type PageFrontmatter struct {
	Summary        PageSummary `json:"summary"`
	ReadingTimeMin int         `json:"reading_time_minutes,omitempty"`
}

// RelatedPage is a page related to a reference slug by shared taxonomy.
type RelatedPage struct {
	Slug             string   `json:"slug"`
	Title            string   `json:"title"`
	CanonicalURL     string   `json:"canonical_url"`
	SharedTags       []string `json:"shared_tags,omitempty"`
	SharedCategories []string `json:"shared_categories,omitempty"`
}

// AgentContext is a complete enriched context bundle for a single published
// page: metadata, estimated reading time, full Markdown content, and related
// pages by taxonomy. Only accessible with a valid bearer.
type AgentContext struct {
	Summary        PageSummary   `json:"summary"`
	ReadingTimeMin int           `json:"reading_time_minutes,omitempty"`
	MarkdownContent string       `json:"markdown_content"`
	Related        []RelatedPage `json:"related,omitempty"`
}

// PageExport is a single entry in a paginated export of page context bundles.
type PageExport struct {
	Summary        PageSummary `json:"summary"`
	ReadingTimeMin int         `json:"reading_time_minutes,omitempty"`
	MarkdownContent string     `json:"markdown_content"`
}

// ExportResult is the result of a paginated export call.
type ExportResult struct {
	Pages      []PageExport `json:"pages"`
	NextCursor string       `json:"next_cursor,omitempty"`
	Total      int          `json:"total"`
}

type TagSummary struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	URL   string `json:"url,omitempty"`
}

type CategorySummary struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	URL   string `json:"url,omitempty"`
}

type SiteInformation struct {
	SiteName        string `json:"site_name"`
	SiteURL         string `json:"site_url"`
	DefaultLanguage string `json:"default_language"`
	PageCount       int    `json:"page_count"`
	TagCount        int    `json:"tag_count"`
	CategoryCount   int    `json:"category_count"`
}
