package storage

// schemaVersion is stored in meta and bumped when the schema changes.
const schemaVersion = "2"

// schema creates the database. Each statement is run on its own, as the
// SQLite plugin accepts exactly one statement at a time.
var schema = []string{
	`CREATE TABLE IF NOT EXISTS meta (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS folders (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		parent_id INTEGER NOT NULL DEFAULT 0,
		name TEXT NOT NULL COLLATE NOCASE,
		sort INTEGER NOT NULL DEFAULT 0
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_folders_parent_name ON folders(parent_id, name)`,
	`CREATE INDEX IF NOT EXISTS idx_folders_parent ON folders(parent_id, sort)`,
	`CREATE TABLE IF NOT EXISTS feeds (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		url TEXT NOT NULL UNIQUE,
		site_url TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		icon_url TEXT NOT NULL DEFAULT '',
		folder_id INTEGER NOT NULL DEFAULT 0,
		sort INTEGER NOT NULL DEFAULT 0,
		custom_title INTEGER NOT NULL DEFAULT 0,
		etag TEXT NOT NULL DEFAULT '',
		last_modified TEXT NOT NULL DEFAULT '',
		last_fetch INTEGER NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL DEFAULT 0
	)`,
	`CREATE INDEX IF NOT EXISTS idx_feeds_folder ON feeds(folder_id, sort)`,
	`CREATE TABLE IF NOT EXISTS articles (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		feed_id INTEGER NOT NULL REFERENCES feeds(id) ON DELETE CASCADE,
		guid TEXT NOT NULL,
		url TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL DEFAULT '',
		author TEXT NOT NULL DEFAULT '',
		summary TEXT NOT NULL DEFAULT '',
		content TEXT NOT NULL DEFAULT '',
		plain TEXT NOT NULL DEFAULT '',
		image_url TEXT NOT NULL DEFAULT '',
		published INTEGER NOT NULL DEFAULT 0,
		fetched INTEGER NOT NULL DEFAULT 0,
		read INTEGER NOT NULL DEFAULT 0,
		starred INTEGER NOT NULL DEFAULT 0,
		UNIQUE(feed_id, guid)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_articles_published ON articles(published DESC, id DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_articles_feed ON articles(feed_id, published DESC, id DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_articles_unread ON articles(read, published DESC, id DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_articles_starred ON articles(starred, published DESC, id DESC)`,
	`CREATE VIRTUAL TABLE IF NOT EXISTS articles_fts USING fts5(
		title, summary, plain,
		content='articles', content_rowid='id', tokenize='trigram'
	)`,
	`CREATE TRIGGER IF NOT EXISTS articles_ai AFTER INSERT ON articles BEGIN
		INSERT INTO articles_fts(rowid, title, summary, plain)
		VALUES (new.id, new.title, new.summary, new.plain);
	END`,
	`CREATE TRIGGER IF NOT EXISTS articles_ad AFTER DELETE ON articles BEGIN
		INSERT INTO articles_fts(articles_fts, rowid, title, summary, plain)
		VALUES ('delete', old.id, old.title, old.summary, old.plain);
	END`,
	`CREATE TRIGGER IF NOT EXISTS articles_au AFTER UPDATE OF title, summary, plain ON articles BEGIN
		INSERT INTO articles_fts(articles_fts, rowid, title, summary, plain)
		VALUES ('delete', old.id, old.title, old.summary, old.plain);
		INSERT INTO articles_fts(rowid, title, summary, plain)
		VALUES (new.id, new.title, new.summary, new.plain);
	END`,
	`CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS searches (
		query TEXT PRIMARY KEY COLLATE NOCASE,
		used_at INTEGER NOT NULL DEFAULT 0
	)`,
}
