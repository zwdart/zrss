package storage

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/plugins/sqlite"
)

// Scope selects the articles a list shows. Smart views, feeds, folders and
// search results all become one of these.
type Scope struct {
	Kind  ScopeKind
	ID    int64  // feed or folder id
	Query string // search text
}

// ScopeKind names a smart view or a container.
type ScopeKind string

const (
	// ScopeAll is every article, newest first.
	ScopeAll ScopeKind = "all"
	// ScopeUnread is every unread article.
	ScopeUnread ScopeKind = "unread"
	// ScopeStarred is every starred article.
	ScopeStarred ScopeKind = "starred"
	// ScopeToday is everything published in the last day.
	ScopeToday ScopeKind = "today"
	// ScopeFeed is one subscription's articles.
	ScopeFeed ScopeKind = "feed"
	// ScopeFolder is every article in a folder's feeds.
	ScopeFolder ScopeKind = "folder"
	// ScopeSearch is a full-text query.
	ScopeSearch ScopeKind = "search"
)

// Article is one item as the reader shows it. Content and Summary are
// Markdown, converted when the item was stored.
type Article struct {
	ID        int64
	FeedID    int64
	FeedTitle string
	GUID      string
	URL       string
	Title     string
	Author    string
	Summary   string
	Content   string
	Plain     string
	ImageURL  string
	Published time.Time
	Fetched   time.Time
	Read      bool
	Starred   bool
}

// ArticleInput is a parsed item on its way into the database.
type ArticleInput struct {
	GUID      string
	URL       string
	Title     string
	Author    string
	Summary   string
	Content   string
	Plain     string
	ImageURL  string
	Published time.Time
}

// Counts are the badge numbers for the smart views.
type Counts struct {
	Unread   int
	Starred  int
	Today    int
	Total    int
	Feeds    int
	Folders  int
	Searches int
}

const articleColumns = `
	a.id, a.feed_id, f.title, a.guid, a.url, a.title, a.author, a.summary,
	a.content, a.plain, a.image_url, a.published, a.fetched, a.read, a.starred`

const articleFrom = `
	FROM articles a JOIN feeds f ON f.id = a.feed_id`

// Articles returns one page of a scope, newest first.
func (s *Store) Articles(ctx context.Context, scope Scope, offset, limit int) ([]Article, error) {
	where, args, err := scopeWhere(scope)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	args = append(args, limit, offset)
	rows, err := s.query(ctx, `SELECT`+articleColumns+articleFrom+`
		WHERE `+where+`
		ORDER BY a.published DESC, a.id DESC
		LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, err
	}
	out := make([]Article, 0, len(rows))
	for _, r := range rows {
		out = append(out, articleRow(r))
	}
	return out, nil
}

// Count returns how many articles a scope holds.
func (s *Store) Count(ctx context.Context, scope Scope) (int, error) {
	where, args, err := scopeWhere(scope)
	if err != nil {
		return 0, err
	}
	rows, err := s.query(ctx, `SELECT COUNT(*)`+articleFrom+` WHERE `+where, args...)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return int(intOf(rows[0][0])), nil
}

// Article returns one article with its feed's title.
func (s *Store) Article(ctx context.Context, id int64) (Article, error) {
	rows, err := s.query(ctx, `SELECT`+articleColumns+articleFrom+` WHERE a.id = ?`, id)
	if err != nil {
		return Article{}, err
	}
	if len(rows) == 0 {
		return Article{}, ErrNotFound
	}
	return articleRow(rows[0]), nil
}

// Neighbours returns the articles just above and below id in the order of
// scope, for keyboard navigation. Either may be zero at the ends of a list.
func (s *Store) Neighbours(ctx context.Context, scope Scope, id int64) (prev, next int64, err error) {
	where, args, err := scopeWhere(scope)
	if err != nil {
		return 0, 0, err
	}
	target, err := s.Article(ctx, id)
	if err != nil {
		return 0, 0, err
	}
	newerArgs := append(append([]any{}, args...), unix(target.Published), unix(target.Published), target.ID)
	rows, err := s.query(ctx, `SELECT a.id`+articleFrom+`
		WHERE `+where+` AND (a.published > ? OR (a.published = ? AND a.id > ?))
		ORDER BY a.published ASC, a.id ASC LIMIT 1`, newerArgs...)
	if err != nil {
		return 0, 0, err
	}
	if len(rows) > 0 {
		prev = intOf(rows[0][0])
	}
	olderArgs := append(append([]any{}, args...), unix(target.Published), unix(target.Published), target.ID)
	rows, err = s.query(ctx, `SELECT a.id`+articleFrom+`
		WHERE `+where+` AND (a.published < ? OR (a.published = ? AND a.id < ?))
		ORDER BY a.published DESC, a.id DESC LIMIT 1`, olderArgs...)
	if err != nil {
		return 0, 0, err
	}
	if len(rows) > 0 {
		next = intOf(rows[0][0])
	}
	return prev, next, nil
}

// UpsertArticles stores parsed items, keeping read and star state of items
// already known. It returns how many articles were new.
func (s *Store) UpsertArticles(ctx context.Context, feedID int64, items []ArticleInput) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}
	before, err := s.feedArticleCount(ctx, feedID)
	if err != nil {
		return 0, err
	}
	now := time.Now()
	for start := 0; start < len(items); start += 200 {
		end := start + 200
		if end > len(items) {
			end = len(items)
		}
		stmts := make([]sqlite.Statement, 0, end-start)
		for _, it := range items[start:end] {
			guid := guidKey(it.GUID, it.URL, it.Title, it.Content, it.Published)
			stmts = append(stmts, sqlite.Statement{
				SQL: `INSERT INTO articles(feed_id, guid, url, title, author, summary, content, plain, image_url, published, fetched)
					VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
					ON CONFLICT(feed_id, guid) DO UPDATE SET
						url = excluded.url,
						title = excluded.title,
						author = excluded.author,
						summary = excluded.summary,
						content = excluded.content,
						plain = excluded.plain,
						image_url = excluded.image_url,
						published = excluded.published
					WHERE articles.title IS NOT excluded.title
					   OR articles.url IS NOT excluded.url
					   OR articles.author IS NOT excluded.author
					   OR articles.summary IS NOT excluded.summary
					   OR articles.content IS NOT excluded.content
					   OR articles.plain IS NOT excluded.plain
					   OR articles.image_url IS NOT excluded.image_url
					   OR articles.published IS NOT excluded.published`,
				Args: []any{feedID, guid, it.URL, it.Title, it.Author, it.Summary,
					it.Content, it.Plain, it.ImageURL, unix(it.Published), unix(now)},
			})
		}
		if _, err := s.transaction(ctx, stmts); err != nil {
			return 0, err
		}
	}
	after, err := s.feedArticleCount(ctx, feedID)
	if err != nil {
		return 0, err
	}
	if after <= before {
		return 0, nil
	}
	return after - before, nil
}

func (s *Store) feedArticleCount(ctx context.Context, feedID int64) (int, error) {
	rows, err := s.query(ctx, `SELECT COUNT(*) FROM articles WHERE feed_id = ?`, feedID)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return int(intOf(rows[0][0])), nil
}

// SetRead marks articles read or unread.
func (s *Store) SetRead(ctx context.Context, ids []int64, read bool) error {
	if len(ids) == 0 {
		return nil
	}
	args := make([]any, 0, len(ids)+1)
	args = append(args, boolInt(read))
	for _, id := range ids {
		args = append(args, id)
	}
	_, err := s.exec(ctx, `UPDATE articles SET read = ? WHERE id IN (`+placeholders(len(ids))+`)`, args...)
	return err
}

// SetStarred marks articles starred or unstarred.
func (s *Store) SetStarred(ctx context.Context, ids []int64, starred bool) error {
	if len(ids) == 0 {
		return nil
	}
	args := make([]any, 0, len(ids)+1)
	args = append(args, boolInt(starred))
	for _, id := range ids {
		args = append(args, id)
	}
	_, err := s.exec(ctx, `UPDATE articles SET starred = ? WHERE id IN (`+placeholders(len(ids))+`)`, args...)
	return err
}

// MarkScopeRead marks every unread article in a scope read and reports how
// many changed.
func (s *Store) MarkScopeRead(ctx context.Context, scope Scope) (int, error) {
	where, args, err := scopeWhere(scope)
	if err != nil {
		return 0, err
	}
	res, err := s.exec(ctx, `UPDATE articles SET read = 1 WHERE read = 0 AND id IN (
		SELECT a.id`+articleFrom+` WHERE `+where+`)`, args...)
	if err != nil {
		return 0, err
	}
	return int(res.Changes), nil
}

// Prune deletes read, unstarred articles beyond the newest keepPerFeed in
// each feed. Unread and starred articles are never removed.
func (s *Store) Prune(ctx context.Context, keepPerFeed int) (int, error) {
	if keepPerFeed < 1 {
		return 0, nil
	}
	res, err := s.exec(ctx, `DELETE FROM articles WHERE read = 1 AND starred = 0 AND id NOT IN (
		SELECT id FROM (
			SELECT id, ROW_NUMBER() OVER (PARTITION BY feed_id ORDER BY published DESC, id DESC) AS rn
			FROM articles
		) WHERE rn <= ?
	)`, keepPerFeed)
	if err != nil {
		return 0, err
	}
	return int(res.Changes), nil
}

// Counts returns the badge numbers for the sidebar.
func (s *Store) Counts(ctx context.Context) (Counts, error) {
	var c Counts
	rows, err := s.query(ctx, `SELECT
		(SELECT COUNT(*) FROM articles WHERE read = 0),
		(SELECT COUNT(*) FROM articles WHERE starred = 1),
		(SELECT COUNT(*) FROM articles WHERE published >= ?),
		(SELECT COUNT(*) FROM articles),
		(SELECT COUNT(*) FROM feeds),
		(SELECT COUNT(*) FROM folders),
		(SELECT COUNT(*) FROM searches)`, startOfToday())
	if err != nil {
		return c, err
	}
	if len(rows) == 0 {
		return c, nil
	}
	r := rows[0]
	c.Unread = int(intOf(r[0]))
	c.Starred = int(intOf(r[1]))
	c.Today = int(intOf(r[2]))
	c.Total = int(intOf(r[3]))
	c.Feeds = int(intOf(r[4]))
	c.Folders = int(intOf(r[5]))
	c.Searches = int(intOf(r[6]))
	return c, nil
}

func articleRow(r []any) Article {
	return Article{
		ID:        intOf(r[0]),
		FeedID:    intOf(r[1]),
		FeedTitle: stringOf(r[2]),
		GUID:      stringOf(r[3]),
		URL:       stringOf(r[4]),
		Title:     stringOf(r[5]),
		Author:    stringOf(r[6]),
		Summary:   stringOf(r[7]),
		Content:   stringOf(r[8]),
		Plain:     stringOf(r[9]),
		ImageURL:  stringOf(r[10]),
		Published: timeOf(r[11]),
		Fetched:   timeOf(r[12]),
		Read:      boolOf(r[13]),
		Starred:   boolOf(r[14]),
	}
}

// scopeWhere returns the SQL for a scope and its arguments. It is used for
// listing, counting, neighbouring and marking read, so all of them agree.
func scopeWhere(scope Scope) (string, []any, error) {
	switch scope.Kind {
	case ScopeAll, "":
		return "1 = 1", nil, nil
	case ScopeUnread:
		return "a.read = 0", nil, nil
	case ScopeStarred:
		return "a.starred = 1", nil, nil
	case ScopeToday:
		return "a.published >= ?", []any{startOfToday()}, nil
	case ScopeFeed:
		return "a.feed_id = ?", []any{scope.ID}, nil
	case ScopeFolder:
		return `a.feed_id IN (
			WITH RECURSIVE folder_tree(id) AS (
				SELECT id FROM folders WHERE id = ?
				UNION ALL
				SELECT f.id FROM folders f JOIN folder_tree t ON f.parent_id = t.id
			)
			SELECT feeds.id FROM feeds JOIN folder_tree ON feeds.folder_id = folder_tree.id
		)`, []any{scope.ID}, nil
	case ScopeSearch:
		return searchWhere(scope.Query)
	}
	return "", nil, fmt.Errorf("storage: unknown scope %q", scope.Kind)
}

// searchWhere uses the trigram index when every token is long enough for it,
// and a LIKE scan for shorter words. Both paths match in the same columns.
func searchWhere(query string) (string, []any, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "0 = 1", nil, nil
	}
	tokens := strings.Fields(query)
	useFTS := true
	for _, t := range tokens {
		if utf8.RuneCountInString(t) < 3 {
			useFTS = false
			break
		}
	}
	if useFTS {
		parts := make([]string, 0, len(tokens))
		for _, t := range tokens {
			t = strings.ReplaceAll(t, `"`, `""`)
			parts = append(parts, `"`+t+`"`)
		}
		return "a.id IN (SELECT rowid FROM articles_fts WHERE articles_fts MATCH ?)",
			[]any{strings.Join(parts, " AND ")}, nil
	}
	parts := make([]string, 0, len(tokens))
	args := make([]any, 0, len(tokens)*3)
	for _, t := range tokens {
		like := "%" + escapeLike(t) + "%"
		parts = append(parts, `(a.title LIKE ? ESCAPE '\' OR a.summary LIKE ? ESCAPE '\' OR a.plain LIKE ? ESCAPE '\')`)
		args = append(args, like, like, like)
	}
	return strings.Join(parts, " AND "), args, nil
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "%", `\%`)
	return strings.ReplaceAll(s, "_", `\_`)
}

func startOfToday() int64 {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
