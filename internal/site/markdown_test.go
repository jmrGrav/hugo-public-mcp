// internal/site/markdown_test.go
package site

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func parseBody(t *testing.T, src string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("html.Parse: %v", err)
	}
	return findElement(doc, "body")
}

func TestHTMLBodyToMarkdown_Headings(t *testing.T) {
	body := parseBody(t, `<body><h1>Title</h1><h2>Sub</h2></body>`)
	got := htmlBodyToMarkdown(body)
	if !strings.Contains(got, "# Title") {
		t.Errorf("missing h1: %q", got)
	}
	if !strings.Contains(got, "## Sub") {
		t.Errorf("missing h2: %q", got)
	}
}

func TestHTMLBodyToMarkdown_Paragraph(t *testing.T) {
	body := parseBody(t, `<body><p>Hello world.</p></body>`)
	got := htmlBodyToMarkdown(body)
	if !strings.Contains(got, "Hello world.") {
		t.Errorf("missing paragraph text: %q", got)
	}
}

func TestHTMLBodyToMarkdown_Link(t *testing.T) {
	body := parseBody(t, `<body><p><a href="https://example.com">click</a></p></body>`)
	got := htmlBodyToMarkdown(body)
	if !strings.Contains(got, "[click](https://example.com)") {
		t.Errorf("missing link: %q", got)
	}
}

func TestHTMLBodyToMarkdown_Strong(t *testing.T) {
	body := parseBody(t, `<body><p><strong>bold</strong></p></body>`)
	got := htmlBodyToMarkdown(body)
	if !strings.Contains(got, "**bold**") {
		t.Errorf("missing bold: %q", got)
	}
}

func TestHTMLBodyToMarkdown_Em(t *testing.T) {
	body := parseBody(t, `<body><p><em>italic</em></p></body>`)
	got := htmlBodyToMarkdown(body)
	if !strings.Contains(got, "*italic*") {
		t.Errorf("missing italic: %q", got)
	}
}

func TestHTMLBodyToMarkdown_InlineCode(t *testing.T) {
	body := parseBody(t, `<body><p><code>x := 1</code></p></body>`)
	got := htmlBodyToMarkdown(body)
	if !strings.Contains(got, "`x := 1`") {
		t.Errorf("missing inline code: %q", got)
	}
}

func TestHTMLBodyToMarkdown_Pre(t *testing.T) {
	body := parseBody(t, "<body><pre><code>func f() {}</code></pre></body>")
	got := htmlBodyToMarkdown(body)
	if !strings.Contains(got, "```") || !strings.Contains(got, "func f() {}") {
		t.Errorf("missing pre block: %q", got)
	}
}

func TestHTMLBodyToMarkdown_UnorderedList(t *testing.T) {
	body := parseBody(t, `<body><ul><li>A</li><li>B</li></ul></body>`)
	got := htmlBodyToMarkdown(body)
	if !strings.Contains(got, "- A") || !strings.Contains(got, "- B") {
		t.Errorf("missing list: %q", got)
	}
}

func TestHTMLBodyToMarkdown_OrderedList(t *testing.T) {
	body := parseBody(t, `<body><ol><li>First</li><li>Second</li></ol></body>`)
	got := htmlBodyToMarkdown(body)
	if !strings.Contains(got, "1. First") || !strings.Contains(got, "2. Second") {
		t.Errorf("missing ordered list: %q", got)
	}
}

func TestHTMLBodyToMarkdown_NilSafe(t *testing.T) {
	got := htmlBodyToMarkdown(nil)
	if got != "" {
		t.Errorf("nil body must return empty string, got %q", got)
	}
}

func TestHTMLBodyToMarkdown_NoPathLeak(t *testing.T) {
	body := parseBody(t, `<body><p>Content.</p></body>`)
	got := htmlBodyToMarkdown(body)
	for _, forbidden := range []string{"/home/", "192.168.", ".git", "secret", "password"} {
		if strings.Contains(strings.ToLower(got), forbidden) {
			t.Errorf("leak detected (%q) in: %q", forbidden, got)
		}
	}
}
