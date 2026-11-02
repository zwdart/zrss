package app

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"xin.dart.zrss/internal/storage"
)

const (
	sidebarWidth = 238
	listWidth    = 382
)

// view draws the whole window: subscriptions, the article list and the
// article being read.
func (a *app) view(c *ui.Context) {
	t := *c.Theme()
	a.pal = paletteFor(&t, a.settings.Accent)
	p := a.pal
	t.Accent, t.AccentText = p.accent, p.onAccent
	t.Selection = p.accent.Alpha(0.28)
	c.SetTheme(&t)

	a.shortcuts(c)

	ui.Column(c).Fill().Children(func() {
		a.titleBar(c)
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			if a.settings.Layout != layoutReader {
				if a.settings.Layout == layoutAll {
					a.sidebarPane(c)
					ui.Box(c).Width(1).FillHeight().Background(p.hairline)
				}
				a.listPane(c)
				ui.Box(c).Width(1).FillHeight().Background(p.hairline)
			}
			a.readerPane(c)
		})
	})

	a.addDialogView(c)
	a.promptDialogView(c)
	a.confirmDialogView(c)
	if a.toast != "" {
		c.Toast(a.toast)
		a.toast = ""
	}
}

func (a *app) shortcuts(c *ui.Context) {
	c.OnShortcut(ui.Cmd, ui.KeyR, func() { a.refreshAll(true) })
	c.OnShortcut(ui.Cmd, ui.KeyN, a.newSubscription)
	c.OnShortcut(ui.Cmd, ui.KeyF, func() { a.searchBox.Focus(c) })
	c.OnShortcut(ui.Cmd, ui.KeyComma, a.openSettings)
	c.OnShortcut(ui.Cmd, ui.KeyO, a.openOriginal)
	c.OnShortcut(ui.Cmd|ui.Shift, ui.KeyE, a.exportOPML)
	c.OnShortcut(ui.Cmd|ui.Shift, ui.KeyI, a.importOPML)
	if c.Focused(a.searchBox) {
		c.OnShortcut(0, ui.KeyEscape, a.clearSearch)
		return
	}
	ctx := context.Background()
	c.OnShortcut(0, ui.KeyJ, func() { a.moveSelection(ctx, 1) })
	c.OnShortcut(0, ui.KeyK, func() { a.moveSelection(ctx, -1) })
	c.OnShortcut(0, ui.KeyDown, func() { a.moveSelection(ctx, 1) })
	c.OnShortcut(0, ui.KeyUp, func() { a.moveSelection(ctx, -1) })
	c.OnShortcut(0, ui.KeyU, func() { a.toggleReadCurrent(ctx) })
	c.OnShortcut(0, ui.KeyS, func() { a.toggleStarCurrent(ctx) })
	c.OnShortcut(0, ui.KeyM, func() { a.markScopeRead(ctx) })
	c.OnShortcut(0, ui.KeyO, func() { a.openOriginal() })
	c.OnShortcut(0, ui.KeyEscape, a.clearSearch)
}

func (a *app) newSubscription() {
	a.openAdd(0)
	if a.scope.Kind == storage.ScopeFolder {
		a.add.folderID = a.scope.ID
	}
}

func (a *app) newSubscriptionIn(folderID int64) {
	a.openAdd(folderID)
}

func (a *app) openAdd(folderID int64) {
	if folderID == 0 {
		folderID = a.ensureDefaultFolder(context.Background())
	}
	a.add.open = true
	a.add.url = ""
	a.add.err = ""
	a.add.folderID = folderID
}

// titleBarHeight is the height of the reader's own title bar, which holds
// the brand, the menus and the window controls.
const titleBarHeight = 40

// titleBar is the strip along the top of the window: the brand and the
// menus at the left, the window controls at the right, and the room between
// them that drags the window.
func (a *app) titleBar(c *ui.Context) {
	ui.Row(c).Height(titleBarHeight).FillWidth().AlignItems(ui.Center).DragWindow().
		Background(a.pal.side).BorderWidth(0, 0, 1, 0).BorderColor(a.pal.hairline).
		PaddingStart(7).PaddingEnd(7).Gap(2).Children(func() {
		a.titleBrand(c)
		a.titleMenuButton(c, "menu-file", "文件", a.menuFile)
		a.titleMenuButton(c, "menu-edit", "编辑", a.menuEdit)
		a.titleMenuButton(c, "menu-view", "显示", a.menuView)
		a.titleMenuButton(c, "menu-window", "窗口", a.menuWindow)
		ui.Spacer(c)
		a.windowControls(c)
	})
}

// titleBrand is the icon and name at the left of the title bar. Its menu
// holds what the app menu holds: settings, updates and quitting.
func (a *app) titleBrand(c *ui.Context) {
	p := a.pal
	b := ui.ButtonBase(c.Key("menu-app")).Height(28).PaddingX(8).Gap(7).Radius(7).
		AlignItems(ui.Center).Label("zrss").FocusRing(false)
	bg := ui.Transparent
	if b.Hovered() {
		bg = p.hover
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 110 * time.Millisecond}))
	b.Children(func() {
		ui.Icon(c, iconRSS).Size(17, 17).TextColor(p.accent)
		ui.Text(c, "zrss").FontSize(13.5).FontWeight(700).TextColor(p.text)
	})
	a.titleMenuPopup(b, a.menuApp)
}

// titleMenuButton is a menu of the title bar's menu bar.
func (a *app) titleMenuButton(c *ui.Context, key, label string, menu *mygo.Menu) {
	p := a.pal
	b := ui.ButtonBase(c.Key(key)).Height(28).PaddingX(9).Radius(7).Label(label).FocusRing(false)
	bg := ui.Transparent
	if b.Hovered() {
		bg = p.hover
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 110 * time.Millisecond}))
	b.Children(func() {
		ui.Text(c, label).FontSize(12.5).FontWeight(500).TextColor(p.text)
	})
	a.titleMenuPopup(b, menu)
}

// titleMenuPopup shows the native menu below the button, with the roles,
// shortcuts and submenus the menu bar would have.
func (a *app) titleMenuPopup(b ui.Element, menu *mygo.Menu) {
	if !b.Clicked() || a.win == nil || menu == nil {
		return
	}
	if r := b.Bounds(); r.W > 0 {
		menu.PopupAt(a.win, int(r.X), int(r.Y+r.H+4))
	}
}

// windowControls draws minimize, maximize or restore, and close at the
// right of the title bar, in the colors of the theme.
func (a *app) windowControls(c *ui.Context) {
	maximized := a.win != nil && a.win.IsMaximized()
	ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
		if a.windowButton(c, "window-minimize", iconMinimize, 12, "最小化", false).Clicked() {
			if a.win != nil {
				a.win.Minimize()
			}
		}
		icon, label := iconMaximize, "最大化"
		if maximized {
			icon, label = iconRestore, "还原"
		}
		max := a.windowButton(c, "window-maximize", icon, 12, label, false)
		if !maximized {
			// Holding the pointer over it shows Windows 11's snap layouts.
			max = max.MaximizeButton()
		}
		if max.Clicked() {
			if a.win != nil {
				a.win.ToggleMaximize()
			}
		}
		if a.windowButton(c, "window-close", iconX, 12, "关闭", true).Clicked() {
			if a.win != nil {
				a.win.Close()
			}
		}
	})
}

// windowButton is a window control: a quiet glyph under the pointer, or a
// red one for close, matching the icon buttons around it.
func (a *app) windowButton(c *ui.Context, key string, icon *ui.SVG, size float32, label string, danger bool) ui.Element {
	b := ui.ButtonBase(c.Key(key)).Size(30, 30).Center().Radius(8).Label(label).Tooltip(label).FocusRing(false)
	bg := ui.Transparent
	color := a.pal.muted
	if b.Hovered() {
		bg, color = a.pal.hover, a.pal.text
		if danger {
			bg, color = ui.Hex("#e5484d"), ui.Hex("#ffffff")
		}
	}
	b.TextColor(color)
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	pose := ui.Pose{}
	if b.Pressed() {
		pose.Scale = 0.88
	}
	b.AnimatePose(pose, ui.Snappy)
	b.Children(func() {
		ui.Icon(c, icon).Size(size, size)
	})
	return b
}

// sidebarPane is the subscriptions column.
func (a *app) sidebarPane(c *ui.Context) {
	p := a.pal
	ui.Column(c).Width(sidebarWidth).Shrink(0).FillHeight().Background(p.side).Padding(12, 14, 10, 10).Gap(2).Children(func() {
		a.searchField(c)

		ui.Scroll(c.Key("nav")).Grow(1).Margin(0, -6).Padding(0, 6).Gap(1).Children(func() {
			a.smartItem(c, storage.Scope{Kind: storage.ScopeAll}, iconInbox, "全部文章", a.counts.Unread)
			a.smartItem(c, storage.Scope{Kind: storage.ScopeUnread}, iconDot, "未读", a.counts.Unread)
			a.smartItem(c, storage.Scope{Kind: storage.ScopeStarred}, iconStar, "星标", a.counts.Starred)
			a.smartItem(c, storage.Scope{Kind: storage.ScopeToday}, iconClock, "今天", a.counts.Today)

			a.sidebarSection(c, "文件夹", func() { a.newFolder(0) })
			for _, folder := range a.folderChildren(0) {
				a.folderRow(c, folder, 1)
			}
			if len(a.folders) == 0 {
				ui.Text(c, "还没有文件夹，可右键或点 + 新建").FontSize(11.5).TextColor(p.faint).Padding(10, 10, 4, 10)
			}

			for _, f := range a.rootFeeds() {
				a.feedRow(c, f, 10)
			}
			if len(a.feeds) == 0 {
				ui.Column(c).Padding(10, 12).Gap(8).Children(func() {
					ui.Text(c, "添加第一个订阅，开始阅读").FontSize(12).TextColor(p.muted).LineHeight(1.45)
					if pillButton(c.Key("first-feed"), iconPlus, "添加订阅", p.accent, p.onAccent).Clicked() {
						a.newSubscription()
					}
				})
			}
		})

		ui.Box(c).Height(1).Background(p.hairline).Margin(0, 6)
		ui.Row(c).Height(34).Gap(4).AlignItems(ui.Center).Children(func() {
			if iconButton(c.Key("add-feed"), iconPlus, 16, p.muted, "添加订阅").Clicked() {
				a.newSubscription()
			}
			if iconButton(c.Key("import"), iconImport, 16, p.muted, "导入 OPML").Clicked() {
				a.importOPML()
			}
			if iconButton(c.Key("export"), iconExport, 16, p.muted, "导出 OPML").Clicked() {
				a.exportOPML()
			}
			ui.Spacer(c)
			if iconButton(c.Key("settings"), iconSettings, 16, p.muted, "设置").Clicked() {
				a.openSettings()
			}
		})
	})
}

func (a *app) searchField(c *ui.Context) {
	p := a.pal
	ui.Row(c).Height(32).Radius(8).Background(p.bg).Border(1, p.hairline).PaddingX(8).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, iconSearch).Size(14, 14).TextColor(p.faint)
		in := ui.TextInput(c.Key("search"), &a.query).Bind(&a.searchBox).
			Placeholder("搜索文章").Label("搜索文章").FontSize(12.5).
			BorderWidth(0).Background(ui.Transparent).Grow(1)
		if in.Changed() {
			a.scheduleSearch()
		}
		if in.Submitted() {
			a.submitSearch(context.Background())
		}
		if a.query != "" {
			if iconButton(c.Key("search-clear"), iconX, 12, p.faint, "清除搜索").Clicked() {
				a.clearSearch()
			}
		}
	})
	if c.Focused(a.searchBox) && strings.TrimSpace(a.query) == "" && len(a.history) > 0 {
		ui.Column(c).Margin(0, 2, 0, 4).Gap(0).Children(func() {
			ui.Row(c).Padding(8, 6, 4, 6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "最近搜索").FontSize(10.5).FontWeight(600).TextColor(p.faint).Grow(1)
				if iconButton(c.Key("clear-history"), iconX, 10, p.faint, "清除搜索记录").Clicked() {
					a.clearSearchHistory(context.Background())
				}
			})
			for _, q := range a.history {
				row := ui.ButtonBase(c.Key("history-" + q)).Height(26).PaddingX(8).Gap(7).Radius(6).AlignItems(ui.Center).FocusRing(false).Label(q)
				if row.Hovered() {
					row.Background(p.hover)
				}
				row.Children(func() {
					ui.Icon(c, iconClock).Size(12, 12).TextColor(p.faint)
					ui.Text(c, q).FontSize(12).TextColor(p.muted).SingleLine()
				})
				if row.Clicked() {
					a.searchHistoryItem(context.Background(), q)
				}
			}
		})
	}
}

func (a *app) sidebarSection(c *ui.Context, title string, add func()) {
	p := a.pal
	ui.Row(c).Height(28).Padding(12, 8, 0, 8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, title).FontSize(10.5).FontWeight(650).TextColor(p.faint).Grow(1)
		if add != nil && iconButton(c.Key("section-add-"+title), iconPlus, 10, p.faint, "新建").Clicked() {
			add()
		}
	})
}

func (a *app) smartItem(c *ui.Context, scope storage.Scope, icon *ui.SVG, label string, badge int) {
	p := a.pal
	selected := a.scope.Kind == scope.Kind
	b := ui.ButtonBase(c.Key("smart-" + label)).Height(32).PaddingX(10).Gap(9).Radius(7).AlignItems(ui.Center).FocusRing(false).Label(label)
	bg := ui.Transparent
	if selected {
		bg = p.accent.Alpha(0.13)
	} else if b.Hovered() {
		bg = p.hover
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 110 * time.Millisecond}))
	color := p.text
	if selected {
		color = p.accent
	}
	b.Children(func() {
		ui.Icon(c, icon).Size(16, 16).TextColor(color)
		ui.Text(c, label).FontSize(13).FontWeight(500).TextColor(p.text).Grow(1)
		countBadge(c, badge, p.muted)
	})
	if b.Clicked() {
		a.setScope(scope)
	}
}

func (a *app) folderRow(c *ui.Context, folder storage.Folder, depth int) {
	p := a.pal
	selected := a.scope.Kind == storage.ScopeFolder && a.scope.ID == folder.ID
	closed := a.collapsed[folder.ID]
	row := ui.Row(c).Height(32).Radius(7).PaddingStart(4 + float32(depth-1)*14).PaddingEnd(4).Gap(0).AlignItems(ui.Center)
	if selected {
		row.Background(p.accent.Alpha(0.10))
	}
	row.Children(func() {
		chev := iconButton(c.Key(fmt.Sprintf("folder-%d-chevron", folder.ID)), iconChevron, 12, p.muted, "展开或折叠文件夹")
		if closed {
			chev.Rotate(0)
		} else {
			chev.Rotate(90)
		}
		if chev.Clicked() {
			a.collapsed[folder.ID] = !closed
		}
		name := ui.ButtonBase(c.Key(fmt.Sprintf("folder-%d", folder.ID))).Grow(1).Height(32).Padding(8, 6).Gap(7).Radius(6).AlignItems(ui.Center).FocusRing(false).Label(folder.Name)
		if name.Hovered() {
			name.Background(p.hover)
		}
		name.Children(func() {
			ui.Icon(c, iconFolder).Size(14, 14).TextColor(p.muted)
			ui.Text(c, folder.Name).FontSize(12.5).FontWeight(500).SingleLine().Grow(1)
			countBadge(c, a.folderUnread(folder.ID), p.muted)
		})
		if name.Clicked() {
			a.setScope(storage.Scope{Kind: storage.ScopeFolder, ID: folder.ID})
		}
		name.ContextMenu(func(m *ui.Menu) {
			if depth < 3 && m.Item("新建子文件夹…").Chosen() {
				a.newFolder(folder.ID)
			}
			if m.Item("在此文件夹添加订阅…").Chosen() {
				a.newSubscriptionIn(folder.ID)
			}
			m.Separator()
			if m.Item("重命名文件夹…").Chosen() {
				a.renameFolder(folder.ID)
			}
			m.Submenu("移动到…", func(m *ui.Menu) {
				if folder.ParentID != 0 && m.Item("顶层").Chosen() {
					a.moveFolder(context.Background(), folder.ID, 0)
				}
				for _, opt := range a.folderOptions() {
					if opt.ID == folder.ID || opt.ID == folder.ParentID || a.folderIsDescendant(opt.ID, folder.ID) {
						continue
					}
					if m.Item(opt.Path).Chosen() {
						a.moveFolder(context.Background(), folder.ID, opt.ID)
					}
				}
			})
			m.Separator()
			if !a.isDefaultFolder(folder.ID) && m.Item("删除文件夹…").Chosen() {
				a.deleteFolder(folder.ID)
			}
		})
	})
	if closed {
		return
	}
	for _, child := range a.folderChildren(folder.ID) {
		a.folderRow(c, child, depth+1)
	}
	indent := float32(14 + depth*14)
	for _, f := range a.folderFeeds(folder.ID) {
		a.feedRow(c, f, indent)
	}
}

func (a *app) feedRow(c *ui.Context, f storage.Feed, indent float32) {
	p := a.pal
	selected := a.scope.Kind == storage.ScopeFeed && a.scope.ID == f.ID
	b := ui.ButtonBase(c.Key(fmt.Sprintf("feed-%d", f.ID))).Height(31).PaddingStart(indent).PaddingEnd(8).Gap(7).Radius(7).AlignItems(ui.Center).FocusRing(false).Label(f.Title)
	bg := ui.Transparent
	if selected {
		bg = p.accent.Alpha(0.12)
	} else if b.Hovered() {
		bg = p.hover
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 110 * time.Millisecond}))
	b.Children(func() {
		a.feedIcon(c, f, 15)
		ui.Text(c, f.Title).FontSize(12.5).FontWeight(500).SingleLine().Grow(1)
		if f.LastError != "" {
			ui.Icon(c, iconAlert).Size(12, 12).TextColor(ui.Hex("#ef4444")).Tooltip(f.LastError)
		}
		countBadge(c, f.Unread, p.muted)
	})
	if b.Clicked() {
		a.openFeed(f.ID)
	}
	b.ContextMenu(func(m *ui.Menu) {
		if m.Item("刷新这个订阅").Chosen() {
			a.refreshOne(context.Background(), f.ID)
		}
		if m.Item("全部标为已读").Chosen() {
			a.markFeedRead(context.Background(), f.ID)
		}
		m.Separator()
		m.Submenu("移动到文件夹", func(m *ui.Menu) {
			for _, opt := range a.folderOptions() {
				if opt.ID == f.FolderID {
					continue
				}
				if m.Item(opt.Path).Chosen() {
					a.moveFeed(context.Background(), f.ID, opt.ID)
				}
			}
		})
		if m.Item("重命名…").Chosen() {
			a.renameFeed(f.ID)
		}
		if f.SiteURL != "" && m.Item("打开网站").Chosen() {
			a.openLink(f.SiteURL)
		}
		m.Separator()
		if m.Item("取消订阅…").Chosen() {
			a.deleteFeed(f.ID)
		}
	})
}

func (a *app) feedIcon(c *ui.Context, f storage.Feed, size float32) {
	p := a.pal
	box := ui.Box(c).Size(size, size).Radius(3).Clip().Background(p.press)
	box.Children(func() {
		if a.imgs != nil && f.IconURL != "" {
			if src := a.imgs.cached(f.IconURL); src != nil {
				ui.Image(c, src).Size(size, size).Fit(ui.Cover)
				return
			}
		}
		letter := firstRune(f.Title)
		if letter == "" {
			letter = "?"
		}
		ui.Text(c, strings.ToUpper(letter)).FontSize(size * 0.62).FontWeight(700).TextColor(p.muted)
	})
}

// listPane is the column of articles.

func (a *app) listPane(c *ui.Context) {
	p := a.pal
	ui.Column(c).Width(listWidth).Shrink(0).FillHeight().Background(p.list).Children(func() {
		ui.Column(c).Padding(18, 16, 10, 14).Gap(8).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, a.scopeTitle).FontSize(17).FontWeight(700).SingleLine().Grow(1)
				if iconButton(c.Key("mark-all"), iconCheckAll, 15, p.muted, "全部标为已读").Clicked() {
					a.markScopeRead(context.Background())
				}
				if iconButton(c.Key("refresh-now"), iconRefresh, 15, p.muted, "刷新订阅").Clicked() {
					a.refreshAll(true)
				}
			})
			ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
				label := fmt.Sprintf("共 %d 篇文章", a.total)
				if a.hasMore {
					label += fmt.Sprintf(" · 已加载 %d 篇", len(a.articles))
				}
				ui.Text(c, label).FontSize(11.5).TextColor(p.faint)
				if a.refreshing {
					ui.Spinner(c).Size(11, 11)
					ui.Text(c, "正在刷新…").FontSize(11.5).TextColor(p.faint)
				} else if !a.lastRefresh.IsZero() {
					ui.Text(c, "· 上次刷新 "+relativeTime(a.lastRefresh)).FontSize(11.5).TextColor(p.faint)
				}
			})
		})
		ui.Box(c).Height(1).Background(p.hairline)

		a.listState.Key = func(i int) any {
			if i >= 0 && i < len(a.articles) {
				return a.articles[i].ID
			}
			return i
		}
		a.listState.Selected = &a.selected
		a.listState.Label = func(i int) string {
			if i >= 0 && i < len(a.articles) {
				return a.articles[i].Title
			}
			return ""
		}

		if a.loadErr != "" {
			ui.Column(c).Grow(1).Center().Gap(10).Children(func() {
				ui.Text(c, a.loadErr).FontSize(13).TextColor(p.muted)
				if ui.Button(c, "重试").Clicked() {
					a.reloadArticles(context.Background())
				}
			})
			return
		}
		if len(a.articles) == 0 {
			a.emptyList(c)
			return
		}
		ui.List(c.Key("articles"), &a.listState, len(a.articles), func(i int) {
			a.articleRow(c, i)
		}).Grow(1)
		if a.hasMore && a.listState.AtEnd() {
			a.loadMore(context.Background())
		}
	})
}

func (a *app) emptyList(c *ui.Context) {
	p := a.pal
	feedErr := a.firstFeedError()
	ui.Column(c).Grow(1).Center().Padding(30, 20).Gap(10).Children(func() {
		ui.Icon(c, iconMailOpen).Size(34, 34).TextColor(p.faint)
		switch {
		case a.scope.Kind == storage.ScopeSearch:
			ui.Text(c, "没有找到相关文章").FontSize(13).FontWeight(600).TextColor(p.muted)
			ui.Text(c, "换个词试试，或按 Esc 清除搜索").FontSize(12).TextColor(p.faint)
		case a.scope.Kind == storage.ScopeUnread:
			ui.Text(c, "都读完了").FontSize(13).FontWeight(600).TextColor(p.muted)
			ui.Text(c, "新文章会在刷新后出现在这里").FontSize(12).TextColor(p.faint)
		case a.scope.Kind == storage.ScopeStarred:
			ui.Text(c, "还没有星标文章").FontSize(13).FontWeight(600).TextColor(p.muted)
			ui.Text(c, "按 S 可以为正在读的文章加星").FontSize(12).TextColor(p.faint)
		case len(a.feeds) == 0:
			ui.Text(c, "开始你的订阅").FontSize(13).FontWeight(600).TextColor(p.muted)
			ui.Text(c, "添加一个 RSS 或 Atom 订阅").FontSize(12).TextColor(p.faint)
			if pillButton(c.Key("empty-add"), iconPlus, "添加订阅", p.accent, p.onAccent).Clicked() {
				a.newSubscription()
			}
		case feedErr != "":
			ui.Text(c, "订阅暂时没有文章").FontSize(13).FontWeight(600).TextColor(p.muted)
			ui.Text(c, "上次刷新失败："+truncate(feedErr, 180)).FontSize(12).TextColor(ui.Hex("#ef4444")).TextAlign(ui.Center)
		default:
			ui.Text(c, "这里还没有文章").FontSize(13).FontWeight(600).TextColor(p.muted)
			ui.Text(c, "刷新订阅看看有没有新内容").FontSize(12).TextColor(p.faint)
		}
	})
}

func (a *app) articleRow(c *ui.Context, i int) {
	p := a.pal
	art := &a.articles[i]
	selected := i == a.selected
	b := ui.ButtonBase(c.Key(art.ID)).WidthPercent(100).Padding(13, 18).Gap(6).Justify(ui.Start).AlignItems(ui.Stretch).FocusRing(false).Label(art.Title)
	bg := ui.Transparent
	switch {
	case selected:
		bg = p.accent.Alpha(0.11)
	case b.Hovered():
		bg = p.hover
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 110 * time.Millisecond}))
	b.BorderWidth(0, 0, 1, 0).BorderColor(p.hairline)
	b.Children(func() {
		ui.Row(c).Grow(1).Shrink(1).MinWidth(0).Gap(10).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).AlignItems(ui.Start).Children(func() {
				weight := 500
				if !art.Read {
					weight = 650
				}
				ui.Text(c, art.Title).FontSize(13.5).FontWeight(weight).LineHeight(1.35).MaxLines(1).TextColor(p.text)
				if art.Summary != "" {
					ui.Text(c, art.Summary).FontSize(12).LineHeight(1.4).MaxLines(1).TextColor(p.muted)
				}
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					if when := relativeTime(art.Published); when != "" {
						ui.Text(c, when).FontSize(11).TextColor(p.faint).Shrink(0)
						ui.Text(c, "·").FontSize(11).TextColor(p.faint).Shrink(0)
					}
					ui.Text(c, art.FeedTitle).FontSize(11.5).TextColor(p.muted).SingleLine().Grow(1)
					if art.Starred {
						ui.Icon(c, iconStarFill).Size(11, 11).TextColor(p.accent)
					}
					if !art.Read {
						ui.Icon(c, iconDot).Size(6, 6).TextColor(p.accent)
					}
				})
			})
			if art.ImageURL != "" && a.imgs != nil {
				if src := a.imgs.cached(art.ImageURL); src != nil {
					ui.Box(c).Size(52, 52).Radius(6).Clip().Children(func() {
						ui.Image(c, src).Size(52, 52).Fit(ui.Cover)
					})
				}
			}
		})
	})
	if b.Clicked() {
		a.selectArticleAt(context.Background(), i)
	}
	b.ContextMenu(func(m *ui.Menu) {
		read := "标为已读"
		if art.Read {
			read = "标为未读"
		}
		if m.Item(read).Chosen() {
			a.selectArticleAt(context.Background(), i)
			a.toggleReadCurrent(context.Background())
		}
		star := "加星标"
		if art.Starred {
			star = "取消星标"
		}
		if m.Item(star).Chosen() {
			a.selectArticleAt(context.Background(), i)
			a.toggleStarCurrent(context.Background())
		}
		m.Separator()
		if art.URL != "" && m.Item("打开原网页").Chosen() {
			a.openLink(art.URL)
		}
		if art.URL != "" && m.Item("复制链接").Chosen() {
			a.selectArticleAt(context.Background(), i)
			a.copyCurrentLink()
		}
	})
}

// readerPane is the article column.

func (a *app) readerPane(c *ui.Context) {
	p := a.pal
	ctx := context.Background()
	ui.Column(c).Grow(1).Shrink(1).MinWidth(0).FillHeight().Background(p.bg).Children(func() {
		if !a.hasCurrent {
			ui.Column(c).Grow(1).Center().Padding(30, 24).Gap(12).Children(func() {
				ui.Icon(c, iconRSS).Size(42, 42).TextColor(p.faint)
				ui.Text(c, "选择一篇文章开始阅读").FontSize(14).FontWeight(600).TextColor(p.muted)
				ui.Text(c, "列表里的文章会显示在这里，图片与链接都留在原处").FontSize(12).TextColor(p.faint)
				if a.settings.Layout == layoutReader {
					// With the list hidden, the reader alone must be able to
					// bring it back.
					if pillButton(c.Key("show-list"), iconInbox, "显示文章列表", p.accent, p.onAccent).Clicked() {
						a.settings.Layout = layoutSplit
						a.changed()
					}
				}
			})
			return
		}
		art := a.current
		ui.Column(c).Padding(26, 22, 12, 16).Gap(8).BorderWidth(0, 0, 1, 0).BorderColor(p.hairline).Children(func() {
			ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconRSS).Size(13, 13).TextColor(p.accent)
				ui.Text(c, art.FeedTitle).FontSize(12).FontWeight(600).TextColor(p.accent).SingleLine().Grow(1)
				starLabel := "加星标"
				if art.Starred {
					starLabel = "取消星标"
				}
				if iconButton(c.Key("reader-star"), starIcon(art.Starred), 15, p.muted, starLabel).Clicked() {
					a.toggleStarCurrent(context.Background())
				}
				readLabel := "标为已读"
				if art.Read {
					readLabel = "标为未读"
				}
				if iconButton(c.Key("reader-read"), readIcon(art.Read), 15, p.muted, readLabel).Clicked() {
					a.toggleReadCurrent(context.Background())
				}
				if iconButton(c.Key("reader-copy"), iconCopy, 15, p.muted, "复制链接").Clicked() {
					a.copyCurrentLink()
				}
				if iconButton(c.Key("reader-open"), iconExternal, 15, p.muted, "打开原网页").Clicked() {
					a.openOriginal()
				}
				ui.Box(c).Width(1).Height(14).Background(p.hairline).Margin(0, 3)
				if a.readerNavButton(c, "reader-prev", iconChevronLeft, 15, "上一篇 (K)", a.selected > 0).Clicked() {
					a.moveSelection(ctx, -1)
				}
				if a.readerNavButton(c, "reader-next", iconChevron, 15, "下一篇 (J)", a.selected < len(a.articles)-1 || a.hasMore).Clicked() {
					a.moveSelection(ctx, 1)
				}
			})
			ui.Text(c, art.Title).FontSize(23).FontWeight(750).LineHeight(1.25).TextColor(p.text)
			meta := art.FeedTitle
			if art.Author != "" {
				meta += " · " + art.Author
			}
			if !art.Published.IsZero() {
				meta = fullDate(art.Published) + " · " + meta
			}
			if n := readMinutes(art.Plain); n > 0 {
				meta += fmt.Sprintf(" · 约 %d 分钟", n)
			}
			ui.Text(c, meta).FontSize(12).TextColor(p.faint).SingleLine()
		})

		body := strings.TrimSpace(art.Content)
		if body == "" {
			body = strings.TrimSpace(art.Summary)
		}
		if body == "" {
			body = "这篇文章没有正文。"
		}
		th := markdown.DefaultTheme(c.Theme())
		th.FontSize = a.settings.ReaderSize
		th.Text = p.text
		th.Muted = p.muted
		th.Link = p.accent
		th.Strong = p.text
		th.Rule = p.hairline
		th.QuoteBar = p.accent
		th.QuoteText = p.muted
		th.Code = p.text
		th.CodeBackground = p.hover
		th.CodeBlockBackground = p.side
		th.CodeBlockBorder = p.hairline
		th.TableBorder = p.hairline
		th.TableHeader = p.side
		a.md.Theme = th
		ui.Scroll(c.Key("reader")).Grow(1).TrackScroll(&a.readerScroll).Children(func() {
			ui.Column(c).MaxWidth(780).WidthPercent(100).AlignSelf(ui.Center).Padding(28, 44, 64, 44).Children(func() {
				markdown.View(c.Key(fmt.Sprintf("article-%d", art.ID)), body, &a.md)
				if art.URL != "" {
					ui.Row(c).Margin(0, 26, 0, 0).Gap(8).AlignItems(ui.Center).Children(func() {
						if ui.Button(c.Key("read-original"), "在浏览器中打开原文").Clicked() {
							a.openOriginal()
						}
						ui.Text(c, hostOf(art.URL)).FontSize(11.5).TextColor(p.faint).SingleLine()
					})
				}
			})
		})
	})
}

// readerNavButton is the previous or next article control. At either end of
// the list it stays in place, faded and unable to move.
func (a *app) readerNavButton(c *ui.Context, key string, icon *ui.SVG, size float32, label string, enabled bool) ui.Element {
	b := iconButton(c.Key(key), icon, size, a.pal.muted, label)
	if !enabled {
		b.Disabled(true).Opacity(0.3)
	}
	return b
}

func starIcon(on bool) *ui.SVG {
	if on {
		return iconStarFill
	}
	return iconStar
}

func readIcon(read bool) *ui.SVG {
	if read {
		return iconMailOpen
	}
	return iconMail
}

// firstRune is the first character of a title, for the icon fallback of a
// feed without one.
func firstRune(s string) string {
	r, n := utf8.DecodeRuneInString(strings.TrimSpace(s))
	if n == 0 || r == utf8.RuneError {
		return ""
	}
	return string(r)
}
