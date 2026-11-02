package app

import (
	"context"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"xin.dart.zrss/internal/storage"
)

// settings are the reader's choices. They live in the settings table of the
// same SQLite database as the subscriptions, so a backup of the database
// keeps them too.
type settings struct {
	Appearance    string // system, light or dark
	Accent        string // red, orange, green, blue or purple
	Layout        string // which panes the main window shows
	RefreshMin    int    // minutes between refreshes, 0 for manual
	MarkRead      bool   // mark an article read as it is opened
	NotifyNew     bool   // notify when a refresh finds new articles
	CloseToTray   bool   // keep running in the tray when the window closes
	ShowTray      bool   // show the tray icon
	KeepArticles  int    // read articles to keep per feed, 0 for all
	ReaderSize    float32
	LastScope     string // the sidebar choice to show at startup
	lastRefreshAt int64  // when a refresh last finished, for the scheduler
}

// The layouts the main window offers: all three panes, the list and the
// article, or the article alone.
const (
	layoutAll    = "all"
	layoutSplit  = "split"
	layoutReader = "reader"
)

func defaultSettings() settings {
	return settings{
		Appearance:   "system",
		Accent:       "red",
		Layout:       layoutAll,
		RefreshMin:   30,
		MarkRead:     true,
		NotifyNew:    true,
		CloseToTray:  true,
		ShowTray:     true,
		KeepArticles: 500,
		ReaderSize:   16,
		LastScope:    "all",
	}
}

func loadSettings(ctx context.Context, s *storage.Store) settings {
	out := defaultSettings()
	all, err := s.Settings(ctx)
	if err != nil {
		return out
	}
	if v := all["appearance"]; v == "light" || v == "dark" || v == "system" {
		out.Appearance = v
	}
	if v := all["accent"]; v != "" {
		if _, ok := accentColor(v, true); ok {
			out.Accent = v
		}
	}
	switch v := all["layout"]; v {
	case layoutAll, layoutSplit, layoutReader:
		out.Layout = v
	}
	if v, err := strconv.Atoi(all["refreshMin"]); err == nil && v >= 0 {
		out.RefreshMin = v
	}
	if v, ok := parseBool(all["markRead"]); ok {
		out.MarkRead = v
	}
	if v, ok := parseBool(all["notifyNew"]); ok {
		out.NotifyNew = v
	}
	if v, ok := parseBool(all["closeToTray"]); ok {
		out.CloseToTray = v
	}
	if v, ok := parseBool(all["showTray"]); ok {
		out.ShowTray = v
	}
	if v, err := strconv.Atoi(all["keepArticles"]); err == nil && v >= 0 {
		out.KeepArticles = v
	}
	if v, err := strconv.Atoi(all["readerSize"]); err == nil && v >= 12 && v <= 24 {
		out.ReaderSize = float32(v)
	}
	if v := all["lastScope"]; v != "" {
		out.LastScope = v
	}
	if v, err := strconv.ParseInt(all["lastRefresh"], 10, 64); err == nil {
		out.lastRefreshAt = v
	}
	out.ApplyAppearance()
	return out
}

func saveSettings(ctx context.Context, s *storage.Store, v settings) error {
	pairs := [][2]string{
		{"appearance", v.Appearance},
		{"accent", v.Accent},
		{"layout", v.Layout},
		{"refreshMin", strconv.Itoa(v.RefreshMin)},
		{"markRead", strconv.FormatBool(v.MarkRead)},
		{"notifyNew", strconv.FormatBool(v.NotifyNew)},
		{"closeToTray", strconv.FormatBool(v.CloseToTray)},
		{"showTray", strconv.FormatBool(v.ShowTray)},
		{"keepArticles", strconv.Itoa(v.KeepArticles)},
		{"readerSize", strconv.Itoa(int(v.ReaderSize))},
		{"lastScope", v.LastScope},
		{"lastRefresh", strconv.FormatInt(v.lastRefreshAt, 10)},
	}
	for _, p := range pairs {
		if err := s.SetSetting(ctx, p[0], p[1]); err != nil {
			return err
		}
	}
	return nil
}

func parseBool(v string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	}
	return false, false
}

// ApplyAppearance makes the app light, dark, or follow the desktop.
func (s settings) ApplyAppearance() {
	switch s.Appearance {
	case "light":
		mygo.Theme.SetSource(mygo.ThemeLight)
	case "dark":
		mygo.Theme.SetSource(mygo.ThemeDark)
	default:
		mygo.Theme.SetSource(mygo.ThemeSystem)
	}
}

// accentColor returns the accent named in the settings, and whether it is
// one of the choices.
func accentColor(name string, dark bool) (ui.Color, bool) {
	hue, ok := map[string]float32{"red": 25, "orange": 65, "green": 150, "blue": 250, "purple": 300}[name]
	if !ok {
		return ui.Color{}, false
	}
	l := float32(0.60)
	if dark {
		l = 0.72
	}
	return ui.Oklch(l, 0.19, hue), true
}

var accentNames = []string{"red", "orange", "green", "blue", "purple"}

var accentLabels = map[string]string{
	"red":    "绯红",
	"orange": "暖橙",
	"green":  "松绿",
	"blue":   "靛蓝",
	"purple": "鸢尾",
}
