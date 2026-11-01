package opml_test

import (
	"strings"
	"testing"

	"xin.dart.zrss/internal/opml"
)

const sample = `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <head><title>My feeds</title></head>
  <body>
    <outline text="Tech" title="Tech">
      <outline type="rss" text="Go Blog" title="The Go Blog" xmlUrl="https://go.dev/blog/feed.atom" htmlUrl="https://go.dev/blog" description="Go news"/>
      <outline text="Nested">
        <outline type="rss" text="LWN" xmlurl="https://lwn.net/headlines/rss" htmlurl="https://lwn.net/"/>
      </outline>
    </outline>
    <outline type="rss" text="Top level" xmlUrl="https://example.com/feed.xml"/>
    <outline text="Empty folder"/>
  </body>
</opml>`

func TestParse(t *testing.T) {
	doc, err := opml.Parse([]byte(sample))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if doc.Title != "My feeds" {
		t.Errorf("Title = %q, want My feeds", doc.Title)
	}
	if len(doc.Feeds) != 1 {
		t.Fatalf("top-level feeds = %v, want 1", doc.Feeds)
	}
	if got := doc.Feeds[0]; got.Title != "Top level" || got.URL != "https://example.com/feed.xml" {
		t.Errorf("top-level feed = %+v", got)
	}
	if len(doc.Folders) != 2 {
		t.Fatalf("folders = %+v, want 2", doc.Folders)
	}
	tech := doc.Folders[0]
	if tech.Name != "Tech" || len(tech.Feeds) != 1 || len(tech.Folders) != 1 {
		t.Fatalf("first folder = %+v", tech)
	}
	if got := tech.Feeds[0]; got.Title != "Go Blog" || got.URL != "https://go.dev/blog/feed.atom" ||
		got.SiteURL != "https://go.dev/blog" || got.Description != "Go news" {
		t.Errorf("feed = %+v", got)
	}
	nested := tech.Folders[0]
	if nested.Name != "Nested" || len(nested.Feeds) != 1 {
		t.Fatalf("nested folder = %+v", nested)
	}
	if got := nested.Feeds[0]; got.URL != "https://lwn.net/headlines/rss" || got.Title != "LWN" || got.SiteURL != "https://lwn.net/" {
		t.Errorf("nested feed = %+v", got)
	}
	if empty := doc.Folders[1]; empty.Name != "Empty folder" || len(empty.Feeds) != 0 || len(empty.Folders) != 0 {
		t.Errorf("empty folder = %+v", empty)
	}
}

func TestParseRejectsOtherDocuments(t *testing.T) {
	if _, err := opml.Parse([]byte(`<?xml version="1.0"?><rss><channel/></rss>`)); err == nil {
		t.Fatal("Parse accepted a non-OPML document")
	}
	if _, err := opml.Parse(nil); err == nil {
		t.Fatal("Parse accepted an empty document")
	}
	if _, err := opml.Parse([]byte(`<opml version="1.0"><body/></opml>`)); err != nil {
		t.Fatalf("Parse empty body: %v", err)
	}
}

func TestEncodeRoundTrip(t *testing.T) {
	doc := &opml.Document{
		Title: "A & B <feeds>",
		Feeds: []opml.Feed{{Title: "Loose", URL: "https://loose.example/rss", SiteURL: "https://loose.example"}},
		Folders: []opml.Folder{
			{Name: "News", Feeds: []opml.Feed{
				{Title: `Quote "friendly"`, URL: "https://news.example/feed?x=1&y=2", Description: "desc & more"},
			}},
			{Name: "", Feeds: []opml.Feed{{Title: "Also loose", URL: "https://also.example/rss"}}},
		},
	}
	data, err := opml.Encode(doc)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !strings.HasPrefix(string(data), "<?xml") {
		t.Fatalf("missing XML header: %s", data)
	}
	back, err := opml.Parse(data)
	if err != nil {
		t.Fatalf("Parse encoded: %v\n%s", err, data)
	}
	if back.Title != doc.Title {
		t.Errorf("Title = %q, want %q", back.Title, doc.Title)
	}
	if len(back.Feeds) != 2 {
		t.Fatalf("top-level feeds = %+v, want 2", back.Feeds)
	}
	if back.Feeds[0].Title != "Loose" || back.Feeds[1].Title != "Also loose" {
		t.Errorf("top-level feeds = %+v", back.Feeds)
	}
	if len(back.Folders) != 1 {
		t.Fatalf("folders = %+v, want 1", back.Folders)
	}
	f := back.Folders[0]
	if f.Name != "News" || len(f.Feeds) != 1 {
		t.Fatalf("folder = %+v", f)
	}
	if got := f.Feeds[0]; got.Title != `Quote "friendly"` || got.URL != "https://news.example/feed?x=1&y=2" || got.Description != "desc & more" {
		t.Errorf("feed = %+v", got)
	}
}

func TestEncodeNestedFolders(t *testing.T) {
	doc := &opml.Document{
		Title: "Nested",
		Folders: []opml.Folder{
			{Name: "Tech", Folders: []opml.Folder{
				{Name: "Go", Feeds: []opml.Feed{{Title: "Go feed", URL: "https://example.com/go"}},
					Folders: []opml.Folder{
						{Name: "Tools", Feeds: []opml.Feed{{Title: "Tool feed", URL: "https://example.com/tools"}}},
					}},
			}},
		},
	}
	data, err := opml.Encode(doc)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	back, err := opml.Parse(data)
	if err != nil {
		t.Fatalf("Parse encoded: %v\n%s", err, data)
	}
	if len(back.Folders) != 1 || back.Folders[0].Name != "Tech" {
		t.Fatalf("folders = %+v", back.Folders)
	}
	tech := back.Folders[0]
	if len(tech.Folders) != 1 || tech.Folders[0].Name != "Go" || len(tech.Folders[0].Feeds) != 1 {
		t.Fatalf("nested folder = %+v", tech)
	}
	goFolder := tech.Folders[0]
	if len(goFolder.Folders) != 1 || goFolder.Folders[0].Name != "Tools" {
		t.Fatalf("third level = %+v", goFolder.Folders)
	}
	if got := goFolder.Folders[0].Feeds; len(got) != 1 || got[0].URL != "https://example.com/tools" {
		t.Fatalf("third level feeds = %+v", got)
	}
}

func TestEncodeEmptyDocument(t *testing.T) {
	data, err := opml.Encode(&opml.Document{})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	doc, err := opml.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v\n%s", err, data)
	}
	if doc.Title != "Subscriptions" {
		t.Errorf("Title = %q, want Subscriptions", doc.Title)
	}
	if len(doc.Feeds) != 0 || len(doc.Folders) != 0 {
		t.Errorf("document = %+v, want empty", doc)
	}
}
