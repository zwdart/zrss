package app

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"xin.dart.zrss/internal/feed"
	"xin.dart.zrss/internal/opml"
	"xin.dart.zrss/internal/storage"
)

// scope selects what the article list shows, and remembers the folder or
// feed it belongs to.
func (a *app) scopeName(s storage.Scope) string {
	switch s.Kind {
	case storage.ScopeUnread:
		return "未读"
	case storage.ScopeStarred:
		return "星标"
	case storage.ScopeToday:
		return "今天"
	case storage.ScopeFeed:
		for _, f := range a.feeds {
			if f.ID == s.ID {
				return f.Title
			}
		}
		return "订阅"
	case storage.ScopeFolder:
		if path := a.folderPath(s.ID); path != "" {
			return path
		}
		return "文件夹"
	case storage.ScopeSearch:
		return "搜索「" + strings.TrimSpace(s.Query) + "」"
	default:
		return "全部文章"
	}
}

// encodeScope writes the sidebar choice for the next launch. Search results
// are a visit, not a place, so they are saved as all articles.
func encodeScope(s storage.Scope) string {
	switch s.Kind {
	case storage.ScopeUnread, storage.ScopeStarred, storage.ScopeToday:
		return string(s.Kind)
	case storage.ScopeFeed:
		return "feed:" + strconv.FormatInt(s.ID, 10)
	case storage.ScopeFolder:
		return "folder:" + strconv.FormatInt(s.ID, 10)
	default:
		return "all"
	}
}

func decodeScope(v string) storage.Scope {
	switch {
	case v == string(storage.ScopeUnread), v == string(storage.ScopeStarred), v == string(storage.ScopeToday):
		return storage.Scope{Kind: storage.ScopeKind(v)}
	case strings.HasPrefix(v, "feed:"):
		if id, err := strconv.ParseInt(strings.TrimPrefix(v, "feed:"), 10, 64); err == nil {
			return storage.Scope{Kind: storage.ScopeFeed, ID: id}
		}
	case strings.HasPrefix(v, "folder:"):
		if id, err := strconv.ParseInt(strings.TrimPrefix(v, "folder:"), 10, 64); err == nil {
			return storage.Scope{Kind: storage.ScopeFolder, ID: id}
		}
	}
	return storage.Scope{Kind: storage.ScopeAll}
}

// restoreScope brings back the sidebar choice of the last launch, unless its
// feed or folder is gone.
func (a *app) restoreScope(ctx context.Context) {
	s := decodeScope(a.settings.LastScope)
	switch s.Kind {
	case storage.ScopeFeed:
		if _, err := a.store.Feed(ctx, s.ID); err != nil {
			s = storage.Scope{Kind: storage.ScopeAll}
		}
	case storage.ScopeFolder:
		ok := false
		for _, f := range a.folders {
			if f.ID == s.ID {
				ok = true
				break
			}
		}
		if !ok {
			s = storage.Scope{Kind: storage.ScopeAll}
		}
	}
	a.scope = s
	a.scopeTitle = a.scopeName(s)
}

const (
	defaultFolderName    = "默认"
	defaultFolderSetting = "defaultFolderID"
)

// ensureDefaultFolder returns the folder that holds subscriptions without a
// chosen folder, creating it on first use. Its id is remembered because the
// user may rename it; the setting also survives a restart.
func (a *app) ensureDefaultFolder(ctx context.Context) int64 {
	if a.store == nil {
		return 0
	}
	folders, err := a.store.Folders(ctx)
	if err != nil {
		return a.defaultFolder
	}
	if a.defaultFolder != 0 {
		for _, f := range folders {
			if f.ID == a.defaultFolder {
				return a.defaultFolder
			}
		}
	}
	if raw, err := a.store.Setting(ctx, defaultFolderSetting, ""); err == nil {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
			for _, f := range folders {
				if f.ID == id {
					a.defaultFolder = id
					return id
				}
			}
		}
	}
	for _, f := range folders {
		if f.ParentID == 0 && strings.EqualFold(f.Name, defaultFolderName) {
			a.defaultFolder = f.ID
			_ = a.store.SetSetting(ctx, defaultFolderSetting, strconv.FormatInt(f.ID, 10))
			return f.ID
		}
	}
	id, err := a.store.CreateFolder(ctx, defaultFolderName, 0)
	if err != nil {
		return 0
	}
	a.defaultFolder = id
	_ = a.store.SetSetting(ctx, defaultFolderSetting, strconv.FormatInt(id, 10))
	return id
}

// adoptRootFeeds moves subscriptions that older versions kept at the top
// level into the default folder, so the sidebar never shows a feed outside a
// folder.
func (a *app) adoptRootFeeds(ctx context.Context) {
	if a.defaultFolder == 0 {
		return
	}
	feeds, err := a.store.Feeds(ctx)
	if err != nil {
		return
	}
	for _, f := range feeds {
		if f.FolderID == 0 {
			_ = a.store.MoveFeed(ctx, f.ID, a.defaultFolder)
		}
	}
}

func (a *app) isDefaultFolder(id int64) bool {
	return id != 0 && id == a.defaultFolder
}

// reloadSidebar reads the folders, subscriptions and badge counts. Every
// subscription lives in a folder: the default folder catches feeds that
// would otherwise sit at the top level, including feeds from older versions.
func (a *app) reloadSidebar(ctx context.Context) {
	if a.store == nil {
		return
	}
	a.defaultFolder = a.ensureDefaultFolder(ctx)
	a.adoptRootFeeds(ctx)
	folders, err := a.store.Folders(ctx)
	if err != nil {
		a.say("无法读取订阅")
		return
	}
	feeds, err := a.store.Feeds(ctx)
	if err != nil {
		a.say("无法读取订阅")
		return
	}
	counts, err := a.store.Counts(ctx)
	if err != nil {
		counts = a.counts
	}
	history, _ := a.store.RecentSearches(ctx, 8)
	a.folders, a.feeds, a.counts, a.history = folders, feeds, counts, history
}

// reloadArticles loads the first page of the current scope. The database is
// local, so this stays on the main thread and the list never shows a frame of
// the previous scope.
func (a *app) reloadArticles(ctx context.Context) {
	if a.store == nil {
		return
	}
	a.pendingMove = 0
	oldID := int64(0)
	if a.hasCurrent {
		oldID = a.current.ID
	}
	rows, err := a.store.Articles(ctx, a.scope, 0, pageSize)
	if err != nil {
		a.loadErr = "无法加载文章"
		a.articles, a.total, a.hasMore, a.selected, a.hasCurrent = nil, 0, false, -1, false
		return
	}
	total, _ := a.store.Count(ctx, a.scope)
	a.loadErr = ""
	a.articles = rows
	a.total = total
	a.hasMore = len(rows) < total
	if total == 0 {
		a.selected, a.hasCurrent = -1, false
		return
	}
	if oldID != 0 {
		for i, art := range a.articles {
			if art.ID == oldID {
				a.selected = i
				a.current = art
				a.hasCurrent = true
				return
			}
		}
	}
	if a.selected >= len(a.articles) {
		a.selected = len(a.articles) - 1
	}
	if a.selected >= 0 {
		a.current = a.articles[a.selected]
		a.hasCurrent = true
		return
	}
	// A new scope starts with nothing chosen: the reader says how to begin.
	a.hasCurrent = false
}

// loadMore appends the next page once the list reaches its end.
func (a *app) loadMore(ctx context.Context) {
	if a.store == nil || a.loading || !a.hasMore {
		return
	}
	offset := len(a.articles)
	scope := a.scope
	a.loading = true
	go func() {
		rows, err := a.store.Articles(ctx, scope, offset, pageSize)
		if err != nil {
			a.post(func() {
				a.loading = false
				a.pendingMove = 0
				a.say("无法加载更多文章")
			})
			return
		}
		a.post(func() {
			a.loading = false
			if a.scope != scope {
				return
			}
			if len(rows) == 0 {
				a.hasMore = false
			} else {
				a.articles = append(a.articles, rows...)
				a.hasMore = len(a.articles) < a.total
			}
			// A reader step waits at the end of the loaded rows: take it
			// now that the next page has arrived.
			if step := a.pendingMove; step != 0 && len(rows) > 0 {
				a.pendingMove = 0
				a.moveSelection(ctx, step)
			}
		})
	}()
}

// setScope chooses a new list and loads it.
func (a *app) setScope(s storage.Scope) {
	if a.scope == s {
		return
	}
	a.scope = s
	a.scopeTitle = a.scopeName(s)
	a.selected = -1
	a.hasCurrent = false
	a.loadErr = ""
	a.reloadArticles(context.Background())
	a.saveSettings(context.Background())
}

func (a *app) openFeed(id int64) {
	for _, f := range a.feeds {
		if f.ID == id {
			a.setScope(storage.Scope{Kind: storage.ScopeFeed, ID: id})
			return
		}
	}
}

func (a *app) selectArticleAt(ctx context.Context, i int) {
	if i < 0 || i >= len(a.articles) {
		return
	}
	old := a.current.ID
	a.selected = i
	a.current = a.articles[i]
	a.hasCurrent = true
	if old != a.current.ID {
		a.readerScroll = ui.ScrollState{}
	}
	if a.settings.MarkRead && !a.current.Read {
		if err := a.store.SetRead(ctx, []int64{a.current.ID}, true); err == nil {
			a.current.Read = true
			a.articles[i].Read = true
			a.reloadSidebar(ctx)
		}
	}
}

// moveSelection opens the article delta rows away, as j and k do.
func (a *app) moveSelection(ctx context.Context, delta int) {
	if len(a.articles) == 0 {
		return
	}
	next := a.selected + delta
	if a.selected < 0 {
		if delta > 0 {
			next = 0
		} else {
			next = len(a.articles) - 1
		}
	}
	if next < 0 {
		a.pendingMove = 0
		return
	}
	if next >= len(a.articles) {
		if a.hasMore {
			// The step waits for the next page, whether it is already on its
			// way or starts now.
			a.pendingMove = delta
			if !a.loading {
				a.loadMore(ctx)
			}
		} else {
			a.pendingMove = 0
		}
		return
	}
	a.pendingMove = 0
	a.selectArticleAt(ctx, next)
	a.listState.ScrollIntoView(next)
}

func (a *app) toggleReadCurrent(ctx context.Context) {
	if !a.hasCurrent {
		return
	}
	read := !a.current.Read
	if err := a.store.SetRead(ctx, []int64{a.current.ID}, read); err != nil {
		a.say("无法标记已读")
		return
	}
	a.current.Read = read
	for i := range a.articles {
		if a.articles[i].ID == a.current.ID {
			a.articles[i].Read = read
		}
	}
	a.reloadSidebar(ctx)
}

func (a *app) toggleStarCurrent(ctx context.Context) {
	if !a.hasCurrent {
		return
	}
	starred := !a.current.Starred
	if err := a.store.SetStarred(ctx, []int64{a.current.ID}, starred); err != nil {
		a.say("无法收藏文章")
		return
	}
	a.current.Starred = starred
	for i := range a.articles {
		if a.articles[i].ID == a.current.ID {
			a.articles[i].Starred = starred
		}
	}
	a.reloadSidebar(ctx)
}

// markScopeRead empties the unread badge of what the list shows.
func (a *app) markScopeRead(ctx context.Context) {
	n, err := a.store.MarkScopeRead(ctx, a.scope)
	if err != nil {
		a.say("无法标记已读")
		return
	}
	for i := range a.articles {
		a.articles[i].Read = true
	}
	if a.hasCurrent {
		a.current.Read = true
	}
	a.reloadSidebar(ctx)
	if a.scope.Kind == storage.ScopeUnread {
		a.reloadArticles(ctx)
	}
	if n > 0 {
		a.say(fmt.Sprintf("已将 %d 篇文章标为已读", n))
	} else {
		a.say("没有未读文章")
	}
}

// markEverythingRead empties every unread badge, from the menu or the tray.
func (a *app) markEverythingRead(ctx context.Context) {
	n, err := a.store.MarkScopeRead(ctx, storage.Scope{Kind: storage.ScopeAll})
	if err != nil {
		a.say("无法标记已读")
		return
	}
	a.reloadSidebar(ctx)
	a.reloadArticles(ctx)
	a.say(fmt.Sprintf("已将 %d 篇文章标为已读", n))
}

// adjustReaderSize changes the reading size in steps, from the View menu.
func (a *app) adjustReaderSize(delta int) {
	size := int(a.settings.ReaderSize) + delta
	if size < 12 {
		size = 12
	}
	if size > 24 {
		size = 24
	}
	a.settings.ReaderSize = float32(size)
	a.changed()
}

func (a *app) markFeedRead(ctx context.Context, id int64) {
	n, err := a.store.MarkScopeRead(ctx, storage.Scope{Kind: storage.ScopeFeed, ID: id})
	if err != nil {
		a.say("无法标记已读")
		return
	}
	a.reloadSidebar(ctx)
	a.reloadArticles(ctx)
	a.say(fmt.Sprintf("已将 %d 篇文章标为已读", n))
}

func (a *app) openOriginal() {
	if !a.hasCurrent {
		return
	}
	target := a.current.URL
	if target == "" {
		for _, f := range a.feeds {
			if f.ID == a.current.FeedID {
				target = f.SiteURL
				break
			}
		}
	}
	if target == "" {
		a.say("这篇文章没有链接")
		return
	}
	a.openLink(target)
}

func (a *app) copyCurrentLink() {
	if !a.hasCurrent || a.current.URL == "" {
		a.say("这篇文章没有链接")
		return
	}
	mygo.Clipboard.WriteText(a.current.URL)
	a.say("已复制链接")
}

// scheduleSearch waits for a pause in typing, then searches what the field
// holds. Each keystroke starts the wait again.
func (a *app) scheduleSearch() {
	if a.searchTimer != nil {
		a.searchTimer.Stop()
	}
	a.searchTimer = time.AfterFunc(220*time.Millisecond, func() {
		a.post(a.applySearch)
	})
}

func (a *app) applySearch() {
	q := strings.TrimSpace(a.query)
	if q == "" {
		if a.scope.Kind == storage.ScopeSearch {
			a.setScope(a.searchBase)
		}
		return
	}
	if a.scope.Kind != storage.ScopeSearch {
		a.searchBase = a.scope
	}
	a.scope = storage.Scope{Kind: storage.ScopeSearch, Query: q}
	a.scopeTitle = a.scopeName(a.scope)
	a.selected = -1
	a.hasCurrent = false
	a.reloadArticles(context.Background())
}

// submitSearch remembers the query after Enter, keeping the history short.
func (a *app) submitSearch(ctx context.Context) {
	q := strings.TrimSpace(a.query)
	if q == "" {
		return
	}
	if err := a.store.AddSearch(ctx, q); err == nil {
		a.reloadSidebar(ctx)
	}
	a.applySearch()
}

func (a *app) clearSearch() {
	a.query = ""
	if a.scope.Kind == storage.ScopeSearch {
		a.setScope(a.searchBase)
	}
}

func (a *app) searchHistoryItem(ctx context.Context, q string) {
	a.query = q
	a.submitSearch(ctx)
}

func (a *app) clearSearchHistory(ctx context.Context) {
	if err := a.store.ClearSearches(ctx); err != nil {
		a.say("无法清除搜索记录")
		return
	}
	a.reloadSidebar(ctx)
	a.say("已清除搜索记录")
}

// Subscriptions and folders.

func (a *app) folderByID(id int64) (storage.Folder, bool) {
	for _, f := range a.folders {
		if f.ID == id {
			return f, true
		}
	}
	return storage.Folder{}, false
}

func (a *app) folderName(id int64) string {
	if f, ok := a.folderByID(id); ok {
		return f.Name
	}
	return ""
}

// folderPath joins a folder and its parents with " / ".
func (a *app) folderPath(id int64) string {
	var names []string
	seen := map[int64]bool{}
	for id != 0 && !seen[id] {
		seen[id] = true
		f, ok := a.folderByID(id)
		if !ok {
			break
		}
		names = append([]string{f.Name}, names...)
		id = f.ParentID
	}
	return strings.Join(names, " / ")
}

func (a *app) folderExists(id int64) bool {
	_, ok := a.folderByID(id)
	return ok
}

func (a *app) folderDepth(id int64) int {
	depth := 0
	seen := map[int64]bool{}
	for id != 0 && !seen[id] {
		seen[id] = true
		f, ok := a.folderByID(id)
		if !ok {
			break
		}
		depth++
		id = f.ParentID
	}
	return depth
}

// folderChildren returns the folders directly under parentID, in sidebar
// order.
func (a *app) folderChildren(parentID int64) []storage.Folder {
	out := make([]storage.Folder, 0, len(a.folders))
	for _, f := range a.folders {
		if f.ParentID == parentID {
			out = append(out, f)
		}
	}
	return out
}

// folderDescendants returns id and every folder below it.
func (a *app) folderDescendants(id int64) map[int64]bool {
	out := map[int64]bool{}
	queue := []int64{id}
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		if out[next] {
			continue
		}
		out[next] = true
		for _, f := range a.folders {
			if f.ParentID == next {
				queue = append(queue, f.ID)
			}
		}
	}
	return out
}

func (a *app) folderFeeds(id int64) []storage.Feed {
	out := make([]storage.Feed, 0, len(a.feeds))
	for _, f := range a.feeds {
		if f.FolderID == id {
			out = append(out, f)
		}
	}
	return out
}

func (a *app) rootFeeds() []storage.Feed {
	return a.folderFeeds(0)
}

// firstFeedError names the first subscription whose last fetch failed, so an
// empty list can explain why it is empty.
func (a *app) firstFeedError() string {
	for _, f := range a.feeds {
		if strings.TrimSpace(f.LastError) != "" {
			return f.Title + "：" + f.LastError
		}
	}
	return ""
}

// folderUnread sums the unread counts of the folder and its descendants.
func (a *app) folderUnread(id int64) int {
	subtree := a.folderDescendants(id)
	n := 0
	for _, f := range a.feeds {
		if subtree[f.FolderID] {
			n += f.Unread
		}
	}
	return n
}

// folderOption is a folder in tree order, labelled by its full path.
type folderOption struct {
	ID    int64
	Path  string
	Depth int
}

func (a *app) folderOptions() []folderOption {
	var out []folderOption
	var walk func(parentID int64, depth int)
	walk = func(parentID int64, depth int) {
		for _, f := range a.folderChildren(parentID) {
			out = append(out, folderOption{ID: f.ID, Path: a.folderPath(f.ID), Depth: depth})
			walk(f.ID, depth+1)
		}
	}
	walk(0, 1)
	return out
}

func (a *app) folderIsDescendant(candidate, ancestor int64) bool {
	return a.folderDescendants(ancestor)[candidate]
}

func (a *app) newFolder(parentID int64) {
	title := "新建文件夹"
	if parentID != 0 {
		title = "新建子文件夹"
	}
	a.prompt.open = true
	a.prompt.title = title
	a.prompt.label = "文件夹名称"
	a.prompt.value = ""
	a.prompt.action = func(name string) {
		ctx := context.Background()
		id, err := a.store.CreateFolder(ctx, name, parentID)
		if err != nil {
			switch {
			case errors.Is(err, storage.ErrFolderDepth):
				a.say("文件夹最多三级")
			case errors.Is(err, storage.ErrFolderExists):
				a.say("同一级已有同名文件夹")
			default:
				a.say("无法创建文件夹")
			}
			return
		}
		if parentID != 0 {
			a.collapsed[parentID] = false
		}
		a.collapsed[id] = false
		a.reloadSidebar(ctx)
	}
}

func (a *app) renameFolder(id int64) {
	name := a.folderName(id)
	a.prompt.open = true
	a.prompt.title = "重命名文件夹"
	a.prompt.label = "文件夹名称"
	a.prompt.value = name
	a.prompt.action = func(next string) {
		ctx := context.Background()
		if err := a.store.RenameFolder(ctx, id, next); err != nil {
			if errors.Is(err, storage.ErrFolderExists) {
				a.say("同一级已有同名文件夹")
			} else {
				a.say("无法重命名文件夹")
			}
			return
		}
		a.reloadSidebar(ctx)
		if a.scope.Kind == storage.ScopeFolder && a.scope.ID == id {
			a.scopeTitle = a.scopeName(a.scope)
		}
	}
}

func (a *app) deleteFolder(id int64) {
	name := a.folderPath(id)
	if name == "" {
		name = a.folderName(id)
	}
	a.confirm.open = true
	a.confirm.title = "删除文件夹"
	a.confirm.message = "确定删除文件夹「" + name + "」吗？它下面的子文件夹会一起删除，订阅会移到上一级，文章不会删除。"
	a.confirm.action = "删除"
	a.confirm.run = func() {
		ctx := context.Background()
		if err := a.store.DeleteFolder(ctx, id); err != nil {
			a.say("无法删除文件夹")
			return
		}
		a.reloadSidebar(ctx)
		if a.scope.Kind == storage.ScopeFolder && !a.folderExists(a.scope.ID) {
			a.setScope(storage.Scope{Kind: storage.ScopeAll})
			return
		}
		a.reloadArticles(ctx)
	}
}

func (a *app) renameFeed(id int64) {
	title := ""
	for _, f := range a.feeds {
		if f.ID == id {
			title = f.Title
			break
		}
	}
	a.prompt.open = true
	a.prompt.title = "重命名订阅"
	a.prompt.label = "订阅名称"
	a.prompt.value = title
	a.prompt.action = func(next string) {
		ctx := context.Background()
		if err := a.store.RenameFeed(ctx, id, next); err != nil {
			a.say("无法重命名订阅")
			return
		}
		a.reloadSidebar(ctx)
		if a.scope.Kind == storage.ScopeFeed && a.scope.ID == id {
			a.scopeTitle = next
		}
	}
}

func (a *app) deleteFeed(id int64) {
	title := ""
	for _, f := range a.feeds {
		if f.ID == id {
			title = f.Title
			break
		}
	}
	a.confirm.open = true
	a.confirm.title = "取消订阅"
	a.confirm.message = "确定不再订阅「" + title + "」吗？它的文章也会从本机删除，且无法恢复。"
	a.confirm.action = "取消订阅"
	a.confirm.run = func() {
		ctx := context.Background()
		if err := a.store.DeleteFeed(ctx, id); err != nil {
			a.say("无法取消订阅")
			return
		}
		if a.scope.Kind == storage.ScopeFeed && a.scope.ID == id {
			a.setScope(storage.Scope{Kind: storage.ScopeAll})
		}
		a.reloadSidebar(ctx)
		a.reloadArticles(ctx)
		a.say("已取消订阅")
	}
}

func (a *app) moveFeed(ctx context.Context, feedID, folderID int64) {
	if err := a.store.MoveFeed(ctx, feedID, folderID); err != nil {
		a.say("无法移动订阅")
		return
	}
	if folderID != 0 {
		a.collapsed[folderID] = false
	}
	a.reloadSidebar(ctx)
}

func (a *app) moveFolder(ctx context.Context, id, parentID int64) {
	if err := a.store.MoveFolder(ctx, id, parentID); err != nil {
		switch {
		case errors.Is(err, storage.ErrFolderDepth):
			a.say("文件夹最多三级")
		case errors.Is(err, storage.ErrFolderCycle):
			a.say("不能把文件夹移动到它自己里面")
		case errors.Is(err, storage.ErrFolderExists):
			a.say("目标位置已有同名文件夹")
		default:
			a.say("无法移动文件夹")
		}
		return
	}
	if parentID != 0 {
		a.collapsed[parentID] = false
	}
	a.reloadSidebar(ctx)
	if a.scope.Kind == storage.ScopeFolder {
		a.scopeTitle = a.scopeName(a.scope)
	}
}

// subscribe validates a URL and adds it. The dialog closes first, and the
// result is reported as a toast.
func (a *app) subscribe(raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		a.add.err = "请输入网站或订阅地址"
		return
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	ctx := context.Background()
	folderID := a.add.folderID
	if folderID == 0 || !a.folderExists(folderID) {
		folderID = a.ensureDefaultFolder(ctx)
	}
	a.add.open = false
	a.add.err = ""
	a.say("正在查找订阅源…")
	go func() {
		f, ok, fetchErr := a.resolveFeed(ctx, raw)
		if !ok {
			msg := "没有找到这个地址的订阅源"
			if fetchErr != nil {
				msg += "：" + truncate(fetchErr.Error(), 160)
			}
			a.post(func() { a.say(msg) })
			return
		}
		f.FolderID = folderID
		id, added, err := a.store.AddFeed(ctx, f)
		if err != nil {
			a.post(func() { a.say("无法添加订阅") })
			return
		}
		if !added {
			a.post(func() {
				a.reloadSidebar(ctx)
				a.openFeed(id)
				a.say("已经订阅过了")
			})
			return
		}
		var refreshErr error
		var newArticles int
		if got, err := a.store.Feed(ctx, id); err == nil {
			newArticles, refreshErr = a.refreshFeed(ctx, got)
		}
		a.post(func() {
			a.reloadSidebar(ctx)
			a.reloadArticles(ctx)
			a.openFeed(id)
			switch {
			case refreshErr != nil:
				a.say("已添加订阅，但抓取文章失败：" + truncate(refreshErr.Error(), 160))
			case newArticles > 0:
				a.say(fmt.Sprintf("已订阅「%s」，收到 %d 篇文章", f.Title, newArticles))
			default:
				a.say("已订阅「" + f.Title + "」")
			}
		})
	}()
}

// resolveFeed tries the address itself, the feed links of an HTML page, and
// the paths feeds commonly live at. The error is the first failed attempt, so
// the dialog can explain a timeout or TLS failure instead of only saying that
// nothing was found.
func (a *app) resolveFeed(ctx context.Context, raw string) (storage.Feed, bool, error) {
	seen := map[string]bool{}
	var firstErr error
	try := func(candidate string) (*feed.FetchResult, string, bool) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || seen[candidate] {
			return nil, "", false
		}
		seen[candidate] = true
		res, err := a.fetcher.Fetch(ctx, candidate, "", "")
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return nil, "", false
		}
		if res == nil || res.Doc == nil {
			return nil, "", false
		}
		return res, candidate, true
	}
	if res, used, ok := try(raw); ok {
		return feedFrom(res, used), true, nil
	}
	if links, err := a.fetcher.Discover(ctx, raw); err == nil {
		for _, link := range links {
			if res, used, ok := try(link); ok {
				return feedFrom(res, used), true, nil
			}
		}
	}
	for _, candidate := range feed.Candidates(raw) {
		if res, used, ok := try(candidate); ok {
			return feedFrom(res, used), true, nil
		}
	}
	return storage.Feed{}, false, firstErr
}

func feedFrom(res *feed.FetchResult, used string) storage.Feed {
	doc := res.Doc
	title := strings.TrimSpace(doc.Title)
	if title == "" {
		u, _ := url.Parse(used)
		title = strings.TrimPrefix(u.Host, "www.")
	}
	if title == "" {
		title = used
	}
	return storage.Feed{
		URL:          used,
		SiteURL:      doc.Link,
		Title:        title,
		Description:  doc.Description,
		IconURL:      doc.IconURL,
		ETag:         res.ETag,
		LastModified: res.LastModified,
		LastFetch:    time.Now(),
	}
}

// OPML import and export.

// exportOPML writes every subscription in folders, keeping the sidebar order.

func (a *app) importOPML() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: a.win,
		Title:  "导入订阅",
		Filters: []mygo.FileFilter{
			{Name: "OPML 订阅列表", Extensions: []string{"opml", "xml"}},
		},
	})
	if err != nil || len(paths) == 0 {
		return
	}
	path := paths[0]
	go func() {
		data, err := os.ReadFile(path)
		if err != nil {
			a.post(func() { a.say("无法读取文件") })
			return
		}
		doc, err := opml.Parse(data)
		if err != nil {
			a.post(func() { a.say("不是有效的 OPML 文件") })
			return
		}
		ctx := context.Background()
		added, skipped := a.addOPML(ctx, doc)
		a.post(func() {
			a.reloadSidebar(ctx)
			a.reloadArticles(ctx)
			if added > 0 {
				a.refreshAll(false)
				a.say(fmt.Sprintf("导入 %d 个订阅", added))
			} else if skipped > 0 {
				a.say("这些订阅都已经在列表里了")
			} else {
				a.say("没有找到订阅")
			}
		})
	}()
}

func (a *app) addOPML(ctx context.Context, doc *opml.Document) (added, skipped int) {
	defaultID := a.ensureDefaultFolder(ctx)
	add := func(f opml.Feed, folderID int64) {
		if strings.TrimSpace(f.URL) == "" {
			return
		}
		if folderID == 0 {
			folderID = defaultID
		}
		_, isNew, err := a.store.AddFeed(ctx, storage.Feed{
			URL:         strings.TrimSpace(f.URL),
			SiteURL:     strings.TrimSpace(f.SiteURL),
			Title:       firstOf(f.Title, hostOf(f.URL), f.URL),
			Description: strings.TrimSpace(f.Description),
			FolderID:    folderID,
			LastFetch:   time.Now(),
		})
		if err != nil {
			return
		}
		if !isNew {
			skipped++
			return
		}
		added++
	}
	var addFolder func(folder opml.Folder, parentID int64, depth int)
	addFolder = func(folder opml.Folder, parentID int64, depth int) {
		name := strings.TrimSpace(folder.Name)
		target := parentID
		if target == 0 {
			target = defaultID
		}
		if name == "" || depth > 3 {
			// Deeper outlines are collapsed into the third level; their
			// subscriptions are kept even though their extra folder names are
			// not.
			for _, f := range folder.Feeds {
				add(f, target)
			}
			for _, child := range folder.Folders {
				addFolder(child, parentID, depth+1)
			}
			return
		}
		id, err := a.store.CreateFolder(ctx, name, parentID)
		if err != nil {
			for _, f := range folder.Feeds {
				add(f, target)
			}
			for _, child := range folder.Folders {
				addFolder(child, parentID, depth+1)
			}
			return
		}
		for _, f := range folder.Feeds {
			add(f, id)
		}
		for _, child := range folder.Folders {
			addFolder(child, id, depth+1)
		}
	}
	for _, f := range doc.Feeds {
		add(f, defaultID)
	}
	for _, folder := range doc.Folders {
		addFolder(folder, 0, 1)
	}
	return added, skipped
}

func (a *app) exportOPML() {
	doc := &opml.Document{Title: "zrss 订阅"}
	for _, f := range a.rootFeeds() {
		doc.Feeds = append(doc.Feeds, opml.Feed{
			Title: f.Title, URL: f.URL, SiteURL: f.SiteURL, Description: f.Description,
		})
	}
	for _, folder := range a.folderChildren(0) {
		doc.Folders = append(doc.Folders, a.exportFolder(folder))
	}
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
		Parent:      a.win,
		Title:       "导出订阅",
		DefaultPath: filepath.Join(homeDir(), "zrss-subscriptions.opml"),
		Filters: []mygo.FileFilter{
			{Name: "OPML 订阅列表", Extensions: []string{"opml"}},
		},
	})
	if err != nil || path == "" {
		return
	}
	data, err := opml.Encode(doc)
	if err != nil {
		a.say("无法生成 OPML")
		return
	}
	if filepath.Ext(path) == "" {
		path += ".opml"
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		a.say("无法写入文件")
		return
	}
	a.say("已导出 " + strconv.Itoa(len(a.feeds)) + " 个订阅")
}

func (a *app) exportFolder(folder storage.Folder) opml.Folder {
	out := opml.Folder{Name: folder.Name}
	for _, f := range a.folderFeeds(folder.ID) {
		out.Feeds = append(out.Feeds, opml.Feed{
			Title: f.Title, URL: f.URL, SiteURL: f.SiteURL, Description: f.Description,
		})
	}
	for _, child := range a.folderChildren(folder.ID) {
		out.Folders = append(out.Folders, a.exportFolder(child))
	}
	return out
}

// vacuum compacts the database from the settings window.
func (a *app) vacuum(ctx context.Context) {
	if err := a.store.Vacuum(ctx); err != nil {
		a.say("整理数据库失败")
		return
	}
	a.say("数据库已整理")
}

// homeDir is where file dialogs start when the reader has no better place.
func homeDir() string {
	if dir, err := os.UserHomeDir(); err == nil {
		return dir
	}
	return ""
}
