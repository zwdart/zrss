package storage

import (
	"context"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"
)

// Folder groups feeds in the sidebar. ParentID is 0 for a root folder.
type Folder struct {
	ID       int64
	ParentID int64
	Name     string
	Sort     int
}

// Feed is a subscription and what the last fetch said of it.
type Feed struct {
	ID           int64
	URL          string
	SiteURL      string
	Title        string
	Description  string
	IconURL      string
	FolderID     int64
	Sort         int
	ETag         string
	LastModified string
	LastFetch    time.Time
	LastError    string
	CreatedAt    time.Time
	Unread       int
	Total        int
}

// FeedMeta is what a fetch learned about a feed.
type FeedMeta struct {
	Title        string
	SiteURL      string
	Description  string
	IconURL      string
	ETag         string
	LastModified string
	LastFetch    time.Time
	LastError    string
}

// Folders returns the folders in sidebar order.
func (s *Store) Folders(ctx context.Context) ([]Folder, error) {
	rows, err := s.query(ctx, `SELECT id, parent_id, name, sort FROM folders ORDER BY parent_id, sort, name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	out := make([]Folder, 0, len(rows))
	for _, r := range rows {
		out = append(out, Folder{
			ID:       intOf(r[0]),
			ParentID: intOf(r[1]),
			Name:     stringOf(r[2]),
			Sort:     int(intOf(r[3])),
		})
	}
	return out, nil
}

// CreateFolder adds a folder under parentID and returns its id. A folder of
// the same name under the same parent is returned instead. Folder nesting is
// limited to three levels.
func (s *Store) CreateFolder(ctx context.Context, name string, parentID int64) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, ErrNotFound
	}
	if parentID != 0 {
		depth, err := s.folderDepth(ctx, parentID)
		if err != nil {
			return 0, err
		}
		if depth >= 3 {
			return 0, ErrFolderDepth
		}
	}
	if id, err := s.folderByName(ctx, name, parentID); err == nil {
		return id, nil
	}
	res, err := s.exec(ctx, `
		INSERT INTO folders(name, parent_id, sort)
		VALUES(?, ?, (SELECT COALESCE(MAX(sort), 0) + 1 FROM folders WHERE parent_id = ?))`,
		name, parentID, parentID)
	if err != nil {
		return 0, err
	}
	return res.LastInsertID, nil
}

// EnsureFolder is CreateFolder, returning the existing folder's id.
func (s *Store) EnsureFolder(ctx context.Context, name string, parentID int64) (int64, error) {
	return s.CreateFolder(ctx, name, parentID)
}

func (s *Store) folderByName(ctx context.Context, name string, parentID int64) (int64, error) {
	rows, err := s.query(ctx, `SELECT id FROM folders WHERE parent_id = ? AND name = ? COLLATE NOCASE`, parentID, name)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, ErrNotFound
	}
	return intOf(rows[0][0]), nil
}

// folderDepth returns 1 for a root folder, 2 for its child, and so on.
func (s *Store) folderDepth(ctx context.Context, id int64) (int, error) {
	depth := 0
	for id != 0 {
		rows, err := s.query(ctx, `SELECT parent_id FROM folders WHERE id = ?`, id)
		if err != nil {
			return 0, err
		}
		if len(rows) == 0 {
			return 0, ErrNotFound
		}
		depth++
		id = intOf(rows[0][0])
		if depth > 3 {
			return 0, ErrFolderDepth
		}
	}
	return depth, nil
}

// folderNameTaken reports whether a sibling already uses name.
func (s *Store) folderNameTaken(ctx context.Context, name string, parentID, excludeID int64) (bool, error) {
	rows, err := s.query(ctx, `
		SELECT id FROM folders
		WHERE parent_id = ? AND name = ? COLLATE NOCASE AND id <> ?`,
		parentID, name, excludeID)
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

// RenameFolder gives a folder another name.
func (s *Store) RenameFolder(ctx context.Context, id int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrNotFound
	}
	rows, err := s.query(ctx, `SELECT parent_id FROM folders WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return ErrNotFound
	}
	parentID := intOf(rows[0][0])
	taken, err := s.folderNameTaken(ctx, name, parentID, id)
	if err != nil {
		return err
	}
	if taken {
		return ErrFolderExists
	}
	_, err = s.exec(ctx, `UPDATE folders SET name = ? WHERE id = ?`, name, id)
	return err
}

// DeleteFolder removes a folder and its subtree. Feeds in the subtree move to
// the deleted folder's parent, so subscriptions and articles are kept.
func (s *Store) DeleteFolder(ctx context.Context, id int64) error {
	rows, err := s.query(ctx, `SELECT parent_id FROM folders WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return ErrNotFound
	}
	parentID := intOf(rows[0][0])
	ids, err := s.folderSubtree(ctx, id)
	if err != nil {
		return err
	}
	placeholders, args := placeholdersFor(ids)
	if len(ids) == 0 {
		return ErrNotFound
	}
	feedArgs := append([]any{parentID}, args...)
	folderArgs := append([]any(nil), args...)
	_, err = s.transaction(ctx, []sqlite.Statement{
		{
			SQL:  `UPDATE feeds SET folder_id = ? WHERE folder_id IN (` + placeholders + `)`,
			Args: feedArgs,
		},
		{
			SQL:  `DELETE FROM folders WHERE id IN (` + placeholders + `)`,
			Args: folderArgs,
		},
	})
	return err
}

// folderSubtree returns id and all folders below it, deepest last.
func (s *Store) folderSubtree(ctx context.Context, id int64) ([]int64, error) {
	rows, err := s.query(ctx, `
		WITH RECURSIVE sub(id) AS (
			SELECT id FROM folders WHERE id = ?
			UNION ALL
			SELECT f.id FROM folders f JOIN sub ON f.parent_id = sub.id
		)
		SELECT id FROM sub`, id)
	if err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(rows))
	for _, r := range rows {
		out = append(out, intOf(r[0]))
	}
	return out, nil
}

func placeholdersFor(ids []int64) (string, []any) {
	marks := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		marks[i] = "?"
		args[i] = id
	}
	return strings.Join(marks, ", "), args
}

// MoveFolder puts a folder under another folder, or at the top level with
// parentID 0. A folder cannot be moved into itself or one of its descendants,
// and the resulting tree must still fit in three levels.
func (s *Store) MoveFolder(ctx context.Context, id, parentID int64) error {
	if id == parentID {
		return ErrFolderCycle
	}
	if _, err := s.folderDepth(ctx, id); err != nil {
		return err
	}
	if parentID != 0 {
		if _, err := s.folderDepth(ctx, parentID); err != nil {
			return err
		}
		inside, err := s.folderIsDescendant(ctx, parentID, id)
		if err != nil {
			return err
		}
		if inside {
			return ErrFolderCycle
		}
	}
	height, err := s.folderHeight(ctx, id)
	if err != nil {
		return err
	}
	parentDepth := 0
	if parentID != 0 {
		parentDepth, err = s.folderDepth(ctx, parentID)
		if err != nil {
			return err
		}
	}
	if parentDepth+height > 3 {
		return ErrFolderDepth
	}
	rows, err := s.query(ctx, `SELECT name FROM folders WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return ErrNotFound
	}
	taken, err := s.folderNameTaken(ctx, stringOf(rows[0][0]), parentID, id)
	if err != nil {
		return err
	}
	if taken {
		return ErrFolderExists
	}
	_, err = s.exec(ctx, `UPDATE folders SET parent_id = ? WHERE id = ?`, parentID, id)
	return err
}

// folderHeight returns the number of levels in a folder's subtree.
func (s *Store) folderHeight(ctx context.Context, id int64) (int, error) {
	rows, err := s.query(ctx, `
		WITH RECURSIVE sub(id, depth) AS (
			SELECT id, 1 FROM folders WHERE id = ?
			UNION ALL
			SELECT f.id, sub.depth + 1 FROM folders f JOIN sub ON f.parent_id = sub.id
		)
		SELECT MAX(depth) FROM sub`, id)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, ErrNotFound
	}
	return int(intOf(rows[0][0])), nil
}

// folderIsDescendant reports whether candidate is below ancestor.
func (s *Store) folderIsDescendant(ctx context.Context, candidate, ancestor int64) (bool, error) {
	for candidate != 0 {
		if candidate == ancestor {
			return true, nil
		}
		rows, err := s.query(ctx, `SELECT parent_id FROM folders WHERE id = ?`, candidate)
		if err != nil {
			return false, err
		}
		if len(rows) == 0 {
			return false, ErrNotFound
		}
		candidate = intOf(rows[0][0])
	}
	return false, nil
}

// MoveFeed puts a feed in a folder, or at the top level with folderID 0.
func (s *Store) MoveFeed(ctx context.Context, feedID, folderID int64) error {
	if folderID != 0 {
		if _, err := s.folderDepth(ctx, folderID); err != nil {
			return err
		}
	}
	_, err := s.exec(ctx, `UPDATE feeds SET folder_id = ? WHERE id = ?`, folderID, feedID)
	return err
}

// Feeds returns every subscription with its unread and total counts.
func (s *Store) Feeds(ctx context.Context) ([]Feed, error) {
	rows, err := s.query(ctx, `
		SELECT f.id, f.url, f.site_url, f.title, f.description, f.icon_url,
		       f.folder_id, f.sort, f.etag, f.last_modified, f.last_fetch,
		       f.last_error, f.created_at,
		       (SELECT COUNT(*) FROM articles a WHERE a.feed_id = f.id AND a.read = 0),
		       (SELECT COUNT(*) FROM articles a WHERE a.feed_id = f.id)
		FROM feeds f
		ORDER BY f.sort, f.title COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	out := make([]Feed, 0, len(rows))
	for _, r := range rows {
		out = append(out, feedRow(r))
	}
	return out, nil
}

// Feed returns one subscription.
func (s *Store) Feed(ctx context.Context, id int64) (Feed, error) {
	rows, err := s.query(ctx, `
		SELECT f.id, f.url, f.site_url, f.title, f.description, f.icon_url,
		       f.folder_id, f.sort, f.etag, f.last_modified, f.last_fetch,
		       f.last_error, f.created_at,
		       (SELECT COUNT(*) FROM articles a WHERE a.feed_id = f.id AND a.read = 0),
		       (SELECT COUNT(*) FROM articles a WHERE a.feed_id = f.id)
		FROM feeds f WHERE f.id = ?`, id)
	if err != nil {
		return Feed{}, err
	}
	if len(rows) == 0 {
		return Feed{}, ErrNotFound
	}
	return feedRow(rows[0]), nil
}

func feedRow(r []any) Feed {
	return Feed{
		ID:           intOf(r[0]),
		URL:          stringOf(r[1]),
		SiteURL:      stringOf(r[2]),
		Title:        stringOf(r[3]),
		Description:  stringOf(r[4]),
		IconURL:      stringOf(r[5]),
		FolderID:     intOf(r[6]),
		Sort:         int(intOf(r[7])),
		ETag:         stringOf(r[8]),
		LastModified: stringOf(r[9]),
		LastFetch:    timeOf(r[10]),
		LastError:    stringOf(r[11]),
		CreatedAt:    timeOf(r[12]),
		Unread:       int(intOf(r[13])),
		Total:        int(intOf(r[14])),
	}
}

// AddFeed subscribes to f. It returns the feed's id and whether it was new.
func (s *Store) AddFeed(ctx context.Context, f Feed) (int64, bool, error) {
	if id, err := s.FeedID(ctx, f.URL); err == nil {
		return id, false, nil
	} else if err != ErrNotFound {
		return 0, false, err
	}
	if f.Sort == 0 {
		f.Sort = 1
	}
	res, err := s.exec(ctx, `
		INSERT INTO feeds(url, site_url, title, description, icon_url, folder_id, sort, etag, last_modified, last_fetch, last_error, created_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		f.URL, f.SiteURL, f.Title, f.Description, f.IconURL, f.FolderID, f.Sort,
		f.ETag, f.LastModified, unix(f.LastFetch), f.LastError, unix(time.Now()))
	if err != nil {
		return 0, false, err
	}
	return res.LastInsertID, true, nil
}

// FeedID finds a subscription by its URL.
func (s *Store) FeedID(ctx context.Context, url string) (int64, error) {
	rows, err := s.query(ctx, `SELECT id FROM feeds WHERE url = ?`, url)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, ErrNotFound
	}
	return intOf(rows[0][0]), nil
}

// UpdateFeed records what a fetch learned. A title the reader gave the feed
// is kept.
func (s *Store) UpdateFeed(ctx context.Context, id int64, m FeedMeta) error {
	_, err := s.exec(ctx, `
		UPDATE feeds SET title = CASE WHEN custom_title = 1 THEN title ELSE ? END,
		                 site_url = ?, description = ?, icon_url = ?,
		                 etag = ?, last_modified = ?, last_fetch = ?, last_error = ?
		WHERE id = ?`,
		m.Title, m.SiteURL, m.Description, m.IconURL,
		m.ETag, m.LastModified, unix(m.LastFetch), m.LastError, id)
	return err
}

// DeleteFeed unsubscribes, taking its articles with it.
func (s *Store) DeleteFeed(ctx context.Context, id int64) error {
	_, err := s.exec(ctx, `DELETE FROM feeds WHERE id = ?`, id)
	return err
}

// RenameFeed records a new title, which later fetches keep.
func (s *Store) RenameFeed(ctx context.Context, id int64, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return ErrNotFound
	}
	_, err := s.exec(ctx, `UPDATE feeds SET title = ?, custom_title = 1 WHERE id = ?`, title, id)
	return err
}
