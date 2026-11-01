package app

import (
	"context"
	_ "embed"
	"log"
	"runtime"

	"github.com/egoist/mygo"
)

// trayTemplate is the black RSS mark macOS tints for the menu bar.
//
//go:embed tray/tray-template.png
var trayTemplate []byte

// trayColor is the colored mark for the notification area.
//
//go:embed tray/tray.png
var trayColor []byte

// applyTray adds or removes the icon in the menu bar or notification area,
// as the settings say.
func (a *app) applyTray() {
	if a.settings.ShowTray && a.tray == nil {
		icon, template := trayColor, false
		if runtime.GOOS == "darwin" {
			icon, template = trayTemplate, true
		}
		t, err := mygo.NewTray(mygo.TrayOptions{
			ID:             "zrss",
			Icon:           icon,
			IconIsTemplate: template,
			ToolTip:        mygo.App.Name(),
			Menu:           a.trayMenu(),
		})
		if err != nil {
			log.Println("tray:", err)
			return
		}
		t.OnClick(a.showMain)
		a.tray = t
		return
	}
	if !a.settings.ShowTray && a.tray != nil {
		a.tray.Destroy()
		a.tray = nil
	}
}

// trayMenu is the icon's menu: the window, a refresh, reading everything,
// settings and quitting.
func (a *app) trayMenu() *mygo.Menu {
	name := mygo.App.Name()
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: "打开" + name, Click: func(*mygo.MenuItem, *mygo.Window) { a.showMain() }},
		mygo.Separator(),
		{Label: "刷新全部订阅", Click: func(*mygo.MenuItem, *mygo.Window) {
			a.refreshAll(true)
			a.redraw()
		}},
		{Label: "全部标为已读", Click: func(*mygo.MenuItem, *mygo.Window) {
			a.markEverythingRead(context.Background())
			a.redraw()
		}},
		mygo.Separator(),
		{Label: "设置…", Click: func(*mygo.MenuItem, *mygo.Window) {
			a.showMain()
			mygo.App.Focus()
			a.openSettings()
		}},
		mygo.Separator(),
		{Label: "退出" + name, Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
}
