package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"xin.dart.zrss/internal/feed"
	"xin.dart.zrss/internal/opml"
	"xin.dart.zrss/internal/storage"
)

func newTestApp(t *testing.T) (*app, context.Context) {
	t.Helper()
	store, err := storage.OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	a := &app{
		store:     store,
		fetcher:   feed.NewFetcher(),
		settings:  defaultSettings(),
		selected:  -1,
		collapsed: map[int64]bool{},
		done:      make(chan struct{}),
	}
	a.imgs = newImageCache(func(fn func()) { fn() })
	return a, context.Background()
}

func addTestFeed(t *testing.T, a *app, ctx context.Context, url, title string) int64 {
	t.Helper()
	id, created, err := a.store.AddFeed(ctx, storage.Feed{URL: url, Title: title})
	if err != nil || !created {
		t.Fatalf("AddFeed(%q): created=%v err=%v", url, created, err)
	}
	return id
}

func addTestArticles(t *testing.T, a *app, ctx context.Context, feedID int64, titles ...string) {
	t.Helper()
	items := make([]storage.ArticleInput, 0, len(titles))
	now := time.Now()
	for i, title := range titles {
		items = append(items, storage.ArticleInput{
			GUID:      fmt.Sprintf("%s-%d", title, i),
			URL:       "https://example.com/" + title,
			Title:     title,
			Summary:   "Summary of " + title,
			Content:   "# " + title + "\n\nBody of " + title,
			Plain:     "Body of " + title,
			Published: now.Add(-time.Duration(i) * time.Minute),
		})
	}
	if _, err := a.store.UpsertArticles(ctx, feedID, items); err != nil {
		t.Fatalf("UpsertArticles: %v", err)
	}
}

func TestArticleInputsConvertHTMLAndResolveLinks(t *testing.T) {
	doc := &feed.Document{
		Feed: feed.Feed{Title: "Example"},
		Items: []feed.Item{
			{
				GUID:    "one",
				Link:    "https://example.com/posts/one",
				Title:   "First post",
				Content: `<h1>Hello</h1><p>Read <a href="/more">more</a>.</p>`,
			},
			{
				GUID:    "two",
				Link:    "https://example.com/posts/two",
				Summary: `<p>Only a summary here.</p>`,
			},
		},
	}
	out := articleInputs(doc, "https://example.com/feed.xml")
	if len(out) != 2 {
		t.Fatalf("articleInputs returned %d items, want 2", len(out))
	}
	if !strings.Contains(out[0].Content, "Hello") || !strings.Contains(out[0].Content, "https://example.com/more") {
		t.Errorf("content did not become Markdown with an absolute link: %q", out[0].Content)
	}
	if strings.Contains(out[0].Content, "<h1>") {
		t.Errorf("content still holds HTML: %q", out[0].Content)
	}
	if out[1].Title != "无标题" {
		t.Errorf("untitled item is %q, want 无标题", out[1].Title)
	}
	if !strings.Contains(out[1].Content, "Only a summary here") || !strings.Contains(out[1].Plain, "Only a summary here") {
		t.Errorf("summary fallback missing: content=%q plain=%q", out[1].Content, out[1].Plain)
	}
}

func TestLayoutSettingIsSaved(t *testing.T) {
	a, ctx := newTestApp(t)
	if a.settings.Layout != layoutAll {
		t.Fatalf("default layout = %q, want %q", a.settings.Layout, layoutAll)
	}
	for _, layout := range []string{layoutSplit, layoutReader, layoutAll} {
		a.settings.Layout = layout
		if err := saveSettings(ctx, a.store, a.settings); err != nil {
			t.Fatalf("saveSettings(%q): %v", layout, err)
		}
		if got, err := a.store.Setting(ctx, "layout", ""); err != nil || got != layout {
			t.Errorf("saved layout = %q, %v; want %q", got, err, layout)
		}
	}
}

func TestMoveSelectionStopsAtBothEnds(t *testing.T) {
	a, ctx := newTestApp(t)
	feedID := addTestFeed(t, a, ctx, "https://example.com/feed", "Example")
	addTestArticles(t, a, ctx, feedID, "One", "Two", "Three")
	a.scope = storage.Scope{Kind: storage.ScopeAll}
	a.reloadSidebar(ctx)
	a.reloadArticles(ctx)
	a.selected = -1
	a.hasCurrent = false

	a.moveSelection(ctx, 1)
	if !a.hasCurrent || a.selected != 0 {
		t.Fatalf("first step selected %d (current %v), want 0", a.selected, a.hasCurrent)
	}
	a.moveSelection(ctx, 1)
	a.moveSelection(ctx, 1)
	if a.selected != 2 {
		t.Fatalf("stepping past the end selected %d, want 2", a.selected)
	}
	a.moveSelection(ctx, 1)
	if a.selected != 2 {
		t.Errorf("a step past the last article moved to %d", a.selected)
	}
	a.moveSelection(ctx, -1)
	if a.selected != 1 {
		t.Fatalf("stepping back selected %d, want 1", a.selected)
	}
	a.moveSelection(ctx, -1)
	a.moveSelection(ctx, -1)
	if a.selected != 0 {
		t.Errorf("stepping before the first article selected %d, want 0", a.selected)
	}
}

func TestScopeEncodingRoundTrip(t *testing.T) {
	cases := []struct {
		scope storage.Scope
		saved string
	}{
		{storage.Scope{Kind: storage.ScopeAll}, "all"},
		{storage.Scope{Kind: storage.ScopeUnread}, "unread"},
		{storage.Scope{Kind: storage.ScopeStarred}, "starred"},
		{storage.Scope{Kind: storage.ScopeToday}, "today"},
		{storage.Scope{Kind: storage.ScopeFeed, ID: 12}, "feed:12"},
		{storage.Scope{Kind: storage.ScopeFolder, ID: 7}, "folder:7"},
		{storage.Scope{Kind: storage.ScopeSearch, Query: "go"}, "all"},
	}
	for _, c := range cases {
		if got := encodeScope(c.scope); got != c.saved {
			t.Errorf("encodeScope(%+v) = %q, want %q", c.scope, got, c.saved)
		}
	}
	for _, saved := range []string{"unread", "starred", "today", "feed:12", "folder:7", "all", "search:go"} {
		got := decodeScope(saved)
		if saved == "all" || saved == "search:go" {
			if got.Kind != storage.ScopeAll {
				t.Errorf("decodeScope(%q) = %+v, want all", saved, got)
			}
			continue
		}
		if encodeScope(got) != saved {
			t.Errorf("decodeScope(%q) round trips to %q", saved, encodeScope(got))
		}
	}
}

func TestSelectingArticleMarksItReadAndUpdatesCounts(t *testing.T) {
	a, ctx := newTestApp(t)
	feedID := addTestFeed(t, a, ctx, "https://example.com/feed", "Example")
	addTestArticles(t, a, ctx, feedID, "One", "Two")
	a.scope = storage.Scope{Kind: storage.ScopeAll}
	a.reloadSidebar(ctx)
	a.reloadArticles(ctx)
	if a.counts.Unread != 2 {
		t.Fatalf("unread before reading = %d, want 2", a.counts.Unread)
	}

	a.selectArticleAt(ctx, 0)
	if !a.current.Read || !a.articles[0].Read {
		t.Fatal("the selected article was not marked read")
	}
	if a.counts.Unread != 1 {
		t.Errorf("unread after reading = %d, want 1", a.counts.Unread)
	}
	stored, err := a.store.Article(ctx, a.current.ID)
	if err != nil || !stored.Read {
		t.Errorf("stored article read = %v, err = %v", stored.Read, err)
	}

	a.toggleStarCurrent(ctx)
	if !a.current.Starred || a.counts.Starred != 1 {
		t.Errorf("star state = %v, starred count = %d", a.current.Starred, a.counts.Starred)
	}
	a.toggleReadCurrent(ctx)
	if a.current.Read || a.counts.Unread != 2 {
		t.Errorf("after marking unread: read=%v unread=%d", a.current.Read, a.counts.Unread)
	}
}

func TestLiveSearchNarrowsTheListWithoutTouchingTheSavedScope(t *testing.T) {
	a, ctx := newTestApp(t)
	feedID := addTestFeed(t, a, ctx, "https://example.com/feed", "Example")
	addTestArticles(t, a, ctx, feedID, "Go feeds", "Cooking")
	a.scope = storage.Scope{Kind: storage.ScopeAll}
	a.reloadSidebar(ctx)
	a.reloadArticles(ctx)

	a.query = "go feed"
	a.applySearch()
	if a.scope.Kind != storage.ScopeSearch {
		t.Fatalf("scope after searching = %q, want search", a.scope.Kind)
	}
	if len(a.articles) != 1 || a.articles[0].Title != "Go feeds" {
		t.Fatalf("search returned %d articles: %+v", len(a.articles), a.articles)
	}
	a.reloadSidebar(ctx)
	if a.counts.Unread != 2 {
		t.Errorf("live search left the unread count at %d, want 2", a.counts.Unread)
	}
	a.clearSearch()
	if a.scope.Kind != storage.ScopeAll || len(a.articles) != 2 {
		t.Errorf("clearing the search left scope=%q with %d articles", a.scope.Kind, len(a.articles))
	}
}

type feedServer struct {
	mu          sync.Mutex
	etag        string
	body        string
	requests    int
	conditional int
}

func (s *feedServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests++
	if r.Header.Get("If-None-Match") != "" {
		s.conditional++
	}
	if s.etag != "" && r.Header.Get("If-None-Match") == s.etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/rss+xml")
	w.Header().Set("ETag", s.etag)
	_, _ = w.Write([]byte(s.body))
}

func rssBody(items ...string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Example feed</title><link>https://example.com</link><description>Example</description>` +
		strings.Join(items, "") + `</channel></rss>`
}

func rssItem(guid, title string) string {
	return `<item><guid>` + guid + `</guid><title>` + title + `</title><link>https://example.com/` + guid + `</link>` +
		`<description>&lt;p&gt;` + title + ` body&lt;/p&gt;</description>` +
		`<pubDate>Tue, 01 Jan 2019 00:00:00 GMT</pubDate></item>`
}

func TestRefreshUsesConditionalGETAndKeepsReadState(t *testing.T) {
	srv := &feedServer{
		etag: `"v1"`,
		body: rssBody(rssItem("one", "One"), rssItem("two", "Two")),
	}
	ts := httptest.NewServer(srv)
	defer ts.Close()

	a, ctx := newTestApp(t)
	id, created, err := a.store.AddFeed(ctx, storage.Feed{URL: ts.URL, Title: "Example"})
	if err != nil || !created {
		t.Fatalf("AddFeed: created=%v err=%v", created, err)
	}
	f, _ := a.store.Feed(ctx, id)
	n, err := a.refreshFeed(ctx, f)
	if err != nil || n != 2 {
		t.Fatalf("first refresh: new=%d err=%v", n, err)
	}
	rows, err := a.store.Articles(ctx, storage.Scope{Kind: storage.ScopeAll}, 0, 10)
	if err != nil || len(rows) != 2 {
		t.Fatalf("articles after first refresh = %d, err=%v", len(rows), err)
	}
	first := rows[0].ID
	if err := a.store.SetRead(ctx, []int64{first}, true); err != nil {
		t.Fatal(err)
	}
	if err := a.store.SetStarred(ctx, []int64{first}, true); err != nil {
		t.Fatal(err)
	}

	f, _ = a.store.Feed(ctx, id)
	if f.ETag != `"v1"` {
		t.Fatalf("stored ETag = %q, want v1", f.ETag)
	}
	n, err = a.refreshFeed(ctx, f)
	if err != nil || n != 0 {
		t.Fatalf("unchanged refresh: new=%d err=%v", n, err)
	}
	if srv.conditional == 0 {
		t.Error("the second request did not send If-None-Match")
	}

	srv.mu.Lock()
	srv.etag = `"v2"`
	srv.body = rssBody(rssItem("one", "One"), rssItem("two", "Two"), rssItem("three", "Three"))
	srv.mu.Unlock()
	f, _ = a.store.Feed(ctx, id)
	n, err = a.refreshFeed(ctx, f)
	if err != nil || n != 1 {
		t.Fatalf("changed refresh: new=%d err=%v", n, err)
	}
	stored, err := a.store.Article(ctx, first)
	if err != nil || !stored.Read || !stored.Starred {
		t.Errorf("read/star state was lost: read=%v starred=%v err=%v", stored.Read, stored.Starred, err)
	}
	total, _ := a.store.Count(ctx, storage.Scope{Kind: storage.ScopeAll})
	if total != 3 {
		t.Errorf("article count after third refresh = %d, want 3", total)
	}
}

func TestRefreshFeedFetchesAgainWhenNoArticlesAreStored(t *testing.T) {
	srv := &feedServer{
		etag: `W/"v1"`,
		body: rssBody(rssItem("one", "One"), rssItem("two", "Two")),
	}
	ts := httptest.NewServer(srv)
	defer ts.Close()

	a, ctx := newTestApp(t)
	id, created, err := a.store.AddFeed(ctx, storage.Feed{URL: ts.URL, Title: "Stale"})
	if err != nil || !created {
		t.Fatalf("AddFeed: created=%v err=%v", created, err)
	}
	if err := a.store.UpdateFeed(ctx, id, storage.FeedMeta{
		Title: "Stale", ETag: `W/"v1"`, LastFetch: time.Now(),
	}); err != nil {
		t.Fatalf("UpdateFeed: %v", err)
	}
	f, err := a.store.Feed(ctx, id)
	if err != nil {
		t.Fatalf("Feed: %v", err)
	}
	if f.Total != 0 || f.ETag != `W/"v1"` {
		t.Fatalf("prepared feed = total %d etag %q", f.Total, f.ETag)
	}

	n, err := a.refreshFeed(ctx, f)
	if err != nil {
		t.Fatalf("refreshFeed: %v", err)
	}
	if n != 2 {
		t.Fatalf("new articles = %d, want 2; a stored validator hid the feed", n)
	}
	if srv.conditional != 0 {
		t.Error("the request was conditional although no articles were stored")
	}
	f, _ = a.store.Feed(ctx, id)
	if f.Total != 2 {
		t.Errorf("feed total = %d, want 2", f.Total)
	}
}

func TestResolveFeedFindsLinkedFeed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/feed.xml" {
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(rssBody(rssItem("one", "One"))))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><link rel="alternate" type="application/rss+xml" href="/feed.xml"></head><body>site</body></html>`))
	}))
	defer ts.Close()

	a, ctx := newTestApp(t)
	f, ok, err := a.resolveFeed(ctx, ts.URL)
	if err != nil {
		t.Fatalf("resolveFeed: %v", err)
	}
	if !ok {
		t.Fatal("resolveFeed did not find the linked feed")
	}
	if !strings.HasSuffix(f.URL, "/feed.xml") {
		t.Errorf("resolved URL = %q, want the linked feed", f.URL)
	}
	if f.Title != "Example feed" {
		t.Errorf("resolved title = %q", f.Title)
	}
}

func TestReloadSidebarCreatesDefaultFolderAndAdoptsRootFeeds(t *testing.T) {
	a, ctx := newTestApp(t)
	feedID := addTestFeed(t, a, ctx, "https://example.com/root", "Root")
	a.reloadSidebar(ctx)
	if a.defaultFolder == 0 {
		t.Fatal("the default folder was not created")
	}
	var name string
	for _, f := range a.folders {
		if f.ID == a.defaultFolder {
			name = f.Name
		}
	}
	if name != "默认" {
		t.Fatalf("default folder name = %q, want 默认", name)
	}
	stored, err := a.store.Feed(ctx, feedID)
	if err != nil {
		t.Fatalf("Feed: %v", err)
	}
	if stored.FolderID != a.defaultFolder {
		t.Fatalf("root feed folder = %d, want the default folder %d", stored.FolderID, a.defaultFolder)
	}
}

func TestAddOPMLKeepsFoldersAndSkipsExistingFeeds(t *testing.T) {
	a, ctx := newTestApp(t)
	addTestFeed(t, a, ctx, "https://example.com/existing", "Existing")

	doc := &opml.Document{
		Title: "Subscriptions",
		Feeds: []opml.Feed{{Title: "Top", URL: "https://example.com/top"}},
		Folders: []opml.Folder{
			{Name: "Tech", Feeds: []opml.Feed{{Title: "Tech feed", URL: "https://example.com/tech"}},
				Folders: []opml.Folder{
					{Name: "Go", Feeds: []opml.Feed{
						{Title: "Go feed", URL: "https://example.com/go"},
						{Title: "Again", URL: "https://example.com/top"},
					}},
				}},
		},
	}
	added, skipped := a.addOPML(ctx, doc)
	if added != 3 {
		t.Errorf("added = %d, want 3", added)
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1 for the repeated URL", skipped)
	}
	a.reloadSidebar(ctx)
	if len(a.feeds) != 4 {
		t.Fatalf("feeds after import = %d, want 4", len(a.feeds))
	}
	var techID, goID, goParent int64
	for _, f := range a.folders {
		switch f.Name {
		case "Tech":
			techID = f.ID
		case "Go":
			goID = f.ID
			goParent = f.ParentID
		}
	}
	if techID == 0 || goID == 0 || goParent != techID {
		t.Fatalf("nested OPML folders = Tech %d, Go %d (parent %d)", techID, goID, goParent)
	}
	found := false
	for _, f := range a.feeds {
		if f.URL == "https://example.com/go" {
			found = f.FolderID == goID
		}
	}
	if !found {
		t.Error("the Go feed was not put in its folder")
	}
	if a.defaultFolder == 0 {
		t.Fatal("the default folder was not created")
	}
	for _, f := range a.feeds {
		if f.URL == "https://example.com/top" || f.URL == "https://example.com/existing" {
			if f.FolderID != a.defaultFolder {
				t.Errorf("feed %q folder = %d, want the default folder %d", f.URL, f.FolderID, a.defaultFolder)
			}
		}
	}
}
