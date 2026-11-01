package app

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// openSettings shows the settings window, opening it the first time.
func (a *app) openSettings() {
	if a.settingsWin != nil {
		a.settingsWin.Show()
		a.settingsWin.Focus()
		return
	}
	a.settingsWin = mygo.NewWindow(mygo.WindowOptions{
		Title:           "设置",
		Width:           600,
		Height:          760,
		MinWidth:        520,
		MinHeight:       460,
		StateKey:        "settings",
		DisableMaximize: true,
		AutoHideMenuBar: true,
		BackgroundColor: "light-dark(#f4f4f6, #1c1c1f)",
		Content:         ui.View(a.settingsView),
	})
	a.settingsWin.OnClosed(func() { a.settingsWin = nil })
}

func (a *app) settingsView(c *ui.Context) {
	t := *c.Theme()
	p := paletteFor(&t, a.settings.Accent)
	t.Accent, t.AccentText = p.accent, p.onAccent
	c.SetTheme(&t)
	c.Root().Background(p.side)
	c.OnShortcut(ui.Cmd, ui.KeyW, func() {
		if a.settingsWin != nil {
			a.settingsWin.Close()
		}
	})
	s := &a.settings

	ui.Scroll(c).Fill().Padding(26, 30, 34, 30).Gap(8).Children(func() {
		group := func(title string, rows func()) {
			ui.Text(c, title).FontSize(12).FontWeight(650).TextColor(p.muted).Margin(14, 4, 4, 4)
			ui.Column(c).Radius(12).Background(p.bg).Border(1, p.hairline).Dividers(1, p.hairline).Children(rows)
		}
		row := func(label, desc string, control func()) {
			ui.Row(c).Padding(12, 16).Gap(16).AlignItems(ui.Center).MinHeight(48).Children(func() {
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
					ui.Text(c, label).FontSize(13.5).FontWeight(500)
					if desc != "" {
						ui.Text(c, desc).FontSize(12).TextColor(p.muted).LineHeight(1.4)
					}
				})
				control()
			})
		}

		group("外观", func() {
			row("外观", "跟随系统，或始终使用浅色 / 深色", func() {
				modes := []string{"system", "light", "dark"}
				i := 0
				for k, m := range modes {
					if m == s.Appearance {
						i = k
					}
				}
				if segmented(c.Key("appearance"), &i, "跟随系统", "浅色", "深色") {
					s.Appearance = modes[i]
					a.changed()
				}
			})
			row("强调色", "用于选中项、链接与星标", func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					for _, name := range accentNames {
						name := name
						color, _ := accentColor(name, t.Dark)
						b := ui.ButtonBase(c.Key("accent-"+name)).Size(24, 24).Radius(999).Background(color).
							Label(accentLabels[name]).Tooltip(accentLabels[name]).FocusRing(false)
						if s.Accent == name {
							b.Border(2, p.text)
						}
						if b.Clicked() {
							s.Accent = name
							a.changed()
						}
					}
				})
			})
			row("阅读字号", "文章正文的字号", func() {
				sizes := []int{14, 16, 18, 20, 22}
				labels := []string{"小", "标准", "大", "特大", "超大"}
				chosen := "标准"
				for i, size := range sizes {
					if size == int(s.ReaderSize) {
						chosen = labels[i]
					}
				}
				if ui.Select(c.Key("reader-size"), &chosen, labels).Label("阅读字号").Width(130).Changed() {
					for i, label := range labels {
						if label == chosen {
							s.ReaderSize = float32(sizes[i])
						}
					}
					a.changed()
				}
			})
		})

		group("阅读", func() {
			row("打开即已读", "在列表里选中文章时，立即把它标为已读", func() {
				if ui.Switch(c.Key("mark-read"), &s.MarkRead).Label("打开即已读").Changed() {
					a.changed()
				}
			})
			row("新文章通知", "窗口在后台时，刷新到新文章发出桌面通知", func() {
				sw := ui.Switch(c.Key("notify-new"), &s.NotifyNew).Label("新文章通知")
				if !mygo.NotificationsSupported() {
					sw.Disabled(true).Opacity(0.45)
				}
				if sw.Changed() {
					a.changed()
				}
			})
			row("保存的文章数量", "每个订阅保留多少篇最近的文章；超出的已读、未星标文章会被清理", func() {
				counts := []int{100, 200, 500, 1000, 0}
				labels := []string{"100 篇", "200 篇", "500 篇", "1000 篇", "全部"}
				chosen := "500 篇"
				for i, n := range counts {
					if n == s.KeepArticles {
						chosen = labels[i]
					}
				}
				if ui.Select(c.Key("keep-articles"), &chosen, labels).Label("保存的文章数量").Width(130).Changed() {
					for i, label := range labels {
						if label == chosen {
							s.KeepArticles = counts[i]
						}
					}
					a.changed()
				}
			})
		})

		group("刷新", func() {
			row("自动刷新", "定时在后台检查订阅更新", func() {
				values := []int{0, 15, 30, 60, 120, 360}
				labels := []string{"手动", "每 15 分钟", "每 30 分钟", "每小时", "每 2 小时", "每 6 小时"}
				chosen := labels[2]
				for i, v := range values {
					if v == s.RefreshMin {
						chosen = labels[i]
					}
				}
				if ui.Select(c.Key("refresh-min"), &chosen, labels).Label("自动刷新").Width(150).Changed() {
					for i, label := range labels {
						if label == chosen {
							s.RefreshMin = values[i]
						}
					}
					a.changed()
				}
			})
			row("立即刷新", refreshDescription(a.lastRefresh), func() {
				if ui.Button(c.Key("refresh-now-settings"), "刷新").Clicked() {
					a.refreshAll(true)
				}
			})
		})

		group("托盘", func() {
			row("显示托盘图标", "在菜单栏或通知区域显示 zrss，可快速刷新、阅读或退出", func() {
				if ui.Switch(c.Key("show-tray"), &s.ShowTray).Label("显示托盘图标").Changed() {
					a.changed()
				}
			})
			row("关闭窗口时留在托盘", "关闭主窗口时继续在后台刷新，点击托盘图标可重新打开", func() {
				sw := ui.Switch(c.Key("close-to-tray"), &s.CloseToTray).Label("关闭窗口时留在托盘")
				sw.Disabled(!s.ShowTray).Opacity(opacity(s.ShowTray))
				if sw.Changed() {
					a.changed()
				}
			})
		})

		group("数据", func() {
			row("数据库", fmt.Sprintf("%s\n%s", a.store.Path(), formatBytes(a.store.Size())), func() {
				if ui.Button(c.Key("vacuum"), "整理").Clicked() {
					a.vacuum(context.Background())
				}
			})
			row("订阅列表", "导入或导出 OPML，与其他阅读器迁移订阅", func() {
				ui.Row(c).Gap(8).Children(func() {
					if ui.Button(c.Key("settings-import"), "导入…").Clicked() {
						a.importOPML()
					}
					if ui.Button(c.Key("settings-export"), "导出…").Clicked() {
						a.exportOPML()
					}
				})
			})
			row("搜索记录", "清空搜索框里的最近搜索", func() {
				if ui.Button(c.Key("clear-searches"), "清空").Clicked() {
					a.clearSearchHistory(context.Background())
				}
			})
		})

		if a.updates {
			group("更新", func() {
				desc := "每天检查一次是否有新版本"
				if t := lastUpdateCheck(); !t.IsZero() {
					desc += "；上次检查：" + relativeTime(t)
				}
				row("自动检查更新", desc, func() {
					on := updateChecks()
					if ui.Switch(c.Key("update-checks"), &on).Label("自动检查更新").Changed() {
						setUpdateChecks(on)
					}
				})
				row("自动下载并安装", "在后台下载新版本，下次打开时生效", func() {
					on := updateDownloads()
					if ui.Switch(c.Key("update-downloads"), &on).Label("自动下载并安装").Disabled(!updateChecks()).Changed() {
						setUpdateDownloads(on)
					}
				})
				row("检查更新", "当前版本 "+mygo.App.Version(), func() {
					if ui.Button(c.Key("check-updates"), "检查更新").Clicked() {
						checkForUpdates()
					}
				})
			})
		}

		group("关于", func() {
			row("zrss", "一个安静、快速的桌面 RSS 阅读器，支持 RSS 1.0、RSS 2.0 与 Atom。", func() {
				ui.Text(c, "版本 "+mygo.App.Version()).FontSize(12).TextColor(p.faint)
			})
		})
	})
}

func refreshDescription(last time.Time) string {
	if last.IsZero() {
		return "还没有刷新过"
	}
	return "上次刷新：" + relativeTime(last)
}

func opacity(on bool) float32 {
	if on {
		return 1
	}
	return 0.45
}

func formatBytes(n int64) string {
	switch {
	case n <= 0:
		return ""
	case n < 1<<10:
		return strconv.FormatInt(n, 10) + " B"
	case n < 1<<20:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	}
}

func segmented(c *ui.Context, selected *int, labels ...string) bool {
	return ui.Segmented(c, selected, labels...).Changed()
}
