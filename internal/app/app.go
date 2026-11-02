// Package app is the reader's window: it shows the subscriptions, the
// articles and the article being read, and joins the SQLite store to the
// feed fetcher.
package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"xin.dart.zrss/internal/feed"
	"xin.dart.zrss/internal/storage"
)

// app is the state of the windows. The views and the goroutines they start
// change it on the main thread only: background work comes back through
// post.
type app struct {
	win         *mygo.Window
	settingsWin *mygo.Window
	tray        *mygo.Tray
	store       *storage.Store
	fetcher     *feed.Fetcher
	imgs        *imageCache
	md          markdown.Options
	pal         palette

	// The menus the title bar opens, and the app menu of macOS.
	menuApp    *mygo.Menu
	menuFile   *mygo.Menu
	menuEdit   *mygo.Menu
	menuView   *mygo.Menu
	menuWindow *mygo.Menu

	settings      settings
	updates       bool        // the update plugin is registered
	canSelfUpdate bool        // updates can replace the app where it is installed
	quitting      atomic.Bool // read by goroutines on their way back to the main thread
	done          chan struct{}
	stopOnce      sync.Once

	// The sidebar.
	folders       []storage.Folder
	feeds         []storage.Feed
	counts        storage.Counts
	collapsed     map[int64]bool
	defaultFolder int64

	// The list and the article it shows.
	scope       storage.Scope
	scopeTitle  string
	articles    []storage.Article
	total       int
	hasMore     bool
	loading     bool
	loadErr     string
	selected    int
	pendingMove int // a step waiting for the next page of the list
	listState   ui.ListState
	current     storage.Article
	hasCurrent  bool

	// The search field and its history.
	query       string
	searchBox   ui.Handle
	searchTimer *time.Timer
	searchBase  storage.Scope
	history     []string

	readerScroll ui.ScrollState

	// Background refreshing.
	refreshing  bool
	lastRefresh time.Time

	add     addDialog
	prompt  promptDialog
	confirm confirmDialog
	toast   string
}

// Run runs the app until it quits.
func Run() error {
	a := &app{fetcher: feed.NewFetcher(), done: make(chan struct{}), collapsed: map[int64]bool{}, selected: -1}
	// Before App.Run: the plugin schedules its checks once the app is ready.
	a.useUpdater()
	mygo.App.WhenReady(func() {
		ctx := context.Background()
		path, err := dataPath("zrss.db")
		if err != nil {
			mygo.Dialog.Error("无法打开数据库", err.Error())
			mygo.App.Quit()
			return
		}
		store, err := storage.Open(ctx, path)
		if err != nil {
			mygo.Dialog.Error("无法打开数据库", fmt.Sprintf("%s\n\n%s", path, err))
			mygo.App.Quit()
			return
		}
		a.store = store
		a.settings = loadSettings(ctx, store)
		if a.settings.lastRefreshAt > 0 {
			a.lastRefresh = time.Unix(a.settings.lastRefreshAt, 0)
		}
		a.imgs = newImageCache(a.post)
		a.md = markdown.Options{
			OnLink:    a.openLink,
			LoadImage: a.imgs.load,
			WrapCode:  false,
		}
		a.reloadSidebar(ctx)
		a.restoreScope(ctx)
		a.reloadArticles(ctx)

		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:         mygo.App.Name(),
			Width:         1240,
			Height:        800,
			MinWidth:      980,
			MinHeight:     620,
			StateKey:      "main",
			TitleBarStyle: mygo.TitleBarHidden,
			// The view draws the window controls itself, in the title bar
			// along the top, and the menu bar stays out of its way.
			HideWindowButtons: true,
			AutoHideMenuBar:   true,
			TitleBarHeight:    titleBarHeight,
			BackgroundColor:   "light-dark(#ffffff, #131315)",
			Content:           ui.View(a.view),
		})
		a.win.OnClose(a.onClose)
		a.setMenu()
		a.applyTray()
		a.startScheduler()
		if a.settings.RefreshMin > 0 {
			time.AfterFunc(700*time.Millisecond, func() {
				a.post(func() { a.refreshAll(false) })
			})
		}
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		a.quitting.Store(true)
		if a.store != nil {
			ctx := context.Background()
			a.saveSettings(ctx)
			a.store.Close()
		}
	})
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		a.stopOnce.Do(func() { close(a.done) })
		if a.tray != nil {
			a.tray.Destroy()
			a.tray = nil
		}
	})
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnActivate(func(bool) { a.showMain() })
	return mygo.App.Run()
}

// post runs fn on the main thread and redraws the windows. Before the first
// window exists, startup work runs fn directly.
func (a *app) post(fn func()) {
	if a.quitting.Load() {
		return
	}
	if a.win == nil {
		fn()
		a.redraw()
		return
	}
	a.win.Update(func() {
		fn()
		a.redraw()
	})
}

func (a *app) redraw() {
	if a.win != nil {
		a.win.Invalidate()
	}
	if a.settingsWin != nil {
		a.settingsWin.Invalidate()
	}
}

func (a *app) showMain() {
	if a.win == nil {
		return
	}
	if a.win.IsMinimized() {
		a.win.Restore()
	}
	a.win.Show()
	a.win.Focus()
	mygo.App.Focus()
}

func (a *app) onClose(e *mygo.CloseEvent) {
	if a.quitting.Load() {
		return
	}
	if a.canHideToTray() || runtime.GOOS == "darwin" {
		e.PreventDefault()
		a.win.Hide()
		return
	}
	mygo.App.Quit()
}

func (a *app) canHideToTray() bool {
	return a.settings.ShowTray && a.settings.CloseToTray && a.tray != nil
}

// say shows a message on the next frame.
func (a *app) say(msg string) {
	a.toast = msg
	a.redraw()
}

func (a *app) openLink(raw string) {
	if raw == "" {
		return
	}
	if err := mygo.Shell.OpenExternal(raw); err != nil {
		a.say("无法打开链接")
	}
}

func (a *app) saveSettings(ctx context.Context) {
	if a.store == nil {
		return
	}
	a.settings.LastScope = encodeScope(a.scope)
	a.settings.lastRefreshAt = a.lastRefresh.Unix()
	if err := saveSettings(ctx, a.store, a.settings); err != nil {
		// The reader keeps working with the choices it has.
		fmt.Fprintf(os.Stderr, "zrss: save settings: %v\n", err)
	}
}

func (a *app) changed() {
	if a.store != nil {
		a.saveSettings(context.Background())
	}
	a.settings.ApplyAppearance()
	a.applyTray()
	a.redraw()
}

// startScheduler checks every 30 seconds whether a refresh is due, so that
// changing the interval takes effect without a restart.
func (a *app) startScheduler() {
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-a.done:
				return
			case <-t.C:
				a.post(func() {
					if a.quitting.Load() || a.refreshing || a.settings.RefreshMin <= 0 {
						return
					}
					due := time.Duration(a.settings.RefreshMin) * time.Minute
					if a.lastRefresh.IsZero() || time.Since(a.lastRefresh) >= due {
						a.refreshAll(false)
					}
				})
			}
		}
	}()
}

func dataPath(name string) (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}
