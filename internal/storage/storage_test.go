package storage_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"

	"xin.dart.zrss/internal/storage"
)

func openTest(t *testing.T) *storage.Store {
	t.Helper()
	s, err := storage.OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return s
}

func addFeed(t *testing.T, s *storage.Store, url, title string) int64 {
	t.Helper()
	id, created, err := s.AddFeed(context.Background(), storage.Feed{URL: url, Title: title})
	if err != nil {
		t.Fatalf("AddFeed(%q): %v", url, err)
	}
	if !created {
		t.Fatalf("AddFeed(%q) reported existing feed", url)
	}
	return id
}

func addFolder(t *testing.T, s *storage.Store, name string, parentID int64) int64 {
	t.Helper()
	id, err := s.CreateFolder(context.Background(), name, parentID)
	if err != nil {
		t.Fatalf("CreateFolder(%q, %d): %v", name, parentID, err)
	}
	return id
}

func upsert(t *testing.T, s *storage.Store, feedID int64, items ...storage.ArticleInput) int {
	t.Helper()
	n, err := s.UpsertArticles(context.Background(), feedID, items)
	if err != nil {
		t.Fatalf("UpsertArticles: %v", err)
	}
	return n
}

func article(title, link, plain string, published time.Time) storage.ArticleInput {
	return storage.ArticleInput{
		Title: title, URL: link, Plain: plain, Content: plain, Published: published,
	}
}

func TestUpsertKeepsStateAndUpdatesContent(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	feedID := addFeed(t, s, "https://example.com/feed", "Example")
	now := time.Now()

	if n := upsert(t, s, feedID, article("First", "https://example.com/1", "hello world", now)); n != 1 {
		t.Fatalf("first insert reported %d new articles, want 1", n)
	}
	if n := upsert(t, s, feedID, article("First", "https://example.com/1", "hello world", now)); n != 0 {
		t.Fatalf("repeat insert reported %d new articles, want 0", n)
	}

	list, err := s.Articles(ctx, storage.Scope{Kind: storage.ScopeAll}, 0, 10)
	if err != nil {
		t.Fatalf("Articles: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("got %d articles, want 1", len(list))
	}
	id := list[0].ID
	if err := s.SetRead(ctx, []int64{id}, true); err != nil {
		t.Fatalf("SetRead: %v", err)
	}
	if err := s.SetStarred(ctx, []int64{id}, true); err != nil {
		t.Fatalf("SetStarred: %v", err)
	}

	updated := article("First, edited", "https://example.com/1", "hello brave world", now)
	if n := upsert(t, s, feedID, updated); n != 0 {
		t.Fatalf("update reported %d new articles, want 0", n)
	}
	got, err := s.Article(ctx, id)
	if err != nil {
		t.Fatalf("Article: %v", err)
	}
	if got.Title != "First, edited" || got.Plain != "hello brave world" {
		t.Fatalf("article not updated: %+v", got)
	}
	if !got.Read || !got.Starred {
		t.Fatalf("read/star state lost: read=%v starred=%v", got.Read, got.Starred)
	}
	if got.FeedTitle != "Example" {
		t.Fatalf("FeedTitle = %q, want Example", got.FeedTitle)
	}
}

func TestScopesAndCounts(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	example := addFeed(t, s, "https://example.com/feed", "Example")
	other := addFeed(t, s, "https://other.example/feed", "Other")
	folderID := addFolder(t, s, "Tech", 0)
	if err := s.MoveFeed(ctx, other, folderID); err != nil {
		t.Fatalf("MoveFeed: %v", err)
	}

	now := time.Now()
	old := now.Add(-72 * time.Hour)
	upsert(t, s, example,
		article("A unread", "https://example.com/a", "alpha", now),
		article("B read", "https://example.com/b", "bravo", now.Add(-time.Hour)),
	)
	upsert(t, s, other, article("C old", "https://other.example/c", "charlie", old))

	list, err := s.Articles(ctx, storage.Scope{Kind: storage.ScopeAll}, 0, 10)
	if err != nil {
		t.Fatalf("Articles: %v", err)
	}
	if len(list) != 3 || list[0].Title != "A unread" {
		t.Fatalf("all scope = %v, want 3 newest first", titles(list))
	}
	if err := s.SetRead(ctx, []int64{list[1].ID}, true); err != nil {
		t.Fatalf("SetRead: %v", err)
	}
	if err := s.SetStarred(ctx, []int64{list[2].ID}, true); err != nil {
		t.Fatalf("SetStarred: %v", err)
	}

	cases := []struct {
		name  string
		scope storage.Scope
		want  int
	}{
		{"all", storage.Scope{Kind: storage.ScopeAll}, 3},
		{"unread", storage.Scope{Kind: storage.ScopeUnread}, 2},
		{"starred", storage.Scope{Kind: storage.ScopeStarred}, 1},
		{"today", storage.Scope{Kind: storage.ScopeToday}, 2},
		{"feed", storage.Scope{Kind: storage.ScopeFeed, ID: example}, 2},
		{"folder", storage.Scope{Kind: storage.ScopeFolder, ID: folderID}, 1},
	}
	for _, tc := range cases {
		got, err := s.Count(ctx, tc.scope)
		if err != nil {
			t.Fatalf("Count(%s): %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("Count(%s) = %d, want %d", tc.name, got, tc.want)
		}
	}

	counts, err := s.Counts(ctx)
	if err != nil {
		t.Fatalf("Counts: %v", err)
	}
	if counts.Unread != 2 || counts.Starred != 1 || counts.Today != 2 || counts.Total != 3 ||
		counts.Feeds != 2 || counts.Folders != 1 {
		t.Fatalf("Counts = %+v", counts)
	}

	feeds, err := s.Feeds(ctx)
	if err != nil {
		t.Fatalf("Feeds: %v", err)
	}
	if len(feeds) != 2 || feeds[0].Unread != 1 || feeds[1].Unread != 1 {
		t.Fatalf("feed unread counts = %+v", feeds)
	}
	if feeds[0].Total != 2 || feeds[1].Total != 1 {
		t.Fatalf("feed totals = %+v", feeds)
	}
}

func TestSearch(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	feedID := addFeed(t, s, "https://example.com/feed", "Example")
	now := time.Now()
	upsert(t, s, feedID,
		storage.ArticleInput{
			Title: "Learning Go", URL: "https://example.com/go",
			Plain: "Go is a programming language with goroutines.", Published: now,
		},
		storage.ArticleInput{
			Title: "今日新闻", URL: "https://example.com/news",
			Plain: "今天的头条新闻。", Published: now.Add(-time.Hour),
		},
	)

	cases := []struct {
		query string
		want  int
	}{
		{"golang", 0},
		{"goroutines", 1},
		{"programming language", 1},
		{"新闻", 1},    // shorter than a trigram, uses LIKE
		{"今日 新闻", 1}, // every short word must match somewhere
		{"missingword", 0},
	}
	for _, tc := range cases {
		n, err := s.Count(ctx, storage.Scope{Kind: storage.ScopeSearch, Query: tc.query})
		if err != nil {
			t.Fatalf("Count(search %q): %v", tc.query, err)
		}
		if n != tc.want {
			t.Errorf("search %q found %d, want %d", tc.query, n, tc.want)
		}
	}
}

func TestMarkScopeRead(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	feedID := addFeed(t, s, "https://example.com/feed", "Example")
	other := addFeed(t, s, "https://other.example/feed", "Other")
	now := time.Now()
	upsert(t, s, feedID,
		article("One", "https://example.com/1", "one", now),
		article("Two", "https://example.com/2", "two", now.Add(-time.Minute)),
	)
	upsert(t, s, other, article("Three", "https://other.example/3", "three", now))

	n, err := s.MarkScopeRead(ctx, storage.Scope{Kind: storage.ScopeFeed, ID: feedID})
	if err != nil {
		t.Fatalf("MarkScopeRead: %v", err)
	}
	if n != 2 {
		t.Fatalf("marked %d read, want 2", n)
	}
	if n, _ := s.Count(ctx, storage.Scope{Kind: storage.ScopeUnread}); n != 1 {
		t.Fatalf("unread after marking feed = %d, want 1", n)
	}
	n, err = s.MarkScopeRead(ctx, storage.Scope{Kind: storage.ScopeUnread})
	if err != nil || n != 1 {
		t.Fatalf("mark all unread = %d, %v; want 1", n, err)
	}
	if n, _ := s.Count(ctx, storage.Scope{Kind: storage.ScopeUnread}); n != 0 {
		t.Fatalf("unread after marking all = %d, want 0", n)
	}
}

func TestFolderTree(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	root := addFolder(t, s, "Tech", 0)
	child := addFolder(t, s, "Go", root)
	grandchild := addFolder(t, s, "Tools", child)

	// The same name is allowed under different parents.
	other := addFolder(t, s, "Go", 0)
	if other == child {
		t.Fatalf("Go at the top level reused the nested folder id %d", child)
	}
	// But a sibling with the same name is the same folder.
	if got := addFolder(t, s, "go", root); got != child {
		t.Fatalf("duplicate sibling returned %d, want %d", got, child)
	}
	// A fourth level is rejected.
	if _, err := s.CreateFolder(ctx, "Deep", grandchild); !errors.Is(err, storage.ErrFolderDepth) {
		t.Fatalf("fourth level error = %v, want ErrFolderDepth", err)
	}
	// A rename may not collide with a sibling.
	if err := s.RenameFolder(ctx, other, "Tech"); !errors.Is(err, storage.ErrFolderExists) {
		t.Fatalf("rename to sibling name error = %v, want ErrFolderExists", err)
	}

	folders, err := s.Folders(ctx)
	if err != nil {
		t.Fatalf("Folders: %v", err)
	}
	parents := map[int64]int64{}
	for _, f := range folders {
		parents[f.ID] = f.ParentID
	}
	if parents[root] != 0 || parents[child] != root || parents[grandchild] != child || parents[other] != 0 {
		t.Fatalf("folder parents = %v", parents)
	}
}

func TestFolderScopeIncludesDescendants(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	root := addFolder(t, s, "Tech", 0)
	child := addFolder(t, s, "Go", root)
	grandchild := addFolder(t, s, "Tools", child)
	outer := addFolder(t, s, "Other", 0)

	outerFeed := addFeed(t, s, "https://example.com/outer", "Outer")
	feed := addFeed(t, s, "https://example.com/deep", "Deep")
	if err := s.MoveFeed(ctx, outerFeed, outer); err != nil {
		t.Fatalf("MoveFeed outer: %v", err)
	}
	if err := s.MoveFeed(ctx, feed, grandchild); err != nil {
		t.Fatalf("MoveFeed deep: %v", err)
	}
	upsert(t, s, feed, article("Deep one", "https://example.com/1", "one", time.Now()))
	upsert(t, s, outerFeed, article("Outer one", "https://example.com/2", "two", time.Now()))

	cases := []struct {
		name string
		id   int64
		want int
	}{
		{"root", root, 1},
		{"child", child, 1},
		{"grandchild", grandchild, 1},
		{"other", outer, 1},
	}
	for _, tc := range cases {
		got, err := s.Count(ctx, storage.Scope{Kind: storage.ScopeFolder, ID: tc.id})
		if err != nil {
			t.Fatalf("Count(%s): %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("Count(%s) = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestMoveFolderChecksCycleDepthAndNames(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	a := addFolder(t, s, "A", 0)
	b := addFolder(t, s, "B", a)
	c := addFolder(t, s, "C", b)
	d := addFolder(t, s, "D", 0)

	if err := s.MoveFolder(ctx, a, c); !errors.Is(err, storage.ErrFolderCycle) {
		t.Fatalf("move into descendant error = %v, want ErrFolderCycle", err)
	}
	if err := s.MoveFolder(ctx, a, a); !errors.Is(err, storage.ErrFolderCycle) {
		t.Fatalf("move into itself error = %v, want ErrFolderCycle", err)
	}
	// A has height 3, so putting it under D would make four levels.
	if err := s.MoveFolder(ctx, a, d); !errors.Is(err, storage.ErrFolderDepth) {
		t.Fatalf("move too deep error = %v, want ErrFolderDepth", err)
	}
	// B has height 2 and fits under D.
	if err := s.MoveFolder(ctx, b, d); err != nil {
		t.Fatalf("move B under D: %v", err)
	}
	// Moving a folder beside a sibling of the same name is refused.
	x := addFolder(t, s, "X", d)
	y := addFolder(t, s, "X", 0)
	if err := s.MoveFolder(ctx, y, d); !errors.Is(err, storage.ErrFolderExists) {
		t.Fatalf("move name conflict error = %v, want ErrFolderExists (x=%d)", err, x)
	}

	feed := addFeed(t, s, "https://example.com/feed", "Example")
	if err := s.MoveFeed(ctx, feed, c); err != nil {
		t.Fatalf("MoveFeed to moved folder: %v", err)
	}
	if got, _ := s.Feed(ctx, feed); got.FolderID != c {
		t.Fatalf("feed folder = %d, want %d", got.FolderID, c)
	}
}

func TestDeleteFolderSubtreePromotesFeeds(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	root := addFolder(t, s, "Tech", 0)
	child := addFolder(t, s, "Go", root)
	grandchild := addFolder(t, s, "Tools", child)

	feedInChild := addFeed(t, s, "https://example.com/child", "Child")
	feedInGrandchild := addFeed(t, s, "https://example.com/grand", "Grand")
	list := []int64{feedInChild, feedInGrandchild}
	folders := []int64{child, grandchild}
	for _, id := range list {
		if err := s.MoveFeed(ctx, id, folders[0]); err != nil {
			t.Fatalf("MoveFeed(%d): %v", id, err)
		}
	}
	if err := s.MoveFeed(ctx, feedInGrandchild, grandchild); err != nil {
		t.Fatalf("MoveFeed grandchild: %v", err)
	}
	upsert(t, s, feedInChild, article("Kept child", "https://example.com/child/1", "text", time.Now()))
	upsert(t, s, feedInGrandchild, article("Kept grand", "https://example.com/grand/1", "text", time.Now()))

	if err := s.DeleteFolder(ctx, child); err != nil {
		t.Fatalf("DeleteFolder: %v", err)
	}
	for _, id := range list {
		feed, err := s.Feed(ctx, id)
		if err != nil {
			t.Fatalf("Feed(%d) after delete: %v", id, err)
		}
		if feed.FolderID != root {
			t.Fatalf("feed %d folder = %d, want parent %d", id, feed.FolderID, root)
		}
	}
	if n, _ := s.Count(ctx, storage.Scope{Kind: storage.ScopeAll}); n != 2 {
		t.Fatalf("articles after delete = %d, want 2", n)
	}
	remaining, err := s.Folders(ctx)
	if err != nil || len(remaining) != 1 || remaining[0].ID != root {
		t.Fatalf("folders after delete = %+v, %v", remaining, err)
	}
}

func TestMigrateV1Folders(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "zrss.db")
	db, err := sqlite.Open(ctx, path, sqlite.OpenOptions{})
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	for _, stmt := range []string{
		`CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE folders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL COLLATE NOCASE UNIQUE,
			sort INTEGER NOT NULL DEFAULT 0
		)`,
		`INSERT INTO meta(key, value) VALUES('version', '1')`,
		`INSERT INTO folders(name, sort) VALUES('Tech', 1)`,
	} {
		if _, err := db.Execute(ctx, stmt); err != nil {
			db.Close()
			t.Fatalf("prepare v1 schema: %v", err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close v1 database: %v", err)
	}

	s, err := storage.Open(ctx, path)
	if err != nil {
		t.Fatalf("storage.Open migrated database: %v", err)
	}
	defer s.Close()

	folders, err := s.Folders(ctx)
	if err != nil {
		t.Fatalf("Folders: %v", err)
	}
	if len(folders) != 1 || folders[0].ID != 1 || folders[0].ParentID != 0 || folders[0].Name != "Tech" {
		t.Fatalf("migrated folders = %+v", folders)
	}
	child, err := s.CreateFolder(ctx, "Tech", folders[0].ID)
	if err != nil {
		t.Fatalf("nested folder after migration: %v", err)
	}
	if child == folders[0].ID {
		t.Fatal("nested folder reused the root id")
	}
}

func TestDeleteFeedRemovesArticlesFromSearch(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	feedID := addFeed(t, s, "https://example.com/feed", "Example")
	upsert(t, s, feedID, article("Goroutines", "https://example.com/go", "goroutines everywhere", time.Now()))
	if err := s.DeleteFeed(ctx, feedID); err != nil {
		t.Fatalf("DeleteFeed: %v", err)
	}
	if n, _ := s.Count(ctx, storage.Scope{Kind: storage.ScopeAll}); n != 0 {
		t.Fatalf("articles after feed delete = %d, want 0", n)
	}
	if n, _ := s.Count(ctx, storage.Scope{Kind: storage.ScopeSearch, Query: "goroutines"}); n != 0 {
		t.Fatalf("search after feed delete = %d, want 0", n)
	}
}

func TestPruneKeepsUnreadAndStarred(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	feedID := addFeed(t, s, "https://example.com/feed", "Example")
	now := time.Now()
	upsert(t, s, feedID,
		article("Oldest", "https://example.com/1", "one", now.Add(-4*time.Hour)),
		article("Older", "https://example.com/2", "two", now.Add(-3*time.Hour)),
		article("Newer", "https://example.com/3", "three", now.Add(-2*time.Hour)),
		article("Newest", "https://example.com/4", "four", now.Add(-time.Hour)),
	)
	list, err := s.Articles(ctx, storage.Scope{Kind: storage.ScopeAll}, 0, 10)
	if err != nil {
		t.Fatalf("Articles: %v", err)
	}
	// list is newest first: Newest, Newer, Older, Oldest
	ids := []int64{list[0].ID, list[1].ID, list[2].ID, list[3].ID}
	if err := s.SetRead(ctx, ids, true); err != nil {
		t.Fatalf("SetRead: %v", err)
	}
	if err := s.SetStarred(ctx, []int64{ids[3]}, true); err != nil {
		t.Fatalf("SetStarred: %v", err)
	}

	n, err := s.Prune(ctx, 2)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if n != 1 {
		t.Fatalf("pruned %d, want 1", n)
	}
	remaining, err := s.Articles(ctx, storage.Scope{Kind: storage.ScopeAll}, 0, 10)
	if err != nil {
		t.Fatalf("Articles: %v", err)
	}
	if len(remaining) != 3 {
		t.Fatalf("remaining = %v, want 3", titles(remaining))
	}
	for _, a := range remaining {
		if a.Title == "Older" {
			t.Fatalf("Older should have been pruned: %v", titles(remaining))
		}
	}
}

func TestSettingsAndSearches(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	v, err := s.Setting(ctx, "theme", "system")
	if err != nil || v != "system" {
		t.Fatalf("Setting default = %q, %v", v, err)
	}
	if err := s.SetSetting(ctx, "theme", "dark"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	if v, _ := s.Setting(ctx, "theme", "system"); v != "dark" {
		t.Fatalf("Setting = %q, want dark", v)
	}
	all, err := s.Settings(ctx)
	if err != nil || all["theme"] != "dark" {
		t.Fatalf("Settings = %v, %v", all, err)
	}
	if err := s.DeleteSetting(ctx, "theme"); err != nil {
		t.Fatalf("DeleteSetting: %v", err)
	}
	if v, _ := s.Setting(ctx, "theme", "system"); v != "system" {
		t.Fatalf("Setting after delete = %q, want system", v)
	}

	for _, q := range []string{"go", "news", "golang"} {
		if err := s.AddSearch(ctx, q); err != nil {
			t.Fatalf("AddSearch(%q): %v", q, err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	recent, err := s.RecentSearches(ctx, 2)
	if err != nil {
		t.Fatalf("RecentSearches: %v", err)
	}
	if len(recent) != 2 || recent[0] != "golang" || recent[1] != "news" {
		t.Fatalf("RecentSearches = %v", recent)
	}
	if err := s.AddSearch(ctx, "go"); err != nil {
		t.Fatalf("AddSearch again: %v", err)
	}
	recent, _ = s.RecentSearches(ctx, 2)
	if len(recent) != 2 || recent[0] != "go" || recent[1] != "golang" {
		t.Fatalf("RecentSearches after repeat = %v", recent)
	}
	if err := s.ClearSearches(ctx); err != nil {
		t.Fatalf("ClearSearches: %v", err)
	}
	if recent, _ := s.RecentSearches(ctx, 2); len(recent) != 0 {
		t.Fatalf("RecentSearches after clear = %v", recent)
	}
}

func TestNeighbours(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	feedID := addFeed(t, s, "https://example.com/feed", "Example")
	now := time.Now()
	upsert(t, s, feedID,
		article("One", "https://example.com/1", "one", now),
		article("Two", "https://example.com/2", "two", now.Add(-time.Minute)),
		article("Three", "https://example.com/3", "three", now.Add(-2*time.Minute)),
	)
	list, err := s.Articles(ctx, storage.Scope{Kind: storage.ScopeAll}, 0, 10)
	if err != nil {
		t.Fatalf("Articles: %v", err)
	}
	// Newest first: One, Two, Three.
	prev, next, err := s.Neighbours(ctx, storage.Scope{Kind: storage.ScopeAll}, list[1].ID)
	if err != nil {
		t.Fatalf("Neighbours: %v", err)
	}
	if prev != list[0].ID || next != list[2].ID {
		t.Fatalf("Neighbours = (%d, %d), want (%d, %d)", prev, next, list[0].ID, list[2].ID)
	}
	prev, next, err = s.Neighbours(ctx, storage.Scope{Kind: storage.ScopeAll}, list[0].ID)
	if err != nil {
		t.Fatalf("Neighbours first: %v", err)
	}
	if prev != 0 || next != list[1].ID {
		t.Fatalf("Neighbours first = (%d, %d)", prev, next)
	}
}

func titles(list []storage.Article) []string {
	out := make([]string, 0, len(list))
	for _, a := range list {
		out = append(out, a.Title)
	}
	return out
}
