// Package storage keeps the reader's feeds, articles and choices in one
// SQLite database. It knows nothing about the UI: the app hands it parsed
// articles and asks for pages of rows.
package storage

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"
)

// Store is a connection to the reader's database. The SQLite plugin
// serializes operations, so a Store may be used from any goroutine.
type Store struct {
	db   *sqlite.DB
	path string
}

// Open opens the database at path, creating it and its tables when needed.
// The parent directory must exist.
func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sqlite.Open(ctx, path, sqlite.OpenOptions{BusyTimeout: 5 * time.Second})
	if err != nil {
		return nil, err
	}
	s := &Store{db: db, path: path}
	if err := s.init(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// OpenMemory opens a private in-memory database, for tests.
func OpenMemory(ctx context.Context) (*Store, error) {
	db, err := sqlite.Open(ctx, ":memory:", sqlite.OpenOptions{})
	if err != nil {
		return nil, err
	}
	s := &Store{db: db, path: ":memory:"}
	if err := s.init(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) init(ctx context.Context) error {
	for _, pragma := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
	} {
		if _, err := s.db.Execute(ctx, pragma); err != nil {
			return fmt.Errorf("storage: %w", err)
		}
	}
	if err := s.migrateFolders(ctx); err != nil {
		return fmt.Errorf("storage: migrate folders: %w", err)
	}
	for _, stmt := range schema {
		if _, err := s.db.Execute(ctx, stmt); err != nil {
			return fmt.Errorf("storage: schema: %w", err)
		}
	}
	_, err := s.db.Execute(ctx, `INSERT INTO meta(key, value) VALUES('version', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, schemaVersion)
	return err
}

// migrateFolders rebuilds the version 1 folders table, which had one flat
// level and a global unique name. Rebuilding keeps folder ids so feeds do not
// have to move. A fresh database has no folders table yet and is left alone.
func (s *Store) migrateFolders(ctx context.Context) error {
	columns, err := s.query(ctx, `PRAGMA table_info(folders)`)
	if err != nil {
		return err
	}
	if len(columns) == 0 {
		return nil
	}
	for _, c := range columns {
		if stringOf(c[1]) == "parent_id" {
			return nil
		}
	}
	_, err = s.db.Transaction(ctx, []sqlite.Statement{
		{SQL: `DROP TABLE IF EXISTS folders_v2`},
		{SQL: `CREATE TABLE folders_v2 (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			parent_id INTEGER NOT NULL DEFAULT 0,
			name TEXT NOT NULL COLLATE NOCASE,
			sort INTEGER NOT NULL DEFAULT 0
		)`},
		{SQL: `INSERT INTO folders_v2(id, parent_id, name, sort)
			SELECT id, 0, name, sort FROM folders`},
		{SQL: `DROP TABLE folders`},
		{SQL: `ALTER TABLE folders_v2 RENAME TO folders`},
	})
	return err
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// Path is the database file.
func (s *Store) Path() string { return s.path }

// Size returns the bytes the database takes, including its write-ahead log
// and shared-memory files, or 0 when unknown.
func (s *Store) Size() int64 {
	if s.path == ":memory:" {
		return 0
	}
	var total int64
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if st, err := os.Stat(s.path + suffix); err == nil {
			total += st.Size()
		}
	}
	return total
}

// Vacuum compacts the database, and refreshes its query planner.
func (s *Store) Vacuum(ctx context.Context) error {
	if s.path == ":memory:" {
		return nil
	}
	if _, err := s.db.Execute(ctx, "VACUUM"); err != nil {
		return err
	}
	_, err := s.db.Execute(ctx, "PRAGMA optimize")
	return err
}

// query runs a query and returns its rows.
func (s *Store) query(ctx context.Context, sql string, args ...any) ([][]any, error) {
	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return rows.Values, nil
}

// exec runs one statement.
func (s *Store) exec(ctx context.Context, sql string, args ...any) (sqlite.Result, error) {
	return s.db.Execute(ctx, sql, args...)
}

// transaction runs statements atomically.
func (s *Store) transaction(ctx context.Context, stmts []sqlite.Statement) ([]sqlite.Result, error) {
	if len(stmts) == 0 {
		return nil, nil
	}
	return s.db.Transaction(ctx, stmts)
}

// unix stores a time as seconds, zero when it has none.
func unix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

func timeOf(v any) time.Time {
	n, _ := v.(int64)
	if n <= 0 {
		return time.Time{}
	}
	return time.Unix(n, 0)
}

func intOf(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	case bool:
		if n {
			return 1
		}
	}
	return 0
}

func stringOf(v any) string {
	s, _ := v.(string)
	return s
}

func boolOf(v any) bool { return intOf(v) != 0 }

// guidKey is the identity of an item within its feed: its GUID when it has
// one, else its link, else a digest of what it says. Feeds repeat items on
// every fetch, so the same item must always make the same key.
func guidKey(guid, link, title, content string, published time.Time) string {
	switch {
	case strings.TrimSpace(guid) != "":
		return strings.TrimSpace(guid)
	case strings.TrimSpace(link) != "":
		return strings.TrimSpace(link)
	}
	h := sha1.Sum([]byte(title + "\x00" + content + "\x00" + fmt.Sprint(unix(published))))
	return "sha1:" + hex.EncodeToString(h[:])
}

// ErrNotFound is returned when a feed or article is gone.
var ErrNotFound = errors.New("storage: not found")

// Folder tree errors.
var (
	ErrFolderDepth  = errors.New("storage: folders can be nested three levels deep")
	ErrFolderCycle  = errors.New("storage: a folder cannot contain itself")
	ErrFolderExists = errors.New("storage: a folder with that name already exists here")
)

func isNoRows(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "no rows")
}

// DirOf returns the directory a database path is in.
func DirOf(path string) string { return filepath.Dir(path) }
