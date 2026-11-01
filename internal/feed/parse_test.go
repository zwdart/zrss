package feed

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func parseFixture(t *testing.T, name, base string) *Document {
	t.Helper()
	var baseURL *url.URL
	if base != "" {
		var err error
		baseURL, err = url.Parse(base)
		if err != nil {
			t.Fatalf("parse base %s: %v", base, err)
		}
	}
	doc, err := Parse(fixture(t, name), baseURL)
	if err != nil {
		t.Fatalf("Parse(%s): %v", name, err)
	}
	return doc
}

func requireTime(t *testing.T, label string, got time.Time, want time.Time) {
	t.Helper()
	if !got.Equal(want) {
		t.Errorf("%s = %s, want %s", label, got, want)
	}
}

func TestParseRSS2(t *testing.T) {
	doc := parseFixture(t, "rss2.xml", "https://example.com/feed.xml")

	if doc.Type != "rss" {
		t.Errorf("Type = %q, want rss", doc.Type)
	}
	if doc.Title != "Example & Friends" {
		t.Errorf("Title = %q", doc.Title)
	}
	if doc.Link != "https://example.com/news" {
		t.Errorf("Link = %q", doc.Link)
	}
	if doc.Description != "News & notes from the team" {
		t.Errorf("Description = %q", doc.Description)
	}
	if doc.Language != "en-US" {
		t.Errorf("Language = %q", doc.Language)
	}
	if doc.IconURL != "https://example.com/assets/logo.png" {
		t.Errorf("IconURL = %q", doc.IconURL)
	}
	if doc.Author != "editor@example.com (Ed Itor)" {
		t.Errorf("Author = %q", doc.Author)
	}
	if len(doc.Items) != 4 {
		t.Fatalf("len(Items) = %d, want 4", len(doc.Items))
	}

	first := doc.Items[0]
	if first.GUID != "post-1" {
		t.Errorf("GUID = %q", first.GUID)
	}
	if first.Link != "https://example.com/posts/1" {
		t.Errorf("Link = %q", first.Link)
	}
	if first.Title != "First Post" {
		t.Errorf("Title = %q", first.Title)
	}
	if first.Author != "Alice" {
		t.Errorf("Author = %q, want dc:creator", first.Author)
	}
	if first.Summary != "Short & sweet summary." {
		t.Errorf("Summary = %q", first.Summary)
	}
	if !strings.Contains(first.Content, "<strong>body</strong>") || !strings.Contains(first.Content, `src="/img/1.jpg"`) {
		t.Errorf("Content = %q, want content:encoded markup", first.Content)
	}
	if first.ImageURL != "https://img.example.com/thumb1.jpg" {
		t.Errorf("ImageURL = %q", first.ImageURL)
	}
	if first.EnclosureURL != "https://example.com/audio/1.mp3" {
		t.Errorf("EnclosureURL = %q", first.EnclosureURL)
	}
	requireTime(t, "Published", first.Published, time.Date(2006, 1, 2, 22, 4, 5, 0, time.UTC))

	second := doc.Items[1]
	if second.GUID != "https://example.com/?p=2" {
		t.Errorf("GUID = %q, want guid text unchanged", second.GUID)
	}
	if second.Link != "https://example.com/posts/two" {
		t.Errorf("Link = %q", second.Link)
	}
	if second.Title != "Second & Third" {
		t.Errorf("Title = %q", second.Title)
	}
	if second.Content != "Plain description with no full content." {
		t.Errorf("Content = %q, want description fallback", second.Content)
	}
	if second.ImageURL != "https://cdn.example.com/media/two.jpg" {
		t.Errorf("ImageURL = %q", second.ImageURL)
	}
	if second.EnclosureURL != "https://cdn.example.com/media/two.jpg" {
		t.Errorf("EnclosureURL = %q", second.EnclosureURL)
	}
	requireTime(t, "Published", second.Published, time.Date(2006, 1, 3, 4, 5, 6, 0, time.UTC))

	unsafe := doc.Items[2]
	if unsafe.Link != "" {
		t.Errorf("javascript link was kept: %q", unsafe.Link)
	}
	if unsafe.GUID != "" {
		t.Errorf("GUID fallback to dropped link = %q, want empty", unsafe.GUID)
	}

	requireTime(t, "Published", doc.Items[3].Published, time.Date(2006, 1, 2, 0, 0, 0, 0, time.UTC))
}

func TestParseRSSITunesImage(t *testing.T) {
	data := []byte(`<?xml version="1.0"?>
<rss version="2.0" xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd">
  <channel>
    <title>Podcast</title>
    <link>https://example.com/</link>
    <itunes:image href="//cdn.example.com/cover.jpg"/>
    <item><title>Episode</title><link>/1</link></item>
  </channel>
</rss>`)
	doc, err := Parse(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if doc.IconURL != "https://cdn.example.com/cover.jpg" {
		t.Errorf("IconURL = %q, want itunes:image href", doc.IconURL)
	}
	if len(doc.Items) != 1 || doc.Items[0].Link != "/1" {
		t.Errorf("items = %+v, want relative link preserved without base", doc.Items)
	}
}

func TestParseRDF(t *testing.T) {
	doc := parseFixture(t, "rdf.xml", "https://example.org/feed.rdf")

	if doc.Type != "rdf" {
		t.Errorf("Type = %q, want rdf", doc.Type)
	}
	if doc.Title != "RDF Example" || doc.Link != "https://example.org/" {
		t.Errorf("feed = %+v", doc.Feed)
	}
	if doc.Description != "An RDF feed & channel." {
		t.Errorf("Description = %q", doc.Description)
	}
	if doc.Language != "en" {
		t.Errorf("Language = %q", doc.Language)
	}
	if doc.IconURL != "https://example.org/logo.png" {
		t.Errorf("IconURL = %q", doc.IconURL)
	}
	if len(doc.Items) != 2 {
		t.Fatalf("len(Items) = %d, want 2", len(doc.Items))
	}

	first := doc.Items[0]
	if first.GUID != "https://example.org/items/1" {
		t.Errorf("GUID = %q, want rdf:about", first.GUID)
	}
	if first.Author != "Bob" {
		t.Errorf("Author = %q", first.Author)
	}
	if first.Summary != "RDF summary." || !strings.Contains(first.Content, "<em>full</em>") {
		t.Errorf("summary/content = %q / %q", first.Summary, first.Content)
	}
	requireTime(t, "Published", first.Published, time.Date(2006, 1, 4, 5, 6, 7, 0, time.UTC))

	second := doc.Items[1]
	if second.GUID != "items/2" {
		t.Errorf("GUID = %q, want rdf:about text", second.GUID)
	}
	if second.Link != "https://example.org/items/2" {
		t.Errorf("Link = %q, want resolved rdf:about", second.Link)
	}
	requireTime(t, "Published", second.Published, time.Date(2006, 1, 5, 0, 0, 0, 0, time.UTC))
}

func TestParseAtom(t *testing.T) {
	doc := parseFixture(t, "atom.xml", "https://example.com/atom.xml")

	if doc.Type != "atom" {
		t.Errorf("Type = %q, want atom", doc.Type)
	}
	if doc.Title != "Tech & Stuff" {
		t.Errorf("Title = %q", doc.Title)
	}
	if doc.Link != "https://example.com/blog/" {
		t.Errorf("Link = %q, want text/html alternate", doc.Link)
	}
	if doc.Description != "News from the lab" {
		t.Errorf("Description = %q", doc.Description)
	}
	if doc.Language != "en" {
		t.Errorf("Language = %q", doc.Language)
	}
	if doc.IconURL != "https://example.com/favicon.ico" {
		t.Errorf("IconURL = %q", doc.IconURL)
	}
	if doc.Author != "Feed Author" {
		t.Errorf("Author = %q", doc.Author)
	}
	if len(doc.Items) != 3 {
		t.Fatalf("len(Items) = %d, want 3", len(doc.Items))
	}

	first := doc.Items[0]
	if first.GUID != "urn:uuid:1" {
		t.Errorf("GUID = %q", first.GUID)
	}
	if first.Link != "https://example.com/posts/one" {
		t.Errorf("Link = %q", first.Link)
	}
	if first.Title != "Post One" {
		t.Errorf("Title = %q", first.Title)
	}
	if first.Author != "Alice" {
		t.Errorf("Author = %q", first.Author)
	}
	if first.Summary != "Summary & more" {
		t.Errorf("Summary = %q", first.Summary)
	}
	if !strings.Contains(first.Content, "<strong>content</strong>") {
		t.Errorf("Content = %q, want xhtml markup", first.Content)
	}
	if first.ImageURL != "https://example.com/thumbs/1.jpg" {
		t.Errorf("ImageURL = %q", first.ImageURL)
	}
	requireTime(t, "Published", first.Published, time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC))

	second := doc.Items[1]
	if second.GUID != "https://example.com/posts/two" {
		t.Errorf("GUID = %q, want link fallback", second.GUID)
	}
	if second.Content != "5 &lt; 6 &amp; true" {
		t.Errorf("Content = %q, want escaped text content", second.Content)
	}
	if second.Summary != "Fallback summary." {
		t.Errorf("Summary = %q", second.Summary)
	}
	if second.Author != "Feed Author" {
		t.Errorf("Author = %q, want inherited feed author", second.Author)
	}
	if second.EnclosureURL != "https://example.com/files/podcast.mp3" {
		t.Errorf("EnclosureURL = %q, want rel=enclosure", second.EnclosureURL)
	}
	requireTime(t, "Published", second.Published, time.Date(2024, 3, 4, 3, 6, 7, 0, time.UTC))

	third := doc.Items[2]
	if third.Content != "See image." {
		t.Errorf("Content = %q", third.Content)
	}
	if third.ImageURL != "https://example.com/thumbs/3.jpg" {
		t.Errorf("ImageURL = %q", third.ImageURL)
	}
	if third.EnclosureURL != "https://cdn.example.com/pic.jpg" {
		t.Errorf("EnclosureURL = %q, want media:content", third.EnclosureURL)
	}
}

func TestParseAtom03(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="utf-8"?>
<feed version="0.3" xmlns="http://purl.org/atom/ns#" xml:lang="fr">
  <title>Old Atom</title>
  <tagline>Tagline text</tagline>
  <link rel="alternate" type="text/html" href="https://example.org/"/>
  <modified>2004-05-10T12:00:00Z</modified>
  <entry>
    <title>Old Entry</title>
    <link rel="alternate" type="text/html" href="https://example.org/1"/>
    <id>tag:example.org,2004:1</id>
    <issued>2004-05-10T10:00:00Z</issued>
    <modified>2004-05-10T11:00:00Z</modified>
    <content type="text/html" mode="escaped">&lt;p&gt;Old body&lt;/p&gt;</content>
  </entry>
</feed>`)
	doc, err := Parse(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Type != "atom" || doc.Description != "Tagline text" || doc.Language != "fr" {
		t.Errorf("feed = %+v", doc.Feed)
	}
	if len(doc.Items) != 1 {
		t.Fatalf("len(Items) = %d", len(doc.Items))
	}
	item := doc.Items[0]
	if item.Content != "<p>Old body</p>" {
		t.Errorf("Content = %q", item.Content)
	}
	requireTime(t, "Published", item.Published, time.Date(2004, 5, 10, 10, 0, 0, 0, time.UTC))
}

func TestParseCharsetGBK(t *testing.T) {
	source := `<?xml version="1.0" encoding="GBK"?>
<rss version="2.0"><channel>
  <title>中文标题</title>
  <description>简介</description>
  <item><title>条目</title><link>https://example.com/1</link></item>
</channel></rss>`
	encoded, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Parse(encoded, nil)
	if err != nil {
		t.Fatalf("Parse GBK: %v", err)
	}
	if doc.Title != "中文标题" || doc.Items[0].Title != "条目" {
		t.Errorf("decoded titles = %q / %q", doc.Title, doc.Items[0].Title)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{"html", `<!DOCTYPE html><html><head></head><body></body></html>`, "unsupported"},
		{"empty", "", "empty document"},
		{"junk", "not a feed", "root"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.data), nil)
			if err == nil {
				t.Fatal("want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestPlainText(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"<p>Hello <b>world</b></p>", "Hello world"},
		{"a<br>b", "a b"},
		{"Tom &amp; Jerry &nbsp; Show", "Tom & Jerry Show"},
		{"<script>alert(1)</script><p>Safe &lt;text&gt;</p>", "Safe <text>"},
		{"  one\n\n\ttwo  ", "one two"},
		{"<ul><li>one</li><li>two</li></ul>", "one two"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := PlainText(tt.in); got != tt.want {
			t.Errorf("PlainText(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
