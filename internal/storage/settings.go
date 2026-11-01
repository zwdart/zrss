package storage

import (
	"context"
	"strings"
	"time"
)

// Setting returns a stored value, or fallback when the key is absent.
func (s *Store) Setting(ctx context.Context, key, fallback string) (string, error) {
	rows, err := s.query(ctx, `SELECT value FROM settings WHERE key = ?`, key)
	if err != nil {
		return fallback, err
	}
	if len(rows) == 0 {
		return fallback, nil
	}
	return stringOf(rows[0][0]), nil
}

// SetSetting stores a value, replacing any previous one.
func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.exec(ctx, `INSERT INTO settings(key, value) VALUES(?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// DeleteSetting forgets a value.
func (s *Store) DeleteSetting(ctx context.Context, key string) error {
	_, err := s.exec(ctx, `DELETE FROM settings WHERE key = ?`, key)
	return err
}

// Settings returns every stored value, for the settings window.
func (s *Store) Settings(ctx context.Context) (map[string]string, error) {
	rows, err := s.query(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[stringOf(r[0])] = stringOf(r[1])
	}
	return out, nil
}

// RecentSearches returns the latest queries, newest first.
func (s *Store) RecentSearches(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 8
	}
	rows, err := s.query(ctx, `SELECT query FROM searches ORDER BY used_at DESC, query COLLATE NOCASE LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, stringOf(r[0]))
	}
	return out, nil
}

// AddSearch remembers a query, moving it to the front when repeated.
func (s *Store) AddSearch(ctx context.Context, query string) error {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	if _, err := s.exec(ctx, `INSERT INTO searches(query, used_at) VALUES(?, ?)
		ON CONFLICT(query) DO UPDATE SET used_at = excluded.used_at`, query, time.Now().UnixMilli()); err != nil {
		return err
	}
	// Keep the list short so it stays a quick jump list.
	_, err := s.exec(ctx, `DELETE FROM searches WHERE query NOT IN (
		SELECT query FROM searches ORDER BY used_at DESC, query COLLATE NOCASE LIMIT 8)`)
	return err
}

// DeleteSearch forgets one query.
func (s *Store) DeleteSearch(ctx context.Context, query string) error {
	_, err := s.exec(ctx, `DELETE FROM searches WHERE query = ? COLLATE NOCASE`, query)
	return err
}

// ClearSearches forgets the whole search history.
func (s *Store) ClearSearches(ctx context.Context) error {
	_, err := s.exec(ctx, `DELETE FROM searches`)
	return err
}
